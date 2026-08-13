package invoicing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	accsvc "github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

var ErrInvoiceNotFound = fmt.Errorf("facture introuvable")
var ErrInvalidTransition = fmt.Errorf("invalid status transition")

// Conversion errors (quote → invoice).
var (
	ErrNotAQuote           = fmt.Errorf("seuls les offres de prix peuvent être converties en facture")
	ErrQuoteAlreadyConv    = fmt.Errorf("cette offre a déjà été convertie en facture")
	ErrQuoteNotConvertible = fmt.Errorf("une offre au statut brouillon ou annulé ne peut pas être convertie")
	ErrInvalidQuoteOutcome = fmt.Errorf("issue d'offre inconnue")
)

// Credit note errors.
var (
	ErrNotAnInvoice         = fmt.Errorf("seule une facture peut faire l'objet d'une note de crédit")
	ErrInvoiceNotCreditable = fmt.Errorf("une facture au statut brouillon ou annulé ne peut pas être créditée")
	ErrCreditExceedsInvoice = fmt.Errorf("le total des notes de crédit dépasserait le montant de la facture")
	// Une facture ne devient pas une offre, ni une note de crédit une facture :
	// leur numérotation, leur portée comptable et leur traitement TVA diffèrent.
	// Changer de nature après émission effacerait la trace de ce qui a été
	// envoyé (CO art. 957a al. 2 ch. 5).
	ErrDocumentTypeImmutable = fmt.Errorf("le type d'un document ne peut pas être changé après sa création")
)

// validTransitions defines the allowed status state machine for a document
// that can be settled — an invoice or a credit note.
var validTransitions = map[models.InvoiceStatus][]models.InvoiceStatus{
	models.InvoiceStatusDraft:     {models.InvoiceStatusSent, models.InvoiceStatusCancelled},
	models.InvoiceStatusSent:      {models.InvoiceStatusPaid, models.InvoiceStatusCancelled},
	models.InvoiceStatusPaid:      {models.InvoiceStatusArchived},
	models.InvoiceStatusCancelled: {models.InvoiceStatusDraft},
	models.InvoiceStatusArchived:  {},
}

// quoteTransitions is the state machine for a price offer. It deliberately has
// no path to "paid": nobody owes anything on an offer, and marking one paid
// used to be possible, which put it in the receivables and — before the
// document_type filters — in the VAT declaration. An accepted offer is not
// settled, it is converted (see Convert); its commercial fate is recorded in
// quote_outcome, not in status.
var quoteTransitions = map[models.InvoiceStatus][]models.InvoiceStatus{
	models.InvoiceStatusDraft:     {models.InvoiceStatusSent, models.InvoiceStatusCancelled},
	models.InvoiceStatusSent:      {models.InvoiceStatusCancelled, models.InvoiceStatusArchived},
	models.InvoiceStatusCancelled: {models.InvoiceStatusDraft},
	models.InvoiceStatusArchived:  {},
}

// transitionsFor picks the state machine matching the document type.
func transitionsFor(documentType string) map[models.InvoiceStatus][]models.InvoiceStatus {
	if documentType == DocumentTypeQuote {
		return quoteTransitions
	}
	return validTransitions
}

// Document types stored in invoices.document_type.
const (
	DocumentTypeInvoice    = "invoice"
	DocumentTypeQuote      = "quote"
	DocumentTypeCreditNote = "credit_note"
)

// Commercial outcomes of a price offer, stored in invoices.quote_outcome.
const (
	QuoteOutcomeAccepted = "accepted"
	QuoteOutcomeRefused  = "refused"
	QuoteOutcomeExpired  = "expired"
)

var validQuoteOutcomes = map[string]bool{
	QuoteOutcomeAccepted: true,
	QuoteOutcomeRefused:  true,
	QuoteOutcomeExpired:  true,
}

// AccountingServiceInterface allows the invoicing service to create and post
// journal entries for automatic reversal on cancellation.
type AccountingServiceInterface interface {
	CreateEntry(ctx context.Context, userID string, req accsvc.CreateEntryRequest) (*models.JournalEntry, error)
	PostEntry(ctx context.Context, userID, entryID, ipAddress string) error
}

type Service struct {
	db            *sql.DB
	usePostgres   bool
	accountingSvc AccountingServiceInterface
}

// New creates a Service without an accounting dependency (backward compatible).
func New(database *sql.DB, usePostgres bool) *Service {
	return &Service{db: database, usePostgres: usePostgres}
}

// NewWithAccounting creates a Service wired to an accounting service,
// enabling automatic journal reversal when an invoice is cancelled.
func NewWithAccounting(database *sql.DB, usePostgres bool, acctSvc AccountingServiceInterface) *Service {
	return &Service{db: database, usePostgres: usePostgres, accountingSvc: acctSvc}
}

// ─── CreateInvoice ────────────────────────────────────────────────────────────

type LineInput struct {
	Description string
	Quantity    float64
	Unit        *string
	UnitPrice   float64
	DiscountPct float64 // percentage, e.g. 10 for 10%
	VATRate     float64 // percentage, e.g. 8.1 for 8.1%
	Sequence    int
}

type CreateInvoiceRequest struct {
	DocumentType string // "invoice" | "quote" | "credit_note"
	ContactID    string
	IssueDate    time.Time
	DueDate      time.Time
	Currency     string
	Notes        *string
	Terms        *string
	Lines        []LineInput
}

// CreateInvoice creates an invoice or quote with totals rounded to 0.05 CHF (5-Rappen rule).
// VAT rates are expressed as percentages (e.g. 8.1 for 8.1%).
func (s *Service) CreateInvoice(ctx context.Context, userID string, req CreateInvoiceRequest) (*models.Invoice, error) {
	if len(req.Lines) == 0 {
		return nil, fmt.Errorf("invoice must have at least one line")
	}
	if req.Currency == "" {
		req.Currency = "CHF"
	}
	if req.DocumentType == "" {
		req.DocumentType = "invoice"
	}

	subtotal, vatAmount, total := computeTotals(req.Lines)

	// Taux représentatif porté par l'en-tête de la facture.
	//
	// Il valait 8.1 par défaut, et n'était remplacé que si la première ligne
	// portait un taux **strictement positif** — ce qui confondait « 0 % » avec
	// « non renseigné ». Une facture sans TVA était donc enregistrée à 8.1 %
	// avec un montant de taxe nul.
	//
	// Ce n'est pas qu'un défaut d'affichage : la déclaration TVA agrège les
	// factures **en groupant par ce taux** (voir services/vat). Le chiffre
	// d'affaires d'un non-assujetti serait remonté comme taxable à 8.1 % sans
	// impôt correspondant — une ligne qui ne se réconcilie pas, et que l'AFC
	// demanderait d'expliquer.
	//
	// Le taux suit désormais les lignes. Une facture sans ligne n'a pas de TVA,
	// donc 0 est la bonne réponse et non une valeur par défaut arbitraire.
	primaryVATRate := headerVATRate(req.Lines)

	// Facturer de la TVA sans être inscrit au registre des assujettis rend
	// l'émetteur redevable de l'impôt mentionné (LTVA art. 27 al. 2), qu'il
	// l'ait encaissé ou non. Le contrôle de cohérence le signalait après coup ;
	// le refuser à la source évite d'avoir à corriger des factures envoyées.
	if err := s.checkVATAllowed(ctx, s.db, vatAmount, primaryVATRate); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	invoiceID := db.NewID()
	number, err := s.nextInvoiceNumber(ctx, tx, req.DocumentType, req.IssueDate)
	if err != nil {
		return nil, fmt.Errorf("next invoice number: %w", err)
	}

	// Rattachement à l'exercice, comme pour les écritures : l'archive légale
	// filtrée par exercice (CO art. 958f) revenait sinon vide de toute facture.
	// Une offre de prix n'est pas une pièce comptable, mais elle devient une
	// facture sans changer de ligne : le rattachement suit donc le document.
	period, err := accsvc.EnsureFiscalPeriod(ctx, tx, s.usePostgres, req.IssueDate)
	if err != nil {
		return nil, err
	}

	// Identité du destinataire figée à l'émission. Sans cela, le PDF relit le
	// contact vivant : renommer un client réécrit toutes ses factures passées,
	// alors que le CO art. 958f impose de conserver la pièce telle qu'elle est
	// et que la LTVA art. 26 exige qu'elle nomme son destinataire.
	var rcp recipientSnapshot
	rcpQ := db.Rebind(`
		SELECT COALESCE(name,''), COALESCE(address,''), COALESCE(postal_code,''),
		       COALESCE(city,''), COALESCE(country,''), COALESCE(vat_number,'')
		FROM contacts WHERE id = ?`, s.usePostgres)
	if err := tx.QueryRowContext(ctx, rcpQ, req.ContactID).Scan(
		&rcp.Name, &rcp.Address, &rcp.PostalCode, &rcp.City, &rcp.Country, &rcp.VATNumber,
	); err != nil {
		return nil, fmt.Errorf("load recipient: %w", err)
	}

	insertInv := db.Rebind(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate,
		                      notes, terms, fiscal_year_id, created_by_id,
		                      recipient_name, recipient_address, recipient_postal_code,
		                      recipient_city, recipient_country, recipient_vat_number,
		                      recipient_backfilled)
		VALUES (?, ?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insertInv,
		invoiceID, number, req.DocumentType, req.ContactID,
		req.IssueDate.Format("2006-01-02"), req.DueDate.Format("2006-01-02"),
		req.Currency, subtotal, vatAmount, total, primaryVATRate,
		req.Notes, req.Terms, period.ID, userID,
		rcp.Name, rcp.Address, rcp.PostalCode, rcp.City, rcp.Country, rcp.VATNumber); err != nil {
		return nil, fmt.Errorf("insert invoice: %w", err)
	}

	insertLine := db.Rebind(`
		INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit, unit_price, discount_pct, vat_rate, line_total, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for _, l := range req.Lines {
		// line_total is the HT amount after discount (before VAT).
		lineTotal := l.Quantity * l.UnitPrice * (1 - l.DiscountPct/100)
		if _, err := tx.ExecContext(ctx, insertLine,
			db.NewID(), invoiceID, l.Description, l.Quantity, l.Unit, l.UnitPrice,
			l.DiscountPct, l.VATRate, lineTotal, l.Sequence); err != nil {
			return nil, fmt.Errorf("insert line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	now := time.Now()
	return &models.Invoice{
		ID:             invoiceID,
		InvoiceNumber:  number,
		DocumentType:   req.DocumentType,
		ContactID:      req.ContactID,
		Status:         models.InvoiceStatusDraft,
		IssueDate:      req.IssueDate,
		DueDate:        req.DueDate,
		Currency:       req.Currency,
		SubtotalAmount: subtotal,
		VATAmount:      vatAmount,
		TotalAmount:    total,
		VATRate:        primaryVATRate,
		Notes:          req.Notes,
		Terms:          req.Terms,
		CreatedByID:    userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// ─── UpdateInvoice ────────────────────────────────────────────────────────────

var ErrInvoicePaid = fmt.Errorf("impossible de modifier une facture avec un paiement enregistré")

// UpdateInvoice replaces the editable fields and all lines of an invoice.
// Blocked if amount_paid > 0 (payment has been validated).
func (s *Service) UpdateInvoice(ctx context.Context, invoiceID string, req CreateInvoiceRequest) (*models.Invoice, error) {
	// Guard: check payment status before touching anything.
	var amountPaid float64
	chkQ := db.Rebind("SELECT amount_paid FROM invoices WHERE id = ?", s.usePostgres)
	if err := s.db.QueryRowContext(ctx, chkQ, invoiceID).Scan(&amountPaid); err == sql.ErrNoRows {
		return nil, ErrInvoiceNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load invoice: %w", err)
	}
	if amountPaid > 0 {
		return nil, ErrInvoicePaid
	}

	if req.Currency == "" {
		req.Currency = "CHF"
	}

	// Le type de document ne se DÉDUIT pas d'un champ omis. Il retombait sur
	// « invoice », si bien qu'une modification sans ce champ transformait une
	// note de crédit en facture : le lien vers la pièce corrigée survivait dans
	// la base mais le document changeait de nature, ce que la LTVA art. 27
	// al. 4 ne prévoit pas — et la note disparaissait du calcul de TVA en tant
	// que correction. Un champ absent veut dire « inchangé ».
	var currentType string
	if err := s.db.QueryRowContext(ctx, db.Rebind(
		`SELECT document_type FROM invoices WHERE id = ?`, s.usePostgres), invoiceID,
	).Scan(&currentType); err != nil {
		return nil, ErrInvoiceNotFound
	}
	if req.DocumentType == "" {
		req.DocumentType = currentType
	}
	if req.DocumentType != currentType {
		return nil, ErrDocumentTypeImmutable
	}

	subtotal, vatAmount, total := computeTotals(req.Lines)
	primaryVATRate := headerVATRate(req.Lines)

	// ── Garde-fou TVA ───────────────────────────────────────────────────────
	if err := s.checkVATAllowed(ctx, s.db, vatAmount, primaryVATRate); err != nil {
		return nil, err
	}

	// ── Garde-fou du plafond de crédit ──────────────────────────────────────
	//
	// Le plafond était vérifié à la CRÉATION d'une note de crédit et nulle part
	// ailleurs. Modifier ensuite ses lignes le contournait entièrement : une
	// note de 500 sur une facture de 500 pouvait passer à 1500 en HTTP 200,
	// sous-évaluant la TVA due d'autant. Vérifié sur un serveur réel avant
	// correction.
	if err := s.checkCreditCap(ctx, s.db, invoiceID, total); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	updQ := db.Rebind(`
		UPDATE invoices SET
			document_type = ?, contact_id = ?, issue_date = ?, due_date = ?,
			currency = ?, subtotal_amount = ?, vat_amount = ?, total_amount = ?,
			vat_rate = ?, notes = ?, terms = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, updQ,
		req.DocumentType, req.ContactID,
		req.IssueDate.Format("2006-01-02"), req.DueDate.Format("2006-01-02"),
		req.Currency, subtotal, vatAmount, total, primaryVATRate,
		req.Notes, req.Terms, invoiceID); err != nil {
		return nil, fmt.Errorf("update invoice: %w", err)
	}

	// Replace all lines atomically.
	delQ := db.Rebind("DELETE FROM invoice_lines WHERE invoice_id = ?", s.usePostgres)
	if _, err := tx.ExecContext(ctx, delQ, invoiceID); err != nil {
		return nil, fmt.Errorf("delete lines: %w", err)
	}
	insertLine := db.Rebind(`
		INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit, unit_price, discount_pct, vat_rate, line_total, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for i, l := range req.Lines {
		lineTotal := l.Quantity * l.UnitPrice * (1 - l.DiscountPct/100)
		seq := l.Sequence
		if seq == 0 {
			seq = i + 1
		}
		if _, err := tx.ExecContext(ctx, insertLine,
			db.NewID(), invoiceID, l.Description, l.Quantity, l.Unit,
			l.UnitPrice, l.DiscountPct, l.VATRate, lineTotal, seq); err != nil {
			return nil, fmt.Errorf("insert line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return nil, nil // caller reloads via GetInvoice
}

// ─── Transition ───────────────────────────────────────────────────────────────

// Transition moves an invoice to the next status if the transition is valid.
// When an invoice transitions from sent → cancelled and has a linked journal entry,
// a reversal entry is automatically created and posted (CO art. 957a).
func (s *Service) Transition(ctx context.Context, invoiceID string, to models.InvoiceStatus) error {
	return s.TransitionBy(ctx, invoiceID, to, Actor{})
}

// TransitionBy applique la transition en traçant son auteur.
//
// Le chemin HTTP passe toujours par ici : sans auteur, une transition
// n'apparaît nulle part, et « qui a annulé cette facture » reste sans réponse.
func (s *Service) TransitionBy(ctx context.Context, invoiceID string, to models.InvoiceStatus, actor Actor) error {
	// Load current status, invoice_number, journal_entry_id, created_by_id, and issue_date.
	getQ := db.Rebind(`
		SELECT status, invoice_number, COALESCE(journal_entry_id, ''), created_by_id, issue_date,
		       COALESCE(document_type, 'invoice')
		FROM invoices WHERE id = ?`, s.usePostgres)
	var current, invoiceNumber, journalEntryID, createdByID, documentType string
	var issueDate time.Time
	if err := s.db.QueryRowContext(ctx, getQ, invoiceID).Scan(
		&current, &invoiceNumber, &journalEntryID, &createdByID, &issueDate, &documentType,
	); err == sql.ErrNoRows {
		return ErrInvoiceNotFound
	} else if err != nil {
		return fmt.Errorf("load invoice: %w", err)
	}

	allowed := transitionsFor(documentType)[models.InvoiceStatus(current)]
	for _, a := range allowed {
		if a == to {
			// Apply the status transition.
			updateQ := db.Rebind("UPDATE invoices SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", s.usePostgres)
			if _, err := s.db.ExecContext(ctx, updateQ, string(to), invoiceID); err != nil {
				return fmt.Errorf("update invoice status: %w", err)
			}

			// Comptabilisation à l'émission : brouillon → envoyée.
			//
			// C'est le moment où l'opération existe pour un tiers, donc celui
			// où elle se rapporte à l'exercice (CO art. 958b). Un échec ne
			// défait pas la transition — la facture EST envoyée, le nier
			// serait pire — mais il remonte, pour que personne ne croie
			// l'écriture passée.
			if models.InvoiceStatus(current) == models.InvoiceStatusDraft &&
				to == models.InvoiceStatusSent &&
				journalEntryID == "" &&
				s.accountingSvc != nil &&
				s.autoPostEnabled(ctx) {

				if _, err := s.PostIssuedDocument(
					ctx, invoiceID, documentType, invoiceNumber, createdByID, issueDate, "",
				); err != nil {
					return fmt.Errorf("document envoyé, mais l'écriture au journal a échoué: %w", err)
				}
			}

			s.record(ctx, actor, accsvc.ActionDocumentTransition, invoiceID, map[string]any{
				"document_type": documentType,
				"number":        invoiceNumber,
				"from":          current,
				"to":            string(to),
			})

			// Automatic reversal: sent → cancelled with a linked journal entry.
			if models.InvoiceStatus(current) == models.InvoiceStatusSent &&
				to == models.InvoiceStatusCancelled &&
				journalEntryID != "" &&
				s.accountingSvc != nil {

				if err := s.createReversalEntry(ctx, createdByID, invoiceNumber, journalEntryID, issueDate); err != nil {
					return fmt.Errorf("create reversal entry: %w", err)
				}
			}

			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, current, to)
}

// createReversalEntry builds a mirror journal entry with debit ↔ credit swapped,
// marks it is_reversal=1, and immediately posts it.
func (s *Service) createReversalEntry(
	ctx context.Context,
	userID, invoiceNumber, originalEntryID string,
	entryDate time.Time,
) error {
	// Load the lines of the original journal entry.
	linesQ := db.Rebind(`
		SELECT account_id,
		       COALESCE(debit_amount, 0),
		       COALESCE(credit_amount, 0),
		       description,
		       sequence
		FROM journal_lines
		WHERE entry_id = ?
		ORDER BY sequence`, s.usePostgres)
	rows, err := s.db.QueryContext(ctx, linesQ, originalEntryID)
	if err != nil {
		return fmt.Errorf("load original lines: %w", err)
	}
	defer rows.Close()

	var lines []accsvc.LineInput
	for rows.Next() {
		var accountID, desc string
		var debit, credit float64
		var seq int
		if err := rows.Scan(&accountID, &debit, &credit, &desc, &seq); err != nil {
			return fmt.Errorf("scan line: %w", err)
		}
		li := accsvc.LineInput{
			AccountID:   accountID,
			Description: desc,
			Sequence:    seq,
		}
		// Swap debit ↔ credit for the reversal.
		if debit != 0 {
			li.CreditAmount = &debit
		}
		if credit != 0 {
			li.DebitAmount = &credit
		}
		lines = append(lines, li)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate lines: %w", err)
	}
	if len(lines) == 0 {
		// Original entry has no lines — nothing to reverse.
		return nil
	}

	// Create the reversal entry as a draft via the accounting service.
	req := accsvc.CreateEntryRequest{
		Date:        entryDate,
		Description: fmt.Sprintf("Contrepassation facture %s", invoiceNumber),
		Lines:       lines,
	}
	reversalEntry, err := s.accountingSvc.CreateEntry(ctx, userID, req)
	if err != nil {
		return fmt.Errorf("create reversal draft: %w", err)
	}

	// Flag the entry as a reversal and link it to the original.
	flagQ := db.Rebind(`
		UPDATE journal_entries
		SET is_reversal = 1, reversal_of_id = ?
		WHERE id = ?`, s.usePostgres)
	if _, err := s.db.ExecContext(ctx, flagQ, originalEntryID, reversalEntry.ID); err != nil {
		return fmt.Errorf("flag reversal: %w", err)
	}

	// Immediately post the reversal (status = 'posted').
	if err := s.accountingSvc.PostEntry(ctx, userID, reversalEntry.ID, ""); err != nil {
		return fmt.Errorf("post reversal: %w", err)
	}

	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// computeTotals calculates subtotal, VAT, and total (rounded to 0.05 CHF).
// VAT rates must be expressed as percentages (e.g. 8.1 for 8.1%).
func computeTotals(lines []LineInput) (subtotal, vatAmount, total float64) {
	for _, l := range lines {
		base := l.Quantity * l.UnitPrice * (1 - l.DiscountPct/100)
		subtotal += base
		vatAmount += base * l.VATRate / 100
	}
	total = compliance.RoundTo5Rappen(subtotal + vatAmount)
	vatAmount = compliance.RoundTo5Rappen(vatAmount)
	return
}

// nextInvoiceNumber generates FA-2026-0001 (invoice) or OF-2026-0001 (quote) style numbers.
func (s *Service) nextInvoiceNumber(ctx context.Context, tx *sql.Tx, documentType string, date time.Time) (string, error) {
	prefix := "FA"
	if documentType == "quote" {
		prefix = "OF"
	} else if documentType == "credit_note" {
		prefix = "NC"
	}
	year := date.Format("2006")
	pattern := prefix + "-" + year + "-%"
	countQ := db.Rebind("SELECT COUNT(*) FROM invoices WHERE invoice_number LIKE ?", s.usePostgres)
	var count int
	if err := tx.QueryRowContext(ctx, countQ, pattern).Scan(&count); err != nil {
		return "", fmt.Errorf("count invoices: %w", err)
	}
	return fmt.Sprintf("%s-%s-%04d", prefix, year, count+1), nil
}

// ─── Quote lifecycle ──────────────────────────────────────────────────────────

// ConvertQuoteRequest carries the dates of the invoice being created. Both are
// optional: the offer's own dates describe how long the offer stands, which
// says nothing about when the resulting invoice falls due.
type ConvertQuoteRequest struct {
	IssueDate time.Time
	DueDate   time.Time
}

// ConvertQuote creates an invoice from a price offer and returns it.
//
// The offer is kept, not transformed. The client already holds a copy of it,
// so mutating the record would leave them quoting a reference that no longer
// exists here — exactly the link CO art. 958f al. 3 requires to stay
// guaranteed. The two documents keep their own numbers (OF- and FA-) and point
// at each other through converted_from_id, which is what CO art. 957a al. 2
// ch. 5 asks of traceability.
func (s *Service) ConvertQuote(ctx context.Context, quoteID, userID string, req ConvertQuoteRequest) (*models.Invoice, error) {
	if req.IssueDate.IsZero() {
		req.IssueDate = time.Now()
	}
	if req.DueDate.IsZero() {
		req.DueDate = req.IssueDate.AddDate(0, 0, 30)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	loadQ := db.Rebind(`
		SELECT COALESCE(document_type, 'invoice'), status, contact_id, currency, notes, terms
		FROM invoices WHERE id = ?`, s.usePostgres)
	var docType, status, contactID, currency string
	var notes, terms *string
	if err := tx.QueryRowContext(ctx, loadQ, quoteID).Scan(
		&docType, &status, &contactID, &currency, &notes, &terms,
	); err == sql.ErrNoRows {
		return nil, ErrInvoiceNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load quote: %w", err)
	}

	if docType != DocumentTypeQuote {
		return nil, ErrNotAQuote
	}
	// A draft has not been sent to anyone yet, and a cancelled offer is dead.
	if status != string(models.InvoiceStatusSent) {
		return nil, ErrQuoteNotConvertible
	}

	// Converting twice would invoice the same work twice. The index on
	// converted_from_id makes this check cheap.
	dupQ := db.Rebind("SELECT COUNT(*) FROM invoices WHERE converted_from_id = ?", s.usePostgres)
	var already int
	if err := tx.QueryRowContext(ctx, dupQ, quoteID).Scan(&already); err != nil {
		return nil, fmt.Errorf("check existing conversion: %w", err)
	}
	if already > 0 {
		return nil, ErrQuoteAlreadyConv
	}

	// Re-read the lines rather than trusting a caller-supplied copy: the
	// invoice must bill exactly what was offered. Loading them in a helper
	// closes the cursor before the inserts below — SQLite dislikes an open
	// read cursor on a transaction that then writes.
	lines, err := s.loadLines(ctx, tx, quoteID)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("l'offre ne contient aucune ligne à facturer")
	}

	subtotal, vatAmount, total := computeTotals(lines)
	primaryVATRate := 8.1
	if lines[0].VATRate > 0 {
		primaryVATRate = lines[0].VATRate
	}

	invoiceID := db.NewID()
	number, err := s.nextInvoiceNumber(ctx, tx, DocumentTypeInvoice, req.IssueDate)
	if err != nil {
		return nil, fmt.Errorf("next invoice number: %w", err)
	}

	insertInv := db.Rebind(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate,
		                      notes, terms, created_by_id, converted_from_id)
		VALUES (?, ?, 'invoice', ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insertInv,
		invoiceID, number, contactID,
		req.IssueDate.Format("2006-01-02"), req.DueDate.Format("2006-01-02"),
		currency, subtotal, vatAmount, total, primaryVATRate,
		notes, terms, userID, quoteID); err != nil {
		return nil, fmt.Errorf("insert converted invoice: %w", err)
	}

	insertLine := db.Rebind(`
		INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit, unit_price, discount_pct, vat_rate, line_total, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for _, l := range lines {
		lineTotal := l.Quantity * l.UnitPrice * (1 - l.DiscountPct/100)
		if _, err := tx.ExecContext(ctx, insertLine,
			db.NewID(), invoiceID, l.Description, l.Quantity, l.Unit, l.UnitPrice,
			l.DiscountPct, l.VATRate, lineTotal, l.Sequence); err != nil {
			return nil, fmt.Errorf("insert converted line: %w", err)
		}
	}

	// The offer keeps its status; what changed is its commercial outcome.
	outQ := db.Rebind(
		"UPDATE invoices SET quote_outcome = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", s.usePostgres)
	if _, err := tx.ExecContext(ctx, outQ, QuoteOutcomeAccepted, quoteID); err != nil {
		return nil, fmt.Errorf("mark quote accepted: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	convertedFrom := quoteID
	return &models.Invoice{
		ID:              invoiceID,
		InvoiceNumber:   number,
		DocumentType:    DocumentTypeInvoice,
		ContactID:       contactID,
		Status:          models.InvoiceStatusDraft,
		IssueDate:       req.IssueDate,
		DueDate:         req.DueDate,
		Currency:        currency,
		SubtotalAmount:  subtotal,
		VATAmount:       vatAmount,
		TotalAmount:     total,
		VATRate:         primaryVATRate,
		Notes:           notes,
		Terms:           terms,
		ConvertedFromID: &convertedFrom,
		CreatedByID:     userID,
	}, nil
}

// SetQuoteOutcome records how a price offer ended. "accepted" is set by
// ConvertQuote and is not accepted here: an offer is accepted by producing the
// invoice, never by flipping a field.
func (s *Service) SetQuoteOutcome(ctx context.Context, quoteID, outcome string) error {
	if !validQuoteOutcomes[outcome] || outcome == QuoteOutcomeAccepted {
		return ErrInvalidQuoteOutcome
	}

	typeQ := db.Rebind("SELECT COALESCE(document_type, 'invoice') FROM invoices WHERE id = ?", s.usePostgres)
	var docType string
	if err := s.db.QueryRowContext(ctx, typeQ, quoteID).Scan(&docType); err == sql.ErrNoRows {
		return ErrInvoiceNotFound
	} else if err != nil {
		return fmt.Errorf("load document type: %w", err)
	}
	if docType != DocumentTypeQuote {
		return ErrNotAQuote
	}

	q := db.Rebind(
		"UPDATE invoices SET quote_outcome = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", s.usePostgres)
	if _, err := s.db.ExecContext(ctx, q, outcome, quoteID); err != nil {
		return fmt.Errorf("set quote outcome: %w", err)
	}
	return nil
}

// loadLines reads an invoice's or offer's lines. Split out so the cursor is
// closed by defer before the caller starts writing on the same transaction.
func (s *Service) loadLines(ctx context.Context, tx *sql.Tx, invoiceID string) ([]LineInput, error) {
	q := db.Rebind(`
		SELECT description, quantity, unit, unit_price, discount_pct, vat_rate, sequence
		FROM invoice_lines WHERE invoice_id = ? ORDER BY sequence`, s.usePostgres)
	rows, err := tx.QueryContext(ctx, q, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("load lines: %w", err)
	}
	defer rows.Close()

	var lines []LineInput
	for rows.Next() {
		var l LineInput
		if err := rows.Scan(&l.Description, &l.Quantity, &l.Unit, &l.UnitPrice,
			&l.DiscountPct, &l.VATRate, &l.Sequence); err != nil {
			return nil, fmt.Errorf("scan line: %w", err)
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read lines: %w", err)
	}
	return lines, nil
}

// ─── Credit notes ─────────────────────────────────────────────────────────────

// CreateCreditNoteRequest describes the correction. Leaving Lines empty credits
// the invoice in full, which is the common case; supplying lines credits part
// of it (a returned item, a disputed position).
type CreateCreditNoteRequest struct {
	IssueDate time.Time
	Reason    *string
	Lines     []LineInput
}

// CreateCreditNote issues a credit note against an invoice.
//
// The link is the point. A credit note that references nothing leaves a
// controller unable to reach the invoice whose VAT it reduced — the
// traceability CO art. 957a al. 2 ch. 5 requires. LTVA art. 27 al. 4 goes
// further and describes a correction as "un document qui mentionne et annule
// la facture d'origine".
//
// Having the link is also what makes the amount checkable: until now nothing
// stopped crediting more than was invoiced.
func (s *Service) CreateCreditNote(ctx context.Context, invoiceID, userID string, req CreateCreditNoteRequest) (*models.Invoice, error) {
	if req.IssueDate.IsZero() {
		req.IssueDate = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	loadQ := db.Rebind(`
		SELECT COALESCE(document_type, 'invoice'), status, contact_id, currency, total_amount, invoice_number
		FROM invoices WHERE id = ?`, s.usePostgres)
	var docType, status, contactID, currency, invoiceNumber string
	var invoiceTotal float64
	if err := tx.QueryRowContext(ctx, loadQ, invoiceID).Scan(
		&docType, &status, &contactID, &currency, &invoiceTotal, &invoiceNumber,
	); err == sql.ErrNoRows {
		return nil, ErrInvoiceNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load invoice: %w", err)
	}

	if docType != DocumentTypeInvoice {
		return nil, ErrNotAnInvoice
	}
	// A draft was never issued and a cancelled invoice is already void; in
	// neither case is there a VAT debt to correct.
	if status != string(models.InvoiceStatusSent) && status != string(models.InvoiceStatusPaid) {
		return nil, ErrInvoiceNotCreditable
	}

	lines := req.Lines
	if len(lines) == 0 {
		// Full credit: re-read the invoice's own lines so the correction
		// matches what was billed, rather than trusting a caller's copy.
		lines, err = s.loadLines(ctx, tx, invoiceID)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			return nil, fmt.Errorf("la facture ne contient aucune ligne à créditer")
		}
	}

	subtotal, vatAmount, total := computeTotals(lines)

	// Guard the total against the invoice, counting credit notes already
	// issued. Cancelled ones do not count — they credit nothing.
	sumQ := db.Rebind(`
		SELECT COALESCE(SUM(total_amount), 0) FROM invoices
		WHERE corrects_invoice_id = ? AND status <> 'cancelled'`, s.usePostgres)
	var alreadyCredited float64
	if err := tx.QueryRowContext(ctx, sumQ, invoiceID).Scan(&alreadyCredited); err != nil {
		return nil, fmt.Errorf("sum existing credit notes: %w", err)
	}
	// One rappen of tolerance: totals are rounded to 0.05 CHF, so crediting an
	// invoice line by line can land a couple of centimes above the original
	// without anyone over-crediting anything.
	if alreadyCredited+total > invoiceTotal+0.01 {
		return nil, fmt.Errorf("%w: %.2f déjà crédités + %.2f demandés > %.2f facturés (%s)",
			ErrCreditExceedsInvoice, alreadyCredited, total, invoiceTotal, invoiceNumber)
	}

	primaryVATRate := 8.1
	if lines[0].VATRate > 0 {
		primaryVATRate = lines[0].VATRate
	}

	creditNoteID := db.NewID()
	number, err := s.nextInvoiceNumber(ctx, tx, DocumentTypeCreditNote, req.IssueDate)
	if err != nil {
		return nil, fmt.Errorf("next credit note number: %w", err)
	}

	insertQ := db.Rebind(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate,
		                      notes, created_by_id, corrects_invoice_id)
		VALUES (?, ?, 'credit_note', ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	issued := req.IssueDate.Format("2006-01-02")
	if _, err := tx.ExecContext(ctx, insertQ,
		creditNoteID, number, contactID, issued, issued,
		currency, subtotal, vatAmount, total, primaryVATRate,
		req.Reason, userID, invoiceID); err != nil {
		return nil, fmt.Errorf("insert credit note: %w", err)
	}

	insertLine := db.Rebind(`
		INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit, unit_price, discount_pct, vat_rate, line_total, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for _, l := range lines {
		lineTotal := l.Quantity * l.UnitPrice * (1 - l.DiscountPct/100)
		if _, err := tx.ExecContext(ctx, insertLine,
			db.NewID(), creditNoteID, l.Description, l.Quantity, l.Unit, l.UnitPrice,
			l.DiscountPct, l.VATRate, lineTotal, l.Sequence); err != nil {
			return nil, fmt.Errorf("insert credit note line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	corrects := invoiceID
	return &models.Invoice{
		ID:                creditNoteID,
		InvoiceNumber:     number,
		DocumentType:      DocumentTypeCreditNote,
		ContactID:         contactID,
		Status:            models.InvoiceStatusDraft,
		IssueDate:         req.IssueDate,
		DueDate:           req.IssueDate,
		Currency:          currency,
		SubtotalAmount:    subtotal,
		VATAmount:         vatAmount,
		TotalAmount:       total,
		VATRate:           primaryVATRate,
		Notes:             req.Reason,
		CorrectsInvoiceID: &corrects,
		CreatedByID:       userID,
	}, nil
}

// recipientSnapshot est l'identité du destinataire au moment de l'émission.
// Elle est copiée sur la facture et n'en bouge plus : c'est ce qui fait de
// celle-ci une pièce comptable autonome, et ce qui permet d'anonymiser un
// contact sans effacer l'identité portée par ses factures.
type recipientSnapshot struct {
	Name       string
	Address    string
	PostalCode string
	City       string
	Country    string
	VATNumber  string
}

// headerVATRate retourne le taux représentatif porté par l'en-tête de facture.
//
// La déclaration TVA agrège les factures en GROUPANT sur cette valeur : elle
// doit donc refléter les lignes, et non une valeur par défaut. Une facture sans
// ligne n'a pas de TVA, d'où 0 plutôt qu'un taux arbitraire.
func headerVATRate(lines []LineInput) float64 {
	if len(lines) == 0 {
		return 0
	}
	return lines[0].VATRate
}

// ─── Garde-fous ──────────────────────────────────────────────────────────────

// ErrVATWithoutNumber refuse d'émettre un document portant de la TVA quand
// aucun numéro de TVA n'est enregistré pour l'entreprise.
// errors.New et non fmt.Errorf : le message contient « 0 % », que le
// vérificateur de format lit comme un verbe inconnu — et il a raison, un
// message statique n'a rien à faire dans une fonction de formatage.
var ErrVATWithoutNumber = errors.New(
	"aucun numéro de TVA n'est enregistré : vous ne pouvez pas facturer de TVA. " +
		"Si vous êtes assujetti, saisissez-le dans Paramètres → Identité ; " +
		"sinon, passez les lignes à 0 % — la LTVA art. 27 al. 1 interdit de faire " +
		"figurer l'impôt, et l'al. 2 vous en rend redevable")

// ErrVATNotLiable refuse la TVA à qui a DÉCLARÉ ne pas y être assujetti.
//
// Le refus est le même que ci-dessus, la phrase ne l'est pas. « Aucun numéro
// n'est enregistré » envoie chercher un numéro ; ici, il n'y en a pas à
// chercher, et rappeler la déclaration faite dit à la fois ce qui bloque et où
// le corriger si elle était fausse.
var ErrVATNotLiable = errors.New(
	"vous avez déclaré ne pas être assujetti à la TVA : la LTVA art. 27 al. 1 " +
		"vous interdit de la faire figurer sur une facture, et l'al. 2 vous en " +
		"rendrait redevable même sans l'avoir encaissée. Passez les lignes à 0 %, " +
		"ou corrigez votre statut dans Paramètres → Banque")

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// checkVATAllowed refuse la TVA à qui n'a pas de numéro de TVA.
//
// Ce n'est pas une préférence de présentation : la LTVA art. 27 al. 1 interdit
// à qui n'est pas inscrit au registre des assujettis de faire figurer l'impôt
// sur ses factures, et l'al. 2 le rend redevable de ce qu'il a mentionné, même
// sans l'avoir encaissé. La LTVA art. 26 al. 2 let. a exige symétriquement ce
// numéro sur toute facture portant de la TVA.
//
// L'absence de fiche société ne bloque pas : une installation neuve doit
// pouvoir établir sa première facture avant d'avoir tout renseigné, et un refus
// à ce stade ressemblerait à une panne.
func (s *Service) checkVATAllowed(ctx context.Context, q querier, vatAmount, vatRate float64) error {
	if vatAmount == 0 && vatRate == 0 {
		return nil
	}
	var vatNumber, vatStatus string
	err := q.QueryRowContext(ctx, db.Rebind(
		`SELECT COALESCE(vat_number, ''), COALESCE(vat_status, '')
		   FROM company_settings LIMIT 1`, s.usePostgres),
	).Scan(&vatNumber, &vatStatus)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load company settings: %w", err)
	}

	// La déclaration passe avant le numéro. Quelqu'un qui s'est déclaré non
	// assujetti n'a pas de numéro à saisir : lui répondre « aucun numéro n'est
	// enregistré » l'enverrait chercher ce qui n'existe pas.
	if vatStatus == models.VatExempt {
		return ErrVATNotLiable
	}
	if strings.TrimSpace(vatNumber) == "" {
		return ErrVATWithoutNumber
	}
	return nil
}

// checkCreditCap refuse qu'une note de crédit, additionnée à ses sœurs, dépasse
// la facture qu'elle corrige. `documentID` est la note elle-même, exclue de la
// somme pour que modifier son montant se compare à ce que les AUTRES ont déjà
// crédité — sans quoi une note se compterait deux fois.
//
// Une tolérance d'un centime : les totaux sont arrondis à 5 centimes, donc
// créditer ligne à ligne peut atterrir un ou deux centimes au-dessus sans que
// personne n'ait sur-crédité quoi que ce soit.
func (s *Service) checkCreditCap(ctx context.Context, q querier, documentID string, newTotal float64) error {
	var correctsID sql.NullString
	var docType string
	if err := q.QueryRowContext(ctx, db.Rebind(
		`SELECT document_type, corrects_invoice_id FROM invoices WHERE id = ?`, s.usePostgres),
		documentID).Scan(&docType, &correctsID); err != nil {
		return nil // le document n'existe pas encore, ou plus : rien à borner
	}
	if docType != DocumentTypeCreditNote || !correctsID.Valid {
		return nil
	}

	var invoiceTotal float64
	var invoiceNumber string
	if err := q.QueryRowContext(ctx, db.Rebind(
		`SELECT total_amount, invoice_number FROM invoices WHERE id = ?`, s.usePostgres),
		correctsID.String).Scan(&invoiceTotal, &invoiceNumber); err != nil {
		return nil
	}

	var others float64
	if err := q.QueryRowContext(ctx, db.Rebind(`
		SELECT COALESCE(SUM(total_amount), 0) FROM invoices
		WHERE corrects_invoice_id = ? AND id <> ? AND status <> 'cancelled'`, s.usePostgres),
		correctsID.String, documentID).Scan(&others); err != nil {
		return fmt.Errorf("sum sibling credit notes: %w", err)
	}

	if others+newTotal > invoiceTotal+0.01 {
		return fmt.Errorf("%w: %.2f déjà crédités + %.2f demandés > %.2f facturés (%s)",
			ErrCreditExceedsInvoice, others, newTotal, invoiceTotal, invoiceNumber)
	}
	return nil
}

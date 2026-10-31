package invoicing

// Reprise de la comptabilisation d'un document déjà envoyé.
//
// # D'où vient le besoin
//
// Jusqu'à la 1.6.5, une facture dont le HT n'était pas un multiple de 0,05 partait
// au client puis voyait son écriture d'émission refusée (audit 9, A-01) : statut
// « envoyée », aucune écriture, et aucun moyen d'y revenir — un second envoi
// répond « transition invalide ». Le correctif de calcul empêche que cela se
// reproduise ; il ne répare pas les documents déjà dans cet état. D'autres causes
// existent aussi (exercice clos au moment de l'envoi, compte retiré du plan).
//
// # Pourquoi une action explicite et non une réparation au démarrage
//
// Le journal est immuable (CO art. 957a) et une installation qui voyait l'échec
// a pu saisir l'écriture à la main. Tout rattraper d'office doublerait le produit
// et la TVA due de celle-là. C'est donc à l'utilisateur de décider, document par
// document, et l'écriture passée porte son nom.
//
// # Ce que la reprise écrit
//
//  1. l'écriture d'émission, si le document n'y est pas lié ;
//  2. l'écriture de chaque encaissement enregistré SANS écriture : RecordPayment ne
//     passe l'encaissement que si l'émission l'était, donc tout paiement saisi
//     pendant que le document était non comptabilisé n'a pas d'écriture non plus.
//
// Idempotente : un second appel ne repasse rien, et un appel interrompu au milieu
// des encaissements reprend où il s'est arrêté.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

var (
	// ErrNothingToPost : tout est déjà au journal. Ce n'est pas une panne, mais le
	// dire évite qu'un second clic ait l'air d'avoir fait quelque chose.
	ErrNothingToPost = errors.New("ce document est déjà comptabilisé : il n'y a rien à reprendre")
	// ErrPostingNotApplicable : brouillon, offre ou annulé. Seul un document
	// envoyé ou encaissé entre dans les livres.
	ErrPostingNotApplicable = errors.New("seul un document envoyé ou encaissé se comptabilise")
	// ErrAutoPostOff : la comptabilité de cette installation est tenue à la main.
	ErrAutoPostOff = errors.New("la comptabilisation automatique des factures est éteinte : saisissez l'écriture à la main ou activez-la dans les paramètres")
	// ErrOriginalNotPosted : une note de crédit contrepasse le produit de sa
	// facture ; passée la première, elle créerait un produit négatif sans contrepartie.
	ErrOriginalNotPosted = errors.New("comptabilisez d'abord la facture que cette note de crédit corrige")
)

// PostPendingResult dit ce que la reprise a écrit.
type PostPendingResult struct {
	JournalEntryID string // écriture d'émission passée ; vide si elle existait déjà
	PaymentEntries int    // encaissements comptabilisés
}

// PostPending reprend la comptabilisation du document : voir l'en-tête du fichier.
func (s *Service) PostPending(ctx context.Context, actor Actor, invoiceID string) (*PostPendingResult, error) {
	if s.accountingSvc == nil {
		return nil, ErrAutoPostOff
	}
	// Les encaissements écrits ici ne doivent pas croiser un RecordPayment.
	payMu.Lock()
	defer payMu.Unlock()

	var inv struct {
		status, number, docType, journalEntryID, createdByID, correctsID string
		issueDate                                                        time.Time
	}
	q := db.Rebind(`
		SELECT status, invoice_number, COALESCE(document_type, 'invoice'),
		       COALESCE(journal_entry_id, ''), created_by_id, issue_date,
		       COALESCE(corrects_invoice_id, '')
		FROM invoices WHERE id = ?`, s.usePostgres)
	if err := s.db.QueryRowContext(ctx, q, invoiceID).Scan(
		&inv.status, &inv.number, &inv.docType, &inv.journalEntryID, &inv.createdByID, &inv.issueDate, &inv.correctsID,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load invoice: %w", err)
	}
	st := models.InvoiceStatus(inv.status)
	if inv.docType == DocumentTypeQuote || (st != models.InvoiceStatusSent && st != models.InvoiceStatusPaid) {
		return nil, ErrPostingNotApplicable
	}
	if !s.autoPostEnabled(ctx) {
		return nil, ErrAutoPostOff
	}

	userID := actor.UserID
	if userID == "" {
		userID = inv.createdByID
	}
	res := &PostPendingResult{}

	if inv.journalEntryID == "" {
		if inv.docType == DocumentTypeCreditNote && inv.correctsID != "" {
			var origLink string
			oq := db.Rebind(`SELECT COALESCE(journal_entry_id, '') FROM invoices WHERE id = ?`, s.usePostgres)
			if err := s.db.QueryRowContext(ctx, oq, inv.correctsID).Scan(&origLink); err != nil {
				return nil, fmt.Errorf("load corrected invoice: %w", err)
			}
			if origLink == "" {
				return nil, ErrOriginalNotPosted
			}
		}
		entryID, err := s.PostIssuedDocument(ctx, invoiceID, inv.docType, inv.number, userID, inv.issueDate, actor.IP)
		if err != nil {
			return nil, err
		}
		inv.journalEntryID = entryID
		res.JournalEntryID = entryID
	}
	if inv.journalEntryID == "" {
		// Un document à zéro n'a rien à porter au journal.
		return nil, ErrNothingToPost
	}

	n, err := s.postUnpostedPayments(ctx, userID, actor.IP, invoiceID, inv.number, inv.docType == DocumentTypeCreditNote)
	res.PaymentEntries = n
	if err != nil {
		return res, err
	}
	if res.JournalEntryID == "" && n == 0 {
		return nil, ErrNothingToPost
	}
	return res, nil
}

// postUnpostedPayments comptabilise les encaissements enregistrés sans écriture.
func (s *Service) postUnpostedPayments(
	ctx context.Context, userID, ip, invoiceID, number string, refund bool,
) (int, error) {
	type pending struct {
		id, method string
		reference  *string
		amount     float64
		date       time.Time
	}
	q := db.Rebind(`
		SELECT id, amount, payment_date, method, reference
		FROM payments
		WHERE invoice_id = ? AND COALESCE(journal_entry_id, '') = ''
		ORDER BY payment_date, created_at`, s.usePostgres)
	rows, err := s.db.QueryContext(ctx, q, invoiceID)
	if err != nil {
		return 0, fmt.Errorf("load payments: %w", err)
	}
	var todo []pending
	for rows.Next() {
		var p pending
		var d string
		if err := rows.Scan(&p.id, &p.amount, &d, &p.method, &p.reference); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan payment: %w", err)
		}
		if len(d) >= 10 {
			d = d[:10]
		}
		if p.date, err = time.Parse("2006-01-02", d); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("payment date %q: %w", d, err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate payments: %w", err)
	}
	_ = rows.Close()

	arID, err := s.accountIDByNumber(ctx, accountReceivables)
	if err != nil {
		return 0, ErrPaymentAccountMissing{Number: accountReceivables}
	}
	done := 0
	for _, p := range todo {
		req := PaymentRequest{InvoiceID: invoiceID, Amount: p.amount, Date: p.date, Method: p.method, Reference: p.reference}
		bankID, err := s.paymentBankAccount(ctx, req)
		if err != nil {
			return done, err
		}
		entryID, err := s.postPaymentEntry(ctx, userID, ip, refund, p.amount, p.date, number, p.id, req, bankID, arID)
		if err != nil {
			return done, err
		}
		upd := db.Rebind(`UPDATE payments SET journal_entry_id = ?, updated_at = ? WHERE id = ?`, s.usePostgres)
		if _, err := s.db.ExecContext(ctx, upd, entryID, time.Now().UTC(), p.id); err != nil {
			return done, fmt.Errorf("l'écriture %s a été passée mais n'a pas pu être liée à l'encaissement %s: %w", entryID, p.id, err)
		}
		done++
	}
	return done, nil
}

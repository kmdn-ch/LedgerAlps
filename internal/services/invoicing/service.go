package invoicing

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	accsvc "github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

var ErrInvoiceNotFound = fmt.Errorf("invoice not found")
var ErrInvalidTransition = fmt.Errorf("invalid status transition")

// validTransitions defines the allowed invoice status state machine.
var validTransitions = map[models.InvoiceStatus][]models.InvoiceStatus{
	models.InvoiceStatusDraft:     {models.InvoiceStatusSent, models.InvoiceStatusCancelled},
	models.InvoiceStatusSent:      {models.InvoiceStatusPaid, models.InvoiceStatusCancelled},
	models.InvoiceStatusPaid:      {models.InvoiceStatusArchived},
	models.InvoiceStatusCancelled: {},
	models.InvoiceStatusArchived:  {},
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

	// Use the first line's VAT rate as the representative rate for the invoice header.
	primaryVATRate := 8.1
	if len(req.Lines) > 0 && req.Lines[0].VATRate > 0 {
		primaryVATRate = req.Lines[0].VATRate
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

	insertInv := db.Rebind(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate,
		                      notes, terms, created_by_id)
		VALUES (?, ?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insertInv,
		invoiceID, number, req.DocumentType, req.ContactID,
		req.IssueDate.Format("2006-01-02"), req.DueDate.Format("2006-01-02"),
		req.Currency, subtotal, vatAmount, total, primaryVATRate,
		req.Notes, req.Terms, userID); err != nil {
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
	if req.DocumentType == "" {
		req.DocumentType = "invoice"
	}

	subtotal, vatAmount, total := computeTotals(req.Lines)
	primaryVATRate := 8.1
	if len(req.Lines) > 0 && req.Lines[0].VATRate > 0 {
		primaryVATRate = req.Lines[0].VATRate
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
	// Load current status, invoice_number, journal_entry_id, created_by_id, and issue_date.
	getQ := db.Rebind(`
		SELECT status, invoice_number, COALESCE(journal_entry_id, ''), created_by_id, issue_date
		FROM invoices WHERE id = ?`, s.usePostgres)
	var current, invoiceNumber, journalEntryID, createdByID string
	var issueDate time.Time
	if err := s.db.QueryRowContext(ctx, getQ, invoiceID).Scan(
		&current, &invoiceNumber, &journalEntryID, &createdByID, &issueDate,
	); err == sql.ErrNoRows {
		return ErrInvoiceNotFound
	} else if err != nil {
		return fmt.Errorf("load invoice: %w", err)
	}

	allowed := validTransitions[models.InvoiceStatus(current)]
	for _, a := range allowed {
		if a == to {
			// Apply the status transition.
			updateQ := db.Rebind("UPDATE invoices SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", s.usePostgres)
			if _, err := s.db.ExecContext(ctx, updateQ, string(to), invoiceID); err != nil {
				return fmt.Errorf("update invoice status: %w", err)
			}

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

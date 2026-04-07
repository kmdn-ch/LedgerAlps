package invoicing

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
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

type Service struct {
	db          *sql.DB
	usePostgres bool
}

func New(database *sql.DB, usePostgres bool) *Service {
	return &Service{db: database, usePostgres: usePostgres}
}

// ─── CreateInvoice ────────────────────────────────────────────────────────────

type LineInput struct {
	Description string
	Quantity    float64
	UnitPrice   float64
	VATRate     float64
	Sequence    int
}

type CreateInvoiceRequest struct {
	ContactID string
	IssueDate time.Time
	DueDate   time.Time
	Currency  string
	VATRate   float64
	Notes     *string
	Terms     *string
	Lines     []LineInput
}

// CreateInvoice creates an invoice with totals rounded to 0.05 CHF (5-Rappen rule).
func (s *Service) CreateInvoice(ctx context.Context, userID string, req CreateInvoiceRequest) (*models.Invoice, error) {
	if len(req.Lines) == 0 {
		return nil, fmt.Errorf("invoice must have at least one line")
	}
	if req.Currency == "" {
		req.Currency = "CHF"
	}

	subtotal, vatAmount, total := computeTotals(req.Lines, req.VATRate)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	invoiceID := db.NewID()
	number, err := s.nextInvoiceNumber(ctx, tx, req.IssueDate)
	if err != nil {
		return nil, fmt.Errorf("next invoice number: %w", err)
	}

	insertInv := db.Rebind(`
		INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate,
		                      notes, terms, created_by_id)
		VALUES (?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insertInv,
		invoiceID, number, req.ContactID,
		req.IssueDate.Format("2006-01-02"), req.DueDate.Format("2006-01-02"),
		req.Currency, subtotal, vatAmount, total, req.VATRate,
		req.Notes, req.Terms, userID); err != nil {
		return nil, fmt.Errorf("insert invoice: %w", err)
	}

	insertLine := db.Rebind(`
		INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price, vat_rate, line_total, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for _, l := range req.Lines {
		lineTotal := l.Quantity * l.UnitPrice * (1 + l.VATRate)
		if _, err := tx.ExecContext(ctx, insertLine,
			db.NewID(), invoiceID, l.Description, l.Quantity, l.UnitPrice, l.VATRate, lineTotal, l.Sequence); err != nil {
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
		ContactID:      req.ContactID,
		Status:         models.InvoiceStatusDraft,
		IssueDate:      req.IssueDate,
		DueDate:        req.DueDate,
		Currency:       req.Currency,
		SubtotalAmount: subtotal,
		VATAmount:      vatAmount,
		TotalAmount:    total,
		VATRate:        req.VATRate,
		Notes:          req.Notes,
		Terms:          req.Terms,
		CreatedByID:    userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// ─── Transition ───────────────────────────────────────────────────────────────

// Transition moves an invoice to the next status if the transition is valid.
func (s *Service) Transition(ctx context.Context, invoiceID string, to models.InvoiceStatus) error {
	getQ := db.Rebind("SELECT status FROM invoices WHERE id = ?", s.usePostgres)
	var current string
	if err := s.db.QueryRowContext(ctx, getQ, invoiceID).Scan(&current); err == sql.ErrNoRows {
		return ErrInvoiceNotFound
	} else if err != nil {
		return fmt.Errorf("load invoice: %w", err)
	}

	allowed := validTransitions[models.InvoiceStatus(current)]
	for _, a := range allowed {
		if a == to {
			updateQ := db.Rebind("UPDATE invoices SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", s.usePostgres)
			_, err := s.db.ExecContext(ctx, updateQ, string(to), invoiceID)
			return err
		}
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, current, to)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// computeTotals calculates subtotal, VAT, and total (rounded to 0.05 CHF).
func computeTotals(lines []LineInput, vatRate float64) (subtotal, vatAmount, total float64) {
	for _, l := range lines {
		subtotal += l.Quantity * l.UnitPrice
	}
	vatAmount = subtotal * vatRate
	total = compliance.RoundTo5Rappen(subtotal + vatAmount)
	// Re-derive vatAmount to be consistent with the rounded total
	vatAmount = compliance.RoundTo5Rappen(vatAmount)
	return
}

// nextInvoiceNumber generates FA-2026-001 style numbers.
func (s *Service) nextInvoiceNumber(ctx context.Context, tx *sql.Tx, date time.Time) (string, error) {
	year := date.Format("2006")
	countQ := db.Rebind("SELECT COUNT(*) FROM invoices WHERE invoice_number LIKE ?", s.usePostgres)
	var count int
	if err := tx.QueryRowContext(ctx, countQ, "FA-"+year+"-%").Scan(&count); err != nil {
		return "", fmt.Errorf("count invoices: %w", err)
	}
	return fmt.Sprintf("FA-%s-%04d", year, count+1), nil
}

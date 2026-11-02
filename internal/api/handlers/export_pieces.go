package handlers

// Les pièces et mouvements de l'archive légale (CO art. 958f), ajoutés sur décision du propriétaire
// (audit croisé Gemini, CRIT-11) : factures fournisseurs, paiements, relevés bancaires, journal d'audit.
//
// L'archive ne contenait que le plan comptable, les écritures, les factures émises, les contacts et les
// exercices. Une charge comptabilisée n'avait plus sa pièce — la facture du fournisseur —, un encaissement
// plus son paiement, un rapprochement plus son relevé, et rien ne prouvait que les livres n'avaient pas
// été retouchés : c'est la chaîne d'empreintes du journal d'audit qui le prouve.

import (
	"context"
	"database/sql"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
)

type exportSupplierInvoiceLine struct {
	ID                 string  `json:"id"`
	SupplierInvoiceID  string  `json:"supplier_invoice_id"`
	Description        string  `json:"description"`
	Quantity           float64 `json:"quantity"`
	UnitPrice          float64 `json:"unit_price"`
	VATRate            float64 `json:"vat_rate"`
	LineTotal          float64 `json:"line_total"`
	ExpenseAccountCode *string `json:"expense_account_code,omitempty"`
	Sequence           int     `json:"sequence"`
}

type exportSupplierInvoice struct {
	ID                 string                      `json:"id"`
	SupplierID         string                      `json:"supplier_id"`
	SupplierReference  string                      `json:"supplier_reference"`
	Status             string                      `json:"status"`
	IssueDate          string                      `json:"issue_date"`
	DueDate            *string                     `json:"due_date,omitempty"`
	Currency           string                      `json:"currency"`
	SubtotalAmount     float64                     `json:"subtotal_amount"`
	VATAmount          float64                     `json:"vat_amount"`
	TotalAmount        float64                     `json:"total_amount"`
	VATRate            float64                     `json:"vat_rate"`
	AmountPaid         float64                     `json:"amount_paid"`
	ExpenseAccountCode *string                     `json:"expense_account_code,omitempty"`
	PaymentReference   *string                     `json:"payment_reference,omitempty"`
	Notes              *string                     `json:"notes,omitempty"`
	JournalEntryID     *string                     `json:"journal_entry_id,omitempty"`
	PaymentEntryID     *string                     `json:"payment_entry_id,omitempty"`
	FiscalYearID       *string                     `json:"fiscal_year_id,omitempty"`
	CreatedByID        *string                     `json:"created_by_id,omitempty"`
	CreatedAt          time.Time                   `json:"created_at"`
	UpdatedAt          time.Time                   `json:"updated_at"`
	Lines              []exportSupplierInvoiceLine `json:"lines"`
}

type exportPayment struct {
	ID             string    `json:"id"`
	InvoiceID      string    `json:"invoice_id"`
	Amount         float64   `json:"amount"`
	PaymentDate    string    `json:"payment_date"`
	Method         string    `json:"method"`
	Reference      *string   `json:"reference,omitempty"`
	JournalEntryID *string   `json:"journal_entry_id,omitempty"`
	CreatedByID    string    `json:"created_by_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type exportBankEntry struct {
	ID            string  `json:"id"`
	Fingerprint   string  `json:"fingerprint"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	IsCredit      bool    `json:"is_credit"`
	BookingDate   string  `json:"booking_date"`
	ValueDate     *string `json:"value_date,omitempty"`
	BankRef       *string `json:"bank_ref,omitempty"`
	EndToEndRef   *string `json:"end_to_end_ref,omitempty"`
	QRReference   *string `json:"qr_reference,omitempty"`
	Counterparty  *string `json:"counterparty,omitempty"`
	Remittance    *string `json:"remittance,omitempty"`
	InvoiceID     *string `json:"invoice_id,omitempty"`
	MatchedAt     *string `json:"matched_at,omitempty"`
	MatchedByID   *string `json:"matched_by_id,omitempty"`
	Ignored       bool    `json:"ignored"`
	IsReversal    bool    `json:"is_reversal"`
	ImportedAt    *string `json:"imported_at,omitempty"`
	CreatedAtText *string `json:"created_at,omitempty"`
}

// exportAuditLog est une ligne du journal d'audit TELLE QUE STOCKÉE : les états avant et après sont déjà
// masqués à l'écriture (nLPD art. 6), et les empreintes sont intactes — c'est ce qui permet de revérifier
// la chaîne plus tard, sans LedgerAlps.
type exportAuditLog struct {
	ID             string  `json:"id"`
	UserID         *string `json:"user_id,omitempty"`
	Action         string  `json:"action"`
	TableName      string  `json:"table_name"`
	RecordID       string  `json:"record_id"`
	BeforeState    *string `json:"before_state,omitempty"`
	AfterState     *string `json:"after_state,omitempty"`
	IPAddress      *string `json:"ip_address,omitempty"`
	EntryHash      *string `json:"entry_hash,omitempty"`
	PrevHash       *string `json:"prev_hash,omitempty"`
	SequenceNumber int64   `json:"sequence_number"`
	HashVersion    int     `json:"hash_version"`
	CreatedAt      *string `json:"created_at,omitempty"`
}

// bornesDeDates rend les bornes de dates des tables qui n'ont pas d'exercice propre : celles de la
// requête, ou à défaut celles de l'exercice demandé. Une borne explicite l'emporte.
func (h *ExportHandler) bornesDeDates(ctx context.Context, f archiveFilters) (from, to string) {
	if f.FiscalYearID != nil {
		_ = h.db.QueryRowContext(ctx,
			db.Rebind(`SELECT start_date, end_date FROM fiscal_years WHERE id = ?`, h.usePostgres),
			*f.FiscalYearID).Scan(&from, &to)
	}
	if f.From != nil {
		from = *f.From
	}
	if f.To != nil {
		to = *f.To
	}
	return from, to
}

func (h *ExportHandler) fetchSupplierInvoices(ctx context.Context, f archiveFilters) ([]exportSupplierInvoice, error) {
	where := " WHERE 1=1"
	args := []any{}
	if f.FiscalYearID != nil {
		where += " AND fiscal_year_id = ?"
		args = append(args, *f.FiscalYearID)
	}
	if f.From != nil {
		where += " AND issue_date >= ?"
		args = append(args, *f.From)
	}
	if f.To != nil {
		where += " AND issue_date <= ?"
		args = append(args, *f.To)
	}

	rows, err := h.db.QueryContext(ctx, db.Rebind(`
		SELECT id, supplier_id, supplier_reference, status, issue_date, due_date, currency,
		       subtotal_amount, vat_amount, total_amount, vat_rate, amount_paid,
		       expense_account_code, payment_reference, notes, journal_entry_id, payment_entry_id,
		       fiscal_year_id, created_by_id, created_at, updated_at
		FROM supplier_invoices`+where+` ORDER BY issue_date, created_at`, h.usePostgres), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invoices := []exportSupplierInvoice{}
	index := map[string]int{}
	for rows.Next() {
		var s exportSupplierInvoice
		if err := rows.Scan(&s.ID, &s.SupplierID, &s.SupplierReference, &s.Status, &s.IssueDate, &s.DueDate,
			&s.Currency, &s.SubtotalAmount, &s.VATAmount, &s.TotalAmount, &s.VATRate, &s.AmountPaid,
			&s.ExpenseAccountCode, &s.PaymentReference, &s.Notes, &s.JournalEntryID, &s.PaymentEntryID,
			&s.FiscalYearID, &s.CreatedByID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Lines = []exportSupplierInvoiceLine{}
		index[s.ID] = len(invoices)
		invoices = append(invoices, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(invoices) == 0 {
		return invoices, nil
	}

	lrows, err := h.db.QueryContext(ctx, db.Rebind(`
		SELECT id, supplier_invoice_id, description, quantity, unit_price, vat_rate, line_total,
		       expense_account_code, sequence
		FROM supplier_invoice_lines
		WHERE supplier_invoice_id IN (SELECT id FROM supplier_invoices`+where+`)
		ORDER BY supplier_invoice_id, sequence`, h.usePostgres), args...)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()
	for lrows.Next() {
		var l exportSupplierInvoiceLine
		if err := lrows.Scan(&l.ID, &l.SupplierInvoiceID, &l.Description, &l.Quantity, &l.UnitPrice,
			&l.VATRate, &l.LineTotal, &l.ExpenseAccountCode, &l.Sequence); err != nil {
			return nil, err
		}
		if i, ok := index[l.SupplierInvoiceID]; ok {
			invoices[i].Lines = append(invoices[i].Lines, l)
		}
	}
	return invoices, lrows.Err()
}

func (h *ExportHandler) fetchPayments(ctx context.Context, f archiveFilters) ([]exportPayment, error) {
	from, to := h.bornesDeDates(ctx, f)
	where, args := " WHERE 1=1", []any{}
	if from != "" {
		where += " AND payment_date >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND payment_date <= ?"
		args = append(args, to)
	}
	rows, err := h.db.QueryContext(ctx, db.Rebind(`
		SELECT id, invoice_id, amount, payment_date, method, reference, journal_entry_id, created_by_id, created_at
		FROM payments`+where+` ORDER BY payment_date, created_at`, h.usePostgres), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []exportPayment{}
	for rows.Next() {
		var p exportPayment
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.PaymentDate, &p.Method, &p.Reference,
			&p.JournalEntryID, &p.CreatedByID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (h *ExportHandler) fetchBankEntries(ctx context.Context, f archiveFilters) ([]exportBankEntry, error) {
	from, to := h.bornesDeDates(ctx, f)
	where, args := " WHERE 1=1", []any{}
	if from != "" {
		where += " AND booking_date >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND booking_date <= ?"
		args = append(args, to)
	}
	rows, err := h.db.QueryContext(ctx, db.Rebind(`
		SELECT id, fingerprint, amount, currency, is_credit, booking_date, value_date, bank_ref,
		       end_to_end_ref, qr_reference, counterparty, remittance, invoice_id, matched_at,
		       matched_by_id, ignored, is_reversal, imported_at, created_at
		FROM bank_entries`+where+` ORDER BY booking_date, id`, h.usePostgres), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []exportBankEntry{}
	for rows.Next() {
		var e exportBankEntry
		var credit, ignored, reversal sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Fingerprint, &e.Amount, &e.Currency, &credit, &e.BookingDate, &e.ValueDate,
			&e.BankRef, &e.EndToEndRef, &e.QRReference, &e.Counterparty, &e.Remittance, &e.InvoiceID,
			&e.MatchedAt, &e.MatchedByID, &ignored, &reversal, &e.ImportedAt, &e.CreatedAtText); err != nil {
			return nil, err
		}
		e.IsCredit, e.Ignored, e.IsReversal = credit.Int64 == 1, ignored.Int64 == 1, reversal.Int64 == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// fetchAuditLogs lit le journal d'audit EN ENTIER, quels que soient les filtres de l'archive : sa chaîne
// d'empreintes (chaque ligne porte celle de la précédente) ne se vérifie que continue. Un extrait d'une
// période se recoupe avec la chaîne complète ; une chaîne trouée ne prouve plus rien.
func (h *ExportHandler) fetchAuditLogs(ctx context.Context) ([]exportAuditLog, error) {
	rows, err := h.db.QueryContext(ctx, db.Rebind(`
		SELECT id, user_id, action, table_name, record_id, before_state, after_state, ip_address,
		       entry_hash, prev_hash, sequence_number, COALESCE(hash_version, 1), created_at
		FROM audit_logs ORDER BY sequence_number`, h.usePostgres))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []exportAuditLog{}
	for rows.Next() {
		var l exportAuditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.TableName, &l.RecordID, &l.BeforeState, &l.AfterState,
			&l.IPAddress, &l.EntryHash, &l.PrevHash, &l.SequenceNumber, &l.HashVersion, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ExportHandler handles legal data export endpoints (CO art. 958f).
type ExportHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewExportHandler(database *sql.DB, usePostgres bool) *ExportHandler {
	return &ExportHandler{db: database, usePostgres: usePostgres}
}

// ─── Manifest types ───────────────────────────────────────────────────────────

type archiveFilters struct {
	FiscalYearID *string `json:"fiscal_year_id,omitempty"`
	From         *string `json:"from,omitempty"`
	To           *string `json:"to,omitempty"`
}

type archiveManifest struct {
	GeneratedAt  string            `json:"generated_at"`
	Version      string            `json:"version"`
	Filters      archiveFilters    `json:"filters"`
	SHA256Hashes map[string]string `json:"sha256_hashes"`
}

// ─── Export DTOs (avoid exposing full IBAN in contacts — nLPD) ───────────────

type exportContact struct {
	ID              string             `json:"id"`
	ContactType     models.ContactType `json:"contact_type"`
	Name            string             `json:"name"`
	Email           *string            `json:"email,omitempty"`
	Phone           *string            `json:"phone,omitempty"`
	Address         *string            `json:"address,omitempty"`
	City            *string            `json:"city,omitempty"`
	PostalCode      *string            `json:"postal_code,omitempty"`
	Country         string             `json:"country"`
	IBANMasked      *string            `json:"iban,omitempty"`   // last 4 digits only (nLPD)
	QRIBANMasked    *string            `json:"qr_iban,omitempty"` // last 4 digits only (nLPD)
	VATNumber       *string            `json:"vat_number,omitempty"`
	PaymentTermDays int                `json:"payment_term_days"`
	IsActive        bool               `json:"is_active"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// maskIBAN replaces all but the last 4 characters of an IBAN with asterisks.
func maskIBAN(iban string) string {
	if len(iban) <= 4 {
		return iban
	}
	return "****" + iban[len(iban)-4:]
}

// ─── LegalArchive GET /api/v1/exports/legal-archive ──────────────────────────

// LegalArchive produces a ZIP archive containing all accounting data required
// for the 10-year retention obligation under CO art. 958f.
// Each file in the archive is JSON (UTF-8); a manifest.json lists SHA-256 hashes.
//
// Query params (all optional):
//   - fiscal_year_id — restrict journal entries and invoices to a fiscal year
//   - from / to      — date range (YYYY-MM-DD); applied to journal date and invoice issue_date
func (h *ExportHandler) LegalArchive(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// ── Parse filters ─────────────────────────────────────────────────────────
	filters := archiveFilters{}
	if fy := c.Query("fiscal_year_id"); fy != "" {
		filters.FiscalYearID = &fy
	}
	if from := c.Query("from"); from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "from must be YYYY-MM-DD"})
			return
		}
		filters.From = &from
	}
	if to := c.Query("to"); to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "to must be YYYY-MM-DD"})
			return
		}
		filters.To = &to
	}

	today := time.Now().Format("2006-01-02")
	dirName := "legal_archive_" + today

	// ── Fetch all data ────────────────────────────────────────────────────────

	accounts, err := h.fetchAccounts(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "accounts query failed: " + err.Error()})
		return
	}

	journalEntries, err := h.fetchJournalEntries(ctx, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "journal_entries query failed: " + err.Error()})
		return
	}

	invoices, err := h.fetchInvoices(ctx, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invoices query failed: " + err.Error()})
		return
	}

	contacts, err := h.fetchContacts(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "contacts query failed: " + err.Error()})
		return
	}

	fiscalYears, err := h.fetchFiscalYears(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fiscal_years query failed: " + err.Error()})
		return
	}

	// ── Marshal all files to JSON bytes ───────────────────────────────────────
	type namedFile struct {
		name string
		data []byte
	}

	files := make([]namedFile, 0, 5)
	for _, nf := range []struct {
		name string
		v    any
	}{
		{"accounts.json", accounts},
		{"journal_entries.json", journalEntries},
		{"invoices.json", invoices},
		{"contacts.json", contacts},
		{"fiscal_years.json", fiscalYears},
	} {
		raw, err := json.MarshalIndent(nf.v, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "json marshal failed for " + nf.name})
			return
		}
		files = append(files, namedFile{nf.name, raw})
	}

	// ── Compute SHA-256 hashes ────────────────────────────────────────────────
	hashes := make(map[string]string, len(files))
	for _, f := range files {
		sum := sha256.Sum256(f.data)
		hashes[f.name] = fmt.Sprintf("%x", sum)
	}

	// ── Build manifest ────────────────────────────────────────────────────────
	manifest := archiveManifest{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Version:      "1.0",
		Filters:      filters,
		SHA256Hashes: hashes,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "manifest marshal failed"})
		return
	}

	// ── Write ZIP in memory ───────────────────────────────────────────────────
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// manifest first
	if err := addZipFile(zw, dirName+"/manifest.json", manifestBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "zip write failed: manifest.json"})
		return
	}

	for _, f := range files {
		if err := addZipFile(zw, dirName+"/"+f.name, f.data); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "zip write failed: " + f.name})
			return
		}
	}

	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "zip finalise failed"})
		return
	}

	// ── Send response ─────────────────────────────────────────────────────────
	filename := fmt.Sprintf("ledgeralps_archive_%s.zip", today)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func addZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (h *ExportHandler) fetchAccounts(ctx context.Context) ([]models.Account, error) {
	q := db.Rebind(`
		SELECT id, code, name, account_type, description, is_active, parent_id, created_at, updated_at
		FROM accounts ORDER BY code`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Account
	for rows.Next() {
		var a models.Account
		var isActive int
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.AccountType, &a.Description,
			&isActive, &a.ParentID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.IsActive = isActive == 1
		result = append(result, a)
	}
	return result, rows.Err()
}

func (h *ExportHandler) fetchFiscalYears(ctx context.Context) ([]models.FiscalYear, error) {
	q := db.Rebind(`
		SELECT id, name, start_date, end_date, is_closed, created_at, updated_at
		FROM fiscal_years ORDER BY start_date`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.FiscalYear
	for rows.Next() {
		var fy models.FiscalYear
		var isClosed int
		if err := rows.Scan(&fy.ID, &fy.Name, &fy.StartDate, &fy.EndDate,
			&isClosed, &fy.CreatedAt, &fy.UpdatedAt); err != nil {
			return nil, err
		}
		fy.IsClosed = isClosed == 1
		result = append(result, fy)
	}
	return result, rows.Err()
}

func (h *ExportHandler) fetchContacts(ctx context.Context) ([]exportContact, error) {
	q := db.Rebind(`
		SELECT id, contact_type, name, email, phone, address, city, postal_code, country,
		       iban, qr_iban, vat_number, payment_term_days, is_active, created_at, updated_at
		FROM contacts ORDER BY name`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []exportContact
	for rows.Next() {
		var ct exportContact
		var isActive int
		var rawIBAN, rawQRIBAN *string
		if err := rows.Scan(&ct.ID, &ct.ContactType, &ct.Name, &ct.Email, &ct.Phone,
			&ct.Address, &ct.City, &ct.PostalCode, &ct.Country,
			&rawIBAN, &rawQRIBAN, &ct.VATNumber, &ct.PaymentTermDays,
			&isActive, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, err
		}
		ct.IsActive = isActive == 1
		// Mask IBAN/QR-IBAN — show only last 4 digits (nLPD art. 5/6)
		if rawIBAN != nil {
			masked := maskIBAN(*rawIBAN)
			ct.IBANMasked = &masked
		}
		if rawQRIBAN != nil {
			masked := maskIBAN(*rawQRIBAN)
			ct.QRIBANMasked = &masked
		}
		result = append(result, ct)
	}
	return result, rows.Err()
}

func (h *ExportHandler) fetchJournalEntries(ctx context.Context, filters archiveFilters) ([]models.JournalEntry, error) {
	where := " WHERE status = 'posted'"
	args := []any{}

	if filters.FiscalYearID != nil {
		where += " AND fiscal_year_id = ?"
		args = append(args, *filters.FiscalYearID)
	}
	if filters.From != nil {
		where += " AND date >= ?"
		args = append(args, *filters.From)
	}
	if filters.To != nil {
		where += " AND date <= ?"
		args = append(args, *filters.To)
	}

	q := db.Rebind(`
		SELECT id, reference, date, description, status, fiscal_year_id, integrity_hash,
		       is_reversal, reversal_of_id, created_by_id, created_at, updated_at
		FROM journal_entries`+where+` ORDER BY date, created_at`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.JournalEntry
	entryIndex := map[string]int{}
	for rows.Next() {
		var e models.JournalEntry
		var isReversal int
		if err := rows.Scan(&e.ID, &e.Reference, &e.Date, &e.Description, &e.Status,
			&e.FiscalYearID, &e.IntegrityHash, &isReversal, &e.ReversalOfID,
			&e.CreatedByID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.IsReversal = isReversal == 1
		e.Lines = []models.JournalLine{}
		entryIndex[e.ID] = len(entries)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return entries, nil
	}

	// Fetch all lines for the matching entries in one query
	// Build IN clause dynamically
	lineWhere := " WHERE entry_id IN (SELECT id FROM journal_entries" + where + ")"
	lineArgs := make([]any, len(args))
	copy(lineArgs, args)

	lq := db.Rebind(`
		SELECT id, entry_id, account_id, debit_amount, credit_amount, description, sequence
		FROM journal_lines`+lineWhere+` ORDER BY entry_id, sequence`, h.usePostgres)

	lrows, err := h.db.QueryContext(ctx, lq, lineArgs...)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()

	for lrows.Next() {
		var l models.JournalLine
		if err := lrows.Scan(&l.ID, &l.EntryID, &l.AccountID,
			&l.DebitAmount, &l.CreditAmount, &l.Description, &l.Sequence); err != nil {
			return nil, err
		}
		if idx, ok := entryIndex[l.EntryID]; ok {
			entries[idx].Lines = append(entries[idx].Lines, l)
		}
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (h *ExportHandler) fetchInvoices(ctx context.Context, filters archiveFilters) ([]models.Invoice, error) {
	where := " WHERE 1=1"
	args := []any{}

	if filters.FiscalYearID != nil {
		where += " AND fiscal_year_id = ?"
		args = append(args, *filters.FiscalYearID)
	}
	if filters.From != nil {
		where += " AND issue_date >= ?"
		args = append(args, *filters.From)
	}
	if filters.To != nil {
		where += " AND issue_date <= ?"
		args = append(args, *filters.To)
	}

	q := db.Rebind(`
		SELECT id, invoice_number, contact_id, status, issue_date, due_date, currency,
		       subtotal_amount, vat_amount, total_amount, vat_rate,
		       notes, terms, qr_reference, journal_entry_id, fiscal_year_id,
		       created_by_id, created_at, updated_at
		FROM invoices`+where+` ORDER BY issue_date, created_at`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []models.Invoice
	invoiceIndex := map[string]int{}
	for rows.Next() {
		var inv models.Invoice
		if err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.ContactID, &inv.Status,
			&inv.IssueDate, &inv.DueDate, &inv.Currency,
			&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate,
			&inv.Notes, &inv.Terms, &inv.QRReference, &inv.JournalEntryID, &inv.FiscalYearID,
			&inv.CreatedByID, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			return nil, err
		}
		inv.Lines = []models.InvoiceLine{}
		invoiceIndex[inv.ID] = len(invoices)
		invoices = append(invoices, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(invoices) == 0 {
		return invoices, nil
	}

	lineWhere := " WHERE invoice_id IN (SELECT id FROM invoices" + where + ")"
	lineArgs := make([]any, len(args))
	copy(lineArgs, args)

	lq := db.Rebind(`
		SELECT id, invoice_id, description, quantity, unit_price, vat_rate, line_total, sequence
		FROM invoice_lines`+lineWhere+` ORDER BY invoice_id, sequence`, h.usePostgres)

	lrows, err := h.db.QueryContext(ctx, lq, lineArgs...)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()

	for lrows.Next() {
		var l models.InvoiceLine
		if err := lrows.Scan(&l.ID, &l.InvoiceID, &l.Description,
			&l.Quantity, &l.UnitPrice, &l.VATRate, &l.LineTotal, &l.Sequence); err != nil {
			return nil, err
		}
		if idx, ok := invoiceIndex[l.InvoiceID]; ok {
			invoices[idx].Lines = append(invoices[idx].Lines, l)
		}
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	return invoices, nil
}

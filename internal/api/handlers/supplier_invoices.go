package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// SupplierInvoicesHandler manages received supplier invoices (factures d'achat).
//
// These are the counterpart of sales invoices: they carry the input VAT
// (impôt préalable) that offsets collected VAT on the AFC declaration.
type SupplierInvoicesHandler struct {
	db          *sql.DB
	usePostgres bool
	// accountingSvc ecrit au journal a la comptabilisation. Nil dans les tests
	// qui n'exercent que la saisie.
	accountingSvc *accounting.Service
}

func NewSupplierInvoicesHandler(database *sql.DB, usePostgres bool) *SupplierInvoicesHandler {
	return &SupplierInvoicesHandler{db: database, usePostgres: usePostgres}
}

// WithAccounting branche l'ecriture au journal a la comptabilisation.
func (h *SupplierInvoicesHandler) WithAccounting(svc *accounting.Service) *SupplierInvoicesHandler {
	h.accountingSvc = svc
	return h
}

// supplierInvoiceStatuses are the permitted status values.
// Only 'booked' and 'paid' count towards the VAT declaration.
var supplierInvoiceStatuses = map[string]bool{
	"draft": true, "booked": true, "paid": true, "cancelled": true,
}

type supplierInvoiceLine struct {
	ID                 string  `json:"id,omitempty"`
	Description        string  `json:"description" binding:"required,min=1,max=500"`
	Quantity           float64 `json:"quantity"`
	UnitPrice          float64 `json:"unit_price"`
	VATRate            float64 `json:"vat_rate"`
	LineTotal          float64 `json:"line_total"`
	ExpenseAccountCode string  `json:"expense_account_code"`
	Sequence           int     `json:"sequence"`
}

type supplierInvoice struct {
	ID                 string                `json:"id"`
	SupplierID         string                `json:"supplier_id"`
	SupplierName       string                `json:"supplier_name,omitempty"`
	SupplierReference  string                `json:"supplier_reference"`
	Status             string                `json:"status"`
	IssueDate          string                `json:"issue_date"`
	DueDate            string                `json:"due_date,omitempty"`
	Currency           string                `json:"currency"`
	SubtotalAmount     float64               `json:"subtotal_amount"`
	VATAmount          float64               `json:"vat_amount"`
	TotalAmount        float64               `json:"total_amount"`
	VATRate            float64               `json:"vat_rate"`
	AmountPaid         float64               `json:"amount_paid"`
	ExpenseAccountCode string                `json:"expense_account_code,omitempty"`
	// PaymentReference est la reference du bulletin de versement — celle qui
	// voyage dans l'ordre de virement pour que le fournisseur rapproche
	// l'encaissement. A ne pas confondre avec SupplierReference, qui est le
	// numero de la facture chez lui.
	PaymentReference string    `json:"payment_reference,omitempty"`
	JournalEntryID   string    `json:"journal_entry_id,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	Lines              []supplierInvoiceLine `json:"lines,omitempty"`
}

type createSupplierInvoiceRequest struct {
	SupplierID         string                `json:"supplier_id"         binding:"required"`
	SupplierReference  string                `json:"supplier_reference"  binding:"required,min=1,max=100"`
	IssueDate          string                `json:"issue_date"          binding:"required"`
	DueDate            string                `json:"due_date"`
	Currency           string                `json:"currency"`
	ExpenseAccountCode string                `json:"expense_account_code"`
	PaymentReference   string                `json:"payment_reference"`
	Notes              string                `json:"notes"`
	Lines              []supplierInvoiceLine `json:"lines"              binding:"required,min=1,dive"`
}

// computeTotals derives HT, VAT and TTC from the lines. Amounts are rounded to
// 5 rappen, the Swiss cash-rounding rule already used across the codebase.
//
// Returns the dominant VAT rate for the header (the rate carrying the largest
// base), which is only a convenience for display — per-rate figures for the
// declaration always come from the lines.
func computeTotals(lines []supplierInvoiceLine) (subtotal, vat, total, dominantRate float64) {
	baseByRate := map[float64]float64{}
	for i := range lines {
		l := &lines[i]
		if l.Quantity == 0 {
			l.Quantity = 1
		}
		lineHT := l.Quantity * l.UnitPrice
		l.LineTotal = lineHT
		subtotal += lineHT
		vat += lineHT * l.VATRate
		baseByRate[l.VATRate] += lineHT
	}
	var maxBase float64
	for rate, base := range baseByRate {
		if base > maxBase {
			maxBase, dominantRate = base, rate
		}
	}
	subtotal = compliance.RoundTo5Rappen(subtotal)
	vat = compliance.RoundTo5Rappen(vat)
	total = compliance.RoundTo5Rappen(subtotal + vat)
	return subtotal, vat, total, dominantRate
}

// ListSupplierInvoices GET /api/v1/supplier-invoices
func (h *SupplierInvoicesHandler) ListSupplierInvoices(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	where := " WHERE 1=1"
	args := []any{}
	if status := c.Query("status"); status != "" {
		if !supplierInvoiceStatuses[status] {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "statut inconnu " + status})
			return
		}
		where += " AND si.status = ?"
		args = append(args, status)
	}
	if supplierID := c.Query("supplier_id"); supplierID != "" {
		where += " AND si.supplier_id = ?"
		args = append(args, supplierID)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var total int
	countQ := db.Rebind("SELECT COUNT(*) FROM supplier_invoices si"+where, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	listQ := db.Rebind(`
		SELECT si.id, si.supplier_id, COALESCE(ct.name, ''), si.supplier_reference, si.status,
		       si.issue_date, COALESCE(si.due_date, ''), si.currency,
		       si.subtotal_amount, si.vat_amount, si.total_amount, si.vat_rate,
		       si.amount_paid, COALESCE(si.expense_account_code, ''),
		       COALESCE(si.payment_reference, ''), COALESCE(si.journal_entry_id, ''),
		       COALESCE(si.notes, ''), si.created_at
		FROM supplier_invoices si
		LEFT JOIN contacts ct ON ct.id = si.supplier_id`+where+`
		ORDER BY si.issue_date DESC, si.created_at DESC
		LIMIT ? OFFSET ?`, h.usePostgres)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := h.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	items := []supplierInvoice{}
	for rows.Next() {
		var s supplierInvoice
		if err := rows.Scan(&s.ID, &s.SupplierID, &s.SupplierName, &s.SupplierReference, &s.Status,
			&s.IssueDate, &s.DueDate, &s.Currency, &s.SubtotalAmount, &s.VATAmount,
			&s.TotalAmount, &s.VATRate, &s.AmountPaid, &s.ExpenseAccountCode,
			&s.PaymentReference, &s.JournalEntryID, &s.Notes, &s.CreatedAt); err != nil {
			// Le detail part dans la reponse : un decalage entre les colonnes
			// lues et les destinations du Scan ne se voit qu'ici, et « database
			// error » le rend indiscernable d'une base injoignable. C'est
			// exactement ce qui s'est produit — deux colonnes ajoutees au SELECT
			// sans l'etre au Scan, et l'ecran ne disait rien de plus que « les
			// factures n'ont pas pu etre lues ».
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "lecture des factures fournisseurs: " + err.Error()})
			return
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	pages := (total + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, gin.H{
		"items": items, "total": total, "page": page, "pages": pages,
	})
}

// GetSupplierInvoice GET /api/v1/supplier-invoices/:id
func (h *SupplierInvoicesHandler) GetSupplierInvoice(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var s supplierInvoice
	q := db.Rebind(`
		SELECT si.id, si.supplier_id, COALESCE(ct.name, ''), si.supplier_reference, si.status,
		       si.issue_date, COALESCE(si.due_date, ''), si.currency,
		       si.subtotal_amount, si.vat_amount, si.total_amount, si.vat_rate,
		       si.amount_paid, COALESCE(si.expense_account_code, ''),
		       COALESCE(si.payment_reference, ''), COALESCE(si.journal_entry_id, ''),
		       COALESCE(si.notes, ''), si.created_at
		FROM supplier_invoices si
		LEFT JOIN contacts ct ON ct.id = si.supplier_id
		WHERE si.id = ?`, h.usePostgres)
	err := h.db.QueryRowContext(ctx, q, id).Scan(&s.ID, &s.SupplierID, &s.SupplierName,
		&s.SupplierReference, &s.Status, &s.IssueDate, &s.DueDate, &s.Currency,
		&s.SubtotalAmount, &s.VATAmount, &s.TotalAmount, &s.VATRate, &s.AmountPaid,
		&s.ExpenseAccountCode, &s.PaymentReference, &s.JournalEntryID,
		&s.Notes, &s.CreatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "facture fournisseur introuvable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	lines, err := h.loadLines(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	s.Lines = lines
	c.JSON(http.StatusOK, s)
}

func (h *SupplierInvoicesHandler) loadLines(ctx context.Context, invoiceID string) ([]supplierInvoiceLine, error) {
	q := db.Rebind(`
		SELECT id, description, quantity, unit_price, vat_rate, line_total,
		       COALESCE(expense_account_code, ''), sequence
		FROM supplier_invoice_lines
		WHERE supplier_invoice_id = ?
		ORDER BY sequence`, h.usePostgres)
	rows, err := h.db.QueryContext(ctx, q, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []supplierInvoiceLine{}
	for rows.Next() {
		var l supplierInvoiceLine
		if err := rows.Scan(&l.ID, &l.Description, &l.Quantity, &l.UnitPrice,
			&l.VATRate, &l.LineTotal, &l.ExpenseAccountCode, &l.Sequence); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

// CreateSupplierInvoice POST /api/v1/supplier-invoices
func (h *SupplierInvoicesHandler) CreateSupplierInvoice(c *gin.Context) {
	var req createSupplierInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if _, err := time.Parse("2006-01-02", req.IssueDate); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'émission doit être au format AAAA-MM-JJ"})
		return
	}
	if req.DueDate != "" {
		if _, err := time.Parse("2006-01-02", req.DueDate); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'échéance doit être au format AAAA-MM-JJ"})
			return
		}
	}
	if req.Currency == "" {
		req.Currency = "CHF"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// The supplier must exist and be flagged as one — booking a purchase
	// against a customer record is almost always a data-entry mistake.
	var contactType string
	supQ := db.Rebind("SELECT contact_type FROM contacts WHERE id = ?", h.usePostgres)
	switch err := h.db.QueryRowContext(ctx, supQ, req.SupplierID).Scan(&contactType); {
	case err == sql.ErrNoRows:
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "fournisseur introuvable"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if contactType != "supplier" && contactType != "both" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": fmt.Sprintf("contact is of type %q; expected 'supplier' or 'both'", contactType),
		})
		return
	}

	subtotal, vat, total, dominantRate := computeTotals(req.Lines)

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer tx.Rollback()

	id := db.NewID()
	now := time.Now().UTC()
	var duePtr *string
	if req.DueDate != "" {
		duePtr = &req.DueDate
	}
	var userPtr *string
	if uid := currentUserID(c); uid != "" {
		userPtr = &uid
	}

	insQ := db.Rebind(`
		INSERT INTO supplier_invoices
		  (id, supplier_id, supplier_reference, status, issue_date, due_date, currency,
		   subtotal_amount, vat_amount, total_amount, vat_rate, amount_paid,
		   expense_account_code, payment_reference, notes, created_by_id, created_at, updated_at)
		VALUES (?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
	if _, err := tx.ExecContext(ctx, insQ, id, req.SupplierID, req.SupplierReference,
		req.IssueDate, duePtr, req.Currency, subtotal, vat, total, dominantRate,
		nullIfEmpty(req.ExpenseAccountCode), strings.TrimSpace(req.PaymentReference),
		nullIfEmpty(req.Notes), userPtr, now, now); err != nil {
		// The UNIQUE(supplier_id, supplier_reference) constraint is the
		// duplicate-payment guard; report it as a conflict, not a 500.
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "cette facture fournisseur est déjà enregistrée (même fournisseur, même référence)",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	lineQ := db.Rebind(`
		INSERT INTO supplier_invoice_lines
		  (id, supplier_invoice_id, description, quantity, unit_price, vat_rate,
		   line_total, expense_account_code, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
	for i, l := range req.Lines {
		if _, err := tx.ExecContext(ctx, lineQ, db.NewID(), id, l.Description,
			l.Quantity, l.UnitPrice, l.VATRate, l.LineTotal,
			nullIfEmpty(l.ExpenseAccountCode), i); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	// Une création ne remplace rien : `before_state` reste NULL, ce qui se
	// distingue d'un état antérieur vide.
	trace(c, h.db, h.usePostgres, TableSupplierInvoices,
		ActionSupplierInvoiceCreated, id, accounting.Creation(map[string]any{
			"reference": req.SupplierReference,
			"total":     total,
			"currency":  req.Currency,
		}))

	c.JSON(http.StatusCreated, gin.H{
		"id": id, "status": "draft",
		"subtotal_amount": subtotal, "vat_amount": vat, "total_amount": total,
	})
}

type transitionSupplierInvoiceRequest struct {
	Status string `json:"status" binding:"required"`
}

// TransitionSupplierInvoice POST /api/v1/supplier-invoices/:id/transition
//
// Only 'booked' and 'paid' invoices contribute input VAT to the declaration,
// so the transition is what makes an expense count fiscally.
func (h *SupplierInvoicesHandler) TransitionSupplierInvoice(c *gin.Context) {
	id := c.Param("id")
	var req transitionSupplierInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if !supplierInvoiceStatuses[req.Status] {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "statut inconnu " + req.Status})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var current string
	getQ := db.Rebind("SELECT status FROM supplier_invoices WHERE id = ?", h.usePostgres)
	switch err := h.db.QueryRowContext(ctx, getQ, id).Scan(&current); {
	case err == sql.ErrNoRows:
		c.JSON(http.StatusNotFound, gin.H{"error": "facture fournisseur introuvable"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if current == "cancelled" {
		c.JSON(http.StatusConflict, gin.H{"error": "une facture fournisseur annulée ne change plus de statut"})
		return
	}

	// Comptabiliser ECRIT au journal. Le statut l'annoncait depuis l'origine
	// sans que rien ne l'ecrive : la charge n'entrait dans les livres que si
	// quelqu'un la saisissait a la main, pendant que la TVA deductible
	// alimentait deja la declaration. L'echec bloque la transition — rien n'est
	// engage vis-a-vis d'un tiers, et laisser passer le statut sans l'ecriture
	// recreerait exactement ce defaut.
	var entryID string
	if req.Status == "booked" && current != "booked" && current != "paid" {
		userID := ""
		if claims := mw.GetClaims(c); claims != nil {
			userID = claims.UserID
		}
		var err error
		entryID, err = h.postSupplierInvoice(ctx, id, userID, c.ClientIP())
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
	}

	updQ := db.Rebind("UPDATE supplier_invoices SET status = ?, updated_at = ? WHERE id = ?", h.usePostgres)
	if _, err := h.db.ExecContext(ctx, updQ, req.Status, time.Now().UTC(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	action := ActionSupplierInvoiceBooked
	if req.Status != "booked" {
		action = ActionSupplierInvoiceUpdated
	}
	// « from / to » écrit à la main disparaît : l'état antérieur a désormais sa
	// place, et la vérification le relit au même titre que le suivant.
	trace(c, h.db, h.usePostgres, TableSupplierInvoices, action, id,
		accounting.Modification(
			map[string]any{"status": current},
			map[string]any{"status": req.Status, "journal_entry_id": entryID},
		))

	c.JSON(http.StatusOK, gin.H{"id": id, "status": req.Status, "journal_entry_id": entryID})
}

// DeleteSupplierInvoice DELETE /api/v1/supplier-invoices/:id
//
// Only drafts may be deleted. Once booked, an expense is part of the accounting
// record the CO (art. 958f) requires be preserved — it must be cancelled, which
// leaves a trace, rather than erased.
func (h *SupplierInvoicesHandler) DeleteSupplierInvoice(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// On relit ce qui va disparaître, pas seulement son statut.
	//
	// La trace survit à la pièce, et c'est le seul cas où elle est la SEULE
	// chose qui reste : « une facture fournisseur a été supprimée » sans dire
	// laquelle ni de quel montant ne répond à aucune question que l'on se pose
	// après coup. Le nom du fournisseur est masqué à l'écriture (nLPD art. 6) ;
	// la référence et le montant, eux, sont des données comptables.
	var status, referenceFournisseur, devise string
	var montantTotal float64
	getQ := db.Rebind(`
		SELECT status, COALESCE(supplier_reference, ''), COALESCE(currency, ''),
		       COALESCE(total_amount, 0)
		  FROM supplier_invoices WHERE id = ?`, h.usePostgres)
	switch err := h.db.QueryRowContext(ctx, getQ, id).Scan(
		&status, &referenceFournisseur, &devise, &montantTotal); {
	case err == sql.ErrNoRows:
		c.JSON(http.StatusNotFound, gin.H{"error": "facture fournisseur introuvable"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if status != "draft" {
		c.JSON(http.StatusConflict, gin.H{
			"error": "seule une facture fournisseur au brouillon peut être supprimée. Annulez-la plutôt : la pièce doit être conservée (CO art. 958f)",
		})
		return
	}

	delQ := db.Rebind("DELETE FROM supplier_invoices WHERE id = ?", h.usePostgres)
	if _, err := h.db.ExecContext(ctx, delQ, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	trace(c, h.db, h.usePostgres, TableSupplierInvoices,
		ActionSupplierInvoiceDeleted, id, accounting.Suppression(map[string]any{
			"status":    status,
			"reference": referenceFournisseur,
			"total":     montantTotal,
			"currency":  devise,
		}))
	c.Status(http.StatusNoContent)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isUniqueViolation reports whether err is a unique-constraint failure on
// either supported driver (modernc SQLite and pgx surface different texts).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique_violation")
}

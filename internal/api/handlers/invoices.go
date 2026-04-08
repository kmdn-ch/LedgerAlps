package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/invoicing"
)

type InvoicesHandler struct {
	db          *sql.DB
	usePostgres bool
	svc         *invoicing.Service
}

func NewInvoicesHandler(database *sql.DB, usePostgres bool, acctSvc *accounting.Service) *InvoicesHandler {
	return &InvoicesHandler{
		db:          database,
		usePostgres: usePostgres,
		svc:         invoicing.NewWithAccounting(database, usePostgres, acctSvc),
	}
}

// ListInvoices GET /api/v1/invoices
func (h *InvoicesHandler) ListInvoices(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	status := c.Query("status")
	where := " WHERE 1=1"
	args := []any{}

	// Data isolation: non-admin users only see their own invoices (nLPD art. 6)
	if uid := currentUserID(c); uid != "" && !isAdmin(c) {
		where += " AND created_by_id = ?"
		args = append(args, uid)
	}

	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	countQ := db.Rebind("SELECT COUNT(*) FROM invoices"+where, h.usePostgres)
	var total int
	if err := h.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	listQ := db.Rebind(`
		SELECT id, invoice_number, contact_id, status, issue_date, due_date, currency,
		       subtotal_amount, vat_amount, total_amount, vat_rate, notes, terms, created_at, updated_at
		FROM invoices`+where+` ORDER BY issue_date DESC, created_at DESC LIMIT ? OFFSET ?`, h.usePostgres)
	offset := (page - 1) * pageSize
	rows, err := h.db.QueryContext(ctx, listQ, append(args, pageSize, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	invoices := []models.Invoice{}
	for rows.Next() {
		var inv models.Invoice
		if err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.ContactID, &inv.Status,
			&inv.IssueDate, &inv.DueDate, &inv.Currency,
			&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate,
			&inv.Notes, &inv.Terms, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		invoices = append(invoices, inv)
	}

	pages := (total + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"items": invoices, "total": total, "page": page, "page_size": pageSize, "pages": pages,
	})
}

// GetInvoice GET /api/v1/invoices/:id
func (h *InvoicesHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, invoice_number, contact_id, status, issue_date, due_date, currency,
		       subtotal_amount, vat_amount, total_amount, vat_rate, notes, terms, created_at, updated_at
		FROM invoices WHERE id = ?`, h.usePostgres)

	var inv models.Invoice
	err := h.db.QueryRowContext(ctx, q, id).Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.ContactID, &inv.Status,
		&inv.IssueDate, &inv.DueDate, &inv.Currency,
		&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate,
		&inv.Notes, &inv.Terms, &inv.CreatedAt, &inv.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, inv)
}

type createInvoiceRequest struct {
	ContactID string    `json:"contact_id" binding:"required"`
	IssueDate string    `json:"issue_date" binding:"required"`
	DueDate   string    `json:"due_date" binding:"required"`
	Currency  string    `json:"currency"`
	VATRate   float64   `json:"vat_rate"`
	Notes     *string   `json:"notes"`
	Terms     *string   `json:"terms"`
	Lines     []lineReq `json:"lines" binding:"required,min=1"`
}

type lineReq struct {
	Description string  `json:"description" binding:"required"`
	Quantity    float64 `json:"quantity" binding:"required,gt=0"`
	UnitPrice   float64 `json:"unit_price" binding:"required"`
	VATRate     float64 `json:"vat_rate"`
	Sequence    int     `json:"sequence"`
}

// CreateInvoice POST /api/v1/invoices
func (h *InvoicesHandler) CreateInvoice(c *gin.Context) {
	var req createInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "issue_date must be YYYY-MM-DD"})
		return
	}
	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "due_date must be YYYY-MM-DD"})
		return
	}

	lines := make([]invoicing.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = invoicing.LineInput{
			Description: l.Description,
			Quantity:    l.Quantity,
			UnitPrice:   l.UnitPrice,
			VATRate:     l.VATRate,
			Sequence:    l.Sequence,
		}
	}

	claims := mw.GetClaims(c)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}
	inv, err := h.svc.CreateInvoice(c.Request.Context(), userID, invoicing.CreateInvoiceRequest{
		ContactID: req.ContactID,
		IssueDate: issueDate,
		DueDate:   dueDate,
		Currency:  req.Currency,
		VATRate:   req.VATRate,
		Notes:     req.Notes,
		Terms:     req.Terms,
		Lines:     lines,
	})
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, inv)
}

// TransitionInvoice POST /api/v1/invoices/:id/transition
func (h *InvoicesHandler) TransitionInvoice(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Transition(c.Request.Context(), id, models.InvoiceStatus(body.Status)); err != nil {
		switch err {
		case invoicing.ErrInvoiceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case invoicing.ErrInvalidTransition:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		}
		return
	}
	h.GetInvoice(c)
}


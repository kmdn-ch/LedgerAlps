package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/invoicing"
)

// ─── PaymentsHandler ──────────────────────────────────────────────────────────

type PaymentsHandler struct {
	db          *sql.DB
	usePostgres bool
	svc         *invoicing.Service
}

func NewPaymentsHandler(database *sql.DB, usePostgres bool, acctSvc *accounting.Service) *PaymentsHandler {
	return &PaymentsHandler{
		db: database, usePostgres: usePostgres,
		svc: invoicing.NewWithAccounting(database, usePostgres, acctSvc),
	}
}

// ─── POST /api/v1/payments ────────────────────────────────────────────────────

type createPaymentRequest struct {
	InvoiceID     string  `json:"invoice_id"     binding:"required"`
	Amount        float64 `json:"amount"         binding:"required,gt=0"`
	PaymentDate   string  `json:"payment_date"   binding:"required"` // YYYY-MM-DD
	Method        string  `json:"method"         binding:"required,oneof=bank_transfer cash card check other"`
	Reference     *string `json:"reference"`
	BankAccountID *string `json:"bank_account_id"` // optional override; defaults to the cash or bank account
}

// CreatePayment records a payment receipt against an invoice.
//
// Tout le travail est dans invoicing.RecordPayment, le même chemin que « Marquer
// payée » : chaque versement — partiel ou non — est plafonné au solde, cumulé,
// passé au journal (banque ou caisse / débiteurs) et solde la facture quand le
// cumul atteint le total.
func (h *PaymentsHandler) CreatePayment(c *gin.Context) {
	var req createPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		refus(c, http.StatusBadRequest, err)
		return
	}

	paymentDate, err := time.Parse("2006-01-02", req.PaymentDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "la date de paiement doit être au format AAAA-MM-JJ"})
		return
	}

	actor := invoicing.Actor{IP: c.ClientIP()}
	if claims := mw.GetClaims(c); claims != nil {
		actor.UserID = claims.UserID
	}
	bankAccountID := ""
	if req.BankAccountID != nil {
		bankAccountID = *req.BankAccountID
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	res, err := h.svc.RecordPayment(ctx, actor, invoicing.PaymentRequest{
		InvoiceID:     req.InvoiceID,
		Amount:        req.Amount,
		Date:          paymentDate,
		Method:        req.Method,
		Reference:     req.Reference,
		BankAccountID: bankAccountID,
	})
	if err != nil {
		var missing invoicing.ErrPaymentAccountMissing
		switch {
		case errors.Is(err, invoicing.ErrInvoiceNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		// 409 : la demande est bien formée, elle se heurte à l'état de la facture.
		// Le motif stable laisse l'interface expliquer au lieu d'échouer en général.
		case errors.Is(err, invoicing.ErrInvoiceNotPayable):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "reason": "invoice_not_payable"})
		case errors.Is(err, invoicing.ErrPaymentExceedsBalance):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "reason": "payment_exceeds_balance"})
		case errors.As(err, &missing):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			refus(c, http.StatusUnprocessableEntity, err)
		}
		return
	}

	body := gin.H{
		"payment":        res.Payment,
		"invoice_status": string(res.Status),
		"amount_paid":    res.AmountPaid,
		"balance":        res.Balance,
		// Faux quand l'émission de la facture n'a pas été comptabilisée : la
		// comptabilité de cette installation est tenue à la main, et passer
		// l'encaissement seul porterait 1100 au crédit sans débit.
		"journal_posted": res.JournalEntryID != "",
	}
	if res.JournalEntryID != "" {
		body["journal_entry_id"] = res.JournalEntryID
	}
	c.JSON(http.StatusCreated, body)
}

// ─── GET /api/v1/payments ─────────────────────────────────────────────────────

// ListPayments returns payments, optionally filtered by invoice_id.
func (h *PaymentsHandler) ListPayments(c *gin.Context) {
	invoiceID := c.Query("invoice_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	baseQ := `
		SELECT id, invoice_id, amount, payment_date, method, reference, journal_entry_id, created_by_id, created_at, updated_at
		FROM payments`
	args := []any{}

	if invoiceID != "" {
		baseQ += " WHERE invoice_id = ?"
		args = append(args, invoiceID)
	}
	baseQ += " ORDER BY payment_date DESC, created_at DESC"

	rows, err := h.db.QueryContext(ctx, db.Rebind(baseQ, h.usePostgres), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	payments := []models.Payment{}
	for rows.Next() {
		var p models.Payment
		if err := rows.Scan(
			&p.ID, &p.InvoiceID, &p.Amount, &p.PaymentDate,
			&p.Method, &p.Reference, &p.JournalEntryID,
			&p.CreatedByID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": payments, "total": len(payments)})
}

// ─── GET /api/v1/payments/:id ─────────────────────────────────────────────────

// GetPayment returns a single payment by ID.
func (h *PaymentsHandler) GetPayment(c *gin.Context) {
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, invoice_id, amount, payment_date, method, reference, journal_entry_id, created_by_id, created_at, updated_at
		FROM payments WHERE id = ?`, h.usePostgres)

	var p models.Payment
	err := h.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.InvoiceID, &p.Amount, &p.PaymentDate,
		&p.Method, &p.Reference, &p.JournalEntryID,
		&p.CreatedByID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "paiement introuvable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	c.JSON(http.StatusOK, p)
}

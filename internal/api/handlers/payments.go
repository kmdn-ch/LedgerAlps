package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// ─── PaymentsHandler ──────────────────────────────────────────────────────────

type PaymentsHandler struct {
	db          *sql.DB
	usePostgres bool
	acctSvc     *accounting.Service
}

func NewPaymentsHandler(database *sql.DB, usePostgres bool, acctSvc *accounting.Service) *PaymentsHandler {
	return &PaymentsHandler{db: database, usePostgres: usePostgres, acctSvc: acctSvc}
}

// ─── POST /api/v1/payments ────────────────────────────────────────────────────

type createPaymentRequest struct {
	InvoiceID     string  `json:"invoice_id"     binding:"required"`
	Amount        float64 `json:"amount"         binding:"required,gt=0"`
	PaymentDate   string  `json:"payment_date"   binding:"required"` // YYYY-MM-DD
	Method        string  `json:"method"         binding:"required,oneof=bank_transfer cash card check other"`
	Reference     *string `json:"reference"`
	BankAccountID *string `json:"bank_account_id"` // optional override; defaults to account code 1020
}

// CreatePayment records a payment receipt against an invoice.
// If amount >= invoice total, the invoice is transitioned to 'paid' and a
// journal entry (Dr Bank/Cash → Cr Accounts Receivable) is created and posted.
func (h *PaymentsHandler) CreatePayment(c *gin.Context) {
	var req createPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	paymentDate, err := time.Parse("2006-01-02", req.PaymentDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payment_date must be YYYY-MM-DD"})
		return
	}

	userID := currentUserID(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// 1. Load invoice — must exist and be in status 'sent'.
	invQ := db.Rebind(`
		SELECT id, total_amount, status, contact_id, invoice_number
		FROM invoices WHERE id = ?`, h.usePostgres)
	var inv struct {
		ID            string
		TotalAmount   float64
		Status        string
		ContactID     string
		InvoiceNumber string
	}
	if err := h.db.QueryRowContext(ctx, invQ, req.InvoiceID).Scan(
		&inv.ID, &inv.TotalAmount, &inv.Status, &inv.ContactID, &inv.InvoiceNumber,
	); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if inv.Status != string(models.InvoiceStatusSent) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": fmt.Sprintf("invoice is in status '%s'; only 'sent' invoices accept payments", inv.Status),
		})
		return
	}

	// 2. Resolve bank/cash account.
	//    If bank_account_id is supplied use it directly; otherwise look up code 1020.
	var bankAccountID string
	if req.BankAccountID != nil && *req.BankAccountID != "" {
		// Verify the provided account ID exists and is an asset account.
		baQ := db.Rebind(`SELECT id FROM accounts WHERE id = ? AND account_type = 'asset' AND is_active = 1`, h.usePostgres)
		if err := h.db.QueryRowContext(ctx, baQ, *req.BankAccountID).Scan(&bankAccountID); err == sql.ErrNoRows {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bank_account_id not found or is not an active asset account"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	} else {
		// Default: Banque compte courant (code 1020).
		defaultBankQ := db.Rebind(`SELECT id FROM accounts WHERE code = '1020' AND is_active = 1 LIMIT 1`, h.usePostgres)
		if err := h.db.QueryRowContext(ctx, defaultBankQ).Scan(&bankAccountID); err == sql.ErrNoRows {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "default bank account (code 1020) not found; supply bank_account_id explicitly"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	}

	// 3. Resolve Accounts Receivable account (code 1100 — Débiteurs suisses).
	var arAccountID string
	arQ := db.Rebind(`SELECT id FROM accounts WHERE code = '1100' AND is_active = 1 LIMIT 1`, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, arQ).Scan(&arAccountID); err == sql.ErrNoRows {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "accounts receivable account (code 1100) not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// 4. Insert payment record (outside transaction so we can use the ID in the JE description).
	paymentID := db.NewID()
	now := time.Now()

	insertPayQ := db.Rebind(`
		INSERT INTO payments (id, invoice_id, amount, payment_date, method, reference, created_by_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
	if _, err := h.db.ExecContext(ctx, insertPayQ,
		paymentID, req.InvoiceID, req.Amount,
		paymentDate.Format("2006-01-02"),
		req.Method, req.Reference,
		userID, now, now,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record payment"})
		return
	}

	payment := models.Payment{
		ID:          paymentID,
		InvoiceID:   req.InvoiceID,
		Amount:      req.Amount,
		PaymentDate: paymentDate,
		Method:      models.PaymentMethod(req.Method),
		Reference:   req.Reference,
		CreatedByID: userID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 5. If payment >= invoice total: create + post journal entry and mark invoice paid.
	if req.Amount >= inv.TotalAmount {
		debit := req.Amount
		credit := req.Amount

		entry, err := h.acctSvc.CreateEntry(ctx, userID, accounting.CreateEntryRequest{
			Date: paymentDate,
			Description: fmt.Sprintf("Payment received — invoice %s (payment %s)",
				inv.InvoiceNumber, paymentID),
			Lines: []accounting.LineInput{
				{
					AccountID:   bankAccountID,
					DebitAmount: &debit,
					Description: fmt.Sprintf("Bank receipt — %s %s", req.Method, safeString(req.Reference)),
					Sequence:    1,
				},
				{
					AccountID:    arAccountID,
					CreditAmount: &credit,
					Description:  fmt.Sprintf("Clear A/R — invoice %s", inv.InvoiceNumber),
					Sequence:     2,
				},
			},
		})
		if err != nil {
			// Payment is already inserted; return a partial success with a warning.
			c.JSON(http.StatusCreated, gin.H{
				"payment": payment,
				"warning": fmt.Sprintf("payment recorded but journal entry creation failed: %v", err),
			})
			return
		}

		// Post the journal entry immediately.
		if err := h.acctSvc.PostEntry(ctx, userID, entry.ID, c.ClientIP()); err != nil {
			c.JSON(http.StatusCreated, gin.H{
				"payment": payment,
				"warning": fmt.Sprintf("payment recorded, journal entry %s created but could not be posted: %v", entry.ID, err),
			})
			return
		}

		// Link journal entry to payment.
		linkPayQ := db.Rebind(`UPDATE payments SET journal_entry_id = ?, updated_at = ? WHERE id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, linkPayQ, entry.ID, time.Now(), paymentID); err != nil {
			// Non-fatal; log and continue.
			_ = err
		}
		payment.JournalEntryID = &entry.ID

		// Transition invoice to paid.
		paidQ := db.Rebind(`UPDATE invoices SET status = 'paid', updated_at = ? WHERE id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, paidQ, time.Now(), req.InvoiceID); err != nil {
			c.JSON(http.StatusCreated, gin.H{
				"payment":         payment,
				"journal_entry_id": entry.ID,
				"warning":         fmt.Sprintf("payment and journal entry recorded but invoice status update failed: %v", err),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"payment":          payment,
			"journal_entry_id": entry.ID,
			"invoice_status":   "paid",
		})
		return
	}

	// Partial payment — payment recorded, no automatic journal entry or status change.
	c.JSON(http.StatusCreated, gin.H{
		"payment": payment,
		"note":    "partial payment recorded; invoice remains in 'sent' status until fully paid",
	})
}

// safeString dereferences a *string or returns "".
func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, p)
}

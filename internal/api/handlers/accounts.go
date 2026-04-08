package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ─── Trial Balance response DTO ───────────────────────────────────────────────

type trialBalanceLine struct {
	ID          string             `json:"id"`
	Code        string             `json:"code"`
	Name        string             `json:"name"`
	AccountType models.AccountType `json:"account_type"`
	TotalDebit  float64            `json:"total_debit"`
	TotalCredit float64            `json:"total_credit"`
	Balance     float64            `json:"balance"`
}

// ─── Account Balance response DTO ────────────────────────────────────────────

type accountBalanceResponse struct {
	AccountID   string             `json:"account_id"`
	Code        string             `json:"code"`
	Name        string             `json:"name"`
	AccountType models.AccountType `json:"account_type"`
	DebitTotal  float64            `json:"debit_total"`
	CreditTotal float64            `json:"credit_total"`
	Balance     float64            `json:"balance"`
	AsOf        string             `json:"as_of"`
}

type AccountsHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewAccountsHandler(database *sql.DB, usePostgres bool) *AccountsHandler {
	return &AccountsHandler{db: database, usePostgres: usePostgres}
}

// ListAccounts GET /api/v1/accounts
func (h *AccountsHandler) ListAccounts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, code, name, account_type, description, is_active, parent_id, created_at, updated_at
		FROM accounts ORDER BY code`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	accounts := []models.Account{}
	for rows.Next() {
		var a models.Account
		var isActive int
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.AccountType, &a.Description, &isActive, &a.ParentID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		a.IsActive = isActive == 1
		accounts = append(accounts, a)
	}
	c.JSON(http.StatusOK, accounts)
}

type createAccountRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	AccountType string `json:"account_type" binding:"required,oneof=asset liability equity revenue expense"`
	Description string `json:"description"`
	ParentID    *string `json:"parent_id"`
}

// CreateAccount POST /api/v1/accounts
func (h *AccountsHandler) CreateAccount(c *gin.Context) {
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	id := db.NewID()
	now := time.Now()
	q := db.Rebind(`
		INSERT INTO accounts (id, code, name, account_type, description, parent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
	if _, err := h.db.ExecContext(ctx, q, id, req.Code, req.Name, req.AccountType, req.Description, req.ParentID, now, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusCreated, models.Account{
		ID:          id,
		Code:        req.Code,
		Name:        req.Name,
		AccountType: models.AccountType(req.AccountType),
		Description: req.Description,
		ParentID:    req.ParentID,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// TrialBalance GET /api/v1/accounts/trial-balance
// Optional query param: as_of=YYYY-MM-DD (filters journal entries with date <= as_of).
func (h *AccountsHandler) TrialBalance(c *gin.Context) {
	asOf := c.Query("as_of")
	if asOf != "" {
		if _, err := time.Parse("2006-01-02", asOf); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "as_of must be YYYY-MM-DD"})
			return
		}
	}

	baseQuery := `
		SELECT
		    a.id, a.code, a.name, a.account_type,
		    COALESCE(SUM(jl.debit_amount), 0)  AS total_debit,
		    COALESCE(SUM(jl.credit_amount), 0) AS total_credit
		FROM accounts a
		LEFT JOIN journal_lines jl ON jl.account_id = a.id
		LEFT JOIN journal_entries je ON je.id = jl.entry_id AND je.status = 'posted'`

	args := []any{}

	if asOf != "" {
		baseQuery += " AND je.date <= ?"
		args = append(args, asOf)
	}

	baseQuery += `
		WHERE a.is_active = 1
		GROUP BY a.id, a.code, a.name, a.account_type
		ORDER BY a.code`

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(baseQuery, h.usePostgres)
	rows, err := h.db.QueryContext(ctx, q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	lines := []trialBalanceLine{}
	for rows.Next() {
		var l trialBalanceLine
		if err := rows.Scan(&l.ID, &l.Code, &l.Name, &l.AccountType, &l.TotalDebit, &l.TotalCredit); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		l.Balance = l.TotalDebit - l.TotalCredit
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, lines)
}

// AccountBalance GET /api/v1/accounts/:code/balance
// Optional query param: as_of=YYYY-MM-DD.
// Returns 404 if the account code does not exist or is inactive.
func (h *AccountsHandler) AccountBalance(c *gin.Context) {
	code := c.Param("code")
	asOf := c.Query("as_of")
	if asOf != "" {
		if _, err := time.Parse("2006-01-02", asOf); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "as_of must be YYYY-MM-DD"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Resolve account by code
	var acct struct {
		ID          string
		Code        string
		Name        string
		AccountType models.AccountType
	}
	accountQ := db.Rebind(
		"SELECT id, code, name, account_type FROM accounts WHERE code = ? AND is_active = 1",
		h.usePostgres,
	)
	err := h.db.QueryRowContext(ctx, accountQ, code).Scan(
		&acct.ID, &acct.Code, &acct.Name, &acct.AccountType,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Aggregate debits and credits from posted entries
	sumQuery := `
		SELECT
		    COALESCE(SUM(jl.debit_amount), 0),
		    COALESCE(SUM(jl.credit_amount), 0)
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.entry_id AND je.status = 'posted'
		WHERE jl.account_id = ?`

	args := []any{acct.ID}

	if asOf != "" {
		sumQuery += " AND je.date <= ?"
		args = append(args, asOf)
	}

	var debitTotal, creditTotal float64
	sumQ := db.Rebind(sumQuery, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, sumQ, args...).Scan(&debitTotal, &creditTotal); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	asOfDisplay := asOf
	if asOfDisplay == "" {
		asOfDisplay = time.Now().Format("2006-01-02")
	}

	c.JSON(http.StatusOK, accountBalanceResponse{
		AccountID:   acct.ID,
		Code:        acct.Code,
		Name:        acct.Name,
		AccountType: acct.AccountType,
		DebitTotal:  debitTotal,
		CreditTotal: creditTotal,
		Balance:     debitTotal - creditTotal,
		AsOf:        asOfDisplay,
	})
}

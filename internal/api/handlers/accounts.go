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

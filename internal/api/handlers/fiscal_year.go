package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/vat"
)

// ─── FiscalYearHandler ────────────────────────────────────────────────────────

// FiscalYearHandler serves fiscal year management and VAT declaration endpoints.
type FiscalYearHandler struct {
	db          *sql.DB
	usePostgres bool
	fySvc       *accounting.FiscalYearService
	vatSvc      *vat.Service
}

// NewFiscalYearHandler creates a FiscalYearHandler wiring up the required services.
func NewFiscalYearHandler(database *sql.DB, usePostgres bool) *FiscalYearHandler {
	return &FiscalYearHandler{
		db:          database,
		usePostgres: usePostgres,
		fySvc:       accounting.NewFiscalYearService(database, usePostgres),
		vatSvc:      vat.New(database, usePostgres),
	}
}

// ─── GET /api/v1/fiscal-years ─────────────────────────────────────────────────

// ListFiscalYears returns all fiscal years ordered by start_date descending.
// Access: any authenticated user.
func (h *FiscalYearHandler) ListFiscalYears(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, name, start_date, end_date, is_closed, created_at, updated_at
		FROM fiscal_years
		ORDER BY start_date DESC
	`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	years := []models.FiscalYear{}
	for rows.Next() {
		var fy models.FiscalYear
		var isClosed int
		if err := rows.Scan(
			&fy.ID, &fy.Name, &fy.StartDate, &fy.EndDate,
			&isClosed, &fy.CreatedAt, &fy.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		fy.IsClosed = isClosed == 1
		years = append(years, fy)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": years, "total": len(years)})
}

// ─── POST /api/v1/fiscal-years/:id/close ─────────────────────────────────────

// CloseFiscalYear triggers the year-end closing procedure (CO art. 958).
// Access: admin only (RequireAdmin middleware must be applied at the router level).
func (h *FiscalYearHandler) CloseFiscalYear(c *gin.Context) {
	fiscalYearID := c.Param("id")
	if fiscalYearID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fiscal year id is required"})
		return
	}

	userID := currentUserID(c)
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin privileges required to close a fiscal year"})
		return
	}

	if err := h.fySvc.CloseYear(c.Request.Context(), fiscalYearID, userID); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "closed", "fiscal_year_id": fiscalYearID})
}

// ─── POST /api/v1/vat/declaration ────────────────────────────────────────────

type vatDeclarationRequest struct {
	PeriodStart string `json:"period_start" binding:"required"` // YYYY-MM-DD
	PeriodEnd   string `json:"period_end" binding:"required"`   // YYYY-MM-DD
	Method      string `json:"method" binding:"required"`       // "effective" or "tdfn"
}

// GenerateVATDeclaration computes the VAT declaration for a given period.
// Access: admin only.
func (h *FiscalYearHandler) GenerateVATDeclaration(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin privileges required to generate VAT declarations"})
		return
	}

	var req vatDeclarationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "period_start must be YYYY-MM-DD"})
		return
	}
	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "period_end must be YYYY-MM-DD"})
		return
	}
	if periodEnd.Before(periodStart) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "period_end must be on or after period_start"})
		return
	}

	decl, err := h.vatSvc.GenerateDeclaration(c.Request.Context(), periodStart, periodEnd, req.Method)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, decl)
}

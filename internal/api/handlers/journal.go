package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

type JournalHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewJournalHandler(database *sql.DB, usePostgres bool) *JournalHandler {
	return &JournalHandler{db: database, usePostgres: usePostgres}
}

// ListJournal godoc
// GET /api/v1/journal?page=1&page_size=20&date_from=&date_to=&status=&reference=
func (h *JournalHandler) ListJournal(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	status := c.Query("status")
	reference := c.Query("reference")

	where := " WHERE 1=1"
	args := []any{}

	// Data isolation: non-admin users only see their own entries (nLPD art. 6)
	if uid := currentUserID(c); uid != "" && !isAdmin(c) {
		where += " AND created_by_id = ?"
		args = append(args, uid)
	}

	if dateFrom != "" {
		if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "date_from must be YYYY-MM-DD"})
			return
		}
		where += " AND date >= ?"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		if _, err := time.Parse("2006-01-02", dateTo); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "date_to must be YYYY-MM-DD"})
			return
		}
		where += " AND date <= ?"
		args = append(args, dateTo)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if reference != "" {
		where += " AND reference LIKE ?"
		args = append(args, "%"+reference+"%")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Count total
	countQuery := db.Rebind("SELECT COUNT(*) FROM journal_entries"+where, h.usePostgres)
	var total int
	if err := h.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Fetch page
	listQuery := db.Rebind(
		"SELECT id, reference, date, description, status, is_reversal, created_at, updated_at"+
			" FROM journal_entries"+where+
			" ORDER BY date DESC, created_at DESC LIMIT ? OFFSET ?",
		h.usePostgres,
	)
	offset := (page - 1) * pageSize
	rows, err := h.db.QueryContext(ctx, listQuery, append(args, pageSize, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	entries := []models.JournalEntry{}
	for rows.Next() {
		var e models.JournalEntry
		var isReversal int
		if err := rows.Scan(&e.ID, &e.Reference, &e.Date, &e.Description, &e.Status, &isReversal, &e.CreatedAt, &e.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		e.IsReversal = isReversal == 1
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	pages := (total + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     entries,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     pages,
	})
}

func queryInt(c *gin.Context, key string, fallback int) int {
	v := c.Query(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

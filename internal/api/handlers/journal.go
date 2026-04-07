package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

type JournalHandler struct {
	db *sql.DB
}

func NewJournalHandler(db *sql.DB) *JournalHandler {
	return &JournalHandler{db: db}
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

	// Build dynamic WHERE clause
	where := " WHERE 1=1"
	args := []any{}

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

	// Count total
	var total int
	if err := h.db.QueryRowContext(c, "SELECT COUNT(*) FROM journal_entries"+where, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	offset := (page - 1) * pageSize
	query := "SELECT id, reference, date, description, status, is_reversal, created_at, updated_at" +
		" FROM journal_entries" + where +
		" ORDER BY date DESC, created_at DESC LIMIT ? OFFSET ?"
	rows, err := h.db.QueryContext(c, query, append(args, pageSize, offset)...)
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

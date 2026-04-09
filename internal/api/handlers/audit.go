package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ─── AuditHandler ────────────────────────────────────────────────────────────

type AuditHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewAuditHandler(database *sql.DB, usePostgres bool) *AuditHandler {
	return &AuditHandler{db: database, usePostgres: usePostgres}
}

// ─── GET /api/v1/audit-logs ──────────────────────────────────────────────────

// ListAuditLogs returns audit log entries matching optional filter parameters.
// Query params: table_name, record_id, from (YYYY-MM-DD), to (YYYY-MM-DD),
//              limit (default 50, max 200), offset (default 0).
// Access: admin only.
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin privileges required to access audit logs"})
		return
	}

	tableName := c.Query("table_name")
	recordID := c.Query("record_id")
	fromStr := c.Query("from")
	toStr := c.Query("to")
	limit := queryInt(c, "limit", 50)
	offset := queryInt(c, "offset", 0)

	if limit < 1 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	if fromStr != "" {
		if _, err := time.Parse("2006-01-02", fromStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
			return
		}
	}
	if toStr != "" {
		if _, err := time.Parse("2006-01-02", toStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Build dynamic WHERE clause.
	where := " WHERE 1=1"
	args := []any{}

	if tableName != "" {
		where += " AND table_name = ?"
		args = append(args, tableName)
	}
	if recordID != "" {
		where += " AND record_id = ?"
		args = append(args, recordID)
	}
	if fromStr != "" {
		where += " AND DATE(created_at) >= ?"
		args = append(args, fromStr)
	}
	if toStr != "" {
		where += " AND DATE(created_at) <= ?"
		args = append(args, toStr)
	}

	// Total count.
	countQ := db.Rebind("SELECT COUNT(*) FROM audit_logs"+where, h.usePostgres)
	var total int
	if err := h.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Paginated results ordered by sequence_number ascending (oldest first).
	listQ := db.Rebind(`
		SELECT id, user_id, action, table_name, record_id,
		       before_state, after_state, ip_address,
		       entry_hash, prev_hash, sequence_number, created_at
		FROM audit_logs`+where+`
		ORDER BY sequence_number ASC
		LIMIT ? OFFSET ?`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, listQ, append(args, limit, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	items := []models.AuditLog{}
	for rows.Next() {
		var al models.AuditLog
		if err := rows.Scan(
			&al.ID, &al.UserID, &al.Action, &al.TableName, &al.RecordID,
			&al.BeforeState, &al.AfterState, &al.IPAddress,
			&al.EntryHash, &al.PrevHash, &al.SequenceNumber, &al.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		items = append(items, al)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"pages":  pages,
	})
}

// ─── GET /api/v1/audit-logs/:id/verify ───────────────────────────────────────

// VerifyAuditLog recomputes the entry_hash for a single audit log record and
// compares it to the stored value, confirming data integrity (CO art. 957a).
// Returns 200 with verified=true if the hash matches, 409 with verified=false
// if it does not (indicating possible tampering or corruption).
// Access: admin only.
func (h *AuditHandler) VerifyAuditLog(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin privileges required to verify audit logs"})
		return
	}

	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT
		    user_id,
		    action,
		    table_name,
		    record_id,
		    COALESCE(before_state, '') AS before_state,
		    COALESCE(after_state,  '') AS after_state,
		    COALESCE(ip_address,   '') AS ip_address,
		    entry_hash,
		    created_at
		FROM audit_logs WHERE id = ?`, h.usePostgres)

	var (
		userID, action, tableName, recordID string
		beforeState, afterState, ipAddress  string
		storedHash                          string
		createdAt                           time.Time
	)
	err := h.db.QueryRowContext(ctx, q, id).Scan(
		&userID, &action, &tableName, &recordID,
		&beforeState, &afterState, &ipAddress,
		&storedHash, &createdAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit log entry not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Recompute the hash using the same algorithm as the accounting service.
	recomputed := security.ComputeEntryHash(
		userID, action, tableName, recordID,
		beforeState, afterState, ipAddress,
		createdAt,
	)

	if recomputed == storedHash {
		c.JSON(http.StatusOK, gin.H{
			"id":       id,
			"verified": true,
			"hash":     storedHash,
		})
		return
	}

	// Hash mismatch — potential tampering or data corruption.
	c.JSON(http.StatusConflict, gin.H{
		"id":              id,
		"verified":        false,
		"stored_hash":     storedHash,
		"recomputed_hash": recomputed,
		"error":           "integrity check failed: stored hash does not match recomputed hash (CO art. 957a)",
	})
}

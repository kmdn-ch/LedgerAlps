package handlers

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// SecurityEventHandler exposes authentication security telemetry (currently
// login lockouts) so an administrator can see brute-force attempts.
type SecurityEventHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewSecurityEventHandler(database *sql.DB, usePostgres bool) *SecurityEventHandler {
	return &SecurityEventHandler{db: database, usePostgres: usePostgres}
}

type securityEvent struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	IPAddress string    `json:"ip_address"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// RecordLoginLockout persists a lockout. It is called from the rate limiter on
// the request path, so it takes its own short timeout and never surfaces an
// error to the client: failing to write telemetry must not affect the response.
func RecordLoginLockout(database *sql.DB, usePostgres bool, ip string, until time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	q := db.Rebind(`
		INSERT INTO security_events (id, event_type, ip_address, detail, created_at)
		VALUES (?, 'login_lockout', ?, ?, ?)`, usePostgres)

	detail := "locked until " + until.UTC().Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, q, db.NewID(), ip, detail, time.Now().UTC()); err != nil {
		log.Printf("WARNING: could not record login lockout for %s: %v", ip, err)
	}
}

// ListSecurityEvents returns security events, newest first.
// GET /api/v1/security-events?limit=100&type=login_lockout
func (h *SecurityEventHandler) ListSecurityEvents(c *gin.Context) {
	limit := 100
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "limit must be between 1 and 1000"})
			return
		}
		limit = n
	}

	query := `SELECT id, event_type, COALESCE(ip_address, ''), COALESCE(detail, ''), created_at
	          FROM security_events`
	args := []any{}
	if t := c.Query("type"); t != "" {
		query += " WHERE event_type = ?"
		args = append(args, t)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.db.QueryContext(ctx, db.Rebind(query, h.usePostgres), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	items := []securityEvent{}
	for rows.Next() {
		var e securityEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.IPAddress, &e.Detail, &e.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

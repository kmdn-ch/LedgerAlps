package handlers

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
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

// RecordAuthLockout trace un verrouillage d'une des routes d'authentification.
//
// Appelée depuis le limiteur, sur le chemin de la requête : elle prend son propre
// délai court et ne remonte jamais d'erreur au client — échouer à écrire la
// télémétrie ne doit pas changer la réponse.
//
// `kind` et `key` viennent de middleware.AuthLimiters : l'adresse seule (mot de
// passe, rafraîchissement), « adresse|compte » (mot de passe sur un compte précis)
// ou un identifiant de compte (second facteur, qui n'a pas d'adresse à noter).
func RecordAuthLockout(database *sql.DB, usePostgres bool, kind, key string, until time.Time) {
	fin := "locked until " + until.UTC().Format(time.RFC3339)
	switch kind {
	case mw.LockoutLoginAccount:
		ip, compte, _ := strings.Cut(key, "|")
		recordLockout(database, usePostgres, "login_lockout", ip, "compte="+compte+" ; "+fin)
	case mw.LockoutMFA:
		recordLockout(database, usePostgres, "mfa_lockout", "", "compte="+key+" ; "+fin)
	case mw.LockoutPassword:
		recordLockout(database, usePostgres, "password_lockout", "", "compte="+key+" ; "+fin)
	default:
		recordLockout(database, usePostgres, "login_lockout", key, fin)
	}
}

func recordLockout(database *sql.DB, usePostgres bool, eventType, ip, detail string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	q := db.Rebind(`
		INSERT INTO security_events (id, event_type, ip_address, detail, created_at)
		VALUES (?, ?, ?, ?, ?)`, usePostgres)

	// Pas d'adresse à noter (second facteur, compté par compte) : NULL, pas une chaîne
	// vide que la purge de rétention prendrait pour une adresse à anonymiser.
	var adresse any
	if ip != "" {
		adresse = ip
	}
	if _, err := database.ExecContext(ctx, q, db.NewID(), eventType, adresse, detail, time.Now().UTC()); err != nil {
		log.Printf("WARNING: could not record %s for %s: %v", eventType, ip, err)
	}
}

// ListSecurityEvents returns security events, newest first.
// GET /api/v1/security-events?limit=100&type=login_lockout
func (h *SecurityEventHandler) ListSecurityEvents(c *gin.Context) {
	limit := 100
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "limit doit être compris entre 1 et 1000"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	items := []securityEvent{}
	for rows.Next() {
		var e securityEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.IPAddress, &e.Detail, &e.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

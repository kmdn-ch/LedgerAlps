package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// dummyHash is a pre-computed bcrypt hash used to equalise timing when a user
// is not found — prevents email enumeration via response-time analysis.
// Cost 12 matches production cost so the dummy comparison burns the same ~100ms.
var dummyHash, _ = security.HashPassword("ledgeralps-dummy-password-for-timing-attack-prevention-do-not-use")

type AuthHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewAuthHandler(db *sql.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login godoc
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var (
		userID       string
		passwordHash string
		isAdmin      bool
		isActive     bool
	)
	err := h.db.QueryRowContext(ctx, `
		SELECT id, password_hash, is_admin, is_active
		FROM users WHERE email = ?`, req.Email).
		Scan(&userID, &passwordHash, &isAdmin, &isActive)

	if err == sql.ErrNoRows {
		// User not found: run bcrypt on dummy hash to equalise timing with the
		// "wrong password" branch (~100ms), preventing email enumeration attacks.
		security.CheckPassword(dummyHash, req.Password)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if !security.CheckPassword(passwordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !isActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
		return
	}

	ttl := time.Duration(h.cfg.JWTAccessMinutes) * time.Minute
	token, err := security.GenerateAccessToken(h.cfg.JWTSecret, userID, isAdmin, ttl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "bearer",
		"expires_in":   int(ttl.Seconds()),
	})
}

// Refresh godoc
// POST /api/v1/auth/refresh
// Validates a refresh token, verifies it is active in DB, and returns a new access token.
func (h *AuthHandler) Refresh(c *gin.Context) {
	rawToken, ok := bearerToken(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
		return
	}

	claims, err := security.ParseToken(h.cfg.JWTSecret, rawToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Verify the token exists in DB, is not revoked, and is not expired.
	q := db.Rebind(`
		SELECT expires_at, revoked_at
		FROM refresh_tokens
		WHERE jti = ?`, h.cfg.UsePostgres())
	var expiresAt time.Time
	var revokedAt sql.NullTime
	if err := h.db.QueryRowContext(ctx, q, claims.JTI).Scan(&expiresAt, &revokedAt); err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if revokedAt.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token has been revoked"})
		return
	}
	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token has expired"})
		return
	}

	ttl := time.Duration(h.cfg.JWTAccessMinutes) * time.Minute
	accessToken, err := security.GenerateAccessToken(h.cfg.JWTSecret, claims.UserID, claims.IsAdmin, ttl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "bearer",
		"expires_in":   int(ttl.Seconds()),
	})
}

// Logout godoc
// POST /api/v1/auth/logout
// Revokes a refresh token by setting revoked_at to the current timestamp.
func (h *AuthHandler) Logout(c *gin.Context) {
	rawToken, ok := bearerToken(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
		return
	}

	claims, err := security.ParseToken(h.cfg.JWTSecret, rawToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	upd := db.Rebind(`
		UPDATE refresh_tokens
		SET revoked_at = ?
		WHERE jti = ? AND revoked_at IS NULL`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(ctx, upd, time.Now().UTC(), claims.JTI); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.Status(http.StatusNoContent)
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
// Returns the token string and true on success, or ("", false) if the header is absent/malformed.
func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	return strings.TrimPrefix(header, "Bearer "), true
}

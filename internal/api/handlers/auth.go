package handlers

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// registerRequest is used for POST /auth/register.
type registerRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Name     string `json:"name"     binding:"required,min=1,max=255"`
	Password string `json:"password" binding:"required,min=8"`
}

// bootstrapRequest is used for POST /auth/bootstrap.
// It extends registerRequest with optional company fields so the first admin
// can seed the company profile in a single request.
type bootstrapRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Name     string `json:"name"     binding:"required,min=1,max=255"`
	Password string `json:"password" binding:"required,min=8"`
	// Company fields (optional at bootstrap)
	CompanyName          string `json:"company_name"`
	LegalForm            string `json:"legal_form"`
	AddressStreet        string `json:"address_street"`
	AddressPostalCode    string `json:"address_postal_code"`
	AddressCity          string `json:"address_city"`
	AddressCountry       string `json:"address_country"`
	CheNumber            string `json:"che_number"`
	VatNumber            string `json:"vat_number"`
	IBAN                 string `json:"iban"`
	FiscalYearStartMonth int    `json:"fiscal_year_start_month"`
	Currency             string `json:"currency"`
}

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
		role         string
		mustChange   int
	)
	err := h.db.QueryRowContext(ctx, `
		SELECT id, password_hash, is_admin, is_active, COALESCE(role,'accountant'),
		       COALESCE(must_change_password,0)
		FROM users WHERE email = ?`, req.Email).
		Scan(&userID, &passwordHash, &isAdmin, &isActive, &role, &mustChange)

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

	accessTTL := time.Duration(h.cfg.JWTAccessMinutes) * time.Minute
	accessToken, err := security.GenerateAccessToken(h.cfg.JWTSecret, userID, isAdmin, accessTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate access token"})
		return
	}

	refreshTTL := time.Duration(h.cfg.JWTRefreshDays) * 24 * time.Hour
	refreshToken, jti, err := security.GenerateRefreshToken(h.cfg.JWTSecret, userID, isAdmin, refreshTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate refresh token"})
		return
	}

	// Persist refresh token so Refresh/Logout endpoints can validate/revoke it.
	//
	// A fresh context: the one above was opened before CheckPassword, and bcrypt
	// is deliberately slow. Reusing it meant a correct password on a slow machine
	// could still end in "database error" — the login had already spent most of
	// its five seconds hashing before reaching this insert.
	insCtx, insCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer insCancel()

	insQ := db.Rebind(`
		INSERT INTO refresh_tokens (id, user_id, jti, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(insCtx, insQ,
		db.NewID(), userID, jti,
		time.Now().UTC().Add(refreshTTL), time.Now().UTC()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// The refresh token goes out as an HttpOnly cookie and is deliberately
	// absent from the body: anything returned here would be readable by script.
	setRefreshCookie(c, refreshToken, refreshTTL)

	// Le rôle part avec la réponse pour que l'interface sache quoi masquer.
	// Ce n'est PAS une autorisation : le serveur ne fait aucune confiance à ce
	// que le navigateur croit, et chaque requête revérifie dans la base. Mais
	// afficher un bouton qui répondra 403 use la confiance dans l'interface
	// aussi sûrement qu'un avertissement périmé.
	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "bearer",
		"expires_in":   int(accessTTL.Seconds()),
		"role":         role,
		// L'interface envoie directement à l'écran de changement. Le serveur
		// n'en dépend pas : chaque route refuse le compte tant que le mot de
		// passe temporaire vaut encore.
		"must_change_password": mustChange == 1,
	})
}

// Refresh godoc
// POST /api/v1/auth/refresh
// Validates a refresh token, verifies it is active in DB, and returns a new access token.
func (h *AuthHandler) Refresh(c *gin.Context) {
	rawToken, ok := refreshTokenFromRequest(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no refresh token supplied"})
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
	// The cookie is cleared whatever happens next: a caller asking to log out
	// must never be left holding a usable credential because the token was
	// already malformed or expired.
	clearRefreshCookie(c)

	rawToken, ok := refreshTokenFromRequest(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no refresh token supplied"})
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

// Register godoc
// POST /api/v1/auth/register
// Creates a new non-admin user. Open endpoint — no auth required.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	id := db.NewID()
	now := time.Now().UTC()
	q := db.Rebind(`
		INSERT INTO users (id, email, name, password_hash, role, is_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'accountant', 0, 1, ?, ?)`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(ctx, q, id, req.Email, req.Name, hash, now, now); err != nil {
		// UNIQUE constraint on email
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"email":      req.Email,
		"name":       req.Name,
		"is_admin":   false,
		"created_at": now,
	})
}

// Bootstrap godoc
// POST /api/v1/auth/bootstrap
// Creates the first admin user. Returns 409 if any user already exists.
// This endpoint is intentionally open (no auth) but only works once.
// Optional company fields may be supplied to seed the company_settings row.
func (h *AuthHandler) Bootstrap(c *gin.Context) {
	var req bootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// This timeout bounds database work only. bcrypt is deliberately slow —
	// around 100 ms here, several times that on an old laptop or under a race
	// detector — and letting it share the budget meant the INSERT that follows
	// could time out and surface as "database error" during first-time setup,
	// on exactly the modest hardware this product targets. Register already
	// hashes outside the context; Bootstrap and Login now match it.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Refuse if any user already exists — bootstrap is one-shot.
	var count int
	if err := h.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "system already bootstrapped — use /auth/register or the admin panel"})
		return
	}

	// Hors du contexte ci-dessus : le hachage n'est pas un travail de base
	// de données et ne doit pas en consommer le budget.
	cancel()

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	id := db.NewID()
	now := time.Now().UTC()
	q := db.Rebind(`
		INSERT INTO users (id, email, name, password_hash, role, is_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'admin', 1, 1, ?, ?)`, h.cfg.UsePostgres())
	insCtx, insCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer insCancel()

	if _, err := h.db.ExecContext(insCtx, q, id, req.Email, req.Name, hash, now, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Reported back so the wizard can tell the user to re-enter the details in
	// Settings rather than let them discover an empty form later.
	companySaved := req.CompanyName != ""

	// If a company name was provided, seed the company_settings singleton.
	if req.CompanyName != "" {
		country := req.AddressCountry
		if country == "" {
			country = "CH"
		}
		currency := req.Currency
		if currency == "" {
			currency = "CHF"
		}
		fyMonth := req.FiscalYearStartMonth
		if fyMonth == 0 {
			fyMonth = 1
		}
		csID := db.NewID()
		csQ := db.Rebind(`
			INSERT INTO company_settings
			    (id, company_name, legal_form,
			     address_street, address_postal_code, address_city, address_country,
			     che_number, vat_number, iban,
			     fiscal_year_start_month, currency,
			     created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.cfg.UsePostgres())
		// A fresh context. This insert used to run on the one opened at the top
		// of the handler, which is now cancelled before hashing — the company
		// details were written with a dead context and silently lost while the
		// wizard reported success.
		csCtx, csCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer csCancel()

		// Still non-fatal — losing the admin account over company details would
		// be worse — but no longer silent. Discarding this error is what let the
		// cancelled-context bug ship: nothing anywhere said the write had failed.
		if _, err := h.db.ExecContext(csCtx, csQ,
			csID, req.CompanyName, req.LegalForm,
			req.AddressStreet, req.AddressPostalCode, req.AddressCity, country,
			req.CheNumber, req.VatNumber, req.IBAN,
			fyMonth, currency,
			now, now,
		); err != nil {
			log.Printf("WARNING: bootstrap created the admin account but could not save company settings: %v", err)
			companySaved = false
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":            id,
		"email":         req.Email,
		"name":          req.Name,
		"is_admin":      true,
		"created_at":    now,
		"company_saved": companySaved,
		"message":       "Admin user created. This endpoint is now disabled.",
	})
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

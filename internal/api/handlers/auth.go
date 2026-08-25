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
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// bootstrapRequest is used for POST /auth/bootstrap.
// Il porte les mêmes champs d'identité qu'une création de compte, plus les
// champs d'entreprise facultatifs, pour que le premier administrateur amorce sa
// fiche en une seule requête.
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "identifiants incorrects"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	if !security.CheckPassword(passwordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "identifiants incorrects"})
		return
	}
	if !isActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "ce compte est désactivé"})
		return
	}

	// Second facteur : le mot de passe ne suffit plus.
	//
	// Rien n'est délivré ici qu'un jeton d'attente de cinq minutes, qui ne vaut
	// que pour /auth/mfa/verify. Ni jeton d'accès, ni cookie de
	// rafraîchissement : une session ne naît qu'après le code.
	if h.mfaEnabled(c.Request.Context(), userID) &&
		!h.deviceIsTrusted(c.Request.Context(), c, userID) {
		challenge, err := security.GenerateMFAChallengeToken(h.cfg.JWTSecret, userID, isAdmin)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "le jeton n'a pas pu être produit"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"mfa_required": true,
			"mfa_token":    challenge,
			"expires_in":   300,
		})
		return
	}

	h.issueSession(c, userID, isAdmin, role, mustChange == 1)
}

// mfaEnabled dit si le compte a un second facteur CONFIRMÉ.
//
// Un secret créé mais jamais confirmé ne compte pas : quelqu'un qui a fermé
// l'assistant en cours de route serait sinon réclamé un code qu'aucun téléphone
// ne calcule, et il ne pourrait plus entrer.
func (h *AuthHandler) mfaEnabled(ctx context.Context, userID string) bool {
	q := db.Rebind(
		`SELECT COUNT(*) FROM user_mfa WHERE user_id = ? AND confirmed_at IS NOT NULL`,
		h.cfg.UsePostgres())
	var n int
	if err := h.db.QueryRowContext(ctx, q, userID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// issueSession délivre la vraie session : jeton d'accès, cookie de
// rafraîchissement, et ce que l'interface doit savoir.
//
// Extraite de Login parce que la vérification du second facteur mène exactement
// au même endroit. Deux copies auraient divergé — et la copie qui aurait oublié
// une règle serait justement celle qu'on emprunte après avoir prouvé son
// identité deux fois.
func (h *AuthHandler) issueSession(c *gin.Context, userID string, isAdmin bool, role string, mustChange bool) {
	accessTTL := time.Duration(h.cfg.JWTAccessMinutes) * time.Minute
	accessToken, err := security.GenerateAccessToken(h.cfg.JWTSecret, userID, isAdmin, accessTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "le jeton d'accès n'a pas pu être produit"})
		return
	}

	refreshTTL := time.Duration(h.cfg.JWTRefreshDays) * 24 * time.Hour
	refreshToken, jti, err := security.GenerateRefreshToken(h.cfg.JWTSecret, userID, isAdmin, refreshTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "le jeton de rafraîchissement n'a pas pu être produit"})
		return
	}

	// Persist refresh token so Refresh/Logout endpoints can validate/revoke it.
	//
	// A fresh context: the one used for the password check was opened before
	// CheckPassword, and bcrypt is deliberately slow. Reusing it meant a correct
	// password on a slow machine could still end in "erreur de base de données" — the login
	// had already spent most of its five seconds hashing before this insert.
	insCtx, insCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer insCancel()

	insQ := db.Rebind(`
		INSERT INTO refresh_tokens (id, user_id, jti, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(insCtx, insQ,
		db.NewID(), userID, jti,
		time.Now().UTC().Add(refreshTTL), time.Now().UTC()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
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
		"must_change_password": mustChange,
		// Même logique pour l'inscription du second facteur : l'interface
		// conduit à l'écran, le serveur refuse de toute façon tant que c'est
		// dû.
		// Administrateur ET comptable : les deux peuvent modifier quelque chose.
		// Le drapeau suit exactement la règle du filtre serveur — s'il en
		// divergeait, l'interface enverrait au tableau de bord un compte que
		// chaque requête refuse ensuite.
		"mfa_enrolment_required": authz.RequiresSecondFactor(authz.Role(role)) &&
			!h.mfaEnabled(c.Request.Context(), userID),
	})
}

// Refresh godoc
// POST /api/v1/auth/refresh
// Validates a refresh token, verifies it is active in DB, and returns a new access token.
func (h *AuthHandler) Refresh(c *gin.Context) {
	rawToken, ok := refreshTokenFromRequest(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "aucun jeton de rafraîchissement fourni"})
		return
	}

	claims, err := security.ParseToken(h.cfg.JWTSecret, rawToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "jeton de rafraîchissement invalide ou expiré"})
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "jeton de rafraîchissement introuvable"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	if revokedAt.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ce jeton de rafraîchissement a été révoqué"})
		return
	}
	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ce jeton de rafraîchissement a expiré"})
		return
	}

	ttl := time.Duration(h.cfg.JWTAccessMinutes) * time.Minute
	accessToken, err := security.GenerateAccessToken(h.cfg.JWTSecret, claims.UserID, claims.IsAdmin, ttl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "le jeton d'accès n'a pas pu être produit"})
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "aucun jeton de rafraîchissement fourni"})
		return
	}

	claims, err := security.ParseToken(h.cfg.JWTSecret, rawToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "jeton de rafraîchissement invalide ou expiré"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	upd := db.Rebind(`
		UPDATE refresh_tokens
		SET revoked_at = ?
		WHERE jti = ? AND revoked_at IS NULL`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(ctx, upd, time.Now().UTC(), claims.JTI); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	c.Status(http.StatusNoContent)
}

// L'INSCRIPTION PUBLIQUE A ÉTÉ RETIRÉE — et ce n'est pas un nettoyage.
//
// `POST /auth/register` créait un compte « comptable » ACTIF, sans mot de passe
// temporaire, sans qu'aucun administrateur l'ait voulu. Or le comptable détient
// PermManage : il règle la fiche entreprise — donc l'IBAN qui s'imprime sur
// toutes les factures émises ensuite —, écrit au journal, clôture un exercice.
// N'importe qui atteignant le port obtenait cela en cinq requêtes, en
// s'inscrivant lui-même son propre second facteur au passage.
//
// La route était par ailleurs MORTE côté interface : aucune page ne l'appelait.
// C'était un vestige d'avant les rôles, que `POST /users` (PermAdmin, mot de
// passe temporaire obligatoire) remplace depuis. Un vestige que personne ne voit
// dans l'écran est justement celui auquel personne ne pense.
//
// Un compte se crée maintenant par un administrateur, et seulement ainsi. Le
// tout premier passe par `POST /auth/bootstrap`, qui ne fonctionne qu'une fois.

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
	// could time out and surface as "erreur de base de données" during first-time setup,
	// on exactly the modest hardware this product targets. Register already
	// hashes outside the context; Bootstrap and Login now match it.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Refuse if any user already exists — bootstrap is one-shot.
	var count int
	if err := h.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "l'installation est déjà initialisée — passez par la création de compte ou le panneau d'administration"})
		return
	}

	// Hors du contexte ci-dessus : le hachage n'est pas un travail de base
	// de données et ne doit pas en consommer le budget.
	cancel()

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "le mot de passe n'a pas pu être haché"})
		return
	}

	id := db.NewID()
	now := time.Now().UTC()
	q := db.Rebind(`
		INSERT INTO users (id, email, name, password_hash, role, is_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'admin', 1, 1, ?, ?)`, h.cfg.UsePostgres())
	insCtx, insCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer insCancel()

	// Le contrôle et l'insertion dans la MÊME transaction.
	//
	// Séparés, ils laissaient passer deux requêtes simultanées : le COUNT(*)
	// plus haut et cet INSERT sont séparés par le hachage bcrypt, soit une
	// centaine de millisecondes — le `cancel()` ci-dessus l'a même placé hors
	// budget à dessein. Deux appels concurrents lisaient donc tous deux
	// count == 0, et comme `users.email` est UNIQUE, deux adresses
	// différentes produisaient DEUX administrateurs.
	//
	// Le second contrôle est fait DANS la transaction, qui prend le verrou
	// d'écriture dès son ouverture (_txlock=immediate) : « ne fonctionne
	// qu'une fois » cesse de vouloir dire « qu'une fois si personne
	// n'appelle en même temps ».
	tx, err := h.db.BeginTx(insCtx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	var encore int
	if err := tx.QueryRowContext(insCtx, "SELECT COUNT(*) FROM users").Scan(&encore); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if encore > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "l'installation est déjà initialisée — passez par la création de compte ou le panneau d'administration"})
		return
	}

	if _, err := tx.ExecContext(insCtx, q, id, req.Email, req.Name, hash, now, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
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

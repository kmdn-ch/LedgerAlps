package handlers

// Second facteur : inscription, vérification, retrait.
//
// # Ce que le second facteur protège, et ce qu'il ne protège pas
//
// Il protège le cas où le MOT DE PASSE fuit : réutilisé sur un autre site,
// deviné, lu par-dessus l'épaule, retrouvé dans un carnet. Sans le téléphone,
// il ne sert plus à entrer.
//
// Il ne protège pas contre quelqu'un qui lit déjà le fichier de base : le
// secret y est, et celui-là n'a besoin d'aucun code. Ce qui répond à cette
// menace est le chiffrement du disque et celui de la base, pas le second
// facteur. Le dire évite de croire couvert un risque qui ne l'est pas.
//
// # Les codes de secours ne sont pas une commodité
//
// Sans eux, un téléphone perdu enferme définitivement le dernier
// administrateur : plus personne ne peut créer de compte, restaurer une
// sauvegarde ni rendre le droit de le faire. Le second facteur créerait la
// panne qu'il est censé prévenir. Ils sont donc obligatoires, montrés une seule
// fois, et hachés comme des mots de passe.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/mfa"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	qrcode "github.com/skip2/go-qrcode"
)

// RecoveryCodeCount : dix codes. Assez pour survivre à plusieurs incidents,
// assez peu pour tenir sur un papier qu'on range vraiment.
const RecoveryCodeCount = 10

// MFAStatus GET /api/v1/auth/mfa
func (h *AuthHandler) MFAStatus(c *gin.Context) {
	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}
	enabled, _, _, err := h.mfaState(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	var remaining int
	q := db.Rebind(
		`SELECT COUNT(*) FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL`,
		h.cfg.UsePostgres())
	_ = h.db.QueryRowContext(c.Request.Context(), q, claims.UserID).Scan(&remaining)

	// L'obligation se lit sur le rôle en base, pas sur le drapeau du jeton :
	// celui-ci a été figé à la connexion et peut avoir une heure de retard.
	var role string
	roleQ := db.Rebind(`SELECT COALESCE(role,'accountant') FROM users WHERE id = ?`,
		h.cfg.UsePostgres())
	_ = h.db.QueryRowContext(c.Request.Context(), roleQ, claims.UserID).Scan(&role)

	c.JSON(http.StatusOK, gin.H{
		"enabled":                enabled,
		"recovery_codes_left":    remaining,
		"required_for_this_role": authz.RequiresSecondFactor(authz.Role(role)),
		"trusted_device_days":    TrustedDeviceDays,
	})
}

// mfaState rend l'état du second facteur pour un compte.
func (h *AuthHandler) mfaState(ctx context.Context, userID string) (enabled bool, secret string, lastWindow int64, err error) {
	q := db.Rebind(
		`SELECT secret, COALESCE(last_window,0), confirmed_at IS NOT NULL
		 FROM user_mfa WHERE user_id = ?`, h.cfg.UsePostgres())
	var confirmed bool
	err = h.db.QueryRowContext(ctx, q, userID).Scan(&secret, &lastWindow, &confirmed)
	if err == sql.ErrNoRows {
		return false, "", 0, nil
	}
	if err != nil {
		return false, "", 0, err
	}
	return confirmed, secret, lastWindow, nil
}

// MFASetup POST /api/v1/auth/mfa/setup
//
// Tire un secret et rend le QR à scanner. Rien n'est activé à ce stade : une
// inscription abandonnée en cours de route ne doit pas enfermer le compte.
func (h *AuthHandler) MFASetup(c *gin.Context) {
	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}
	enabled, _, _, err := h.mfaState(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if enabled {
		c.JSON(http.StatusConflict, gin.H{
			"error": "le second facteur est déjà actif sur ce compte"})
		return
	}

	secret, err := mfa.NewSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var email string
	emailQ := db.Rebind(`SELECT email FROM users WHERE id = ?`, h.cfg.UsePostgres())
	_ = h.db.QueryRowContext(c.Request.Context(), emailQ, claims.UserID).Scan(&email)

	// Remplacer un secret non confirmé plutôt qu'en accumuler : relancer
	// l'assistant après avoir fermé l'onglet est le cas normal.
	upsert := db.Rebind(`
		INSERT INTO user_mfa (user_id, secret, confirmed_at, last_window, created_at, updated_at)
		VALUES (?, ?, NULL, 0, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret,
		                                   confirmed_at = NULL,
		                                   last_window = 0,
		                                   updated_at = excluded.updated_at`, h.cfg.UsePostgres())
	now := time.Now().UTC()
	if _, err := h.db.ExecContext(c.Request.Context(), upsert, claims.UserID, secret, now, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	uri := mfa.ProvisioningURI("LedgerAlps", email, secret)
	png, err := qrcode.Encode(uri, qrcode.Medium, 240)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "génération du QR: " + err.Error()})
		return
	}

	// Le QR part en data: URI dans le corps JSON, et non depuis une adresse
	// séparée : une image d'inscription servie par une route serait rejouable
	// tant que le secret n'est pas confirmé.
	c.JSON(http.StatusOK, gin.H{
		"secret":  secret, // pour la saisie manuelle : tous les téléphones ne scannent pas
		"uri":     uri,
		"qr_png":  "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"issuer":  "LedgerAlps",
		"account": email,
		"next":    "Scannez ce code, puis saisissez celui que l'application affiche pour confirmer.",
	})
}

// MFAConfirm POST /api/v1/auth/mfa/confirm
//
// Active le second facteur après vérification d'un premier code, et rend les
// codes de secours — une seule fois.
func (h *AuthHandler) MFAConfirm(c *gin.Context) {
	var body struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}

	ctx := c.Request.Context()
	var secret string
	q := db.Rebind(`SELECT secret FROM user_mfa WHERE user_id = ?`, h.cfg.UsePostgres())
	if err := h.db.QueryRowContext(ctx, q, claims.UserID).Scan(&secret); err == sql.ErrNoRows {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "commencez par l'étape précédente : aucun secret n'a été préparé"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	window, ok := mfa.Verify(secret, body.Code, time.Now(), 0)
	if !ok {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "ce code ne correspond pas. Vérifiez l'heure de l'appareil qui porte votre application : " +
				"un décalage de plus d'une minute suffit à décaler tous les codes."})
		return
	}

	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	upd := db.Rebind(
		`UPDATE user_mfa SET confirmed_at = ?, last_window = ?, updated_at = ? WHERE user_id = ?`,
		h.cfg.UsePostgres())
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, upd, now, window, now, claims.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	// Les anciens codes de secours partent avec l'ancienne inscription.
	if _, err := tx.ExecContext(ctx,
		db.Rebind(`DELETE FROM mfa_recovery_codes WHERE user_id = ?`, h.cfg.UsePostgres()),
		claims.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	ins := db.Rebind(
		`INSERT INTO mfa_recovery_codes (id, user_id, code_hash, created_at) VALUES (?,?,?,?)`,
		h.cfg.UsePostgres())
	for _, hsh := range hashes {
		if _, err := tx.ExecContext(ctx, ins, db.NewID(), claims.UserID, hsh, now); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	forgetDevices(ctx, h.db, h.cfg.UsePostgres(), claims.UserID)

	recordSecurityEvent(ctx, h.db, h.cfg.UsePostgres(),
		"mfa_enabled", c.ClientIP(), "compte="+claims.UserID)

	c.JSON(http.StatusOK, gin.H{
		"enabled":        true,
		"recovery_codes": codes,
		"warning": "Notez ces codes maintenant, ailleurs que sur ce PC et hors de l'appareil " +
			"qui porte votre application. Ils ne seront plus jamais affichés. Sans eux, la " +
			"perte de cette application vous ferme définitivement l'accès.",
	})
}

// MFAVerify POST /api/v1/auth/mfa/verify
//
// Deuxième étape de la connexion : le jeton d'attente est échangé contre un
// vrai jeton d'accès, contre un code de l'application ou un code de secours.
func (h *AuthHandler) MFAVerify(c *gin.Context) {
	var body struct {
		Code string `json:"code" binding:"required"`
		// RememberDevice dispense ce poste de code pendant trente jours. Le
		// choix appartient à l'utilisateur et n'est jamais coché d'office : une
		// protection qu'on lève sans le savoir n'en est plus une.
		RememberDevice bool `json:"remember_device"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	claims := mw.GetClaims(c)
	if claims == nil || !claims.MFAPending {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "cette étape suit une saisie de mot de passe : recommencez la connexion"})
		return
	}

	ctx := c.Request.Context()
	enabled, secret, lastWindow, err := h.mfaState(ctx, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if !enabled {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "aucun second facteur n'est actif sur ce compte"})
		return
	}

	code := strings.TrimSpace(body.Code)
	accepted := false

	if window, ok := mfa.Verify(secret, code, time.Now(), lastWindow); ok {
		accepted = true
		upd := db.Rebind(`UPDATE user_mfa SET last_window = ?, updated_at = ? WHERE user_id = ?`,
			h.cfg.UsePostgres())
		if _, err := h.db.ExecContext(ctx, upd, window, time.Now().UTC(), claims.UserID); err != nil {
			// Le code est bon ; ne pas bloquer l'entrée pour un défaut
			// d'écriture. La réutilisation reste bornée par la fenêtre.
			_ = err
		}
	} else if ok, err := h.consumeRecoveryCode(ctx, claims.UserID, code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	} else if ok {
		accepted = true
		recordSecurityEvent(ctx, h.db, h.cfg.UsePostgres(),
			"mfa_recovery_code_used", c.ClientIP(), "compte="+claims.UserID)
	}

	if !accepted {
		recordSecurityEvent(ctx, h.db, h.cfg.UsePostgres(),
			"mfa_failed", c.ClientIP(), "compte="+claims.UserID)
		// Aucun détail sur ce qui a échoué : dire « ce n'était pas un code de
		// secours » renseignerait sur le format attendu.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "code incorrect"})
		return
	}

	// L'état du compte est relu ICI, et pas repris du jeton d'attente. Entre la
	// saisie du mot de passe et celle du code, cinq minutes peuvent passer — de
	// quoi désactiver un compte ou changer son rôle, et il serait absurde que la
	// session naisse avec des droits périmés.
	var (
		role       string
		isActive   int
		mustChange int
		isAdmin    bool
	)
	stateQ := db.Rebind(`
		SELECT COALESCE(role,'accountant'), is_active, COALESCE(must_change_password,0), is_admin
		FROM users WHERE id = ?`, h.cfg.UsePostgres())
	if err := h.db.QueryRowContext(ctx, stateQ, claims.UserID).
		Scan(&role, &isActive, &mustChange, &isAdmin); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ce compte n'est plus disponible"})
		return
	}
	if isActive != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "ce compte est désactivé"})
		return
	}

	if body.RememberDevice {
		h.rememberDevice(c, claims.UserID)
	}

	h.issueSession(c, claims.UserID, isAdmin, role, mustChange == 1)
}

// consumeRecoveryCode consomme un code de secours s'il correspond.
//
// La comparaison passe par le hachage de mot de passe : les codes sont stockés
// hachés, et quelqu'un qui lit la base ne doit pas y trouver de quoi contourner
// le second facteur.
func (h *AuthHandler) consumeRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	// La normalisation est délibérément indulgente : ce code se recopie à la
	// main depuis un papier, souvent des mois plus tard, parfois dicté au
	// téléphone. Les tirets de lisibilité, les espaces de frappe et la casse ne
	// portent aucune information — les refuser transformerait la dernière porte
	// de secours en énigme, au pire moment.
	normalised := strings.ToUpper(strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, code))
	if len(normalised) < 8 {
		return false, nil
	}
	q := db.Rebind(
		`SELECT id, code_hash FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL`,
		h.cfg.UsePostgres())
	rows, err := h.db.QueryContext(ctx, q, userID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	type candidate struct{ id, hash string }
	var all []candidate
	for rows.Next() {
		var cd candidate
		if err := rows.Scan(&cd.id, &cd.hash); err != nil {
			return false, err
		}
		all = append(all, cd)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	for _, cd := range all {
		if security.CheckPassword(cd.hash, normalised) {
			upd := db.Rebind(`UPDATE mfa_recovery_codes SET used_at = ? WHERE id = ?`,
				h.cfg.UsePostgres())
			if _, err := h.db.ExecContext(ctx, upd, time.Now().UTC(), cd.id); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// MFADisable DELETE /api/v1/auth/mfa
//
// Retire le second facteur. Le mot de passe est redemandé : sans cela, un poste
// laissé ouvert suffirait à désactiver la protection.
func (h *AuthHandler) MFADisable(c *gin.Context) {
	var body struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}

	ctx := c.Request.Context()
	var hash string
	q := db.Rebind(`SELECT password_hash FROM users WHERE id = ?`, h.cfg.UsePostgres())
	if err := h.db.QueryRowContext(ctx, q, claims.UserID).Scan(&hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if !security.CheckPassword(hash, body.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "mot de passe incorrect"})
		return
	}

	if _, err := h.db.ExecContext(ctx,
		db.Rebind(`DELETE FROM user_mfa WHERE user_id = ?`, h.cfg.UsePostgres()),
		claims.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if _, err := h.db.ExecContext(ctx,
		db.Rebind(`DELETE FROM mfa_recovery_codes WHERE user_id = ?`, h.cfg.UsePostgres()),
		claims.UserID); err != nil {
		_ = err
	}

	forgetDevices(ctx, h.db, h.cfg.UsePostgres(), claims.UserID)

	recordSecurityEvent(ctx, h.db, h.cfg.UsePostgres(),
		"mfa_disabled", c.ClientIP(), "compte="+claims.UserID)
	c.JSON(http.StatusOK, gin.H{"message": "Second facteur retiré."})
}

// newRecoveryCodes tire les codes et leurs empreintes.
//
// L'alphabet exclut I, O, 0, 1 : ces codes se recopient à la main depuis un
// papier, souvent des mois plus tard, et la confusion entre un zéro et un O
// est la première cause d'échec.
func newRecoveryCodes() (plain []string, hashed []string, err error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for i := 0; i < RecoveryCodeCount; i++ {
		buf := make([]byte, 10)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("génération d'un code de secours: %w", err)
		}
		var sb strings.Builder
		for j, b := range buf {
			if j == 5 {
				sb.WriteByte('-') // deux groupes de cinq : plus facile à lire et à dicter
			}
			sb.WriteByte(alphabet[int(b)%len(alphabet)])
		}
		code := sb.String()
		hsh, err := security.HashPassword(strings.ReplaceAll(code, "-", ""))
		if err != nil {
			return nil, nil, err
		}
		plain = append(plain, code)
		hashed = append(hashed, hsh)
	}
	return plain, hashed, nil
}

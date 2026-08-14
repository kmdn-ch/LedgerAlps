package handlers

// « Se souvenir de cet ordinateur ».
//
// # Pourquoi cette option existe
//
// Le second facteur protège du cas où le MOT DE PASSE fuit. Sur le poste où l'on
// travaille tous les jours, redemander un code à chaque connexion n'ajoute
// presque rien à cette protection : quelqu'un qui a déjà la main sur la machine
// n'attend pas la prochaine connexion. En revanche, une protection vécue comme
// une brimade finit désactivée — et c'est la façon la plus sûre de la perdre
// entièrement.
//
// # Trente jours, et pas un de plus à l'usage
//
// La date d'expiration est ABSOLUE : se connecter ne la prolonge pas. Sans cela,
// un poste utilisé chaque semaine ne redemanderait plus jamais de code, et
// l'option cesserait d'être une exception pour devenir la règle.
//
// # Ce qui est stocké
//
// Le haché du jeton, jamais le jeton. Le navigateur en garde l'unique copie dans
// un cookie HttpOnly — hors de portée d'un script. Quelqu'un qui lit la base ne
// peut donc pas s'en servir pour contourner le second facteur, exactement comme
// pour les codes de secours.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

const (
	// TrustedDeviceDays est la durée de confiance accordée à un poste.
	//
	// Trente jours : assez pour couvrir un mois de travail sans redemander,
	// assez court pour qu'un portable oublié redevienne protégé avant qu'on ait
	// fini de le chercher.
	TrustedDeviceDays = 30

	trustedDeviceCookie = "ledgeralps_device"
)

// newDeviceToken tire un jeton et rend sa forme lisible et son haché.
func newDeviceToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	return token, hashDeviceToken(token), nil
}

// hashDeviceToken utilise SHA-256 et non bcrypt.
//
// Le jeton fait 256 bits d'aléa : il ne se devine pas, donc rien à ralentir. Un
// hachage lent obligerait à le comparer à chaque ligne de la table à chaque
// connexion, ce qui rendrait la recherche par index impossible.
func hashDeviceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// deviceLabel décrit l'appareil sans l'identifier.
//
// Le nom du navigateur et du système suffisent à reconnaître une ligne dans une
// liste. Une empreinte de navigateur complète serait une donnée personnelle de
// plus, collectée pour un confort d'affichage (nLPD art. 6 : minimisation).
func deviceLabel(userAgent string) string {
	ua := strings.ToLower(userAgent)
	nav := "Navigateur"
	switch {
	case strings.Contains(ua, "firefox"):
		nav = "Firefox"
	case strings.Contains(ua, "edg/"):
		nav = "Edge"
	case strings.Contains(ua, "chrome"):
		nav = "Chrome"
	case strings.Contains(ua, "safari"):
		nav = "Safari"
	}
	sys := ""
	switch {
	case strings.Contains(ua, "windows"):
		sys = " sur Windows"
	case strings.Contains(ua, "mac os"), strings.Contains(ua, "macintosh"):
		sys = " sur macOS"
	case strings.Contains(ua, "linux"):
		sys = " sur Linux"
	}
	return nav + sys
}

// rememberDevice pose le cookie et enregistre l'empreinte du jeton.
//
// Un échec n'interrompt pas la connexion : ne pas se souvenir du poste est un
// désagrément, refuser d'entrer serait une panne.
func (h *AuthHandler) rememberDevice(c *gin.Context, userID string) {
	token, hash, err := newDeviceToken()
	if err != nil {
		return
	}
	expires := time.Now().UTC().Add(TrustedDeviceDays * 24 * time.Hour)

	q := db.Rebind(`
		INSERT INTO trusted_devices (id, user_id, token_hash, label, last_ip, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(c.Request.Context(), q,
		db.NewID(), userID, hash, deviceLabel(c.GetHeader("User-Agent")),
		c.ClientIP(), expires, time.Now().UTC()); err != nil {
		return
	}

	// SameSite=Strict : ce cookie dispense d'un second facteur, il n'a aucune
	// raison d'accompagner une requête venue d'un autre site.
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(trustedDeviceCookie, token, TrustedDeviceDays*24*3600, "/",
		"", c.Request.TLS != nil, true)
}

// deviceIsTrusted dit si la requête vient d'un poste déjà vérifié.
//
// Le jeton est lié au COMPTE : un poste de confiance pour l'un ne l'est pas pour
// l'autre. Sans cette liaison, deux personnes partageant un poste
// s'affranchiraient mutuellement du second facteur.
func (h *AuthHandler) deviceIsTrusted(ctx context.Context, c *gin.Context, userID string) bool {
	token, err := c.Cookie(trustedDeviceCookie)
	if err != nil || token == "" {
		return false
	}
	q := db.Rebind(`
		SELECT id, expires_at FROM trusted_devices
		WHERE user_id = ? AND token_hash = ?`, h.cfg.UsePostgres())
	var id string
	var expires time.Time
	if err := h.db.QueryRowContext(ctx, q, userID, hashDeviceToken(token)).
		Scan(&id, &expires); err != nil {
		return false
	}
	if time.Now().UTC().After(expires) {
		// Périmé : on nettoie plutôt que de laisser la ligne traîner. La table
		// se garderait sinon indéfiniment des postes qui ne reviendront pas.
		_, _ = h.db.ExecContext(ctx,
			db.Rebind(`DELETE FROM trusted_devices WHERE id = ?`, h.cfg.UsePostgres()), id)
		return false
	}

	// La date d'expiration n'est PAS prolongée : seule la trace du dernier usage
	// l'est. Prolonger ferait qu'un poste utilisé chaque semaine ne redemanderait
	// plus jamais de code, et l'exception deviendrait la règle.
	_, _ = h.db.ExecContext(ctx, db.Rebind(
		`UPDATE trusted_devices SET last_used_at = ?, last_ip = ? WHERE id = ?`,
		h.cfg.UsePostgres()), time.Now().UTC(), c.ClientIP(), id)
	return true
}

// forgetDevices révoque tous les postes de confiance d'un compte.
//
// Appelé quand le second facteur est retiré ou réinscrit, et quand le mot de
// passe change : dans les trois cas, la confiance accordée à un poste reposait
// sur une situation qui n'existe plus.
func forgetDevices(ctx context.Context, database *sql.DB, usePostgres bool, userID string) {
	_, _ = database.ExecContext(ctx,
		db.Rebind(`DELETE FROM trusted_devices WHERE user_id = ?`, usePostgres), userID)
}

// ListTrustedDevices GET /api/v1/auth/devices
func (h *AuthHandler) ListTrustedDevices(c *gin.Context) {
	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}
	q := db.Rebind(`
		SELECT id, label, last_ip, expires_at, created_at, last_used_at
		FROM trusted_devices WHERE user_id = ? ORDER BY created_at DESC`, h.cfg.UsePostgres())
	rows, err := h.db.QueryContext(c.Request.Context(), q, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	type view struct {
		ID        string     `json:"id"`
		Label     string     `json:"label"`
		LastIP    string     `json:"last_ip"`
		ExpiresAt time.Time  `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
		LastUsed  *time.Time `json:"last_used_at"`
	}
	items := []view{}
	for rows.Next() {
		var v view
		var last sql.NullTime
		if err := rows.Scan(&v.ID, &v.Label, &v.LastIP, &v.ExpiresAt, &v.CreatedAt, &last); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
		if last.Valid {
			v.LastUsed = &last.Time
		}
		items = append(items, v)
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "days": TrustedDeviceDays})
}

// ForgetTrustedDevices DELETE /api/v1/auth/devices
//
// Le geste à faire depuis un autre poste quand un ordinateur est perdu ou
// vendu : tous les postes redemandent un code à la connexion suivante.
func (h *AuthHandler) ForgetTrustedDevices(c *gin.Context) {
	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}
	forgetDevices(c.Request.Context(), h.db, h.cfg.UsePostgres(), claims.UserID)

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(trustedDeviceCookie, "", -1, "/", "", c.Request.TLS != nil, true)

	recordSecurityEvent(c.Request.Context(), h.db, h.cfg.UsePostgres(),
		"trusted_devices_cleared", c.ClientIP(), "compte="+claims.UserID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Tous les ordinateurs de confiance ont été oubliés. Un code sera " +
			"redemandé à la prochaine connexion, sur chacun d'eux.",
	})
}

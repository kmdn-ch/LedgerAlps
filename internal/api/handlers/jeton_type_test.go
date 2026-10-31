package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Audit 9, A-02 : les jetons d'accès, de rafraîchissement et d'attente du second
// facteur partageaient structure, secret et algorithme. Un jeton de
// rafraîchissement (trente jours) présenté en `Authorization: Bearer` ouvrait
// toute l'API, survivait à la déconnexion et au changement de mot de passe, et
// contournait le second facteur. Chaque porte n'accepte désormais que son jeton.
func TestChaqueJetonNOuvreQueSaPorte(t *testing.T) {
	database, _, cfg := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAuthHandler(database, cfg)
	r.GET("/acces", mw.RequireAuth(cfg.JWTSecret), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.GET("/attente", mw.RequireMFAChallenge(cfg.JWTSecret), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.POST("/auth/refresh", h.Refresh)

	acces, err := security.GenerateAccessToken(cfg.JWTSecret, "u1", false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	attente, err := security.GenerateMFAChallengeToken(cfg.JWTSecret, "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	rafraichissement, jti, err := security.GenerateRefreshToken(cfg.JWTSecret, "u1", false, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO refresh_tokens (id, user_id, jti, expires_at, created_at) VALUES (?, 'u1', ?, ?, ?)`,
		db.NewID(), jti, time.Now().UTC().Add(30*24*time.Hour), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// Un jeton d'avant cette version : aucun type.
	ancien, err := jwt.NewWithClaims(jwt.SigningMethodHS256, security.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Subject:   "u1",
		},
		UserID: "u1",
		JTI:    jti,
	}).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}

	portes := []struct {
		nom, methode, chemin string
		accepte              string // le jeton qui doit passer
	}{
		{"API protégée", http.MethodGet, "/acces", "accès"},
		{"vérification du second facteur", http.MethodGet, "/attente", "attente"},
		{"rafraîchissement", http.MethodPost, "/auth/refresh", "rafraîchissement"},
	}
	jetons := []struct{ nom, valeur string }{
		{"accès", acces}, {"attente", attente}, {"rafraîchissement", rafraichissement}, {"sans type", ancien},
	}
	for _, p := range portes {
		for _, j := range jetons {
			w := call(r, p.methode, p.chemin, j.valeur, nil)
			passe := w.Code == http.StatusOK
			if want := j.nom == p.accepte; passe != want {
				t.Errorf("porte « %s » avec le jeton « %s » : statut %d, passage attendu = %v", p.nom, j.nom, w.Code, want)
			}
		}
	}
}

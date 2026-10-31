package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Audit 9, M-01 : le limiteur de connexion remet son compteur à zéro sur tout
// succès, et la route publique /uid-lookup en partageait l'instance. Un appel
// public (200) entre deux essais de mot de passe effaçait donc les échecs :
// 200 essais acceptés sur 200, au lieu de 10.

func appel(r *gin.Engine, method, path string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.0.2.10:1234"
	r.ServeHTTP(w, req)
	return w.Code
}

// Deux limiteurs distincts : un succès de l'un n'efface rien chez l'autre.
func TestUnSuccesPublicNEffacePasLesEchecsDeConnexion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	connexion := NewLoginRateLimiter(10, time.Minute, time.Hour)
	recherche := NewRequestRateLimiter(1000, time.Minute, time.Hour)

	r := gin.New()
	r.POST("/login", connexion.Middleware(), func(c *gin.Context) { c.Status(http.StatusUnauthorized) })
	r.GET("/public", recherche.Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	accepte := 0
	for i := 0; i < 200; i++ {
		if appel(r, http.MethodGet, "/public") != http.StatusOK {
			t.Fatal("la route publique est refusée avant son plafond")
		}
		if appel(r, http.MethodPost, "/login") == http.StatusUnauthorized {
			accepte++
		}
	}
	if accepte != 10 {
		t.Fatalf("%d essais de mot de passe acceptés sur 200, attendu 10 — un appel public a remis le compteur à zéro", accepte)
	}
}

// La route qui coûte un appel sortant compte CHAQUE appel, réussi ou non : sans
// cela un tiers la fait émettre en boucle tant que les réponses sont des 200.
func TestLeLimiteurDeDebitCompteLesSucces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewRequestRateLimiter(30, time.Minute, time.Hour)
	r := gin.New()
	r.GET("/recherche", l.Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 1; i <= 30; i++ {
		if got := appel(r, http.MethodGet, "/recherche"); got != http.StatusOK {
			t.Fatalf("appel %d : %d, attendu 200", i, got)
		}
	}
	if got := appel(r, http.MethodGet, "/recherche"); got != http.StatusTooManyRequests {
		t.Fatalf("31e appel : %d, attendu 429 — des succès ne comptent pas", got)
	}
}

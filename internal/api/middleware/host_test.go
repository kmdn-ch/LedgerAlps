package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func requeteHote(r *gin.Engine, host string) int {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = host
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// Audit 9, F-06 : sans contrôle de l'en-tête Host, un site hostile qui fait
// pointer son nom vers 127.0.0.1 (« DNS rebinding ») lit les réponses du serveur
// local.
func TestUnHostEtrangerEstRefuseSurLaLoopback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HostGuard("127.0.0.1", "http://localhost:5173", "https://compta.exemple.ch"))
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	for host, want := range map[string]int{
		"localhost:8000":         http.StatusOK,
		"LOCALHOST":              http.StatusOK,
		"127.0.0.1:8000":         http.StatusOK,
		"[::1]:8000":             http.StatusOK,
		"compta.exemple.ch":      http.StatusOK, // origine déclarée, derrière un mandataire TLS local
		"evil.example":           http.StatusMisdirectedRequest,
		"evil.example:8000":      http.StatusMisdirectedRequest,
		"127.0.0.1.evil.example": http.StatusMisdirectedRequest,
		"localhost.evil.example": http.StatusMisdirectedRequest,
		"0.0.0.0:8000":           http.StatusMisdirectedRequest,
		"192.168.1.20:8000":      http.StatusMisdirectedRequest,
	} {
		if got := requeteHote(r, host); got != want {
			t.Errorf("Host %q : %d, attendu %d", host, got, want)
		}
	}
}

// Exposé volontairement sur le réseau, le garde se retire : l'installation y est
// joignable par des noms qu'il ne connaît pas.
func TestLeGardeDeHoteSeRetireQuandLeServeurEstExpose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ecoute := range []string{"0.0.0.0", "", "192.168.1.20", "ledger.lan"} {
		r := gin.New()
		r.Use(HostGuard(ecoute))
		r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
		if got := requeteHote(r, "ledger.lan:8443"); got != http.StatusOK {
			t.Errorf("écoute %q : %d, le garde devrait être retiré", ecoute, got)
		}
	}
}

// Audit 9, F-01 : aucune réponse de l'API n'était marquée non cachable.
func TestLesReponsesDeLAPINeSeMettentPasEnCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	for _, p := range []string{"/api/v1/accounts", "/health", "/assets/app.js"} {
		r.GET(p, func(c *gin.Context) { c.Status(http.StatusOK) })
	}
	for chemin, noStore := range map[string]bool{"/api/v1/accounts": true, "/health": true, "/assets/app.js": false} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, chemin, nil))
		if got := w.Header().Get("Cache-Control") == "no-store"; got != noStore {
			t.Errorf("%s : Cache-Control=%q, no-store attendu = %v", chemin, w.Header().Get("Cache-Control"), noStore)
		}
		if w.Header().Get("Permissions-Policy") == "" {
			t.Errorf("%s : Permissions-Policy absent", chemin)
		}
	}
}

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
func TestUnHostEtrangerEstRefuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HostGuard())
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	for host, want := range map[string]int{
		"localhost:8000":         http.StatusOK,
		"LOCALHOST":              http.StatusOK,
		"127.0.0.1:8000":         http.StatusOK,
		"[::1]:8000":             http.StatusOK,
		"evil.example":           http.StatusMisdirectedRequest,
		"evil.example:8000":      http.StatusMisdirectedRequest,
		"127.0.0.1.evil.example": http.StatusMisdirectedRequest,
		"localhost.evil.example": http.StatusMisdirectedRequest,
		"0.0.0.0:8000":           http.StatusMisdirectedRequest,
		"192.168.1.20:8000":      http.StatusMisdirectedRequest,
		// Un nom public — celui qu'un mandataire TLS local retransmettait avant la 1.6.6 : plus accepté.
		"compta.exemple.ch": http.StatusMisdirectedRequest,
	} {
		if got := requeteHote(r, host); got != want {
			t.Errorf("Host %q : %d, attendu %d", host, got, want)
		}
	}
}

// Le mode récupération applique la même règle que le serveur normal.
func TestHostAllowedSuitLaMemeRegleQueLeGarde(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost:8000": true, "127.0.0.1": true, "[::1]:8000": true,
		"evil.example": false, "compta.exemple.ch": false, "192.168.1.20": false, "": false,
	} {
		if got := HostAllowed(host); got != want {
			t.Errorf("HostAllowed(%q) = %v, attendu %v", host, got, want)
		}
	}
}

// Pas de HSTS : un navigateur qui l'enregistre pour « localhost » refuse ensuite http:// sur cet hôte.
func TestAucunEnTeteStrictTransportSecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	for _, proto := range []string{"", "https"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if v := w.Header().Get("Strict-Transport-Security"); v != "" {
			t.Errorf("X-Forwarded-Proto=%q : Strict-Transport-Security = %q, attendu absent", proto, v)
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

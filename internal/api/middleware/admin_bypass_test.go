package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// RequireAdmin laissait passer tout utilisateur authentifié.
//
// Il était écrit « auth(c) » puis contrôle de IsAdmin, où auth valait
// RequireAuth — dont la dernière instruction est c.Next(). c.Next() exécute LA
// SUITE DE LA CHAÎNE, handler compris. Le handler répondait donc, et le
// contrôle admin n'intervenait qu'après, sur une réponse déjà partie en 200 :
// le 403 finissait collé derrière la charge utile, sans effet.
//
// Trouvé en appelant GET /api/v1/backups sur un serveur qui tourne, avec un
// jeton non-administrateur, et en lisant les octets renvoyés — le corps
// contenait la liste des sauvegardes ET le message d'erreur.
//
// Ce test échoue si le raccourci revient : il vérifie les deux choses qui
// comptent — le statut, et le fait que le handler n'ait PAS tourné.

const testSecret = "secret-de-test-pour-le-middleware-0123"

func tokenFor(t *testing.T, isAdmin bool) string {
	t.Helper()
	tok, err := security.GenerateAccessToken(testSecret, "u1", isAdmin, time.Hour)
	if err != nil {
		t.Fatalf("génération du jeton: %v", err)
	}
	return tok
}

func routerWithAdminRoute(t *testing.T) (*gin.Engine, *bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handlerRan := false
	// Le groupe porte RequireAuth, exactement comme en production, pour que le
	// test reproduise la vraie chaîne et pas une version simplifiée.
	api := r.Group("/api/v1")
	api.Use(RequireAuth(testSecret))
	api.GET("/secret", RequireAdmin(testSecret), func(c *gin.Context) {
		handlerRan = true
		c.JSON(http.StatusOK, gin.H{"secret": "liste des sauvegardes"})
	})
	return r, &handlerRan
}

func TestUnNonAdminNObtientPasLaChargeUtileAdmin(t *testing.T) {
	r, handlerRan := routerWithAdminRoute(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, false))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("statut = %d, attendu 403", w.Code)
	}
	if *handlerRan {
		t.Error("le handler admin a tourné pour un non-administrateur")
	}
	if strings.Contains(w.Body.String(), "liste des sauvegardes") {
		t.Errorf("la charge utile admin a été renvoyée à un non-administrateur: %s", w.Body.String())
	}
}

func TestUnAdminPasseNormalement(t *testing.T) {
	r, handlerRan := routerWithAdminRoute(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, true))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("statut = %d pour un administrateur, attendu 200 (%s)", w.Code, w.Body.String())
	}
	if !*handlerRan {
		t.Error("le handler n'a pas tourné pour un administrateur")
	}
}

func TestSansJetonLaRouteAdminRepond401(t *testing.T) {
	r, handlerRan := routerWithAdminRoute(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("statut = %d sans jeton, attendu 401", w.Code)
	}
	if *handlerRan {
		t.Error("le handler a tourné sans authentification")
	}
}

// Le corps ne doit contenir qu'une seule réponse. Deux JSON collés étaient le
// symptôme visible du défaut, et le genre de chose qu'un client tolère en
// silence — axios lit le premier objet et ignore la suite.
func TestUneSeuleReponseDansLeCorps(t *testing.T) {
	r, _ := routerWithAdminRoute(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, false))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if n := strings.Count(w.Body.String(), "}{"); n > 0 {
		t.Errorf("le corps contient %d réponses concaténées: %s", n+1, w.Body.String())
	}
}

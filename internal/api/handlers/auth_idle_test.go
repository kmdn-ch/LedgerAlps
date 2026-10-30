package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
)

// Le cookie de rafraîchissement ne doit pas redonner de session à un compte
// resté inactif plus longtemps que le délai réglé : sans cela, le délai se
// contourne d'un appel à /auth/refresh.
func TestLeRafraichissementRefuseUneSessionInactive(t *testing.T) {
	database, _, cfg := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	cfg.IdleLogoutMinutes = 10

	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	tracker := mw.NewActivityTrackerWithClock(func() time.Time { return now })
	h := NewAuthHandler(database, cfg)
	h.activity = tracker

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/auth/login", h.Login)
	r.POST("/auth/refresh", h.Refresh)

	body := `{"email":"u1@t.ch","password":"MotDePasseSolide1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("connexion : %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("aucun cookie de rafraîchissement")
	}

	refresh := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	now = now.Add(5 * time.Minute)
	if w := refresh(); w.Code != http.StatusOK {
		t.Fatalf("rafraîchissement dans le délai : %d %s", w.Code, w.Body.String())
	}

	now = now.Add(30 * time.Minute)
	w = refresh()
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "session_idle") {
		t.Fatalf("rafraîchissement après inactivité : %d %s, attendu 401 session_idle", w.Code, w.Body.String())
	}

	// Délai désactivé : le même cookie redonne une session.
	cfg.IdleLogoutMinutes = 0
	if w := refresh(); w.Code != http.StatusOK {
		t.Fatalf("délai désactivé : %d %s", w.Code, w.Body.String())
	}
}

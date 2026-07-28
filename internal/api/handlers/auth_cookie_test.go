package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func cookieFromRecorder(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func runWithRequest(req *http.Request, h gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The whole point of the change: a script must not be able to read the refresh
// token. HttpOnly is what enforces that, so it is asserted first.
func TestRefreshCookieIsHttpOnly(t *testing.T) {
	w := runWithRequest(httptest.NewRequest(http.MethodGet, "/probe", nil), func(c *gin.Context) {
		setRefreshCookie(c, "jeton-secret", 30*24*time.Hour)
		c.Status(http.StatusOK)
	})

	ck := cookieFromRecorder(w, refreshCookieName)
	if ck == nil {
		t.Fatal("no refresh cookie was set")
	}
	if !ck.HttpOnly {
		t.Error("cookie must be HttpOnly — otherwise script can read the refresh token, which is the flaw being fixed")
	}
	if ck.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", ck.SameSite)
	}
	if ck.Path != refreshCookiePath {
		t.Errorf("Path = %q, want %q so the cookie is not attached to every request", ck.Path, refreshCookiePath)
	}
	if ck.MaxAge <= 0 {
		t.Errorf("MaxAge = %d, want the refresh lifetime", ck.MaxAge)
	}
}

// LedgerAlps is local-first: most installs serve plain HTTP on localhost, where
// a Secure cookie is dropped by the browser and would lock the user out.
func TestSecureFlagFollowsTheConnection(t *testing.T) {
	plain := runWithRequest(httptest.NewRequest(http.MethodGet, "/probe", nil), func(c *gin.Context) {
		setRefreshCookie(c, "t", time.Hour)
		c.Status(http.StatusOK)
	})
	if ck := cookieFromRecorder(plain, refreshCookieName); ck == nil || ck.Secure {
		t.Error("over plain HTTP the cookie must not be Secure, or localhost users cannot log in")
	}

	fwd := httptest.NewRequest(http.MethodGet, "/probe", nil)
	fwd.Header.Set("X-Forwarded-Proto", "https")
	behindProxy := runWithRequest(fwd, func(c *gin.Context) {
		setRefreshCookie(c, "t", time.Hour)
		c.Status(http.StatusOK)
	})
	if ck := cookieFromRecorder(behindProxy, refreshCookieName); ck == nil || !ck.Secure {
		t.Error("behind an HTTPS reverse proxy the cookie must be Secure")
	}
}

func TestClearRefreshCookieExpiresIt(t *testing.T) {
	w := runWithRequest(httptest.NewRequest(http.MethodGet, "/probe", nil), func(c *gin.Context) {
		clearRefreshCookie(c)
		c.Status(http.StatusOK)
	})
	ck := cookieFromRecorder(w, refreshCookieName)
	if ck == nil {
		t.Fatal("clearing must still emit a Set-Cookie so the browser drops it")
	}
	if ck.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative to expire the cookie", ck.MaxAge)
	}
	if ck.Value != "" {
		t.Errorf("value = %q, want empty", ck.Value)
	}
}

func TestRefreshTokenReadFromCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "depuis-le-cookie"})

	var got string
	var ok bool
	runWithRequest(req, func(c *gin.Context) {
		got, ok = refreshTokenFromRequest(c)
		c.Status(http.StatusOK)
	})
	if !ok || got != "depuis-le-cookie" {
		t.Errorf("got (%q, %v), want the cookie value", got, ok)
	}
}

// Scripts and non-browser clients keep working, and a session in flight from a
// previous version is not cut off mid-use.
func TestBearerStillAcceptedAsFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer depuis-l-entete")

	var got string
	var ok bool
	runWithRequest(req, func(c *gin.Context) {
		got, ok = refreshTokenFromRequest(c)
		c.Status(http.StatusOK)
	})
	if !ok || got != "depuis-l-entete" {
		t.Errorf("got (%q, %v), want the header value", got, ok)
	}
}

func TestCookieWinsOverHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer entete")
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie"})

	var got string
	runWithRequest(req, func(c *gin.Context) {
		got, _ = refreshTokenFromRequest(c)
		c.Status(http.StatusOK)
	})
	if got != "cookie" {
		t.Errorf("got %q, want the cookie to take precedence", got)
	}
}

func TestNoTokenAnywhere(t *testing.T) {
	var ok bool
	runWithRequest(httptest.NewRequest(http.MethodGet, "/probe", nil), func(c *gin.Context) {
		_, ok = refreshTokenFromRequest(c)
		c.Status(http.StatusOK)
	})
	if ok {
		t.Error("no cookie and no header must report absence")
	}
}

// An empty cookie must not be mistaken for a token — that is what the browser
// sends back after a logout has expired it.
func TestEmptyCookieFallsThroughToHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: ""})
	req.Header.Set("Authorization", "Bearer secours")

	var got string
	runWithRequest(req, func(c *gin.Context) {
		got, _ = refreshTokenFromRequest(c)
		c.Status(http.StatusOK)
	})
	if got != "secours" {
		t.Errorf("got %q, want the header once the cookie is empty", got)
	}
}

// Regression guard for the reason this exists: the token must never appear in
// a place JavaScript can reach.
func TestSetCookieHeaderCarriesHttpOnlyLiteral(t *testing.T) {
	w := runWithRequest(httptest.NewRequest(http.MethodGet, "/probe", nil), func(c *gin.Context) {
		setRefreshCookie(c, "abc", time.Hour)
		c.Status(http.StatusOK)
	})
	raw := w.Header().Get("Set-Cookie")
	if !strings.Contains(strings.ToLower(raw), "httponly") {
		t.Errorf("Set-Cookie %q must contain HttpOnly", raw)
	}
}

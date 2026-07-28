package handlers

// Refresh-token transport.
//
// The refresh token used to travel in the JSON login response and was kept in
// localStorage by the frontend, where any JavaScript could read it. It lives for
// thirty days, so a single cross-site scripting flaw meant a month of silent
// access to a company's accounts.
//
// It now travels in an HttpOnly cookie: script cannot read it, and the browser
// attaches it only to the auth endpoints. The short-lived access token stays in
// the JSON response and is held in memory by the frontend, so the worst an
// injected script can reach is a token that expires within the hour.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// refreshCookieName is deliberately unrelated to the storage key the old
	// frontend used, so an upgraded client cannot confuse the two.
	refreshCookieName = "ledgeralps_refresh"

	// refreshCookiePath scopes the cookie to the auth endpoints. Every other
	// request — the bulk of the traffic — never carries it, which limits what a
	// logging proxy or a mistaken handler could ever see.
	refreshCookiePath = "/api/v1/auth"
)

// isSecureRequest reports whether the connection is TLS-protected, directly or
// behind a reverse proxy that says so.
//
// The Secure attribute is set only then: LedgerAlps is local-first and most
// installations serve plain HTTP on localhost, where a Secure cookie would be
// dropped by the browser and lock the user out of their own accounts.
func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return c.GetHeader("X-Forwarded-Proto") == "https"
}

// setRefreshCookie stores the refresh token for the browser.
func setRefreshCookie(c *gin.Context, token string, ttl time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		// Strict rather than Lax: the refresh endpoint is only ever called by
		// the application itself, never by following a link from elsewhere, so
		// there is no reason for another site's navigation to carry it.
		SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie expires the cookie on logout.
func clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteStrictMode,
	})
}

// refreshTokenFromRequest returns the refresh token, preferring the cookie.
//
// The Authorization header is still accepted as a fallback so that scripts and
// non-browser clients keep working, and so an in-flight session from a previous
// version is not cut off mid-use. This is not a weakening: the token is no
// longer written anywhere a script can read, which is what the change is for.
func refreshTokenFromRequest(c *gin.Context) (string, bool) {
	if cookie, err := c.Cookie(refreshCookieName); err == nil && cookie != "" {
		return cookie, true
	}
	return bearerToken(c)
}

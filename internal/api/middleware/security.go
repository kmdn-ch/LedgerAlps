package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds defensive HTTP headers to every response.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// Rien de ce produit n'a besoin de l'appareil de la personne.
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		// Aucune réponse de l'API ne doit rester dans un cache : jetons d'accès,
		// données comptables, PDF de factures. Sans cet en-tête, un cache partagé
		// (mandataire d'entreprise) ou le disque du navigateur les conservait
		// (audit 9, F-01). Les fichiers de l'interface gardent leur cache. Un
		// gestionnaire qui pose son propre Cache-Control (un PDF) l'emporte, posé
		// après.
		if p := c.Request.URL.Path; strings.HasPrefix(p, "/api/") || p == "/health" {
			c.Header("Cache-Control", "no-store")
		}
		// img-src includes data: for base64 company logos stored as data URLs.
		// style-src includes 'unsafe-inline' for Tailwind utility classes injected at runtime.
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"font-src 'self' data:; "+
				"connect-src 'self'; "+
				"frame-src 'self' blob:; "+ // PDF preview uses blob: iframe
				"object-src 'none'; "+
				"base-uri 'self'")

		// Pas de Strict-Transport-Security : LedgerAlps ne sert qu'en HTTP sur la boucle locale, sans TLS.
		// Un navigateur qui enregistrerait HSTS pour « localhost » refuserait ensuite http:// sur cet
		// hôte, et l'utilisateur perdrait l'accès à sa comptabilité.

		c.Next()
	}
}

package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds defensive HTTP headers to every response.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
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

		// HSTS uniquement sur HTTPS REEL.
		//
		// La condition passait par c.GetHeader, donc croyait un en-tete que
		// n'importe qui pose. Emettre HSTS sur une reponse servie en clair
		// n'est pas une protection de trop : un navigateur qui l'enregistre
		// pour « localhost » refuse ensuite http:// sur cet hote, et
		// l'utilisateur perd l'acces a sa comptabilite. Voir mandataire.go.
		if ProtocoleAnnonceHTTPS(c) {
			c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		c.Next()
	}
}

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS adds Cross-Origin Resource Sharing headers.
// allowedOrigins should be the exact origins of the frontend (e.g. "http://localhost:5173").
// Never use "*" with credentials — this would bypass authentication.
func CORS(allowedOrigins ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(c *gin.Context) {
		// `Vary: Origin` sur TOUTES les réponses, pas seulement quand l'origine est admise.
		// La réponse dépend de l'en-tête Origin dans les deux cas : sans lui, un cache
		// intermédiaire qui garde la réponse faite à une origine refusée (sans
		// Access-Control-Allow-Origin) la servirait ensuite à une origine admise, dont le
		// navigateur bloquerait alors la lecture (audit croisé Gemini, MED-12).
		c.Writer.Header().Add("Vary", "Origin")

		origin := c.GetHeader("Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept")
			c.Header("Access-Control-Max-Age", "3600")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

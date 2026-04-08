package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

const claimsKey = "claims"

// RequireAuth validates the Bearer JWT in the Authorization header.
// On success it stores the *security.Claims in the Gin context under "claims".
func RequireAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := security.ParseToken(jwtSecret, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

// RequireAdmin extends RequireAuth by additionally checking IsAdmin.
func RequireAdmin(jwtSecret string) gin.HandlerFunc {
	auth := RequireAuth(jwtSecret)
	return func(c *gin.Context) {
		auth(c)
		if c.IsAborted() {
			return
		}
		claims := GetClaims(c)
		if claims == nil || !claims.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin privileges required"})
			return
		}
		c.Next()
	}
}

// GetClaims retrieves the JWT claims stored by RequireAuth.
func GetClaims(c *gin.Context) *security.Claims {
	v, ok := c.Get(claimsKey)
	if !ok {
		return nil
	}
	claims, _ := v.(*security.Claims)
	return claims
}

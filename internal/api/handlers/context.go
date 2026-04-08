package handlers

import (
	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
)

// currentUserID extracts the authenticated user's ID from the JWT claims.
// Returns "" if no claims are present (should not happen on protected routes).
func currentUserID(c *gin.Context) string {
	claims := mw.GetClaims(c)
	if claims == nil {
		return ""
	}
	return claims.UserID
}

// isAdmin returns true if the authenticated user has admin privileges.
func isAdmin(c *gin.Context) bool {
	claims := mw.GetClaims(c)
	return claims != nil && claims.IsAdmin
}

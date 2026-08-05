package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

const claimsKey = "claims"

// authenticate validates the Bearer JWT and stores the claims. It reports
// whether the request may continue, and aborts with the right status when not.
//
// It deliberately does NOT call c.Next(). That distinction is the whole point of
// this function existing: RequireAdmin used to be written as `auth(c)` followed
// by an IsAdmin check, where `auth` was RequireAuth — whose last statement is
// c.Next(). c.Next() runs the REST OF THE CHAIN, handler included. So the route
// handler executed and wrote its response, and only then did RequireAdmin look
// at IsAdmin and try to send 403 — on a response already committed with 200.
//
// The effect: every admin-only route answered any authenticated user in full,
// with the ignored 403 body appended after the payload. Found by calling
// GET /api/v1/backups on a running server with a non-admin token and reading
// the bytes that came back.
func authenticate(c *gin.Context, jwtSecret string) bool {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
		return false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	claims, err := security.ParseToken(jwtSecret, token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return false
	}
	// Un jeton d'attente de second facteur n'est pas une session. Le refus est
	// ici, au point de passage obligé, et non sur chaque route : une route
	// ajoutée demain le rejette sans que personne ait à y penser. L'inverse —
	// autoriser par défaut et bloquer route par route — s'oublie une fois, et
	// cette fois-là une moitié de connexion vaut une session complète.
	if claims.MFAPending {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":        "connexion incomplète : le second facteur n'a pas été validé",
			"mfa_required": true,
		})
		return false
	}
	c.Set(claimsKey, claims)
	return true
}

// RequireMFAChallenge n'accepte QUE le jeton d'attente.
//
// La symétrie compte : ce filtre est le seul endroit qui accepte un jeton
// d'attente, et il refuse un jeton d'accès complet. Présenter une session déjà
// valide à l'étape de vérification n'a pas de sens, et l'accepter permettrait
// de consommer un code de secours depuis une session ordinaire.
func RequireMFAChallenge(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "missing or malformed Authorization header"})
			return
		}
		claims, err := security.ParseToken(jwtSecret, strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "cette étape a expiré : recommencez la connexion"})
			return
		}
		if !claims.MFAPending {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "cette étape suit une saisie de mot de passe"})
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

// RequireAuth validates the Bearer JWT in the Authorization header.
// On success it stores the *security.Claims in the Gin context under "claims".
func RequireAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authenticate(c, jwtSecret) {
			return
		}
		c.Next()
	}
}

// RequireAdmin a été RETIRÉ.
//
// Il lisait le drapeau administrateur DANS LE JETON. Un jeton d'accès vivant
// une heure, rétrograder ou désactiver quelqu'un le laissait administrer
// pendant tout ce temps — une heure durant laquelle on croit avoir coupé
// l'accès.
//
// Le contrôle vit désormais dans middleware.Authorizer, qui lit le rôle dans la
// base à chaque requête. La fonction est supprimée et non dépréciée : laissée
// disponible, elle serait reprise par réflexe sur la prochaine route
// d'administration, et le défaut reviendrait sans que rien ne le signale.

// GetClaims retrieves the JWT claims stored by RequireAuth.
func GetClaims(c *gin.Context) *security.Claims {
	v, ok := c.Get(claimsKey)
	if !ok {
		return nil
	}
	claims, _ := v.(*security.Claims)
	return claims
}

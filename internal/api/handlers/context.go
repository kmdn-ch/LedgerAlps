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

// isAdmin a été SUPPRIMÉE.
//
// Elle lisait le drapeau administrateur DANS LE JETON — figé à la connexion.
// Rétrograder ou désactiver quelqu'un le laissait donc agir avec ses anciens
// droits jusqu'à l'expiration du jeton, une heure durant laquelle on croit
// avoir coupé l'accès. Neuf handlers l'appelaient encore : attestation
// d'intégrité, journal d'audit, anonymisation, exercices comptables, contacts.
//
// Elle écartait en outre le comptable de son propre métier : clôturer un
// exercice ou vérifier la chaîne d'empreintes n'a rien d'une fonction
// d'administration du logiciel.
//
// Le contrôle vit dans middleware.Authorizer, déclaré par route et lu dans la
// base à chaque requête. La fonction est supprimée et non dépréciée : laissée
// disponible, elle serait reprise par réflexe sur le prochain handler, et le
// défaut reviendrait sans que rien ne le signale — c'est exactement ce qui
// s'est produit après la suppression de RequireAdmin.

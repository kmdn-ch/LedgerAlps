package middleware

// Contrôle des droits.
//
// Le rôle est lu DANS LA BASE à chaque requête, jamais dans le jeton.
//
// C'est la décision qui porte tout le reste. Un jeton d'accès vit une heure ; si
// le rôle y était inscrit, rétrograder ou désactiver quelqu'un le laisserait
// agir avec ses anciens droits pendant tout ce temps — une heure durant laquelle
// on croit avoir coupé l'accès. La base est locale et la lecture est un accès
// par clé primaire : le coût est nul, et toute une classe de privilèges périmés
// disparaît.
//
// Le jeton ne prouve donc plus que l'identité. Ce qu'un compte a le droit de
// faire se demande à la base, au moment où il le fait.

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

const roleKey = "authz_role"

// Authorizer résout le rôle courant d'un compte.
type Authorizer struct {
	db          *sql.DB
	usePostgres bool
	jwtSecret   string
}

func NewAuthorizer(database *sql.DB, usePostgres bool, jwtSecret string) *Authorizer {
	return &Authorizer{db: database, usePostgres: usePostgres, jwtSecret: jwtSecret}
}

// currentRole rend le rôle du compte et son état d'activité, tels qu'ils sont
// MAINTENANT.
//
// Un compte introuvable ou désactivé n'a aucun rôle. Vérifier l'activité ici et
// pas seulement à la connexion est ce qui fait que désactiver quelqu'un le
// déconnecte vraiment, au lieu de le laisser travailler jusqu'à l'expiration de
// son jeton.
func (a *Authorizer) currentRole(userID string) (authz.Role, bool) {
	q := db.Rebind(`SELECT role, is_active FROM users WHERE id = ?`, a.usePostgres)
	var role string
	var active int
	if err := a.db.QueryRow(q, userID).Scan(&role, &active); err != nil {
		return "", false
	}
	if active != 1 {
		return "", false
	}
	r := authz.Role(role)
	if !r.Valid() {
		// Rôle inconnu — base bricolée, restaurée d'une version future, ou
		// migration ratée. Refuser, jamais deviner : deviner ici accorderait
		// des droits sur la foi d'une chaîne qu'on ne comprend pas.
		return "", false
	}
	return r, true
}

// Require exige une permission. C'est la barrière déclarée par route.
func (a *Authorizer) Require(p authz.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authenticate(c, a.jwtSecret) {
			return
		}
		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
			return
		}
		role, ok := a.currentRole(claims.UserID)
		if !ok {
			// Ne pas distinguer « compte supprimé » de « compte désactivé » :
			// la différence n'aide que celui qui sonde les comptes.
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "ce compte n'est plus actif"})
			return
		}
		if !authz.Can(role, p) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "votre rôle (" + role.Label() + ") ne permet pas cette action",
			})
			return
		}
		c.Set(roleKey, role)
		c.Next()
	}
}

// DenyWritesWithoutPermission est la seconde barrière, indépendante de la
// première.
//
// Les permissions par route dépendent de ce que le développeur a déclaré, et
// oublier une déclaration est humain — c'est même la façon la plus courante
// d'ouvrir un trou, parce que rien ne le signale. Ce filtre global refuse toute
// méthode d'écriture à un rôle qui n'écrit pas, quelle que soit la route et
// quoi qu'on ait déclaré ou oublié.
//
// Il ne remplace pas les permissions par route : un comptable passe ici et doit
// quand même être arrêté sur les routes d'administration. Les deux barrières
// couvrent des erreurs différentes, et tombent rarement ensemble.
func (a *Authorizer) DenyWritesWithoutPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.IsWriteMethod(c.Request.Method) {
			c.Next()
			return
		}
		claims := GetClaims(c)
		if claims == nil {
			// Route publique — connexion, rafraîchissement. L'authentification
			// est le problème d'un autre filtre ; celui-ci ne se prononce pas.
			c.Next()
			return
		}
		role, ok := a.currentRole(claims.UserID)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "ce compte n'est plus actif"})
			return
		}
		if role == authz.RoleViewer {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "votre rôle (" + role.Label() + ") ne permet aucune modification",
			})
			return
		}
		c.Next()
	}
}

// RoleOf rend le rôle résolu pour cette requête, quand un filtre l'a posé.
func RoleOf(c *gin.Context) (authz.Role, bool) {
	v, ok := c.Get(roleKey)
	if !ok {
		return "", false
	}
	r, ok := v.(authz.Role)
	return r, ok
}

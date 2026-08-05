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
	role, ok, _ := a.currentState(userID)
	return role, ok
}

// currentState rend le rôle, l'activité et l'obligation de changer le mot de
// passe, en une seule lecture.
func (a *Authorizer) currentState(userID string) (authz.Role, bool, bool) {
	q := db.Rebind(
		`SELECT role, is_active, COALESCE(must_change_password,0) FROM users WHERE id = ?`,
		a.usePostgres)
	var role string
	var active, mustChange int
	if err := a.db.QueryRow(q, userID).Scan(&role, &active, &mustChange); err != nil {
		return "", false, false
	}
	if active != 1 {
		return "", false, false
	}
	r := authz.Role(role)
	if !r.Valid() {
		// Rôle inconnu — base bricolée, restaurée d'une version future, ou
		// migration ratée. Refuser, jamais deviner : deviner ici accorderait
		// des droits sur la foi d'une chaîne qu'on ne comprend pas.
		return "", false, false
	}
	return r, true, mustChange == 1
}

// RequirePasswordChanged bloque tout compte dont le mot de passe temporaire n'a
// pas encore été remplacé.
//
// Le blocage est technique et non cosmétique : cacher les écrans n'aurait fermé
// aucune porte — l'adresse reste tapable, l'appel réseau reste faisable. Un
// compte marqué ne peut donc RIEN faire, pas même lire, tant qu'il n'a pas
// choisi son propre mot de passe.
//
// Ce que cela protège : le mot de passe créé par l'administrateur est connu
// d'au moins deux personnes et a voyagé par un canal qui n'est pas fait pour
// ça. Tant qu'il vaut, une action tracée sous ce compte ne prouve pas qui l'a
// faite — ce qui rend le journal d'audit trompeur, et non simplement incomplet.
func (a *Authorizer) RequirePasswordChanged() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.Next() // route publique : l'authentification est le problème d'un autre filtre
			return
		}
		_, ok, mustChange := a.currentState(claims.UserID)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "ce compte n'est plus actif"})
			return
		}
		if mustChange {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":                "changez d'abord votre mot de passe temporaire",
				"must_change_password": true,
			})
			return
		}
		c.Next()
	}
}

// RequireMFAEnrolled exige un second facteur inscrit sur les comptes
// administrateurs.
//
// # Pourquoi seulement les administrateurs
//
// Un compte administrateur peut créer des comptes, restaurer une sauvegarde,
// déverrouiller une période, changer les droits de tout le monde. Son mot de
// passe est la seule chose qui sépare quelqu'un de l'ensemble de la
// comptabilité. Un comptable, lui, écrit dans un journal chaîné et tracé, et
// n'a pas les clés de l'installation : lui imposer un téléphone coûterait plus
// qu'il ne protège.
//
// # Pourquoi le blocage est technique
//
// Comme pour le mot de passe temporaire : cacher les écrans ne ferme aucune
// porte. Un administrateur non inscrit ne peut RIEN faire d'autre que
// s'inscrire — les routes d'inscription vivent hors de ce groupe, sans quoi
// elles se bloqueraient elles-mêmes et le compte serait enfermé.
//
// # À la première connexion après la mise à jour
//
// Les installations existantes ont un administrateur sans second facteur. Il
// sera conduit à l'inscription avant de pouvoir travailler. C'est voulu : une
// protection qu'on peut remettre à plus tard n'est jamais activée.
func (a *Authorizer) RequireMFAEnrolled() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.Next() // route publique : ce filtre ne se prononce pas
			return
		}
		role, ok, _ := a.currentState(claims.UserID)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "ce compte n'est plus actif"})
			return
		}
		if role != authz.RoleAdmin {
			c.Next()
			return
		}
		if a.mfaConfirmed(claims.UserID) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "un compte administrateur doit être protégé par un second facteur : " +
				"inscrivez votre application d'authentification pour continuer",
			"mfa_enrolment_required": true,
		})
	}
}

// mfaConfirmed dit si le compte a une inscription CONFIRMÉE.
//
// Confirmée, et pas seulement commencée : un secret créé puis abandonné en cours
// d'assistant ne doit pas compter, sinon quelqu'un qui ferme l'onglet au milieu
// se retrouverait à devoir fournir un code qu'aucun téléphone ne calcule.
func (a *Authorizer) mfaConfirmed(userID string) bool {
	q := db.Rebind(
		`SELECT COUNT(*) FROM user_mfa WHERE user_id = ? AND confirmed_at IS NOT NULL`,
		a.usePostgres)
	var n int
	if err := a.db.QueryRow(q, userID).Scan(&n); err != nil {
		// Table absente (base plus ancienne que la migration) ou lecture en
		// échec : ne pas enfermer l'administrateur hors de sa propre
		// installation pour un défaut de lecture. Le mot de passe reste exigé,
		// et l'écran d'inscription reste accessible.
		return true
	}
	return n > 0
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

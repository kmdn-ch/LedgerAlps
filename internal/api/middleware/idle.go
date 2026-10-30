package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Déconnexion sur inactivité, côté serveur.
//
// Le réglage `idle_logout_minutes` n'était appliqué que par l'interface : un
// jeton d'accès copié, ou un onglet fermé sans déconnexion, restait utilisable
// toute sa durée de vie (soixante minutes par défaut), et le cookie de
// rafraîchissement en redonnait un nouveau. Le délai est maintenant tenu ici
// aussi, et l'interface reste la voie normale : elle prévient, laisse cliquer,
// et ne déconnecte pas quelqu'un en pleine saisie.
//
// # Ce qui compte comme activité
//
// Toute requête authentifiée, plus un battement d'activité (POST
// /auth/activity) que l'interface envoie quand quelqu'un se sert du clavier ou
// de la souris. Sans ce battement, saisir une facture de quinze lignes pendant
// plus longtemps que le délai ferait refuser l'enregistrement — le travail
// perdu que le comportement de l'interface s'attache à éviter.
//
// # Ce que ça ne fait pas
//
// L'état vit en mémoire : un redémarrage du serveur l'oublie, et la première
// requête qui suit repart d'un délai complet. C'est voulu — un processus
// relancé n'est pas une session restée ouverte.

// IdleGrace s'ajoute au délai : c'est la durée de l'avertissement de l'interface
// (soixante secondes), pendant laquelle quelqu'un peut encore cliquer « rester ».
// Le serveur ne coupe pas avant que l'interface ait fini de prévenir.
const IdleGrace = time.Minute

// ActivityTracker retient le dernier signe d'activité de chaque compte.
type ActivityTracker struct {
	mu   sync.Mutex
	last map[string]time.Time
	now  func() time.Time
}

func NewActivityTracker() *ActivityTracker {
	return NewActivityTrackerWithClock(time.Now)
}

// NewActivityTrackerWithClock permet aux tests de maîtriser l'horloge.
func NewActivityTrackerWithClock(now func() time.Time) *ActivityTracker {
	return &ActivityTracker{last: map[string]time.Time{}, now: now}
}

// Touch note que ce compte vient d'être actif.
func (t *ActivityTracker) Touch(userID string) {
	if t == nil || userID == "" {
		return
	}
	t.mu.Lock()
	t.last[userID] = t.now()
	t.mu.Unlock()
}

// Expired dit si ce compte est resté inactif plus longtemps que `limit`.
// Un compte jamais vu n'est pas expiré : voir la note sur le redémarrage.
func (t *ActivityTracker) Expired(userID string, limit time.Duration) bool {
	if t == nil || limit <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.last[userID]
	return ok && t.now().Sub(last) > limit
}

// IdleLimit refuse (401, `reason: session_idle`) toute requête d'un compte resté
// inactif plus longtemps que le délai réglé, et note l'activité de celles qu'il
// laisse passer. Il se place APRÈS RequireAuth, dont il lit les droits.
//
// `minutes` est relu à chaque requête : le réglage prend effet tout de suite,
// sans redémarrer. Zéro désactive la coupure — l'activité reste notée, pour
// qu'activer le réglage ensuite ne déconnecte personne d'un coup.
func IdleLimit(t *ActivityTracker, minutes func() int) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.Next()
			return
		}
		if m := minutes(); m > 0 &&
			t.Expired(claims.UserID, time.Duration(m)*time.Minute+IdleGrace) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":  "session fermée pour inactivité",
				"reason": "session_idle",
			})
			return
		}
		t.Touch(claims.UserID)
		c.Next()
	}
}

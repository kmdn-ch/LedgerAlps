package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// HostGuard refuse une requête dont l'en-tête Host n'est pas celui de la machine elle-même (audit 9,
// F-06).
//
// # Le défaut visé : le « DNS rebinding »
//
// Le serveur écoute sur 127.0.0.1. « Local » ne veut pas dire « inaccessible à une
// page web » : un site hostile peut faire pointer son propre nom de domaine vers
// 127.0.0.1 après coup. Le navigateur, qui croit toujours parler au même site,
// laisse alors la page lire les réponses du serveur local — et l'en-tête Host que
// reçoit LedgerAlps est celui du site hostile. Refuser tout Host inconnu referme
// cette porte, sans dépendre de CORS.
//
// # Ce qui est accepté
//
// localhost, 127.0.0.1 et ::1, avec n'importe quel port — et rien d'autre. Depuis la 1.6.6 LedgerAlps
// n'est plus joignable par le réseau : il n'y a plus de nom public à déclarer (les origines déclarées,
// qui servaient derrière un mandataire TLS, sont supprimées). Un mandataire local qui retransmettrait
// le nom public d'un site recevrait donc un 421.
func HostGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HostAllowed(c.Request.Host) {
			c.AbortWithStatusJSON(http.StatusMisdirectedRequest, gin.H{
				"error": "hôte non autorisé : ouvrez LedgerAlps par son adresse locale"})
			return
		}
		c.Next()
	}
}

// HostAllowed applique la règle de HostGuard à un serveur qui n'est pas gin : le mode
// récupération (cmd/server/recovery.go), qui demande la phrase enveloppant la clé de la base et
// n'avait aucun contrôle d'hôte (audit croisé Gemini, HIGH-17).
func HostAllowed(requestHost string) bool {
	switch nomDHote(requestHost) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// nomDHote extrait le nom d'hôte, sans port ni crochets, en minuscules.
func nomDHote(hostport string) string {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// HostGuard refuse une requête dont l'en-tête Host n'est pas celui d'une adresse
// légitime de l'installation (audit 9, F-06).
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
//   - localhost, 127.0.0.1 et ::1, avec n'importe quel port ;
//   - le nom d'hôte de chaque origine déclarée (ALLOWED_ORIGINS) : derrière un
//     mandataire TLS local, le Host transmis est celui du nom public.
//
// Quand le serveur est volontairement exposé sur le réseau (HOST non-loopback), le
// garde se retire : l'installation y est joignable par ses noms propres, et la
// liste exacte de ceux-ci n'est pas connue ici. Ce garde est une défense de la
// configuration par défaut, qui est la loopback.
func HostGuard(listenHost string, allowedOrigins ...string) gin.HandlerFunc {
	if !hoteLoopback(listenHost) {
		return func(c *gin.Context) { c.Next() }
	}
	autorises := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	for _, o := range allowedOrigins {
		if u, err := url.Parse(strings.TrimSpace(o)); err == nil && u.Hostname() != "" {
			autorises[strings.ToLower(u.Hostname())] = true
		}
	}
	return func(c *gin.Context) {
		if !autorises[nomDHote(c.Request.Host)] {
			c.AbortWithStatusJSON(http.StatusMisdirectedRequest, gin.H{
				"error": "hôte non autorisé : ouvrez LedgerAlps par son adresse locale"})
			return
		}
		c.Next()
	}
}

// nomDHote extrait le nom d'hôte, sans port ni crochets, en minuscules.
func nomDHote(hostport string) string {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

// hoteLoopback dit si l'adresse d'écoute ne sort pas de la machine. Vide ou
// « 0.0.0.0 » écoute sur toutes les interfaces : ce n'est PAS la loopback.
func hoteLoopback(h string) bool {
	h = strings.Trim(strings.ToLower(strings.TrimSpace(h)), "[]")
	if h == "localhost" || h == "::1" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

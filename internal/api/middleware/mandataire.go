package middleware

// Confiance dans les en-têtes de mandataire.
//
// # Le trou que ceci ferme
//
// `X-Forwarded-For` était déjà correctement traité : `SetTrustedProxies` est
// appelé au démarrage avec `TRUSTED_PROXIES`, et tout le code passe par
// `c.ClientIP()`, que gin fait dépendre de cette liste. Le raisonnement est
// écrit dans internal/config/config.go : sans cette garde, le verrouillage de
// connexion devient décoratif — une adresse différente à chaque requête est une
// clé différente — et l'adresse scellée dans la chaîne d'audit (CO art. 957a)
// devient celle que l'attaquant a choisie.
//
// `X-Forwarded-Proto` échappait entièrement à ce raisonnement. Il était lu par
// `c.GetHeader`, hors du mécanisme de confiance, aux deux seuls endroits où il
// apparaît : le drapeau `Secure` du cookie de rafraîchissement, et l'émission
// de HSTS. Le second est le plus gênant — un navigateur qui enregistre
// `Strict-Transport-Security` pour `localhost` refuse ensuite `http://` sur cet
// hôte, et l'utilisateur perd l'accès à sa comptabilité pour deux ans, ou
// jusqu'à ce qu'il sache purger cet état.
//
// # Pourquoi un intergiciel plutôt qu'un appel direct
//
// `isTrustedProxy` de gin n'est pas exporté : on ne peut pas lui demander son
// verdict. Le recalculer à chaque lecture d'en-tête voudrait dire passer la
// liste des mandataires jusque dans les gestionnaires, qui n'en ont que faire.
//
// La décision est donc prise UNE FOIS, au plus tôt dans la chaîne, et déposée
// dans le contexte. Ce qui la lit ensuite n'a rien à savoir de la
// configuration : la question n'est plus « quels mandataires sont déclarés »
// mais « ce pair est-il l'un d'eux », et elle a déjà sa réponse.

import (
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
)

const cleMandataireDeConfiance = "mandataire_de_confiance"

// ConfianceMandataire décide si le pair figure parmi les mandataires déclarés,
// et dépose le verdict dans le contexte.
//
// À enregistrer AVANT tout ce qui lit un en-tête `X-Forwarded-*`. Un contexte
// sans verdict vaut « pas de confiance » : c'est le bon défaut, et il vaut
// aussi pour les tests qui montent un routeur sans cet intergiciel.
func ConfianceMandataire(mandataires []string) gin.HandlerFunc {
	prefixes, adresses := analyserMandataires(mandataires)

	return func(c *gin.Context) {
		c.Set(cleMandataireDeConfiance, pairDeConfiance(c.RemoteIP(), prefixes, adresses))
		c.Next()
	}
}

// analyserMandataires accepte des adresses et des CIDR, comme TRUSTED_PROXIES.
//
// Une entrée illisible est ignorée plutôt que fatale : la liste est déjà
// validée au démarrage par SetTrustedProxies, qui refuse de démarrer sur une
// entrée fausse. Refuser une seconde fois ici n'ajouterait rien ; ignorer
// silencieusement ce qui a passé cette porte est cohérent, et ne peut
// qu'ÉLARGIR le refus, jamais la confiance.
func analyserMandataires(liste []string) ([]netip.Prefix, []netip.Addr) {
	var prefixes []netip.Prefix
	var adresses []netip.Addr
	for _, brut := range liste {
		e := strings.TrimSpace(brut)
		if e == "" {
			continue
		}
		if p, err := netip.ParsePrefix(e); err == nil {
			prefixes = append(prefixes, p)
			continue
		}
		if a, err := netip.ParseAddr(e); err == nil {
			adresses = append(adresses, a)
		}
	}
	return prefixes, adresses
}

func pairDeConfiance(remoteIP string, prefixes []netip.Prefix, adresses []netip.Addr) bool {
	// Aucun mandataire déclaré : c'est le défaut, et il veut dire que
	// LedgerAlps écoute en direct. Aucun en-tête ne doit alors peser sur quoi
	// que ce soit.
	if len(prefixes) == 0 && len(adresses) == 0 {
		return false
	}
	ip, err := netip.ParseAddr(remoteIP)
	if err != nil {
		return false
	}
	// Une adresse IPv4 arrivant sous forme mappée IPv6 (::ffff:10.0.0.1) doit
	// se comparer à un CIDR IPv4 comme l'adresse qu'elle est.
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	for _, a := range adresses {
		if a == ip {
			return true
		}
	}
	return false
}

// ProtocoleAnnonceHTTPS dit si la requête est arrivée en HTTPS.
//
// TLS terminé ici : la question ne se pose pas. Sinon, on ne croit
// `X-Forwarded-Proto` que d'un mandataire déclaré — sans quoi n'importe qui
// choisirait la réponse.
func ProtocoleAnnonceHTTPS(c *gin.Context) bool {
	if c.Request != nil && c.Request.TLS != nil {
		return true
	}
	if !c.GetBool(cleMandataireDeConfiance) {
		return false
	}
	return c.GetHeader("X-Forwarded-Proto") == "https"
}

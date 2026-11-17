package config

// Aucun accès par le réseau.
//
// LedgerAlps n'écoute que sur la boucle locale. Jusqu'à la 1.6.5, on pouvait le rendre joignable
// depuis un autre poste (adresse d'écoute, TLS, certificat auto-signé, mandataires, origines
// déclarées) ; ces réglages sont supprimés (décision du propriétaire, 1.6.6). Il n'y a plus de TLS :
// le trafic ne quitte pas la machine, un certificat n'apporterait qu'un avertissement du navigateur.
//
// L'adresse n'est PAS un réglage : c'est une constante. Ce qui en faisait un réglage — une variable
// d'environnement, une clé de config.json, un écran — a été retiré, pas masqué, pour qu'aucune
// valeur lue quelque part ne puisse rouvrir l'accès sans qu'un humain relise le code.

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
)

// ListenHost est la seule adresse sur laquelle le serveur écoute.
const ListenHost = "127.0.0.1"

// ListenAddr est l'adresse d'écoute du serveur : la boucle locale, sur le port configuré.
func (c *Config) ListenAddr() string {
	return net.JoinHostPort(ListenHost, c.Port)
}

// ReglagesReseauIgnores liste ce qui, dans l'environnement ou dans config.json, demandait encore un
// accès par le réseau, et que le serveur ignore désormais.
//
// Une installation qui avait ouvert l'accès (Paramètres → Sécurité et réseau, ou `HOST=0.0.0.0`) cesse
// d'être joignable à la mise à jour : sans ce relevé, personne ne saurait pourquoi. Il est écrit au
// journal du serveur au démarrage. Un réglage qui demande déjà la boucle locale (`HOST=127.0.0.1`,
// l'origine `http://localhost:8000` que l'assistant écrivait) n'est pas signalé : il n'y a rien à
// ignorer.
func ReglagesReseauIgnores() []string {
	var out []string
	vus := map[string]bool{}
	ajouter := func(s string) {
		if !vus[s] {
			vus[s] = true
			out = append(out, s)
		}
	}

	// Environnement.
	if h := os.Getenv("HOST"); !hoteLocal(h) {
		ajouter("HOST=" + h + " (environnement)")
	}
	for _, k := range []string{"TLS_CERT", "TLS_KEY", "TRUSTED_PROXIES"} {
		if strings.TrimSpace(os.Getenv(k)) != "" {
			ajouter(k + " (environnement)")
		}
	}
	if os.Getenv("ALLOW_INSECURE_HTTP") == "true" {
		ajouter("ALLOW_INSECURE_HTTP=true (environnement)")
	}
	if o := os.Getenv("ALLOWED_ORIGINS"); !originesLocales(o) {
		ajouter("ALLOWED_ORIGINS=" + o + " (environnement)")
	}

	// config.json : lu en carte générique, les clés n'existent plus dans la structure.
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		return out
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return out
	}
	chaine := func(k string) string { s, _ := m[k].(string); return s }
	if h := chaine("host"); !hoteLocal(h) {
		ajouter("host=" + h + " (config.json)")
	}
	for _, k := range []string{"tls_cert", "tls_key"} {
		if strings.TrimSpace(chaine(k)) != "" {
			ajouter(k + " (config.json)")
		}
	}
	if b, _ := m["allow_insecure_http"].(bool); b {
		ajouter("allow_insecure_http (config.json)")
	}
	if l, _ := m["trusted_proxies"].([]any); len(l) > 0 {
		ajouter("trusted_proxies (config.json)")
	}
	if o := chaine("allowed_origins"); !originesLocales(o) {
		ajouter("allowed_origins=" + o + " (config.json)")
	}
	return out
}

// hoteLocal dit si une adresse d'écoute demandée est la boucle locale — ou absente, ce qui revient au
// même : le serveur n'écoute de toute façon que là.
func hoteLocal(h string) bool {
	h = strings.Trim(strings.ToLower(strings.TrimSpace(h)), "[]")
	if h == "" || h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// originesLocales dit si chaque origine d'une liste séparée par des virgules est une origine locale.
func originesLocales(liste string) bool {
	for _, o := range strings.Split(liste, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || !hoteLocal(u.Hostname()) {
			return false
		}
	}
	return true
}

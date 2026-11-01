package main

// Où joindre le serveur que le lanceur a lancé.
//
// # Le défaut (audit croisé Gemini, HIGH-25)
//
// Le lanceur composait « http://localhost:<port> » sans lire autre chose que le port de
// `config.json`, et le commentaire de `waitForServer` le disait : « le lanceur ne parle qu'au
// serveur local, servi en clair ». Or l'application règle elle-même, depuis Paramètres → Sécurité et
// réseau, l'adresse d'écoute et le TLS — et le serveur sert en TLS dès que son hôte n'est pas la
// boucle locale (certificat auto-signé) ou qu'un certificat est configuré.
//
// Sur une installation ainsi réglée, `http://localhost:8000/health` répond 400 (« client sent an
// HTTP request to an HTTPS server », rejoué sur un serveur de bac à sable) : le lanceur tenait le
// serveur pour absent, en lançait un second (port déjà pris), attendait vingt secondes et abandonnait
// sans jamais ouvrir le navigateur.
//
// # Ce que fait ce fichier
//
// Il cherche à quelle adresse le serveur RÉPOND, parmi celles que la configuration rend plausibles —
// l'hôte précis s'il y en a un, sinon la boucle locale ; TLS d'abord si un certificat est configuré,
// clair d'abord sinon — et rend la première qui répond à /health. Il ne devine ni ne reproduit les
// règles du serveur : il demande.

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"
)

// hotesDuServeur : les noms à essayer, du plus probable au moins probable.
//
// Un hôte précis dans la configuration (une adresse du réseau local) est celui où le serveur écoute
// UNIQUEMENT : « localhost » n'y répond pas forcément.
func hotesDuServeur(cfg *config) []string {
	h := strings.TrimSpace(cfg.Host)
	switch h {
	case "", "0.0.0.0", "::", "[::]", "127.0.0.1", "::1", "[::1]", "localhost":
		return []string{"localhost"}
	}
	return []string{h, "localhost"}
}

// candidatsDuServeur : les adresses de base à essayer, sans barre finale.
func candidatsDuServeur(cfg *config) []string {
	schemas := []string{"http", "https"}
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		schemas = []string{"https", "http"}
	}
	var out []string
	for _, hote := range hotesDuServeur(cfg) {
		for _, s := range schemas {
			out = append(out, s+"://"+net.JoinHostPort(hote, cfg.Port))
		}
	}
	return out
}

// clientDeSonde : un client pour interroger /health.
//
// Il accepte un certificat qu'il ne peut pas vérifier : celui du serveur est AUTO-SIGNÉ par défaut
// (le journal du serveur le dit), et ce client ne fait qu'un GET sur /health, sans rien envoyer ni
// lire d'autre — le navigateur, lui, affichera son avertissement. Il ne sert jamais à autre chose.
func clientDeSonde() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // voir ci-dessus
		},
	}
}

// serveurJoignable rend l'adresse à laquelle le serveur répond maintenant, ou faux.
func serveurJoignable(cfg *config) (string, bool) {
	client := clientDeSonde()
	for _, base := range candidatsDuServeur(cfg) {
		if isServerRunning(base, client) {
			return base, true
		}
	}
	return "", false
}

// joindreServeur attend que le serveur réponde, à n'importe laquelle des adresses plausibles, et
// rend celle qui a répondu.
func joindreServeur(ctx context.Context, cfg *config) (string, error) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-tick.C:
			if base, ok := serveurJoignable(cfg); ok {
				return base, nil
			}
		}
	}
}

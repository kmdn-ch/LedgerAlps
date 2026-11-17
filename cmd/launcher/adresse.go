package main

// Où joindre le serveur que le lanceur a lancé.
//
// Le serveur n'écoute que sur la boucle locale, en HTTP : il n'y a plus ni TLS ni adresse d'écoute
// réglable (1.6.6), donc plus rien à chercher. Avant, ce fichier sondait plusieurs adresses (hôte
// précis, https d'abord si un certificat était configuré) parce que le serveur pouvait servir en TLS ;
// ce chemin est supprimé avec le TLS.

import (
	"context"
	"net"
	"net/http"
	"time"
)

// adresseDuServeur est l'adresse de base du serveur local, sans barre finale.
func adresseDuServeur(cfg *config) string {
	return "http://" + net.JoinHostPort("localhost", cfg.Port)
}

// serveurJoignable rend l'adresse à laquelle le serveur répond maintenant, ou faux.
func serveurJoignable(cfg *config) (string, bool) {
	base := adresseDuServeur(cfg)
	if isServerRunning(base, &http.Client{Timeout: 2 * time.Second}) {
		return base, true
	}
	return "", false
}

// joindreServeur attend que le serveur réponde et rend son adresse.
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

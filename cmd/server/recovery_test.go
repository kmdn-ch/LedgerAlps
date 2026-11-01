package main

// Le mode récupération demande la phrase qui enveloppe la clé de dix ans de
// comptabilité. Deux choses le protègent, et aucune n'existait :
//
//   - le TRANSPORT suit la même règle que le serveur normal (resolveTLS), au
//     lieu d'être en clair quoi qu'il arrive — vérifié par construction dans
//     runRecoveryServer, qui n'a plus qu'un seul chemin vers ListenAndServe ;
//   - les TENTATIVES sont bornées, ce que rien ne faisait ici alors que la
//     connexion ordinaire a son limiteur depuis longtemps.
//
// Ce fichier tient la seconde. Chaque essai coûte 64 Mio via Argon2id : sans
// borne, l'essai automatisé est gratuit pour l'attaquant et ruineux pour la
// machine.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func limiteurNeuf() *essaisRecuperation {
	return &essaisRecuperation{essais: map[string][]time.Time{}}
}

// Trois essais passent, le quatrième non.
func TestLaPhraseDeRecuperationEstBornee(t *testing.T) {
	l := limiteurNeuf()

	for i := 1; i <= essaisMax; i++ {
		if !l.autorise("192.0.2.10") {
			t.Fatalf("essai %d refusé, alors que %d sont permis", i, essaisMax)
		}
	}
	if l.autorise("192.0.2.10") {
		t.Errorf("le %de essai est passé — rien ne borne les tentatives", essaisMax+1)
	}
}

// Le verrou est par adresse : bloquer un poste ne doit pas bloquer le voisin.
//
// Le mode récupération sert justement quand la machine a changé de main ; punir
// tout le réseau parce qu'un poste s'est trompé fermerait la porte à celui qui
// détient la vraie phrase.
func TestLeVerrouNeDebordePasSurUneAutreAdresse(t *testing.T) {
	l := limiteurNeuf()

	for i := 0; i < essaisMax; i++ {
		l.autorise("192.0.2.10")
	}
	if l.autorise("192.0.2.10") {
		t.Fatal("la première adresse aurait dû être bloquée")
	}
	if !l.autorise("192.0.2.99") {
		t.Error("une autre adresse est bloquée par les essais de la première")
	}
}

// La fenêtre glisse : après une minute de calme, on peut réessayer.
//
// Un verrou définitif transformerait trois fautes de frappe en perte de la
// comptabilité — exactement ce que la phrase de récupération existe pour éviter.
func TestLaFenetreGlisse(t *testing.T) {
	l := limiteurNeuf()

	// Trois essais déjà anciens : ils ne doivent plus compter.
	vieux := time.Now().Add(-fenetreEssais - time.Second)
	l.essais["192.0.2.10"] = []time.Time{vieux, vieux, vieux}

	if !l.autorise("192.0.2.10") {
		t.Error("des essais vieux de plus d'une minute bloquent encore")
	}
}

// ─── Audit croisé Gemini, HIGH-17 : l'hôte et l'origine du mode récupération ─────────────────────

func requeteRecuperation(methode, hote, origine string) *httptest.ResponseRecorder {
	suite := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := protegerRecuperation("127.0.0.1", []string{"http://localhost:5173"}, suite)
	req := httptest.NewRequest(methode, "http://"+hote+"/", nil)
	req.Host = hote
	if origine != "" {
		req.Header.Set("Origin", origine)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// Un nom de domaine hostile qui pointe vers 127.0.0.1 (« DNS rebinding ») atteignait la page de
// récupération sans que rien ne regarde l'en-tête Host. Même règle que le serveur normal.
func TestLeModeRecuperationRefuseUnHoteInconnu(t *testing.T) {
	for _, hote := range []string{"localhost:8000", "127.0.0.1:8000", "[::1]:8000", "localhost"} {
		if w := requeteRecuperation(http.MethodGet, hote, ""); w.Code != http.StatusOK {
			t.Errorf("hôte %s refusé : %d", hote, w.Code)
		}
	}
	for _, hote := range []string{"evil.example:8000", "127.0.0.1.evil.example", "attaquant.test"} {
		if w := requeteRecuperation(http.MethodGet, hote, ""); w.Code != http.StatusMisdirectedRequest {
			t.Errorf("hôte %s : %d, attendu 421", hote, w.Code)
		}
	}
}

// Un formulaire posté depuis une AUTRE origine est refusé ; celui de la page elle-même, une requête
// sans en-tête Origin (un outil en ligne de commande) passent.
func TestLeModeRecuperationRefuseUnPostDUneAutreOrigine(t *testing.T) {
	if w := requeteRecuperation(http.MethodPost, "localhost:8000", "http://localhost:8000"); w.Code != http.StatusOK {
		t.Errorf("POST de la page elle-même : %d, attendu 200", w.Code)
	}
	if w := requeteRecuperation(http.MethodPost, "localhost:8000", ""); w.Code != http.StatusOK {
		t.Errorf("POST sans Origin : %d, attendu 200", w.Code)
	}
	for _, origine := range []string{"https://evil.example", "http://localhost:9999", "null"} {
		if w := requeteRecuperation(http.MethodPost, "localhost:8000", origine); w.Code != http.StatusForbidden {
			t.Errorf("POST depuis %s : %d, attendu 403", origine, w.Code)
		}
	}
	// Ni cache ni cadre.
	w := requeteRecuperation(http.MethodGet, "localhost:8000", "")
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("en-têtes de protection absents : %v", w.Header())
	}
}

// Exposé volontairement sur le réseau, le serveur de récupération se comporte comme le serveur normal :
// la liste exacte des noms de l'installation n'est pas connue, le contrôle d'hôte se retire.
func TestLeModeRecuperationExposeSurLeReseauNeFiltrePasLHote(t *testing.T) {
	suite := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := protegerRecuperation("0.0.0.0", nil, suite)
	req := httptest.NewRequest(http.MethodGet, "http://compta.exemple.ch/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("serveur exposé : %d, attendu 200", w.Code)
	}
}

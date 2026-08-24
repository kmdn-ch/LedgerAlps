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

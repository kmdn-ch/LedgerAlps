package frontend

// Aucun catalogue ne doit être incomplet, et aucune valeur ne doit rester en
// français dans les autres langues.
//
// # Pourquoi un test, alors que TypeScript vérifie déjà les clés
//
// Il vérifie qu'une clé EXISTE, pas qu'elle a été traduite. Copier le catalogue
// français en `de.ts` compile parfaitement et produit une interface allemande
// entièrement en français — l'erreur la plus facile à commettre pendant une
// traduction, et la plus difficile à voir en relisant un fichier de 95 lignes.
//
// Ce test tourne dans la CI existante, sans ajouter de dépendance de test au
// frontend.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reCle = regexp.MustCompile(`(?m)^\s*'([\w.]+)':\s*'((?:[^'\\]|\\.)*)'`)

func catalogue(t *testing.T, code string) map[string]string {
	t.Helper()
	chemin := filepath.Join("..", "..", "frontend", "src", "i18n", code+".ts")
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("%s : %v", chemin, err)
	}
	out := map[string]string{}
	for _, m := range reCle.FindAllStringSubmatch(string(b), -1) {
		out[m[1]] = strings.ReplaceAll(m[2], `\'`, `'`)
	}
	if len(out) == 0 {
		t.Fatalf("%s : aucune clé lue — le format du catalogue a changé", chemin)
	}
	return out
}

// Chaque catalogue porte exactement les clés du français.
func TestLesCataloguesOntLesMemesCles(t *testing.T) {
	ref := catalogue(t, "fr")
	for _, code := range []string{"de", "it", "en"} {
		autre := catalogue(t, code)
		for cle := range ref {
			if _, ok := autre[cle]; !ok {
				t.Errorf("%s : clé %q absente", code, cle)
			}
		}
		for cle := range autre {
			if _, ok := ref[cle]; !ok {
				t.Errorf("%s : clé %q en trop — elle n'existe pas en français", code, cle)
			}
		}
	}
}

// Aucune valeur n'est vide.
func TestAucuneTraductionNEstVide(t *testing.T) {
	for _, code := range []string{"fr", "de", "it", "en"} {
		for cle, val := range catalogue(t, code) {
			if strings.TrimSpace(val) == "" {
				t.Errorf("%s : %q est vide", code, cle)
			}
		}
	}
}

// LE test qui compte : rien ne doit être resté en français.
//
// Certaines valeurs sont IDENTIQUES d'une langue à l'autre en toute légitimité
// — « IBAN », « QR-IBAN », « Journal », « Saldo ». Ce sont des termes que les
// Implementation Guidelines de SIX ou l'usage laissent inchangés, et les
// signaler serait du bruit. Ils sont donc listés ici, explicitement : ajouter
// une exception oblige à y penser.
func TestRienNEstResteEnFrancais(t *testing.T) {
	// Identiques par nature : sigles, termes SIX, ou mots qui se disent
	// pareil. Chaque entrée est un choix, pas un oubli.
	memeMot := map[string]map[string]bool{
		"de": {
			"nav.journal": true, "paiement.iban": true, "paiement.qrIban": true,
			"compta.solde": true, "securite.phraseDePasse": true,
			"role.admin": true, "statut.archivee": false,
			// « Total CHF » est un en-tête de colonne : le sigle de la monnaie
			// ne se traduit pas, et « Total » est le même mot.
			"fact.colTotal": true,
		},
		"it": {
			"paiement.iban": true, "paiement.qrIban": true, "compta.solde": true,
			"securite.phraseDePasse": true, "securite.motDePasse": true,
			"nav.contacts": true,
			// « E-mail » s'écrit pareil dans les quatre langues.
			"ach.email": true,
		},
		"en": {
			"paiement.iban": true, "paiement.qrIban": true,
			"securite.phraseDePasse": true, "nav.contacts": true,
			"nav.journal": true,
			// Mots identiques en français et en anglais.
			"fact.colDate": true, "fact.colContact": true, "fact.colTotal": true,
			"compta.credit": true, "securite.motDePasse": false,
			// « document » et « e-mail » s'écrivent pareil dans les deux langues.
			"fact.unDocument": true, "fact.desDocuments": true, "ach.email": true,
		},
	}

	ref := catalogue(t, "fr")
	for _, code := range []string{"de", "it", "en"} {
		autre := catalogue(t, code)
		identiques := 0
		for cle, valFR := range ref {
			val, ok := autre[cle]
			if !ok {
				continue // signalé par l'autre test
			}
			if val != valFR {
				continue
			}
			if memeMot[code][cle] {
				continue // identique volontairement
			}
			identiques++
			t.Errorf("%s : %q vaut encore le français %q — non traduit ?", code, cle, valFR)
		}
		if identiques > 0 {
			t.Logf("%s : %d valeur(s) encore identiques au français", code, identiques)
		}
	}
}

// Les repères d'interpolation survivent à la traduction.
//
// « {taux} % — taux normal » devient « {taux} % — Normalsatz » : le repère doit
// rester, et rester le même. Un traducteur qui écrit « {Satz} » produit un
// texte où le nombre n'apparaît jamais — et personne ne le voit tant que
// l'écran n'est pas ouvert dans cette langue.
func TestLesReperesDInterpolationSontConserves(t *testing.T) {
	reRepere := regexp.MustCompile(`\{(\w+)\}`)
	ref := catalogue(t, "fr")
	for _, code := range []string{"de", "it", "en"} {
		autre := catalogue(t, code)
		for cle, valFR := range ref {
			attendus := reRepere.FindAllString(valFR, -1)
			if len(attendus) == 0 {
				continue
			}
			obtenus := reRepere.FindAllString(autre[cle], -1)
			if len(obtenus) != len(attendus) {
				t.Errorf("%s : %q porte %v, le français porte %v",
					code, cle, obtenus, attendus)
				continue
			}
			for _, a := range attendus {
				if !strings.Contains(autre[cle], a) {
					t.Errorf("%s : %q a perdu le repère %s", code, cle, a)
				}
			}
		}
	}
}

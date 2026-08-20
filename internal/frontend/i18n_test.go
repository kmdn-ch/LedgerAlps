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

// Un nombre — ou son repère d'interpolation — suivi d'un seul mot : une durée,
// pas une phrase. « 5 minutes » et « {n} minutes » s'écrivent de la même façon
// en français et en anglais, et le repère ne change rien à ce constat.
var reDuree = regexp.MustCompile(`^(\d+|\{\w+\})\s+\p{L}+$`)

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
func TestRienNEstResteEnFrancais(t *testing.T) {
	ref := catalogue(t, "fr")
	for _, code := range []string{"de", "it", "en"} {
		autre := catalogue(t, code)
		identiques := 0
		for cle, valFR := range ref {
			val, ok := autre[cle]
			if !ok {
				continue // signalé par l'autre test
			}
			if val != valFR || identiqueParNature(cle, valFR) {
				continue
			}
			identiques++
			t.Errorf("%s : %q vaut encore le français %q — non traduit ?", code, cle, valFR)
		}
		if identiques > 0 {
			t.Logf("%s : %d valeur(s) encore identiques au français", code, identiques)
		}
	}
}

// identiqueParNature dit si une valeur a de bonnes raisons d'être la même dans
// toutes les langues.
//
// # Pourquoi des catégories plutôt qu'une liste de clés
//
// La première version listait chaque exception nommément. Cela tenait à
// quatre-vingts clés et devenait ingérable à mille : chaque lot de traduction
// ajoutait cinq lignes à la liste, et une liste qu'on rallonge machinalement
// finit par être remplie sans qu'on lise ce qu'on y met — ce qui vide le test
// de son sens.
//
// Une catégorie, elle, se justifie une fois : un exemple de saisie n'est pas du
// texte à traduire, un sigle n'a pas de version française. Et les mots que le
// hasard rend identiques sont reconnus par leur VALEUR, si bien que « Date »
// vaut exception partout où il apparaît, sur cent écrans comme sur un.
func identiqueParNature(cle, valeur string) bool {
	// Les EXEMPLES de saisie — « Acme SA », « +41 24 000 00 00 », une adresse
	// lausannoise. Ce sont des illustrations de format, pas des phrases : les
	// traduire n'apprendrait rien et inventerait des entreprises.
	if strings.HasSuffix(cle, "Exemple") {
		return true
	}

	// Ce qui ne se traduit dans aucune langue : sigles, identifiants
	// normalisés, termes que les Implementation Guidelines de SIX laissent tels
	// quels.
	inchangeable := map[string]bool{
		"paiement.iban": true, "paiement.qrIban": true, "pr.bic": true,
		"securite.phraseDePasse": true, "pr.deviseEUR": true,
		"fd.qrRef": true, "cf.olicoLien": true,
	}
	if inchangeable[cle] {
		return true
	}

	// Les DURÉES : un nombre suivi de son unité, « 5 minutes », « 1 hour ».
	// Français et anglais écrivent « minutes » de la même façon ; l'allemand
	// écrit « Minuten » et sera donc contrôlé normalement. Une catégorie plutôt
	// que trois entrées : le prochain « 30 minutes » n'aura rien à rallonger.
	if reDuree.MatchString(valeur) {
		return true
	}

	// Mots que le hasard rend identiques d'une langue à l'autre.
	memeMot := map[string]bool{
		"Date": true, "Contact": true, "Contacts": true, "Description": true,
		"Description *": true, "Date *": true, "Journal": true, "Solde": true,
		"E-mail": true, "Administrateur": true, "Total CHF": true, "Mai": true,
		"Adresse": true, "NPA": true, "Novembre": true, "Maintenance": true,
		"{n} document": true, "{n} documents": true, "Crédit": true,
		"Mot de passe": true, "Archivée": true, "Contact *": true,
		"Type *": true, "File": true, "Notes": true,
		"Version": true, "Information": true, "Protections": true,
		"intact": true,
		"Total":  true, "Type": true, "Documents ({n})": true,
		"Document": true, "Action": true, "TOTAL": true,
		"Solide": true, "Acceptable": true,
		// Le français et l'italien élident tous deux devant la voyelle.
		"l’IBAN": true,
	}
	return memeMot[valeur]
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

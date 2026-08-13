package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Aucun message adressé à l'utilisateur ne doit échapper au catalogue.
//
// # Pourquoi ce test existe
//
// Le catalogue est indexé par la phrase française. Rien, dans le compilateur,
// n'oblige un gestionnaire écrit demain à y figurer : il compilera, il
// fonctionnera, et il s'affichera en français au milieu d'un écran allemand.
// C'est exactement le défaut qui a traîné six mois côté interface.
//
// Ce test relit les sources, retrouve les phrases que le serveur adresse à
// l'utilisateur, et échoue sur celles qui ne sont pas traduites. Il tient donc
// la couverture sans dépendre de la vigilance de personne.
//
// # Ce qu'il ne réclame pas
//
// Deux populations seulement :
//
//   - Les messages qui EMBALLENT une cause avec `%w`. Ils finissent au journal
//     du serveur, et le texte sert à retrouver la ligne.
//   - Ceux que diagnostic.go déclare, un par un, avec leur raison.
//
// La première version de ce test devinait « est-ce du français ? » d'après les
// accents. « identifiants incorrects » n'en a aucun : le message le plus vu du
// produit est passé à travers, et le test l'a laissé passer en silence. La
// règle est donc inversée — tout est à traduire, les exceptions se déclarent.

var (
	// Les endroits d'où part un texte vers l'utilisateur.
	//
	// Les appels `t("…")` — la fermeture locale d'un gestionnaire, la méthode
	// `inv.t()` du PDF — ont été ajoutés après coup : ils échappaient au
	// balayage, et une phrase absente du catalogue y retombe en français sans
	// que rien ne le signale. C'est exactement le trou qu'on vient de corriger
	// pour « identifiants incorrects », reproduit un cran plus loin.
	reContexte = regexp.MustCompile(
		`(?:gin\.H\{\s*)?"(?:error|message)":\s*|errors\.New\(\s*|fmt\.Errorf\(\s*|(?:^|[^A-Za-z0-9_])\.?t\(\s*`)
	// Un littéral Go, éventuellement suivi d'autres collés par des `+`.
	reSuite = regexp.MustCompile(`"(?:[^"\\]|\\.)*"(?:\s*\+\s*"(?:[^"\\]|\\.)*")*`)
	reBout  = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
)

func TestTousLesMessagesSontAuCatalogue(t *testing.T) {
	racine := filepath.Join("..", "..", "internal")
	var manquantes []string
	vues := map[string]bool{}

	err := filepath.Walk(racine, func(chemin string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(chemin, ".go") || strings.HasSuffix(chemin, "_test.go") {
			return nil
		}
		// Le paquet i18n porte le catalogue lui-même : ses littéraux SONT les
		// phrases, les relire reviendrait à se demander à soi-même.
		if strings.Contains(filepath.ToSlash(chemin), "/internal/i18n/") {
			return nil
		}
		b, err := os.ReadFile(chemin)
		if err != nil {
			return err
		}
		src := string(b)
		for _, loc := range reContexte.FindAllStringIndex(src, -1) {
			suite := reSuite.FindString(src[loc[1]:])
			if suite == "" || !strings.HasPrefix(src[loc[1]:], suite) {
				continue
			}
			phrase := décolle(suite)
			if len(phrase) < 8 || strings.Contains(phrase, "%w") {
				continue
			}
			if vues[phrase] {
				continue
			}
			vues[phrase] = true
			if !Traduit(phrase) && !EstDiagnostic(phrase) {
				manquantes = append(manquantes,
					filepath.Base(chemin)+" : "+court(phrase))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parcours des sources : %v", err)
	}

	if len(manquantes) > 0 {
		t.Errorf("%d message(s) adressés à l'utilisateur ne sont pas au catalogue "+
			"(internal/i18n/catalogue.go) :", len(manquantes))
		for _, m := range manquantes {
			t.Errorf("  %s", m)
		}
	}
}

// décolle recompose « "a" + "b" » en « ab ».
//
// Un message écrit sur quatre lignes vaut UNE chaîne à l'exécution : c'est
// celle-là qu'il faut chercher au catalogue, pas son premier morceau.
func décolle(suite string) string {
	var b strings.Builder
	for _, bout := range reBout.FindAllString(suite, -1) {
		s := bout[1 : len(bout)-1]
		s = strings.ReplaceAll(s, `\"`, `"`)
		s = strings.ReplaceAll(s, `\\`, `\`)
		s = strings.ReplaceAll(s, `\n`, "\n")
		b.WriteString(s)
	}
	return b.String()
}

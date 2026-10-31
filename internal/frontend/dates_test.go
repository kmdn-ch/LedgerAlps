package frontend

// La date du jour ne se lit pas dans `toISOString()`.
//
// # Ce que faisait le motif (audit croisé Gemini, MED-01)
//
// `new Date().toISOString().slice(0, 10)` donne la date UTC. En Suisse, entre minuit et 1 h
// (2 h l'été), c'est la veille : une écriture ou un paiement saisi à 0 h 30 portait la date de
// la veille. Pire, `new Date(an, 0, 1).toISOString().slice(0, 10)` — le 1er janvier à minuit
// LOCAL — donne TOUJOURS le 31 décembre de l'année précédente : la période par défaut des
// rapports commençait la veille et avalait l'écriture de clôture de l'année d'avant. Quatorze
// occurrences, dont trois seulement se trompaient la nuit.
//
// L'aide qui convient est `dateLocale()` (utils/index.ts). Ce test lit les sources, comme les
// autres de ce paquet : TypeScript ne voit rien, le résultat est une chaîne comme une autre.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reDateUTC = regexp.MustCompile(`toISOString\(\)\s*\.\s*(slice\(\s*0\s*,\s*(7|10)\s*\)|split\(\s*['"]T['"]\s*\))`)

// utc.go : les fichiers où la date UTC est VOULUE, parce qu'ils fabriquent la date depuis
// `Date.UTC(...)` — aucun fuseau n'y entre, c'est le calcul calendaire qui l'exige.
var autoriseUTC = map[string]bool{
	filepath.Join("utils", "exercices.ts"): true,
}

func TestAucuneDateDuJourNeSeLitDansToISOString(t *testing.T) {
	racine := filepath.Join("..", "..", "frontend", "src")
	lus := 0
	_ = filepath.Walk(racine, func(chemin string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !(strings.HasSuffix(chemin, ".ts") || strings.HasSuffix(chemin, ".tsx")) {
			return nil
		}
		rel, _ := filepath.Rel(racine, chemin)
		if autoriseUTC[rel] {
			return nil
		}
		b, err := os.ReadFile(chemin)
		if err != nil {
			t.Errorf("%s : %v", chemin, err)
			return nil
		}
		lus++
		for i, ligne := range strings.Split(string(b), "\n") {
			if reDateUTC.MatchString(ligne) {
				t.Errorf("%s:%d : la date vient de toISOString() — c'est la date UTC, la veille entre minuit et 2 h en Suisse. "+
					"Utilisez dateLocale() (utils/index.ts) : %s", rel, i+1, strings.TrimSpace(ligne))
			}
		}
		return nil
	})
	if lus == 0 {
		t.Fatal("aucun fichier lu — le test ne vérifie rien")
	}
}

func TestLeMotifUTCSeReconnait(t *testing.T) {
	for _, ligne := range []string{
		"new Date().toISOString().slice(0, 10)",
		"new Date(a, 0, 1).toISOString().slice(0,10)",
		"d.toISOString().split('T')[0]",
		"new Date().toISOString().slice(0, 7)",
	} {
		if !reDateUTC.MatchString(ligne) {
			t.Errorf("motif non reconnu : %s", ligne)
		}
	}
	if reDateUTC.MatchString("dateLocale()") || reDateUTC.MatchString("d.toISOString()") {
		t.Error("le motif accuse un code correct")
	}
}

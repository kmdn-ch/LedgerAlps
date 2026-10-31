package frontend

// Un garde de navigation sans boîte gèle l'interface.
//
// # Ce que faisait le défaut (audit croisé Gemini, CRIT-18)
//
// `useUnsavedGuard(dirty)` BLOQUE les changements de page tant qu'une saisie est en
// cours, puis rend l'état du blocage pour que la page affiche sa confirmation. Les
// écrans Journal et Achats appelaient le hook sans lire son retour : dès qu'un
// caractère était tapé, chaque lien du menu devenait sans effet — l'adresse ne
// changeait pas, aucune boîte n'apparaissait — jusqu'au rechargement de la page,
// c'est-à-dire jusqu'à la perte de la saisie que le garde devait protéger.
//
// TypeScript ne pouvait rien dire : un retour ignoré est valide. Ce test lit donc
// les sources, comme les autres tests de ce paquet : chaque appel doit être affecté,
// et les trois sorties du garde (`blocked`, `confirmLeave`, `cancelLeave`) doivent
// être employées dans la même page.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	reAppelGarde   = regexp.MustCompile(`useUnsavedGuard\(`)
	reGardeAffecte = regexp.MustCompile(`(?:const|let)\s+(\w+)\s*=\s*useUnsavedGuard\(`)
)

// defautsDeGarde rend ce qui cloche dans une page : un appel dont le retour est
// ignoré, ou une sortie du garde jamais utilisée.
func defautsDeGarde(src string) []string {
	var defauts []string
	appels := len(reAppelGarde.FindAllString(src, -1))
	if appels == 0 {
		return nil
	}
	affectes := reGardeAffecte.FindAllStringSubmatch(src, -1)
	if len(affectes) != appels {
		defauts = append(defauts, "le retour de useUnsavedGuard est ignoré : la navigation est bloquée sans qu'aucune boîte ne le dise")
	}
	for _, m := range affectes {
		for _, sortie := range []string{"blocked", "confirmLeave", "cancelLeave"} {
			if !strings.Contains(src, m[1]+"."+sortie) {
				defauts = append(defauts, "`"+m[1]+"."+sortie+"` n'est jamais employé : la boîte de confirmation est incomplète")
			}
		}
	}
	return defauts
}

func TestChaqueGardeDeNavigationAfficheSaBoite(t *testing.T) {
	var fichiers []string
	for _, dossier := range []string{"pages", "components"} {
		_ = filepath.Walk(filepath.Join("..", "..", "frontend", "src", dossier), func(chemin string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(chemin, ".tsx") {
				fichiers = append(fichiers, chemin)
			}
			return nil
		})
	}
	if len(fichiers) == 0 {
		t.Fatal("aucune page lue — le test ne peut rien vérifier")
	}
	pagesGardees := 0
	for _, f := range fichiers {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s : %v", f, err)
		}
		if reAppelGarde.Match(b) {
			pagesGardees++
		}
		for _, d := range defautsDeGarde(string(b)) {
			t.Errorf("%s : %s", f, d)
		}
	}
	if pagesGardees == 0 {
		t.Fatal("aucune page n'emploie useUnsavedGuard — le test ne vérifie plus rien, a-t-il changé de nom ?")
	}
}

// Le contrôle doit pouvoir ÉCHOUER : sans cela, un test vert ne prouve rien.
func TestLeControleDeGardeVoitLeDefaut(t *testing.T) {
	ignore := "  const dirty = x\n  useUnsavedGuard(dirty)\n"
	if len(defautsDeGarde(ignore)) == 0 {
		t.Error("un retour ignoré n'est pas signalé")
	}
	incomplet := "  const guard = useUnsavedGuard(dirty)\n  <ConfirmDialog open={guard.blocked} />\n"
	if len(defautsDeGarde(incomplet)) == 0 {
		t.Error("une boîte sans « quitter » ni « rester » n'est pas signalée")
	}
	complet := "  const guard = useUnsavedGuard(dirty)\n  <C open={guard.blocked} onConfirm={guard.confirmLeave} onCancel={guard.cancelLeave} />\n"
	if d := defautsDeGarde(complet); len(d) != 0 {
		t.Errorf("une page correcte est signalée : %v", d)
	}
}

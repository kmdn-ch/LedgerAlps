package frontend

// Toute action d'audit déclarée doit être ÉCRITE quelque part.
//
// # Le défaut que ce test ferme
//
// Le second audit a compté quinze constantes d'action déclarées, dont ONZE
// n'étaient écrites nulle part. Le journal d'audit annonçait donc une couverture
// qu'il n'avait pas : on le consultait en croyant qu'il disait tout, et
// l'absence d'une ligne se lisait « cela n'a pas eu lieu » alors qu'elle voulait
// dire « cela n'a jamais été écrit ».
//
// Pire, `internal/api/handlers/audit_trace.go` portait en tête une phrase AU
// PASSÉ — « les constantes … étaient déclarées et jamais appelées » — qui
// donnait la faute pour réparée. Elle nommait trois constantes qui, au moment où
// elle a été écrite, n'étaient toujours pas appelées : l'unique occurrence de
// `ActionContactUpdated` dans tout le dépôt était ce commentaire.
//
// Un commentaire faux coûte plus cher qu'un commentaire absent. Ce test-ci ne
// se relit pas : il échoue.
//
// # Ce qu'il ne vérifie pas
//
// Qu'une action soit écrite au BON endroit, ni que l'état transmis soit juste.
// Il vérifie qu'elle est écrite. C'est la propriété qui manquait.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Déclaration d'une constante d'action : `ActionQuelqueChose = "quelque_chose"`.
var reDeclarationAction = regexp.MustCompile(
	`(?m)^\s*(Action[A-Za-z0-9]+)\s*=\s*"[^"]*"`)

// Fichiers qui DÉCLARENT les actions. Leurs lignes de déclaration ne comptent
// évidemment pas comme des usages.
var fichiersDeclarants = []string{
	filepath.Join("..", "api", "handlers", "audit_trace.go"),
	filepath.Join("..", "services", "accounting", "document_audit.go"),
}

func TestChaqueActionDAuditEstEcriteQuelquePart(t *testing.T) {
	declarees := map[string]string{} // nom -> fichier de déclaration
	for _, f := range fichiersDeclarants {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s : %v", f, err)
		}
		for _, m := range reDeclarationAction.FindAllStringSubmatch(string(b), -1) {
			declarees[m[1]] = f
		}
	}
	if len(declarees) == 0 {
		t.Fatal("aucune constante d'action lue — la forme des déclarations a changé, " +
			"et ce test ne protège plus rien")
	}

	source := sourceDuProduit(t)

	var mortes []string
	for nom, fichierDeclarant := range declarees {
		if !estEcrite(nom, source, fichierDeclarant) {
			mortes = append(mortes, nom)
		}
	}

	if len(mortes) > 0 {
		t.Errorf("ces actions d'audit sont déclarées mais ne sont écrites nulle part :\n  %s\n\n"+
			"Un journal à trous est pire qu'un journal absent : on le consulte en croyant "+
			"qu'il dit tout, et l'absence d'une ligne se lit « cela n'a pas eu lieu » alors "+
			"qu'elle veut dire « cela n'a jamais été écrit ».\n"+
			"Câblez chaque action sur son point d'appel, ou retirez la constante.",
			strings.Join(mortes, "\n  "))
	}
}

// estEcrite dit si l'action apparaît ailleurs que dans sa propre déclaration.
func estEcrite(nom string, source map[string]string, fichierDeclarant string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(nom) + `\b`)
	for chemin, contenu := range source {
		for _, ligne := range strings.Split(contenu, "\n") {
			if !re.MatchString(ligne) {
				continue
			}
			nue := strings.TrimSpace(ligne)
			// La ligne de déclaration elle-même, dans son propre fichier.
			if sameFile(chemin, fichierDeclarant) &&
				regexp.MustCompile(`^`+regexp.QuoteMeta(nom)+`\s*=`).MatchString(nue) {
				continue
			}
			// Un commentaire n'écrit rien. C'est précisément l'illusion que ce
			// test existe pour dissiper.
			if strings.HasPrefix(nue, "//") {
				continue
			}
			return true
		}
	}
	return false
}

func sameFile(a, b string) bool {
	return filepath.Base(a) == filepath.Base(b)
}

// sourceDuProduit lit les .go de internal/ et cmd/, hors tests.
func sourceDuProduit(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, racine := range []string{filepath.Join("..", ".."), ""} {
		if racine == "" {
			continue
		}
		for _, sousDossier := range []string{"internal", "cmd"} {
			base := filepath.Join(racine, sousDossier)
			err := filepath.Walk(base, func(chemin string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(chemin, ".go") || strings.HasSuffix(chemin, "_test.go") {
					return nil
				}
				b, err := os.ReadFile(chemin)
				if err != nil {
					return nil
				}
				out[chemin] = string(b)
				return nil
			})
			if err != nil {
				t.Fatalf("parcours de %s : %v", base, err)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("aucun fichier source lu — l'arborescence a changé")
	}
	return out
}

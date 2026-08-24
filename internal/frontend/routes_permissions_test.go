package frontend

// Toute route d'écriture du serveur doit DÉCLARER sa permission.
//
// # Pourquoi ce test, alors qu'un garde global existe déjà
//
// `DenyWritesWithoutPermission` refuse toute méthode d'écriture à un rôle en
// lecture seule, quelle que soit la route — y compris celles qui n'existent pas
// encore. C'est une bonne seconde barrière, et elle a rendu inoffensives les
// quatorze routes qui ne déclaraient rien.
//
// Mais elle ne connaît que le binaire lecteur / non-lecteur. Elle ne distingue
// pas l'administrateur du comptable, et elle ne le distinguera jamais : ce
// n'est pas son rôle. Le jour où `PermManage` est retirée au comptable, ou
// qu'un quatrième rôle apparaît, toute route sans déclaration s'ouvre en
// silence — et rien ne le signale, puisque rien n'a changé dans ces fichiers.
//
// L'écart se voyait déjà : `PUT /settings/company` exigeait `PermManage`
// pendant que `POST /settings/logo`, qui écrit dans la MÊME ligne, n'exigeait
// rien. Deux portes vers une seule donnée, deux régimes.
//
// Ce test lit la table des routes et échoue sur toute écriture non déclarée.
//
// # Ce qu'il ne fait pas
//
// Il ne juge pas si la permission choisie est la BONNE — cela demande de savoir
// ce que fait le gestionnaire. Il vérifie qu'une décision a été prise, ce qui
// est précisément ce qui manquait.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Une route montée sur le groupe protégé, avec sa méthode.
var reRouteEcriture = regexp.MustCompile(
	`(?m)^\s*api\.(POST|PUT|PATCH|DELETE)\(\s*"([^"]+)"`)

func TestChaqueRouteDEcritureDeclareSaPermission(t *testing.T) {
	chemin := filepath.Join("..", "..", "cmd", "server", "main.go")
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("%s : %v", chemin, err)
	}
	source := string(b)

	correspondances := reRouteEcriture.FindAllStringSubmatchIndex(source, -1)
	if len(correspondances) == 0 {
		t.Fatal("aucune route d'écriture lue — la forme de la table des routes a changé, " +
			"et ce test ne protège plus rien")
	}

	var manquantes []string
	for _, m := range correspondances {
		methode := source[m[2]:m[3]]
		route := source[m[4]:m[5]]

		// La déclaration peut vivre sur la même ligne ou sur la suivante : une
		// route à trois filtres passe souvent à la ligne. On regarde donc
		// jusqu'à la parenthèse fermante de l'appel.
		fin := strings.IndexByte(source[m[0]:], '\n')
		bloc := source[m[0]:]
		// Deux lignes suffisent : au-delà, c'est une autre route.
		for i := 0; i < 2 && fin > 0 && m[0]+fin+1 < len(source); i++ {
			suite := strings.IndexByte(source[m[0]+fin+1:], '\n')
			if suite < 0 {
				break
			}
			fin += suite + 1
		}
		if fin > 0 {
			bloc = source[m[0] : m[0]+fin]
		}

		if !strings.Contains(bloc, "authorizer.Require(") {
			manquantes = append(manquantes, methode+" "+route)
		}
	}

	if len(manquantes) > 0 {
		t.Errorf("ces routes d'écriture ne déclarent aucune permission :\n  %s\n\n"+
			"Le garde global les protège du rôle en lecture seule, mais il ne distingue "+
			"pas l'administrateur du comptable. Sans déclaration, elles s'ouvriront en "+
			"silence le jour où les droits d'un rôle changent.\n"+
			"Ajoutez authorizer.Require(authz.Perm…) sur chacune.",
			strings.Join(manquantes, "\n  "))
	}
}

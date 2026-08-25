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

// Une route d'ecriture, sur quelque groupe qu'elle soit montee.
//
// Le motif ne se limite plus a `api.` : une route posee sur `v1.` echappe a
// TOUTE la pile de filtres -- RequireAuth, DenyWritesWithoutPermission,
// RequirePasswordChanged, RequireMFAEnrolled -- et c'est exactement le trou
// qu'etait POST /auth/register. Un test qui ne regardait que `api.` ne pouvait
// pas le voir.
var reRouteEcriture = regexp.MustCompile(
	`(?m)^\s*(?:api|v1|r)\.(POST|PUT|PATCH|DELETE)\(\s*"([^"]+)"`)

// horsDuGroupe nomme les routes qui vivent DELIBEREMENT hors du groupe protege.
//
// Ce sont celles qui doivent rester joignables avant l'autorisation complete :
// se connecter, rafraichir son jeton, changer un mot de passe expire, inscrire
// ou presenter son second facteur, initialiser l'installation. Les y placer est
// une DECISION ; en trouver une ici qui n'y figure pas est un accident. La
// liste se relit ; un oubli ne se relit pas.
var horsDuGroupe = map[string]bool{
	"POST /auth/login":           true,
	"POST /auth/refresh":         true,
	"POST /auth/logout":          true,
	"POST /auth/change-password": true,
	"POST /auth/mfa/verify":      true,
	"POST /auth/mfa/setup":       true,
	"POST /auth/mfa/confirm":     true,
	"DELETE /auth/mfa":           true,
	"DELETE /auth/devices":       true,
	"POST /auth/bootstrap":       true,
}

// blocDeLAppel rend le texte d'un appel, de sa premiere parenthese a celle qui
// la referme, en ignorant les parentheses des chaines litterales.
//
// La fenetre de trois lignes qu'employait ce test empruntait le
// `authorizer.Require(` de la route SUIVANTE : dans une table ou les routes
// sont enregistrees en blocs serres -- c'est le cas de main.go --, une omission
// avait toutes les chances d'etre masquee par sa voisine. Verifie : une route
// non declaree suivie d'une route declaree passait le test.
func blocDeLAppel(s string) string {
	debut := strings.IndexByte(s, '(')
	if debut < 0 {
		return s
	}
	profondeur, dansChaine := 0, false
	for i := debut; i < len(s); i++ {
		switch s[i] {
		case '"':
			if i == 0 || s[i-1] != '\\' {
				dansChaine = !dansChaine
			}
		case '(':
			if !dansChaine {
				profondeur++
			}
		case ')':
			if !dansChaine {
				profondeur--
				if profondeur == 0 {
					return s[:i+1]
				}
			}
		}
	}
	return s
}

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

		if horsDuGroupe[methode+" "+route] {
			continue
		}

		// Le bloc s'arrête à la parenthèse fermante de CET appel, pas au bout
		// d'un nombre fixe de lignes.
		bloc := blocDeLAppel(source[m[0]:])
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

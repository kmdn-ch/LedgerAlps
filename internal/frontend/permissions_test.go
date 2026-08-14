package frontend

// L'écran ne doit offrir aucune écriture à un compte en lecture seule — et cela
// doit être VÉRIFIÉ, pas relu.
//
// # Pourquoi ce test existe
//
// La première tentative avait recensé les écrans par leurs appels à
// `useMutation`. Le tableau de bord portait « Nouvelle facture » sous forme de
// simple lien : aucune mutation dans le fichier, donc absent du recensement,
// donc laissé ouvert. Un compte en lecture seule créait une facture depuis la
// page d'accueil.
//
// Chercher les commandes une par une revient à refaire la recherche à chaque
// écran ajouté, avec la même chance de se tromper. Ce test parcourt les sources
// et échoue à la place de l'utilisateur.
//
// # Ce qu'il vérifie, et ce qu'il ne peut pas vérifier
//
// Il vérifie deux choses mécaniques : que les routes qui n'existent QUE pour
// écrire sont enfermées dans `RequireWrite`, et que tout fichier menant vers
// l'une d'elles consulte les permissions. Ce sont exactement les deux failles
// rencontrées.
//
// Il ne prouve pas qu'un bouton est correctement placé dans son `{peutEcrire &&
// …}` — cela demanderait de comprendre le JSX. La barrière qui tient vraiment
// reste le serveur (`authz.DenyWritesWithoutPermission`) ; ceci empêche
// l'écran de promettre ce que le serveur refusera.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Les adresses qui n'ont aucune raison d'être atteintes sans droit d'écriture.
var routesDEcriture = []string{
	"invoices/new",
	"invoices/:invoiceId/edit",
}

func lire(t *testing.T, chemin string) string {
	t.Helper()
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("%s : %v", chemin, err)
	}
	return string(b)
}

// Les deux routes d'écriture pure passent par RequireWrite.
//
// C'est la barrière qui tient quel que soit le chemin emprunté pour y arriver :
// bouton oublié, lien collé dans la barre d'adresse, favori, bouton « retour ».
func TestLesRoutesDEcritureSontGardees(t *testing.T) {
	src := lire(t, filepath.Join("..", "..", "frontend", "src", "router.tsx"))

	if !strings.Contains(src, "function RequireWrite") {
		t.Fatal("router.tsx ne définit plus RequireWrite — la garde de route a disparu")
	}
	for _, route := range routesDEcriture {
		i := strings.Index(src, "'"+route+"'")
		if i < 0 {
			continue // la route a été renommée ou retirée : rien à garder
		}
		fin := strings.Index(src[i:], "\n")
		if fin < 0 {
			fin = len(src) - i
		}
		ligne := src[i : i+fin]
		if !strings.Contains(ligne, "RequireWrite") {
			t.Errorf("la route %q n'est pas enfermée dans RequireWrite :\n  %s", route, ligne)
		}
	}
}

// Tout écran qui mène vers une route d'écriture consulte les permissions.
//
// Le défaut réel : DashboardPage pointait vers `/invoices/new` sans jamais
// demander le rôle. Le fichier ne contenait aucune mutation, donc aucune revue
// ne l'avait regardé.
func TestUnEcranQuiMeneAUneEcritureConsulteLesDroits(t *testing.T) {
	racine := filepath.Join("..", "..", "frontend", "src")

	err := filepath.Walk(racine, func(chemin string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(chemin, ".tsx") {
			return err
		}
		// Le routeur porte la garde lui-même ; les écrans d'écriture pure sont
		// derrière elle et n'ont pas à se re-tester.
		base := filepath.Base(chemin)
		if base == "router.tsx" || base == "NewInvoicePage.tsx" || base == "EditInvoicePage.tsx" {
			return nil
		}

		src := string(mustRead(chemin))
		mene := strings.Contains(src, "/invoices/new") ||
			strings.Contains(src, "}/edit`") ||
			strings.Contains(src, "/edit\"")
		if !mene {
			return nil
		}
		if !strings.Contains(src, "useCanWrite") {
			t.Errorf("%s mène vers une route d'écriture sans consulter useCanWrite — "+
				"un compte en lecture seule y accéderait", chemin)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Le miroir des permissions existe et nomme la frontière du serveur.
//
// Si quelqu'un supprime ce fichier ou renomme le rôle, les gardes deviennent
// silencieusement inopérantes — un `undefined` est faux, donc tout paraîtrait
// verrouillé, jusqu'à ce qu'on « corrige » en retirant la condition.
func TestLeMiroirDesPermissionsExiste(t *testing.T) {
	src := lire(t, filepath.Join("..", "..", "frontend", "src", "hooks", "usePermissions.ts"))
	for _, attendu := range []string{"useCanWrite", "'admin'", "'accountant'"} {
		if !strings.Contains(src, attendu) {
			t.Errorf("usePermissions.ts ne contient plus %q — le miroir du rôle serveur est cassé", attendu)
		}
	}
}

func mustRead(chemin string) []byte {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil
	}
	return b
}

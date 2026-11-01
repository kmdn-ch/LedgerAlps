package frontend

// Audit croisé Gemini, série 4 : l'interface (HIGH-20, MED-15, MED-16).
//
// TypeScript ne peut rien dire de ces défauts : un hook qu'on n'appelle pas, un Blob qu'on ne lit
// pas, une chaîne française en dur sont tous valides. Ces tests lisent donc les sources, comme les
// autres de ce paquet.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func source(t *testing.T, chemin ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "frontend", "src"}, chemin...)...))
	if err != nil {
		t.Fatalf("%v", err)
	}
	return string(b)
}

func fichiersTSX(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dossier := range []string{"pages", "components"} {
		_ = filepath.Walk(filepath.Join("..", "..", "frontend", "src", dossier), func(chemin string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(chemin, ".tsx") {
				out = append(out, chemin)
			}
			return nil
		})
	}
	if len(out) == 0 {
		t.Fatal("aucun écran lu — le test ne peut rien vérifier")
	}
	return out
}

// HIGH-20. Un écran hors de la coquille protégée (`RequireAuth`) n'a pas sa restauration de session :
// après un F5, le jeton d'accès (en mémoire) est perdu et chaque appel part sans jeton. Chacun doit
// appeler le hook partagé, et renvoyer vers la connexion quand il n'y a plus de session.
func TestLesEcransHorsDeLaCoquilleRestaurentLaSession(t *testing.T) {
	router := source(t, "router.tsx")
	if !strings.Contains(router, "useRestoreSession()") {
		t.Error("RequireAuth n'appelle plus useRestoreSession")
	}
	// Les routes déclarées HORS de la coquille : celles qui précèdent `path: '/'`.
	reHors := regexp.MustCompile(`\{\s*path:\s*'(/[a-z-]+)',\s*element:\s*<(\w+)\s*/>\s*\}`)
	avantCoquille := router[:strings.Index(router, "path: '/',")]
	vus := 0
	for _, m := range reHors.FindAllStringSubmatch(avantCoquille, -1) {
		page := m[2]
		if page == "LoginPage" {
			continue // la connexion n'a pas de session à restaurer
		}
		vus++
		src := source(t, "pages", page+".tsx")
		if !strings.Contains(src, "useRestoreSession()") {
			t.Errorf("%s (%s) est hors de la coquille protégée et n'appelle pas useRestoreSession : après un F5 chaque appel part sans jeton", page, m[1])
		}
		if !strings.Contains(src, `<Navigate to="/login"`) {
			t.Errorf("%s ne renvoie pas vers la connexion quand la session est perdue", page)
		}
		if !strings.Contains(src, "authApi.logout()") {
			t.Errorf("%s n'offre aucune sortie (se déconnecter) : l'écran est une impasse", page)
		}
	}
	if vus < 2 {
		t.Fatalf("%d écran(s) hors coquille reconnus, attendu au moins 2 (changement de mot de passe, second facteur) — la forme du routeur a changé", vus)
	}
}

// MED-15. En mode `blob`, axios rend aussi le corps d'une ERREUR en Blob. Sans relecture,
// `refusalMessage` ne trouve ni `error` ni `reason` et affiche son texte de repli : le refus d'un
// ordre de paiement, qui nomme la facture fautive, était perdu. Et l'adresse d'un téléchargement ne
// se révoque pas aussitôt (Firefox, Safari).
func TestLesRefusDesTelechargementsSontLusEtLAdresseNEstPasRevoqueeAussitot(t *testing.T) {
	client := source(t, "api", "client.ts")
	if !strings.Contains(client, "instanceof Blob") || !strings.Contains(client, "lireRefusBlob(error)") {
		t.Error("l'intercepteur d'erreurs ne relit plus le corps d'un refus rendu en Blob")
	}
	// Le paiement fournisseur demande `responseType: 'text'` : le refus y arrive en TEXTE, pas en Blob.
	if !strings.Contains(client, "typeof rep.data === 'string'") {
		t.Error("l'intercepteur d'erreurs ne relit plus le corps d'un refus rendu en texte (ordre de paiement)")
	}
	i := strings.Index(client, "export function downloadBlob")
	if i < 0 {
		t.Fatal("downloadBlob introuvable")
	}
	corps := client[i:]
	if fin := strings.Index(corps, "\n}\n"); fin > 0 {
		corps = corps[:fin]
	}
	if !strings.Contains(corps, "setTimeout(") || !strings.Contains(corps, "revokeObjectURL") {
		t.Error("downloadBlob ne révoque plus l'adresse, ou la révoque sans délai")
	}
	// La révocation ne doit être appelée que DANS le setTimeout.
	if sync := regexp.MustCompile(`(?m)^\s*URL\.revokeObjectURL\(`).MatchString(corps); sync {
		t.Error("downloadBlob révoque l'adresse aussitôt après le clic : le téléchargement est coupé sous Firefox et Safari")
	}
	vite := source(t, "..", "vite.config.ts")
	if !strings.Contains(vite, "'/health'") {
		t.Error("le serveur de développement ne relaie pas /health")
	}
}

// MED-16. Aucun texte français en dur dans un écran : il sortirait tel quel au milieu d'une
// interface allemande, italienne ou anglaise. Les replis d'erreur (`refusalMessage(e, "…")`) en
// étaient le gros ; `fr-CH` figé en était un autre.
func TestAucunTexteFrancaisEnDurDansLesEcrans(t *testing.T) {
	reRepli := regexp.MustCompile("refusalMessage\\([^,()]+,\\s*[\"'`]")
	reErrorBanner := regexp.MustCompile(`<ErrorBanner message="[^"]`)
	reFrCH := regexp.MustCompile(`['"]fr-CH['"]`)
	for _, f := range fichiersTSX(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for n, ligne := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(ligne)
			if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "*") || strings.HasPrefix(l, "{/*") || strings.HasPrefix(l, "/*") {
				continue
			}
			switch {
			case reRepli.MatchString(ligne):
				t.Errorf("%s:%d : texte de repli en dur dans refusalMessage — une clé du catalogue (t('…'))", f, n+1)
			case reErrorBanner.MatchString(ligne):
				t.Errorf("%s:%d : <ErrorBanner message=\"…\"> en dur — une clé du catalogue", f, n+1)
			case reFrCH.MatchString(ligne):
				t.Errorf("%s:%d : 'fr-CH' figé — localeIntl(langue)", f, n+1)
			}
		}
	}
}

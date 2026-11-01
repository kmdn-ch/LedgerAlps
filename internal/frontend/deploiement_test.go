package frontend

// Audit croisé Gemini, série 4 : déploiement et CI (HIGH-25, HIGH-26, MED-17).
//
// Ces fichiers (PowerShell, NSIS, shell, YAML) ne se lancent pas dans la CI Linux : leur
// comportement réel se vérifie à la main et sur la répétition de publication. Ce que la CI peut
// tenir, c'est que les garde-fous ajoutés ne disparaissent pas sans que quelqu'un le décide.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func racine(t *testing.T, chemin ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, chemin...)...))
	if err != nil {
		t.Fatalf("%v", err)
	}
	return string(b)
}

// HIGH-26. Le frontend n'était compilé qu'à la répétition de publication : une erreur de type
// traversait chaque poussée et chaque PR.
func TestLaCIDeChaquePousseeCompileLeFrontend(t *testing.T) {
	test := racine(t, ".github", "workflows", "test.yml")
	i := strings.Index(test, "\n  frontend:")
	if i < 0 {
		t.Fatal("test.yml n'a plus de job frontend : une erreur TypeScript ne serait vue qu'à la publication")
	}
	job := test[i:]
	if fin := regexp.MustCompile(`\n  [a-z-]+:\n`).FindStringIndex(job[len("\n  frontend:"):]); fin != nil {
		job = job[:len("\n  frontend:")+fin[0]]
	}
	for _, attendu := range []string{"npm ci", "npm run typecheck", "npm run build"} {
		if !strings.Contains(job, attendu) {
			t.Errorf("le job frontend ne lance plus %q", attendu)
		}
	}
	// Épinglé par empreinte, comme toutes les actions du dépôt.
	if !regexp.MustCompile(`uses:\s*actions/setup-node@[0-9a-f]{40}`).MatchString(job) {
		t.Error("setup-node n'est pas épinglé par une empreinte de 40 caractères")
	}
}

// HIGH-26. Relancer le job qui publie l'empreinte de l'installeur en ajoutait une seconde ligne.
func TestLEmpreinteDeLInstalleurNEstAjouteeQuUneFois(t *testing.T) {
	r := racine(t, ".github", "workflows", "release.yml")
	i := strings.Index(r, "Publish the installer checksum")
	if i < 0 {
		t.Fatal("l'étape d'empreinte de l'installeur a disparu")
	}
	etape := r[i:]
	if !strings.Contains(etape, `grep -v -F " ${installer}"`) {
		t.Error("l'étape n'écarte plus la ligne existante avant d'ajouter la sienne : relancer le job la double")
	}
	if strings.Index(etape, "grep -v") > strings.Index(etape, "sha256sum") {
		t.Error("la ligne existante est retirée APRÈS l'ajout")
	}
}

// HIGH-25. Un dossier de données préexistant n'est accepté que s'il est digne de confiance, et le
// service est arrêté avant que ses binaires soient remplacés.
func TestLInstallationWindowsGardeSesGardeFous(t *testing.T) {
	ps := racine(t, "scripts", "install.ps1")
	for _, attendu := range []string{
		"function Assert-DossierDeDonnees",
		"ReparsePoint",
		"Assert-DossierDeDonnees -Chemin $DataDir",
	} {
		if !strings.Contains(ps, attendu) {
			t.Errorf("install.ps1 ne contient plus %q", attendu)
		}
	}
	// L'arrêt du service précède la copie des binaires.
	stop := strings.Index(ps, `Stop-Service -Name "LedgerAlps"`)
	copie := strings.Index(ps, `Copy-Item $serverExe.FullName`)
	if stop < 0 || copie < 0 || stop > copie {
		t.Errorf("le service n'est pas arrêté AVANT la copie des binaires (arrêt %d, copie %d)", stop, copie)
	}
	// Le contrôle du dossier précède sa création.
	if strings.Index(ps, "Assert-DossierDeDonnees -Chemin $DataDir") > strings.Index(ps, "Install-Binaries -Tag") {
		t.Error("le dossier de données est contrôlé APRÈS le début de l'installation")
	}
}

// HIGH-25. « Données supprimées » ne s'annonce que si le dossier a réellement disparu.
func TestLeDesinstalleurNeMentPasSurLaSuppression(t *testing.T) {
	nsi := racine(t, "infrastructure", "windows", "installer.nsi")
	i := strings.Index(nsi, `RMDir /r "$APPDATA\LedgerAlps"`)
	if i < 0 {
		t.Fatal("la suppression du dossier de données a disparu de l'installeur")
	}
	suite := nsi[i : i+700]
	if !strings.Contains(suite, `IfFileExists "$APPDATA\LedgerAlps\*.*" lbl_data_partial`) {
		t.Error("l'installeur annonce la suppression sans vérifier qu'elle a eu lieu")
	}
	if strings.Index(suite, "DataDeleted") < strings.Index(suite, "IfFileExists") {
		t.Error("« Données supprimées » est annoncé avant la vérification")
	}
	for _, langue := range []string{"ENGLISH", "FRENCH", "GERMAN", "ITALIAN"} {
		if !strings.Contains(nsi, "LangString DataNotDeleted ${LANG_"+langue+"}") {
			t.Errorf("le message « suppression partielle » manque en %s", langue)
		}
	}
}

// MED-17. Les téléchargements de install.sh : HTTPS seulement, TLS 1.2 au moins.
func TestInstallShNeTelechargeQuEnHTTPSTLS12(t *testing.T) {
	sh := racine(t, "scripts", "install.sh")
	appelCurl := regexp.MustCompile(`(^|\$\()curl\s`)
	appels := 0
	for _, l := range strings.Split(sh, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") || !appelCurl.MatchString(l) {
			continue
		}
		appels++
		if !strings.Contains(l, "--proto '=https'") || !strings.Contains(l, "--tlsv1.2") {
			t.Errorf("appel curl sans --proto '=https' --tlsv1.2 : %s", l)
		}
	}
	if appels < 3 {
		t.Errorf("%d appel(s) curl reconnu(s), attendu au moins 3 — la forme du script a changé", appels)
	}
}

// HIGH-26. La release naît en brouillon et n'est publiée qu'APRÈS son installeur et l'empreinte de
// celui-ci. Elle était publiée d'emblée : pendant la construction de l'installeur, et pour toujours si
// ce job échouait, la page « Latest » proposait une version sans son téléchargement recommandé sous
// Windows. Ce test ne remplace pas une publication réelle (un brouillon ne se répète pas à blanc) : il
// garde que l'ordre ne s'inverse pas sans que quelqu'un le décide.
func TestLaReleaseNEstPublieeQuApresSonInstalleur(t *testing.T) {
	if !regexp.MustCompile(`(?m)^  draft: true\s*$`).MatchString(racine(t, ".goreleaser.yaml")) {
		t.Error(".goreleaser.yaml ne crée plus la release en brouillon : elle serait visible avant son installeur")
	}
	r := racine(t, ".github", "workflows", "release.yml")
	publier := strings.Index(r, `gh release edit "$VERSION" --draft=false`)
	if publier < 0 {
		t.Fatal("release.yml ne publie plus la release : elle resterait un brouillon pour toujours")
	}
	for _, avant := range []string{
		`gh release upload "$VERSION" "$installer" --clobber`,
		`gh release upload "$VERSION" "sums/$sums" --clobber`,
	} {
		if i := strings.Index(r, avant); i < 0 || i > publier {
			t.Errorf("%q n'est pas AVANT la publication de la release", avant)
		}
	}
	if strings.Count(r, "--draft=false") != 1 {
		t.Error("la release doit être publiée par une seule étape, la dernière du job de l'installeur")
	}
}

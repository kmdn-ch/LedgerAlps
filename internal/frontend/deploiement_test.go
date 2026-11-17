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

// CRIT-15 (décision du propriétaire : retirer le chemin « service »). ledgeralps-server.exe n'appelle
// jamais StartServiceCtrlDispatcher : un service qui pointe sur lui échoue au démarrage (erreur 1053).
// scripts/install.ps1, qui en enregistrait un, a été supprimé ; rien ne doit en poser un autre.
//
// Le binaire n'a toujours pas le protocole de service : s'il l'a un jour, cette décision se rouvre.
func TestLeBinaireNEstPasUnServiceWindows(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "scripts", "install.ps1")); err == nil {
		t.Error("scripts/install.ps1 est revenu : il a été retiré, et son service ne pouvait pas démarrer")
	}
	for _, dossier := range []string{"cmd", "internal"} {
		_ = filepath.Walk(filepath.Join("..", "..", dossier), func(chemin string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(chemin, ".go") || strings.HasSuffix(chemin, "_test.go") {
				return nil
			}
			b, _ := os.ReadFile(chemin)
			if strings.Contains(string(b), "windows/svc") {
				t.Errorf("%s utilise windows/svc : le binaire peut maintenant être un service, la décision CRIT-15 se rouvre", chemin)
			}
			return nil
		})
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

// 1.6.6 — décision du propriétaire : LedgerAlps n'est plus joignable par le réseau. Il n'écoute que sur
// 127.0.0.1, en HTTP ; il n'y a plus de TLS, d'adresse d'écoute réglable, de mandataire ni d'origine à
// déclarer. Ces gardes tiennent que ce chemin ne revient pas par inadvertance : le rouvrir est une
// décision, pas un effet de bord.
func TestLeServeurNEcouteQueSurLaBoucleLocale(t *testing.T) {
	// Aucun code de production ne sert en TLS, n'écoute sur toutes les interfaces, ni n'ouvre une
	// socket d'écoute ailleurs que sur la boucle locale.
	for _, dossier := range []string{"cmd", "internal"} {
		_ = filepath.Walk(filepath.Join("..", "..", dossier), func(chemin string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(chemin, ".go") || strings.HasSuffix(chemin, "_test.go") {
				return nil
			}
			b, _ := os.ReadFile(chemin)
			src := string(b)
			for _, interdit := range []string{"ListenAndServeTLS", "tls.Listen(", "tls.NewListener(", `"0.0.0.0`, "SetTrustedProxies(cfg"} {
				if strings.Contains(src, interdit) {
					t.Errorf("%s contient %q : l'accès par le réseau a été retiré", chemin, interdit)
				}
			}
			for _, ligne := range strings.Split(src, "\n") {
				if strings.Contains(ligne, "net.Listen(") && !strings.Contains(ligne, `"127.0.0.1:`) {
					t.Errorf("%s ouvre une socket d'écoute hors de la boucle locale : %s", chemin, strings.TrimSpace(ligne))
				}
			}
			return nil
		})
	}

	// Le serveur principal et le mode récupération prennent leur adresse de la configuration, où elle
	// est une constante ; le premier ne croit aucun en-tête de mandataire.
	for _, fichier := range []string{"main.go", "recovery.go"} {
		src := racine(t, "cmd", "server", fichier)
		if !strings.Contains(src, "cfg.ListenAddr()") {
			t.Errorf("cmd/server/%s n'écoute plus sur cfg.ListenAddr() (la boucle locale)", fichier)
		}
		for _, interdit := range []string{"cfg.Host", "cfg.TLSCert", "cfg.AllowedOrigins", "cfg.TrustedProxies"} {
			if strings.Contains(src, interdit) {
				t.Errorf("cmd/server/%s lit %s : ce réglage n'existe plus", fichier, interdit)
			}
		}
	}
	if !strings.Contains(racine(t, "cmd", "server", "main.go"), "SetTrustedProxies(nil)") {
		t.Error("cmd/server/main.go ne désactive plus la confiance de gin dans X-Forwarded-For (SetTrustedProxies(nil))")
	}

	// Plus d'écran ni de route pour régler l'accès réseau — la rotation de la clé, elle, reste.
	main := racine(t, "cmd", "server", "main.go")
	for _, route := range []string{`"/settings/server",`, "GetServerSettings", "PutServerSettings"} {
		if strings.Contains(main, route) {
			t.Errorf("cmd/server/main.go déclare encore %s", route)
		}
	}
	if !strings.Contains(main, `"/settings/server/rotate-secret"`) {
		t.Error("la route de rotation de la clé de signature a disparu avec les réglages réseau")
	}
}

// La sauvegarde automatique n'était prise qu'au démarrage du serveur. Sur une machine allumée des
// semaines, la copie la plus récente avait donc l'âge du dernier démarrage. Le serveur la rappelle
// maintenant à intervalle régulier, rattachée à la durée de vie de l'application (elle s'arrête avec
// elle). Le planificateur lui-même est testé dans internal/db ; ceci tient que le serveur l'appelle.
func TestLeServeurRappelleLaSauvegardeAutomatique(t *testing.T) {
	src := racine(t, "cmd", "server", "main.go")
	if !strings.Contains(src, "db.StartAutoBackupScheduler(ctxApp, db.AutoBackupCheckEvery,") {
		t.Error("cmd/server/main.go ne rappelle plus la sauvegarde automatique : elle ne serait prise qu'au démarrage, " +
			"et une machine allumée des semaines n'aurait aucune copie récente")
	}
	// La phrase de passe est relue à chaque passage : elle se change sans redémarrer.
	i := strings.Index(src, "db.StartAutoBackupScheduler(")
	if i < 0 || !strings.Contains(src[i:], "backupPolicy.Passphrase()") {
		t.Error("le passage périodique ne relit pas la phrase de passe : un changement de réglage ne serait suivi qu'au redémarrage")
	}
}

// Le nom du fichier PDF d'un devis s'écrivait « facture_… » : l'aperçu et la liste l'écrivaient en dur.
// Une seule fonction (`pdfFileName`) le décide d'après le TYPE du document, et les trois endroits qui
// nomment un PDF l'appellent.
func TestLeNomDuFichierPDFSuitLeTypeDuDocument(t *testing.T) {
	utils := racine(t, "frontend", "src", "utils", "index.ts")
	for _, attendu := range []string{"export function pdfFileName(", "'offre'", "'note_credit'", "'facture'"} {
		if !strings.Contains(utils, attendu) {
			t.Errorf("utils/index.ts ne contient plus %q", attendu)
		}
	}
	for _, page := range []string{"InvoiceDetailPage.tsx", "InvoicesPage.tsx"} {
		src := racine(t, "frontend", "src", "pages", page)
		if !strings.Contains(src, "pdfFileName(") {
			t.Errorf("%s ne nomme plus ses PDF par pdfFileName", page)
		}
		if strings.Contains(src, "`facture_$") {
			t.Errorf("%s écrit « facture_ » en dur : un devis sortirait nommé « facture_… »", page)
		}
	}
	// L'écran d'une facture numérote ses lignes par POSITION, pas par le rang stocké (0 avant la 1.6.6).
	detail := racine(t, "frontend", "src", "pages", "InvoiceDetailPage.tsx")
	if strings.Contains(detail, "{line.sequence}") {
		t.Error("InvoiceDetailPage affiche line.sequence : une facture d'avant la 1.6.6 montrerait « 0 » sur chaque ligne")
	}
}

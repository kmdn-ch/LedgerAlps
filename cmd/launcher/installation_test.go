package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Audit croisé Gemini, CRIT-17 : l'assistant d'installation se lançait sur n'importe quelle
// erreur de lecture de la configuration, et écrasait une configuration — et la clé d'une base
// chiffrée — qu'une simple virgule oubliée avait rendue illisible.

var errLecture = errors.New("invalid character '}' looking for beginning of object key string")

func TestSurUneMachineViergeL_AssistantPeutTourner(t *testing.T) {
	if ok, raison := peutLancerAssistant(t.TempDir(), os.ErrNotExist); !ok {
		t.Fatalf("assistant refusé sur une machine vierge : %s", raison)
	}
}

func TestUneConfigurationIllisibleNeLanceJamaisL_Assistant(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"port": "8000",}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, raison := peutLancerAssistant(dir, errLecture)
	if ok {
		t.Fatal("l'assistant se lancerait sur une configuration abîmée : il l'écraserait")
	}
	if !strings.Contains(raison, "config.json") || !strings.Contains(raison, "n'est PAS lancé") {
		t.Errorf("la raison ne dit pas quoi faire : %s", raison)
	}
	// Le fichier de l'utilisateur n'a pas été touché.
	if b, _ := os.ReadFile(filepath.Join(dir, "config.json")); string(b) != `{"port": "8000",}` {
		t.Errorf("config.json a été modifié : %q", b)
	}
}

func TestDesDonneesSansConfigurationNeLancentPasL_Assistant(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ledgeralps.db"), []byte("SQLite format 3"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, raison := peutLancerAssistant(dir, os.ErrNotExist)
	if ok {
		t.Fatal("l'assistant se lancerait par-dessus une base existante")
	}
	if !strings.Contains(raison, "ledgeralps.db") {
		t.Errorf("la raison ne nomme pas ce qui a été trouvé : %s", raison)
	}
}

func TestUneCleDeBaseSansConfigurationNeLancePasL_Assistant(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if _, err := db.NewDatabaseKeys(dir).Create("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}
	ok, raison := peutLancerAssistant(dir, os.ErrNotExist)
	if ok {
		t.Fatal("l'assistant se lancerait alors qu'une clé de base existe : il l'écraserait")
	}
	if !strings.Contains(raison, "clé") {
		t.Errorf("la raison ne parle pas de la clé : %s", raison)
	}
}

// ─── CRIT-16 : le lanceur ne fige plus le secret de signature ────────────────

// Le lanceur passait JWT_SECRET dans l'environnement du serveur. Le serveur le lit dans
// config.json ; la variable, elle, fait de lui « un secret venu de l'environnement », que
// la rotation ne touche jamais. Sur le chemin d'installation NSIS — celui de tout le monde —
// la clé de signature ne tournait donc pas, contrairement à ce que le code annonçait.
func TestLeLanceurNePasseJamaisLeSecretParLEnvironnement(t *testing.T) {
	c := &config{JWTSecret: strings.Repeat("ab", 32), SQLitePath: `C:\données\ledgeralps.db`, Port: "8000", AllowedOrigins: "http://localhost:8000"}
	for _, e := range serverEnv(nil, c) {
		if strings.HasPrefix(e, "JWT_SECRET=") {
			t.Fatalf("le secret de signature est passé par l'environnement : %s", e)
		}
	}
	// Le reste est bien transmis.
	env := strings.Join(serverEnv(nil, c), "\n")
	for _, attendu := range []string{"SQLITE_PATH=", "PORT=8000", "ALLOWED_ORIGINS=http://localhost:8000", "LEDGERALPS_INSTALL_DIR="} {
		if !strings.Contains(env, attendu) {
			t.Errorf("%q absent de l'environnement du serveur", attendu)
		}
	}
}

// De bout en bout : avec l'environnement que le lanceur compose, un config.json dont la clé a
// plus de quatre-vingt-dix jours est bien régénéré au démarrage du serveur.
func TestLaRotationTourneSurLeCheminDuLanceur(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("HOME", appData)
	// Le dossier de données dépend du système (APPDATA\LedgerAlps sous Windows, ~/.ledgeralps
	// ailleurs) : on le demande à la configuration, comme le serveur.
	dir := appconfig.AppDataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ancien := strings.Repeat("0123456789abcdef", 4)
	vieux := time.Now().AddDate(0, 0, -200).UTC().Format(time.RFC3339)
	conf := `{"jwt_secret":"` + ancien + `","jwt_secret_rotated_at":"` + vieux + `","sqlite_path":"` + filepath.ToSlash(filepath.Join(dir, "x.db")) + `","port":"8000"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}

	// L'environnement que le lanceur compose, appliqué au processus de test.
	os.Unsetenv("JWT_SECRET")
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	for _, e := range serverEnv(nil, &config{SQLitePath: filepath.Join(dir, "x.db"), Port: "8000", AllowedOrigins: "http://localhost:8000"}) {
		if k, v, ok := strings.Cut(e, "="); ok {
			t.Setenv(k, v)
		}
	}

	cfg := appconfig.Load()
	if cfg.JWTSecretDepuisEnv {
		t.Fatal("la clé est tenue pour venue de l'environnement : la rotation ne tournera jamais")
	}
	tourne, err := appconfig.MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil || !tourne {
		t.Fatalf("la clé de 200 jours n'a pas été régénérée (tournée=%v, erreur=%v)", tourne, err)
	}
	if cfg.JWTSecret == ancien {
		t.Error("la clé n'a pas changé")
	}
}

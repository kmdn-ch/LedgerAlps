package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeConfigFile puts a config.json where Load will find it.
//
// AppDataDir has no test seam, so instead of adding production surface the test
// redirects the variables it already reads: APPDATA on Windows, HOME elsewhere.
// t.Setenv restores them afterwards.
func writeConfigFile(t *testing.T, fc map[string]any) {
	t.Helper()
	base := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", base)
	} else {
		t.Setenv("HOME", base)
		t.Setenv("USERPROFILE", base)
	}

	dir := AppDataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	// Guard against a silent no-op: if the redirection failed, Load would read
	// the real configuration and the assertions below would prove nothing.
	if _, err := os.Stat(ConfigFilePath()); err != nil {
		t.Fatalf("le fichier de configuration de test n'est pas là où Load le cherche (%s)", ConfigFilePath())
	}
}

// The defect this guards: config.json won outright, so every operational
// setting was unreachable on a Windows install — the wizard always writes a
// config.json, and it carries none of these keys.
func TestEnvironmentOverridesTheConfigFile(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":  "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path": "depuis-le-fichier.db",
		"port":        "8000",
	})

	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("TLS_CERT", "/depuis/l-environnement/cert.pem")
	t.Setenv("SQLITE_PATH", "depuis-l-environnement.db")

	cfg := Load()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0 — HOST doit primer sur le fichier", cfg.Host)
	}
	if cfg.TLSCert != "/depuis/l-environnement/cert.pem" {
		t.Errorf("TLSCert = %q — TLS_CERT était défini et doit primer", cfg.TLSCert)
	}
	if cfg.SQLitePath != "depuis-l-environnement.db" {
		t.Errorf("SQLitePath = %q — SQLITE_PATH doit primer ; c'est cette surprise qui a fait viser la base live", cfg.SQLitePath)
	}
}

// An unset variable must not wipe a configured value — otherwise loading would
// silently reset half the configuration on any machine with a sparse
// environment.
func TestUnsetEnvironmentKeepsTheFileValue(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":      "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path":     "depuis-le-fichier.db",
		"port":            "9999",
		"allowed_origins": "http://exemple.local",
	})

	// Explicitly cleared, as a sparse environment would have them.
	t.Setenv("PORT", "")
	t.Setenv("ALLOWED_ORIGINS", "")

	cfg := Load()

	if cfg.Port != "9999" {
		t.Errorf("Port = %q, want 9999 — une variable vide ne doit rien écraser", cfg.Port)
	}
	if cfg.AllowedOrigins != "http://exemple.local" {
		t.Errorf("AllowedOrigins = %q — une variable vide ne doit rien écraser", cfg.AllowedOrigins)
	}
	if cfg.SQLitePath != "depuis-le-fichier.db" {
		t.Errorf("SQLitePath = %q — valeur du fichier attendue", cfg.SQLitePath)
	}
}

// The safe default matters as much as the override: binding every interface is
// what used to expose a laptop's accounts to whatever network it was on.
func TestHostDefaultsToLoopback(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":  "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path": "test.db",
	})
	if cfg := Load(); cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", cfg.Host)
	}
}

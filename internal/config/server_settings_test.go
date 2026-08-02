package config

import (
	"encoding/json"
	"os"
	"testing"
)

// The defect this guards: config.json is written once by the setup wizard and
// never touched again, so an upgraded installation carries only the keys it was
// born with. Saving must add the new ones without losing the old.
//
// Losing jwt_secret would lock every user out of their own accounts, which is
// why this writes through a generic map rather than marshalling a struct.
func TestSavingPreservesUnknownKeys(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":      "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path":     "ledgeralps.db",
		"port":            "8000",
		"allowed_origins": "http://localhost:8000",
		"debug":           false,
		// A key from a version this build knows nothing about.
		"une_option_future": "à préserver",
	})

	if err := SaveServerSettings(ServerSettings{Host: "0.0.0.0", ForceTLS: true}); err != nil {
		t.Fatalf("SaveServerSettings: %v", err)
	}

	raw, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("le fichier écrit n'est pas du JSON valide: %v", err)
	}

	if got["jwt_secret"] != "un-secret-suffisamment-long-pour-passer-la-validation" {
		t.Error("jwt_secret a été perdu — tous les utilisateurs seraient déconnectés de leur comptabilité")
	}
	if got["sqlite_path"] != "ledgeralps.db" {
		t.Error("sqlite_path a été perdu — l'application ouvrirait une autre base")
	}
	if got["une_option_future"] != "à préserver" {
		t.Error("une clé inconnue a été supprimée ; ce sont les données d'une autre version, pas du bruit")
	}
	if got["host"] != "0.0.0.0" || got["force_tls"] != true {
		t.Errorf("les nouveaux réglages n'ont pas été écrits: host=%v force_tls=%v", got["host"], got["force_tls"])
	}
}

// The round trip that failed in practice: the user turns TLS on, restarts, and
// the setting must actually be in effect.
func TestSavedSettingsAreReadBackByLoad(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":  "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path": "ledgeralps.db",
	})

	if err := SaveServerSettings(ServerSettings{Host: "127.0.0.1", ForceTLS: true}); err != nil {
		t.Fatal(err)
	}
	// No FORCE_TLS in the environment: the launcher used to pass it as "false",
	// which overrode the file and made the setting impossible to turn on.
	os.Unsetenv("FORCE_TLS")

	if cfg := Load(); !cfg.ForceTLS {
		t.Error("ForceTLS relu à false alors qu'il vient d'être enregistré à true")
	}
}

// Clearing a certificate path must remove the key, not store an empty string —
// an empty path would be handed to the TLS stack and fail the start.
func TestClearingACertificatePathRemovesTheKey(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret": "un-secret-suffisamment-long-pour-passer-la-validation",
		"tls_cert":   "/ancien/cert.pem",
		"tls_key":    "/ancien/key.pem",
	})

	if err := SaveServerSettings(ServerSettings{Host: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(ConfigFilePath())
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, present := got["tls_cert"]; present {
		t.Error("tls_cert vidé mais toujours présent : un chemin vide serait passé à la pile TLS")
	}
	if _, present := got["tls_key"]; present {
		t.Error("tls_key vidé mais toujours présent")
	}
}

// A corrupt config.json must abort the save rather than overwrite it: the file
// still holds the JWT secret and the database path.
func TestCorruptConfigIsNotOverwritten(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret": "un-secret-suffisamment-long-pour-passer-la-validation",
	})
	if err := os.WriteFile(ConfigFilePath(), []byte("{ ceci n'est pas du JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SaveServerSettings(ServerSettings{Host: "0.0.0.0"}); err == nil {
		t.Fatal("l'enregistrement a réussi sur un fichier illisible")
	}
	raw, _ := os.ReadFile(ConfigFilePath())
	if string(raw) != "{ ceci n'est pas du JSON" {
		t.Error("le fichier illisible a été écrasé ; il pouvait encore être réparé à la main")
	}
}

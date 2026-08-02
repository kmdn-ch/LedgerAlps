package config

// Writing the network settings back to config.json.
//
// config.json is created by the setup wizard on first run and never touched
// again. An upgrade therefore never adds new keys: an installation from v1.1
// still carries the five keys it was born with. Anything added since — host,
// TLS, the insecure opt-in — is simply absent, reads as its zero value, and no
// amount of editing the environment helps on Windows because the launcher
// starts the server itself.
//
// So the settings have to be writable from the application. Hand-editing JSON
// in %APPDATA% is not an answer for accounting software.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerSettings is the network-facing subset a user can change at runtime.
// Everything here takes effect at the next start, which is why saving one
// returns a restart request rather than pretending it applied.
type ServerSettings struct {
	Host              string `json:"host"`
	ForceTLS          bool   `json:"force_tls"`
	TLSCert           string `json:"tls_cert"`
	TLSKey            string `json:"tls_key"`
	AllowInsecureHTTP bool   `json:"allow_insecure_http"`
}

// CurrentServerSettings reports what is in effect now.
func CurrentServerSettings(cfg *Config) ServerSettings {
	return ServerSettings{
		Host:              cfg.Host,
		ForceTLS:          cfg.ForceTLS,
		TLSCert:           cfg.TLSCert,
		TLSKey:            cfg.TLSKey,
		AllowInsecureHTTP: cfg.AllowInsecureHTTP,
	}
}

// SaveServerSettings merges s into config.json, preserving every other key.
//
// Read-modify-write over a generic map rather than marshalling a struct: a
// struct would silently drop keys it does not know about — jwt_secret among
// them, which would lock every user out of their own accounts. Unknown keys
// are data belonging to someone else's version, not noise to discard.
func SaveServerSettings(s ServerSettings) error {
	path := ConfigFilePath()

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("le fichier de configuration est illisible, rien n'a été modifié: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lecture de la configuration: %w", err)
	}

	existing["host"] = s.Host
	existing["force_tls"] = s.ForceTLS
	existing["allow_insecure_http"] = s.AllowInsecureHTTP
	// Empty means "not configured": storing "" would be indistinguishable from
	// a path the user cleared on purpose, and both should read as absent.
	setOrDelete(existing, "tls_cert", s.TLSCert)
	setOrDelete(existing, "tls_key", s.TLSKey)

	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("encodage de la configuration: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("création du dossier de configuration: %w", err)
	}

	// Write to a sibling then rename: a crash mid-write would otherwise leave a
	// truncated config.json, and the application would not start again. Rename
	// within the same directory is atomic on both Windows and POSIX.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("écriture de la configuration: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remplacement de la configuration: %w", err)
	}
	return nil
}

func setOrDelete(m map[string]any, key, value string) {
	if value == "" {
		delete(m, key)
		return
	}
	m[key] = value
}

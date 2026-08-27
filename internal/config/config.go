package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// knownWeakSecrets are default/test values that must not be used in production.
var knownWeakSecrets = []string{
	"changeme",
	"changeme_in_production_use_32_chars_minimum",
	"secret",
	"password",
	"ledgeralps",
	"",
}

// Config holds all runtime configuration. Values are read from the config file
// or environment variables; sensible defaults apply for local (SQLite) usage.
type Config struct {
	// Server
	//
	// Host defaults to 127.0.0.1. Until v1.4.5 the server bound to every
	// interface, which meant a laptop on a café network was serving its
	// accounts to that network in clear. Reaching LedgerAlps from another
	// machine is now something you ask for, and asking for it brings TLS.
	Host  string
	Port  string
	Debug bool

	// TLS. Supplying both a certificate and a key serves HTTPS. Leaving them
	// empty while Host is not loopback makes the server generate a self-signed
	// certificate rather than fall back to clear text.
	TLSCert string
	TLSKey  string

	// AllowInsecureHTTP serves clear HTTP on a non-loopback interface. It
	// exists for the case where a reverse proxy already terminates TLS on the
	// same host — and nowhere else. Anything else puts the login password, the
	// session token and the backup passphrase on the wire.
	AllowInsecureHTTP bool

	// Database — SQLite by default, PostgreSQL if DSN is set
	SQLitePath  string
	PostgresDSN string // if non-empty, PostgreSQL is used instead of SQLite

	// Auth
	JWTSecret        string
	JWTAccessMinutes int
	JWTRefreshDays   int
	// JWTSecretRotatedAt est la date du dernier tirage de la clé de signature.
	// Nulle sur toute installation antérieure à la rotation automatique, ce qui
	// compte comme « jamais tournée » : on ignore l'âge de cette clé, et sur une
	// installation qui date, la réponse est « longtemps ».
	JWTSecretRotatedAt time.Time
	// La périodicité de la rotation n'est pas un réglage : voir
	// JWTSecretRotationDays.
	//
	// IdleLogoutMinutes déconnecte après cette durée sans activité. Zéro
	// désactive.
	IdleLogoutMinutes int

	// Application
	LogLevel       string
	AllowedOrigins string // comma-separated CORS origins

	// TrustedProxies liste les mandataires dont on accepte les en-têtes
	// d'adresse (`X-Forwarded-For`, `X-Real-IP`), en adresses ou en CIDR.
	//
	// VIDE PAR DÉFAUT, et c'est le point. gin fait l'inverse : ses mandataires
	// de confiance valent `0.0.0.0/0` et `::/0`, si bien que `c.ClientIP()`
	// rend l'en-tête posé par l'appelant. Le verrouillage des connexions
	// devient alors décoratif — une valeur différente à chaque requête est une
	// clé différente — et l'adresse scellée dans la chaîne d'audit
	// (CO art. 957a) devient celle que l'attaquant a choisie.
	//
	// LedgerAlps écoute en direct. Une installation derrière un reverse proxy
	// déclare le sien ici, explicitement, plutôt que de tout accepter.
	TrustedProxies []string

	// UpdateCheck controls the single outbound request LedgerAlps ever makes:
	// asking whether a newer release exists, so a user is told to update when a
	// compliance fix ships. It sends no identifiers and no user data. Set to
	// false for a fully air-gapped installation.
	UpdateCheck bool
}

// fileConfig is the JSON structure stored in the config file.
type fileConfig struct {
	JWTSecret   string `json:"jwt_secret"`
	SQLitePath  string `json:"sqlite_path"`
	PostgresDSN string `json:"postgres_dsn,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        string `json:"port"`
	Debug       bool   `json:"debug"`
	TLSCert     string `json:"tls_cert,omitempty"`
	TLSKey      string `json:"tls_key,omitempty"`
	// Pointer so an absent key keeps the safe default (false).
	AllowInsecureHTTP *bool  `json:"allow_insecure_http,omitempty"`
	AllowedOrigins    string `json:"allowed_origins"`
	// Mandataires de confiance, en adresses ou en CIDR. Absent ou vide = aucun,
	// donc l'adresse observée est celle de la connexion elle-même.
	TrustedProxies []string `json:"trusted_proxies,omitempty"`
	// Pointer so that an absent key keeps the default (enabled) while an
	// explicit `"update_check": false` is honoured.
	UpdateCheck *bool `json:"update_check,omitempty"`
	// Date du dernier tirage de la clé de signature. Sa périodicité, elle, n'est
	// plus lue ici : elle est constante — un fichier qui porte encore
	// `jwt_secret_max_age_days` est simplement ignoré, et la clé est purgée à la
	// première rotation.
	JWTSecretRotatedAt string `json:"jwt_secret_rotated_at,omitempty"`
	// Pointeur pour distinguer « absent » — donc valeur par défaut — de « posé à
	// zéro », qui veut dire « désactivé » et doit être respecté.
	IdleLogoutMinutes *int `json:"idle_logout_minutes,omitempty"`
}

// AppDataDir returns the platform-specific application data directory for LedgerAlps.
// Windows: %APPDATA%\LedgerAlps
// Other:   ~/.ledgeralps
func AppDataDir() string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "LedgerAlps")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ledgeralps"
	}
	return filepath.Join(home, ".ledgeralps")
}

// ConfigFilePath returns the path to the JSON config file.
func ConfigFilePath() string {
	return filepath.Join(AppDataDir(), "config.json")
}

// Load reads configuration from the JSON config file (preferred), falling back
// to environment variables. It calls os.Exit(1) if secrets are weak/missing.
func Load() *Config {
	// Try config file first (written by the setup wizard / installer).
	if fc, err := loadFromFile(ConfigFilePath()); err == nil {
		cfg := &Config{
			Host:             fc.Host,
			Port:             fc.Port,
			TLSCert:          fc.TLSCert,
			TLSKey:           fc.TLSKey,
			Debug:            fc.Debug,
			SQLitePath:       fc.SQLitePath,
			PostgresDSN:      fc.PostgresDSN,
			JWTSecret:        fc.JWTSecret,
			JWTAccessMinutes: 60,
			JWTRefreshDays:   30,
			LogLevel:         "INFO",
			AllowedOrigins:   fc.AllowedOrigins,
			TrustedProxies:   fc.TrustedProxies,
			UpdateCheck:      fc.UpdateCheck == nil || *fc.UpdateCheck,
		}
		if cfg.Port == "" {
			cfg.Port = "8000"
		}
		if cfg.Host == "" {
			cfg.Host = "127.0.0.1"
		}
		if fc.AllowInsecureHTTP != nil {
			cfg.AllowInsecureHTTP = *fc.AllowInsecureHTTP
		}
		// Absent = valeur par defaut ; pose a zero = desactive volontairement.
		// La distinction ne peut se faire qu'avec un pointeur, sans quoi
		// « desactiver la deconnexion » se relirait comme « regler par defaut »
		// au demarrage suivant.
		cfg.IdleLogoutMinutes = DefaultIdleLogoutMinutes
		if fc.IdleLogoutMinutes != nil {
			cfg.IdleLogoutMinutes = *fc.IdleLogoutMinutes
		}
		if fc.JWTSecretRotatedAt != "" {
			if t, err := time.Parse(time.RFC3339, fc.JWTSecretRotatedAt); err == nil {
				cfg.JWTSecretRotatedAt = t
			}
			// Une date illisible reste nulle, donc « jamais tournee », donc la
			// cle tourne au prochain demarrage. C'est le bon sens de l'erreur.
		}
		if cfg.AllowedOrigins == "" {
			cfg.AllowedOrigins = "http://localhost:" + cfg.Port
		}
		// Environment variables override the file.
		//
		// The file used to win outright, which made every operational setting
		// unreachable on a Windows install: the wizard always writes a
		// config.json, so HOST, TLS_CERT and the rest were read from
		// a file that has no such keys and silently defaulted. The launcher even
		//
		// It also surprised us once in a way that mattered: SQLITE_PATH was set
		// to aim a restore at a scratch database, the file won, and the restore
		// ran against the live one.
		//
		// Only variables actually present in the environment override, so an
		// unset variable never wipes a configured value.
		applyEnvOverrides(cfg)
		cfg.validateSecrets()
		return cfg
	}

	// No config file: environment variables only (Linux, systemd, CI).
	cfg := &Config{
		Host:              getEnv("HOST", "127.0.0.1"),
		Port:              getEnv("PORT", "8000"),
		TLSCert:           getEnv("TLS_CERT", ""),
		TLSKey:            getEnv("TLS_KEY", ""),
		AllowInsecureHTTP: getEnv("ALLOW_INSECURE_HTTP", "false") == "true",
		Debug:             getEnv("DEBUG", "false") == "true",
		SQLitePath:        getEnv("SQLITE_PATH", "ledgeralps.db"),
		PostgresDSN:       getEnv("POSTGRES_DSN", ""),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		JWTAccessMinutes:  60,
		JWTRefreshDays:    30,
		IdleLogoutMinutes: DefaultIdleLogoutMinutes,
		LogLevel:          getEnv("LOG_LEVEL", "INFO"),
		AllowedOrigins:    getEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
		TrustedProxies:    listeDeProxies(getEnv("TRUSTED_PROXIES", "")),
		UpdateCheck:       getEnv("UPDATE_CHECK", "true") != "false",
	}
	cfg.validateSecrets()
	return cfg
}

func loadFromFile(path string) (*fileConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var fc fileConfig
	if err := json.NewDecoder(f).Decode(&fc); err != nil {
		return nil, err
	}
	return &fc, nil
}

// UsePostgres returns true when a PostgreSQL DSN is configured.
func (c *Config) UsePostgres() bool {
	return c.PostgresDSN != ""
}

// validateSecrets aborts startup if any secret equals a known weak value
// or does not meet minimum length requirements.
func (c *Config) validateSecrets() {
	secrets := map[string]string{
		"JWT_SECRET": c.JWTSecret,
	}
	for name, val := range secrets {
		for _, weak := range knownWeakSecrets {
			if val == weak {
				fmt.Fprintf(os.Stderr,
					"FATAL: %s is set to a known weak/default value. "+
						"Generate a strong secret before starting LedgerAlps.\n", name)
				os.Exit(1)
			}
		}
		// JWT_SECRET must be at least 32 characters to resist brute-force.
		if name == "JWT_SECRET" && len(val) < 32 {
			fmt.Fprintf(os.Stderr,
				"FATAL: JWT_SECRET must be at least 32 characters (got %d). "+
					"Generate one with: openssl rand -hex 32\n", len(val))
			os.Exit(1)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// listeDeProxies lit « 10.0.0.1, 192.168.1.0/24 » en liste.
//
// Rend `nil` — et non une tranche vide — quand il n'y a rien à lire :
// `SetTrustedProxies(nil)` fait rendre à `ClientIP()` l'adresse de la connexion
// seule, ce qui est exactement le défaut voulu. Les entrées vides laissées par
// une virgule en trop sont écartées : une chaîne vide passée à gin serait une
// erreur de démarrage, pour une faute de frappe.
func listeDeProxies(brut string) []string {
	var out []string
	for _, p := range strings.Split(brut, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applyEnvOverrides lets the environment win over the config file, for the
// variables that are actually set. See Load for why.
func applyEnvOverrides(cfg *Config) {
	setStr := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			*dst = v
		}
	}
	setBool := func(key string, dst *bool) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			*dst = v == "true"
		}
	}

	setStr("HOST", &cfg.Host)
	setStr("PORT", &cfg.Port)
	setStr("SQLITE_PATH", &cfg.SQLitePath)
	setStr("POSTGRES_DSN", &cfg.PostgresDSN)
	setStr("JWT_SECRET", &cfg.JWTSecret)
	setStr("ALLOWED_ORIGINS", &cfg.AllowedOrigins)
	if v, ok := os.LookupEnv("TRUSTED_PROXIES"); ok {
		cfg.TrustedProxies = listeDeProxies(v)
	}
	setStr("LOG_LEVEL", &cfg.LogLevel)
	setStr("TLS_CERT", &cfg.TLSCert)
	setStr("TLS_KEY", &cfg.TLSKey)

	setBool("DEBUG", &cfg.Debug)
	setBool("ALLOW_INSECURE_HTTP", &cfg.AllowInsecureHTTP)
	// UPDATE_CHECK is the one where anything other than "false" means enabled,
	// matching how it is documented.
	if v, ok := os.LookupEnv("UPDATE_CHECK"); ok && v != "" {
		cfg.UpdateCheck = v != "false"
	}
}

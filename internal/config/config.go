package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

	// Application
	LogLevel       string
	AllowedOrigins string // comma-separated CORS origins

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
	// Pointer so that an absent key keeps the default (enabled) while an
	// explicit `"update_check": false` is honoured.
	UpdateCheck *bool `json:"update_check,omitempty"`
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
		if cfg.AllowedOrigins == "" {
			cfg.AllowedOrigins = "http://localhost:" + cfg.Port
		}
		cfg.validateSecrets()
		return cfg
	}

	// Fall back to environment variables (dev / Docker / CI usage).
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
		LogLevel:          getEnv("LOG_LEVEL", "INFO"),
		AllowedOrigins:    getEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
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

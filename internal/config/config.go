package config

import (
	"fmt"
	"os"
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

// Config holds all runtime configuration. Values are read from environment
// variables; sensible defaults apply for local (SQLite) usage.
type Config struct {
	// Server
	Port  string
	Debug bool

	// Database — SQLite by default, PostgreSQL if DSN is set
	SQLitePath string
	PostgresDSN string // if non-empty, PostgreSQL is used instead of SQLite

	// Auth
	JWTSecret          string
	JWTAccessMinutes   int
	JWTRefreshDays     int

	// Application
	LogLevel string
}

// Load reads environment variables and validates critical values.
// It calls os.Exit(1) if any secret is set to a known weak default.
func Load() *Config {
	cfg := &Config{
		Port:               getEnv("PORT", "8000"),
		Debug:              getEnv("DEBUG", "false") == "true",
		SQLitePath:         getEnv("SQLITE_PATH", "ledgeralps.db"),
		PostgresDSN:        getEnv("POSTGRES_DSN", ""),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		JWTAccessMinutes:   60,
		JWTRefreshDays:     30,
		LogLevel:           getEnv("LOG_LEVEL", "INFO"),
	}

	cfg.validateSecrets()
	return cfg
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

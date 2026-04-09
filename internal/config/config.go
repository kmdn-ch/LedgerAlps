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
	Port  string
	Debug bool

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
}

// fileConfig is the JSON structure stored in the config file.
type fileConfig struct {
	JWTSecret      string `json:"jwt_secret"`
	SQLitePath     string `json:"sqlite_path"`
	PostgresDSN    string `json:"postgres_dsn,omitempty"`
	Port           string `json:"port"`
	Debug          bool   `json:"debug"`
	AllowedOrigins string `json:"allowed_origins"`
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
			Port:             fc.Port,
			Debug:            fc.Debug,
			SQLitePath:       fc.SQLitePath,
			PostgresDSN:      fc.PostgresDSN,
			JWTSecret:        fc.JWTSecret,
			JWTAccessMinutes: 60,
			JWTRefreshDays:   30,
			LogLevel:         "INFO",
			AllowedOrigins:   fc.AllowedOrigins,
		}
		if cfg.Port == "" {
			cfg.Port = "8000"
		}
		if cfg.AllowedOrigins == "" {
			cfg.AllowedOrigins = "http://localhost:" + cfg.Port
		}
		cfg.validateSecrets()
		return cfg
	}

	// Fall back to environment variables (dev / Docker / CI usage).
	cfg := &Config{
		Port:             getEnv("PORT", "8000"),
		Debug:            getEnv("DEBUG", "false") == "true",
		SQLitePath:       getEnv("SQLITE_PATH", "ledgeralps.db"),
		PostgresDSN:      getEnv("POSTGRES_DSN", ""),
		JWTSecret:        getEnv("JWT_SECRET", ""),
		JWTAccessMinutes: 60,
		JWTRefreshDays:   30,
		LogLevel:         getEnv("LOG_LEVEL", "INFO"),
		AllowedOrigins:   getEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
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

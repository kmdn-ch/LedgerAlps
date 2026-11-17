package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	// Le serveur n'écoute que sur la boucle locale (ListenHost) : il n'y a ni adresse d'écoute à régler,
	// ni TLS, ni mandataire à déclarer. LedgerAlps n'est pas joignable depuis le réseau (1.6.6).
	Port  string
	Debug bool

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
	// JWTSecretDepuisEnv dit que la clé vient de la variable d'environnement
	// JWT_SECRET, et non du fichier de configuration.
	//
	// Sur le chemin d'installation « service » — Linux/systemd via
	// `EnvironmentFile=` — cette variable est chargée à chaque démarrage. (Il n'y a pas de service
	// Windows : le binaire ne sait pas en être un.) `applyEnvOverrides` la réimpose alors par-dessus toute
	// clé tournée relue du fichier, et cette préséance est VOULUE (voir le
	// commentaire de Load : le fichier gagnait, ce qui rendait SQLITE_PATH et
	// consorts inatteignables).
	//
	// Conséquence : sur ces déploiements, la rotation ne peut pas aboutir. Elle
	// écrivait pourtant un secret dans config.json et faisait avancer la date
	// affichée — une promesse que le mode de déploiement empêche de tenir.
	// Ce drapeau permet de ne plus la faire.
	JWTSecretDepuisEnv bool
	// La périodicité de la rotation n'est pas un réglage : voir
	// JWTSecretRotationDays.
	//
	// IdleLogoutMinutes déconnecte après cette durée sans activité. Zéro
	// désactive.
	IdleLogoutMinutes int

	// Application
	LogLevel string

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
	Port        string `json:"port"`
	Debug       bool   `json:"debug"`
	// Les clés `host`, `tls_cert`, `tls_key`, `allow_insecure_http`, `trusted_proxies` et
	// `allowed_origins` d'un fichier écrit par une version antérieure sont ignorées à la lecture :
	// voir ReglagesReseauIgnores.
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
			Port:             fc.Port,
			Debug:            fc.Debug,
			SQLitePath:       fc.SQLitePath,
			PostgresDSN:      fc.PostgresDSN,
			JWTSecret:        fc.JWTSecret,
			JWTAccessMinutes: 60,
			JWTRefreshDays:   30,
			LogLevel:         "INFO",
			UpdateCheck:      fc.UpdateCheck == nil || *fc.UpdateCheck,
		}
		if cfg.Port == "" {
			cfg.Port = "8000"
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
		// Environment variables override the file.
		//
		// The file used to win outright, which made every operational setting
		// unreachable on a Windows install: the wizard always writes a
		// config.json, so SQLITE_PATH and the rest were read from
		// a file that has no such keys and silently defaulted. The launcher even
		//
		// It also surprised us once in a way that mattered: SQLITE_PATH was set
		// to aim a restore at a scratch database, the file won, and the restore
		// ran against the live one.
		//
		// Only variables actually present in the environment override, so an
		// unset variable never wipes a configured value.
		applyEnvOverrides(cfg)
		// Relevé APRÈS applyEnvOverrides, qui est l'étape qui décide : si la
		// variable est posée, c'est elle qui a gagné, et elle regagnera à chaque
		// démarrage. La rotation en tiendra compte plutôt que de promettre ce
		// qu'elle ne peut pas tenir.
		_, cfg.JWTSecretDepuisEnv = os.LookupEnv("JWT_SECRET")
		cfg.validateSecrets()
		return cfg
	}

	// No config file: environment variables only (Linux, systemd, CI).
	cfg := &Config{
		Port:              getEnv("PORT", "8000"),
		Debug:             getEnv("DEBUG", "false") == "true",
		SQLitePath:        getEnv("SQLITE_PATH", "ledgeralps.db"),
		PostgresDSN:       getEnv("POSTGRES_DSN", ""),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		JWTAccessMinutes:  60,
		JWTRefreshDays:    30,
		IdleLogoutMinutes: DefaultIdleLogoutMinutes,
		LogLevel:          getEnv("LOG_LEVEL", "INFO"),
		UpdateCheck:       getEnv("UPDATE_CHECK", "true") != "false",
	}
	// Sans fichier de configuration, la clé ne peut venir que de
	// l'environnement — c'est le cas du tout premier démarrage d'un service
	// Linux ou Windows. La rotation écrirait un config.json que le démarrage
	// suivant relirait pour rien : la variable regagnerait.
	_, cfg.JWTSecretDepuisEnv = os.LookupEnv("JWT_SECRET")
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

	setStr("PORT", &cfg.Port)
	setStr("SQLITE_PATH", &cfg.SQLitePath)
	setStr("POSTGRES_DSN", &cfg.PostgresDSN)
	setStr("JWT_SECRET", &cfg.JWTSecret)
	setStr("LOG_LEVEL", &cfg.LogLevel)

	setBool("DEBUG", &cfg.Debug)
	// UPDATE_CHECK is the one where anything other than "false" means enabled,
	// matching how it is documented.
	if v, ok := os.LookupEnv("UPDATE_CHECK"); ok && v != "" {
		cfg.UpdateCheck = v != "false"
	}
}

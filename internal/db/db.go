package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	_ "modernc.org/sqlite"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations
var migrationsFS embed.FS

// Open returns an *sql.DB connected to either SQLite or PostgreSQL depending
// on the configuration. SQLite is opened in WAL mode for concurrency.
func Open(cfg *config.Config) (*sql.DB, error) {
	var (
		driver string
		dsn    string
	)

	if cfg.UsePostgres() {
		driver = "pgx"
		dsn = cfg.PostgresDSN
	} else {
		driver = "sqlite"
		// WAL mode + foreign keys enabled via connection string
		dsn = fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on", cfg.SQLitePath)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database (%s): %w", driver, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database (%s): %w", driver, err)
	}
	return db, nil
}

// Migrate applies all pending SQL migration files embedded in the binary.
// Files must follow the naming convention: NNNN_description.up.sql
func Migrate(db *sql.DB) error {
	// Ensure the migrations tracking table exists
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}

	// Filter and sort .up.sql files
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".up.sql")

		// Skip already-applied migrations
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			// PostgreSQL uses $1 placeholder — try again
			if err2 := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`, version).Scan(&count); err2 != nil {
				return fmt.Errorf("checking migration %s: %w", version, err2)
			}
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			// PostgreSQL
			if _, err2 := db.Exec(`INSERT INTO schema_migrations(version) VALUES($1)`, version); err2 != nil {
				return fmt.Errorf("recording migration %s: %w", version, err2)
			}
		}

		fmt.Printf("  ✓ applied migration: %s\n", name)
	}
	return nil
}

package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path" // always forward-slash — required by embed.FS on all platforms
	"sort"
	"strings"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations
var migrationsFS embed.FS

// Open returns an *sql.DB connected to SQLite (WAL mode) or PostgreSQL depending
// on configuration. Connection pool defaults are set for typical SME workloads.
func Open(cfg *config.Config) (*sql.DB, error) {
	var driver, dsn string

	if cfg.UsePostgres() {
		driver = "pgx"
		dsn = cfg.PostgresDSN
	} else {
		driver = "sqlite"
		// WAL mode + foreign keys enforced via DSN parameters (modernc.org/sqlite)
		dsn = fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on", cfg.SQLitePath)
	}

	database, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database (%s): %w", driver, err)
	}

	// Connection pool tuning
	if cfg.UsePostgres() {
		database.SetMaxOpenConns(50)
		database.SetMaxIdleConns(10)
		database.SetConnMaxLifetime(5 * time.Minute)
	} else {
		// SQLite WAL: concurrent readers, serialised writers
		database.SetMaxOpenConns(25)
		database.SetMaxIdleConns(5)
		database.SetConnMaxLifetime(10 * time.Minute)
	}

	// Ping with timeout to catch misconfigured DSNs at startup
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database (%s): %w", driver, err)
	}

	return database, nil
}

// Migrate applies all pending SQL migration files embedded in the binary.
// Each migration is applied atomically in a transaction (DDL is transactional
// in both SQLite and PostgreSQL).
// Files must follow the naming convention: NNNN_description.up.sql
func Migrate(database *sql.DB, usePostgres bool) error {
	// Ensure the migrations tracking table exists (outside the per-migration tx)
	createTable := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`
	if _, err := database.Exec(createTable); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".up.sql")

		// Check if already applied (use correct placeholder for DB dialect)
		checkQ := Rebind("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", usePostgres)
		var count int
		if err := database.QueryRow(checkQ, version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		// Apply migration atomically: DDL + schema_migrations record in one transaction
		tx, err := database.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %s: %w", name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		insertQ := Rebind("INSERT INTO schema_migrations(version) VALUES(?)", usePostgres)
		if _, err := tx.Exec(insertQ, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}

		fmt.Printf("  [OK] applied migration: %s\n", name)
	}
	return nil
}

// Command ledgeralps-cli is the administrative CLI for LedgerAlps.
//
// Usage:
//
//	ledgeralps-cli version
//	ledgeralps-cli migrate
//	ledgeralps-cli bootstrap --email=admin@example.com --password=xxx [--name="Admin"] [--url=http://localhost:8000]
//	ledgeralps-cli health [--url=http://localhost:8000]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version", "-version", "--version", "-v":
		fmt.Println(version.Info())
	case "migrate":
		cmdMigrate()
	case "bootstrap":
		cmdBootstrap(os.Args[2:])
	case "health":
		cmdHealth(os.Args[2:])
	case "backup":
		cmdBackup(os.Args[2:])
	case "backups":
		cmdBackups(os.Args[2:])
	case "restore":
		cmdRestore(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `ledgeralps-cli — LedgerAlps administrative CLI (%s)

USAGE:
  ledgeralps-cli <command> [flags]

COMMANDS:
  version                                  Print version and build metadata
  migrate                                  Apply pending DB migrations (reads env for config)
  bootstrap  --email=  --password=         Create the first admin user via the API
             [--name=Admin] [--url=http://localhost:8000]
  health     [--url=http://localhost:8000] Check the server health endpoint
  backup     [--keep=14] [--dir=]          Snapshot the database (safe while running)
             [--sqlite-path=]
  backups    [--dir=]                      List available snapshots, newest first
  restore    --file=<snapshot> --confirm   Restore a snapshot (STOP THE SERVER FIRST)
             [--dir=] [--sqlite-path=]     Prints the target and aborts without --confirm

ENVIRONMENT (used by migrate):
  SQLITE_PATH   Path to SQLite database file  (default: ledgeralps.db)
  POSTGRES_DSN  PostgreSQL connection string  (if set, PostgreSQL is used)
  JWT_SECRET    Secret key for JWT tokens     (required, min 32 chars)
  PORT          HTTP port                     (default: 8000)
  DEBUG         Enable debug logging          (default: false)

EXAMPLES:
  export JWT_SECRET=$(openssl rand -hex 32)
  ledgeralps-cli migrate
  ledgeralps-cli bootstrap --email=admin@company.ch --password=s3cur3p@ss
  ledgeralps-cli health --url=http://localhost:8000

`, version.Version())
}

// cmdMigrate loads config from env and applies all pending migrations.
func cmdMigrate() {
	cfg := config.Load()
	database, err := db.Open(cfg)
	if err != nil {
		fatalf("cannot open database: %v", err)
	}
	defer database.Close()

	fmt.Println("ledgeralps-cli: applying migrations…")
	if err := db.Migrate(database, cfg.UsePostgres()); err != nil {
		fatalf("migration failed: %v", err)
	}
	fmt.Println("ledgeralps-cli: all migrations up to date.")
}

// cmdBootstrap creates the first admin user by calling POST /api/v1/auth/bootstrap.
func cmdBootstrap(args []string) {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	email := fs.String("email", "", "Admin e-mail address (required)")
	password := fs.String("password", "", "Admin password (required)")
	name := fs.String("name", "Admin", "Admin display name")
	serverURL := fs.String("url", envOrDefault("LEDGERALPS_URL", "http://localhost:8000"), "LedgerAlps server base URL")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "error: --email and --password are required")
		fs.Usage()
		os.Exit(1)
	}

	payload := map[string]string{
		"email":    *email,
		"password": *password,
		"name":     *name,
	}
	body, _ := json.Marshal(payload)

	endpoint := *serverURL + "/api/v1/auth/bootstrap"
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		fatalf("could not build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		fatalf("HTTP request failed: %v\n  Is the server running at %s?", err, *serverURL)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("ledgeralps-cli: bootstrap succeeded (HTTP %d)\n", resp.StatusCode)
		var pretty map[string]any
		if json.Unmarshal(respBody, &pretty) == nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(pretty)
		} else {
			fmt.Println(string(respBody))
		}
	} else {
		fmt.Fprintf(os.Stderr, "ledgeralps-cli: bootstrap failed (HTTP %d)\n%s\n",
			resp.StatusCode, string(respBody))
		os.Exit(1)
	}
}

// cmdHealth calls GET /health and prints the result.
func cmdHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	serverURL := fs.String("url", envOrDefault("LEDGERALPS_URL", "http://localhost:8000"), "LedgerAlps server base URL")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	endpoint := *serverURL + "/health"
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		fatalf("could not build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatalf("HTTP request failed: %v\n  Is the server running at %s?", err, *serverURL)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		fmt.Printf("ledgeralps-cli: server healthy (HTTP %d)\n%s\n", resp.StatusCode, string(body))
	} else {
		fmt.Fprintf(os.Stderr, "ledgeralps-cli: server unhealthy (HTTP %d)\n%s\n",
			resp.StatusCode, string(body))
		os.Exit(1)
	}
}

// cmdBackup writes a consistent snapshot of the database. Safe to run while the
// server is serving requests — SQLite's VACUUM INTO takes care of consistency.
func cmdBackup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	keep := fs.Int("keep", db.DefaultKeep, "number of snapshots to retain")
	dir := fs.String("dir", "", "backup directory (default: <app data>/backups)")
	sqlitePath := fs.String("sqlite-path", "", "database to back up (default: the configured one)")
	passphrase := fs.String("passphrase", "",
		"encrypt the snapshot with this passphrase (or set BACKUP_PASSPHRASE)")
	_ = fs.Parse(args)

	cfg := config.Load()
	applySQLitePath(cfg, *sqlitePath)
	target := backupDirOrDefault(*dir)
	fmt.Printf("ledgeralps-cli: backing up %s\n", cfg.SQLitePath)

	database, err := db.Open(cfg)
	if err != nil {
		fatalf("cannot open database: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pass := resolvePassphrase(*passphrase)
	path, err := db.Backup(ctx, database, cfg, target, pass)
	if err != nil {
		fatalf("backup failed: %v", err)
	}
	if pass == "" {
		if err := db.Verify(ctx, path); err != nil {
			fatalf("backup written to %s but failed verification: %v", path, err)
		}
		fmt.Printf("ledgeralps-cli: backup written and verified: %s\n", path)
		fmt.Println("  NOT encrypted — on removable media, use --passphrase (nLPD art. 8)")
	} else {
		// An encrypted snapshot is not a database, so Verify cannot read it
		// here. Backup already decrypted it and ran the integrity check before
		// removing the plaintext: the same verification, done earlier.
		fmt.Printf("ledgeralps-cli: encrypted backup written and verified: %s\n", path)
		fmt.Println("  Keep the passphrase somewhere other than this machine.")
		fmt.Println("  Without it the snapshot cannot be restored — by anyone, including you.")
	}

	removed, err := db.Prune(target, *keep)
	if err != nil {
		fatalf("backup succeeded but pruning failed: %v", err)
	}
	for _, name := range removed {
		fmt.Printf("  pruned old snapshot: %s\n", name)
	}
}

// cmdBackups lists snapshots, newest first.
func cmdBackups(args []string) {
	fs := flag.NewFlagSet("backups", flag.ExitOnError)
	dir := fs.String("dir", "", "backup directory (default: <app data>/backups)")
	_ = fs.Parse(args)

	target := backupDirOrDefault(*dir)
	list, err := db.ListBackups(target)
	if err != nil {
		fatalf("cannot list backups: %v", err)
	}
	if len(list) == 0 {
		fmt.Printf("ledgeralps-cli: no backups found in %s\n", target)
		return
	}
	fmt.Printf("ledgeralps-cli: %d backup(s) in %s\n\n", len(list), target)
	for _, b := range list {
		// Say which copies are protected: "chiffrée" or not is the difference
		// between a mislaid USB stick being a nuisance and being a breach.
		state := "en clair"
		if enc, _ := db.IsEncrypted(b.Path); enc {
			state = "chiffrée"
		}
		fmt.Printf("  %-56s %8.2f MB  %-8s  %s\n",
			b.Name, float64(b.SizeBytes)/(1024*1024), state, b.CreatedAt.Format(time.RFC3339))
	}
}

// cmdRestore replaces the live database with a snapshot. This swaps the file
// out from under any open connection, so the server must be stopped first.
func cmdRestore(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	file := fs.String("file", "", "snapshot to restore (required)")
	dir := fs.String("dir", "", "backup directory (default: <app data>/backups)")
	sqlitePath := fs.String("sqlite-path", "", "database to overwrite (default: the configured one)")
	confirm := fs.Bool("confirm", false, "required: acknowledge that the target database will be overwritten")
	restorePass := fs.String("passphrase", "",
		"passphrase of an encrypted snapshot (or set BACKUP_PASSPHRASE)")
	_ = fs.Parse(args)

	if *file == "" {
		fatalf("restore requires --file=<snapshot>; run 'ledgeralps-cli backups' to list them")
	}

	cfg := config.Load()
	applySQLitePath(cfg, *sqlitePath)
	target := backupDirOrDefault(*dir)

	// Restore overwrites live accounting records, so the destination is printed
	// and an explicit --confirm is required. Without it the command is a no-op:
	// the configured database is not always the one the caller has in mind.
	if !*confirm {
		fmt.Fprintf(os.Stderr, `ledgeralps-cli: restore would OVERWRITE this database:

    %s

  with the snapshot:

    %s

  The server must be stopped first — restoring under a running server corrupts state.
  Re-run with --confirm to proceed, or pass --sqlite-path to target another database.
`, cfg.SQLitePath, *file)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("ledgeralps-cli: restoring %s over %s\n", *file, cfg.SQLitePath)

	prev, err := db.Restore(ctx, cfg, *file, target, resolvePassphrase(*restorePass))
	if err != nil {
		fatalf("restore failed: %v", err)
	}
	if prev != "" {
		fmt.Printf("  previous database saved to: %s\n", prev)
	}
	fmt.Println("ledgeralps-cli: restore complete.")
}

// applySQLitePath lets a caller point backup/restore at a specific database.
// config.Load() prefers the config file over environment variables, so an
// explicit flag is the only reliable way to override the configured path.
func applySQLitePath(cfg *config.Config, path string) {
	if path == "" {
		return
	}
	cfg.SQLitePath = path
	cfg.PostgresDSN = "" // an explicit SQLite path implies SQLite
}

func backupDirOrDefault(dir string) string {
	if dir != "" {
		return dir
	}
	return db.BackupDir()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ledgeralps-cli: "+format+"\n", args...)
	os.Exit(1)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// resolvePassphrase prefers the flag, falling back to BACKUP_PASSPHRASE.
//
// The environment variable exists because a passphrase on the command line ends
// up in the shell history and, on Linux, in /proc where any local user can read
// it. Scheduled backups should use the variable.
func resolvePassphrase(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("BACKUP_PASSPHRASE")
}

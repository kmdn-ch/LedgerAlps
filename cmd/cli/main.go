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
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
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
	resp, err := client.Get(endpoint)
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

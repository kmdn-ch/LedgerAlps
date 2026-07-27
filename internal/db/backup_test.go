package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

// newTestDB creates a small SQLite database with one table and one row.
func newTestDB(t *testing.T) (*sql.DB, *config.Config) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	cfg := &config.Config{SQLitePath: dbPath}

	database, err := sql.Open("sqlite", "file:"+dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`CREATE TABLE ledger (id INTEGER PRIMARY KEY, memo TEXT)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO ledger (id, memo) VALUES (1, 'facture FA2025-0001')`); err != nil {
		t.Fatalf("seeding row: %v", err)
	}
	return database, cfg
}

func TestBackupProducesVerifiableSnapshot(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()

	path, err := Backup(context.Background(), database, cfg, dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	if err := Verify(context.Background(), path); err != nil {
		t.Errorf("Verify on a fresh snapshot: %v", err)
	}
}

func TestBackupSnapshotContainsData(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()

	path, err := Backup(context.Background(), database, cfg, dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	snap, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening snapshot: %v", err)
	}
	defer snap.Close()

	var memo string
	if err := snap.QueryRow(`SELECT memo FROM ledger WHERE id = 1`).Scan(&memo); err != nil {
		t.Fatalf("reading from snapshot: %v", err)
	}
	if memo != "facture FA2025-0001" {
		t.Errorf("snapshot data = %q, want %q", memo, "facture FA2025-0001")
	}
}

func TestBackupDoesNotOverwriteExistingSnapshot(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()

	first, err := Backup(context.Background(), database, cfg, dir)
	if err != nil {
		t.Fatalf("first Backup: %v", err)
	}
	second, err := Backup(context.Background(), database, cfg, dir)
	if err != nil {
		t.Fatalf("second Backup: %v", err)
	}
	if first == second {
		t.Fatal("two backups in the same second must not resolve to the same path")
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("snapshot %s missing: %v", filepath.Base(p), err)
		}
	}
}

func TestPruneKeepsNewestN(t *testing.T) {
	dir := t.TempDir()
	// Names sort lexicographically by timestamp, so these are ordered oldest→newest.
	names := []string{
		"ledgeralps-2026-01-01T00-00-00.db",
		"ledgeralps-2026-01-02T00-00-00.db",
		"ledgeralps-2026-01-03T00-00-00.db",
		"ledgeralps-2026-01-04T00-00-00.db",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatalf("seeding %s: %v", n, err)
		}
	}

	removed, err := Prune(dir, 2)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed %d snapshots, want 2 (%v)", len(removed), removed)
	}

	left, err := ListBackups(dir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(left) != 2 {
		t.Fatalf("remaining = %d, want 2", len(left))
	}
	// The two newest must survive.
	if left[0].Name != names[3] || left[1].Name != names[2] {
		t.Errorf("Prune kept %q and %q, want the two newest", left[0].Name, left[1].Name)
	}
}

func TestPruneWithNonPositiveKeepDoesNotWipeBackups(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ledgeralps-2026-01-01T00-00-00.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prune(dir, 0); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	left, _ := ListBackups(dir)
	if len(left) != 1 {
		t.Errorf("keep=0 must fall back to the default, not delete everything; remaining = %d", len(left))
	}
}

func TestListBackupsOnMissingDirIsEmptyNotError(t *testing.T) {
	list, err := ListBackups(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("ListBackups on a missing directory should not error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected no backups, got %d", len(list))
	}
}

func TestVerifyRejectsNonDatabase(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(bad, []byte("this is definitely not a sqlite file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), bad); err == nil {
		t.Error("Verify should reject a file that is not a SQLite database")
	}
}

func TestVerifyRejectsMissingFile(t *testing.T) {
	if err := Verify(context.Background(), filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Error("Verify should reject a missing file")
	}
}

func TestRestoreReplacesLiveDatabaseAndSnapshotsPrevious(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()

	// Snapshot the original state, then mutate the live database.
	snapshot, err := Backup(context.Background(), database, cfg, dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if _, err := database.Exec(`UPDATE ledger SET memo = 'modifie apres backup' WHERE id = 1`); err != nil {
		t.Fatalf("mutating: %v", err)
	}
	database.Close() // restore requires the server to be stopped

	prev, err := Restore(context.Background(), cfg, snapshot, dir)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if prev == "" {
		t.Error("Restore should snapshot the database it replaces")
	} else if _, err := os.Stat(prev); err != nil {
		t.Errorf("pre-restore snapshot missing: %v", err)
	}

	reopened, err := sql.Open("sqlite", "file:"+cfg.SQLitePath)
	if err != nil {
		t.Fatalf("reopening restored database: %v", err)
	}
	defer reopened.Close()

	var memo string
	if err := reopened.QueryRow(`SELECT memo FROM ledger WHERE id = 1`).Scan(&memo); err != nil {
		t.Fatalf("reading restored row: %v", err)
	}
	if memo != "facture FA2025-0001" {
		t.Errorf("restored memo = %q, want the pre-mutation value", memo)
	}
}

func TestRestoreRefusesCorruptSource(t *testing.T) {
	_, cfg := newTestDB(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(bad, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), cfg, bad, dir); err == nil {
		t.Error("Restore must refuse a snapshot that fails verification")
	}
}

func TestMaybeAutoBackupRespectsInterval(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()

	first, err := MaybeAutoBackup(context.Background(), database, cfg, dir, time.Hour, DefaultKeep)
	if err != nil {
		t.Fatalf("first MaybeAutoBackup: %v", err)
	}
	if first == "" {
		t.Fatal("expected a backup when none exists yet")
	}

	// A second call inside the interval must be a no-op.
	second, err := MaybeAutoBackup(context.Background(), database, cfg, dir, time.Hour, DefaultKeep)
	if err != nil {
		t.Fatalf("second MaybeAutoBackup: %v", err)
	}
	if second != "" {
		t.Errorf("expected no backup inside the interval, got %s", second)
	}

	// With a zero-length interval a fresh snapshot is due immediately.
	third, err := MaybeAutoBackup(context.Background(), database, cfg, dir, time.Nanosecond, DefaultKeep)
	if err != nil {
		t.Fatalf("third MaybeAutoBackup: %v", err)
	}
	if third == "" {
		t.Error("expected a backup once the interval has elapsed")
	}
}

func TestBackupRejectsPostgres(t *testing.T) {
	database, _ := newTestDB(t)
	cfg := &config.Config{PostgresDSN: "postgres://localhost/ledgeralps"}
	if _, err := Backup(context.Background(), database, cfg, t.TempDir()); err != ErrPostgresUnsupported {
		t.Errorf("Backup on PostgreSQL = %v, want ErrPostgresUnsupported", err)
	}
}

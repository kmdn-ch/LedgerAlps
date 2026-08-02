package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

	path, err := Backup(context.Background(), database, cfg, dir, "")
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

	path, err := Backup(context.Background(), database, cfg, dir, "")
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

	first, err := Backup(context.Background(), database, cfg, dir, "")
	if err != nil {
		t.Fatalf("first Backup: %v", err)
	}
	second, err := Backup(context.Background(), database, cfg, dir, "")
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
	snapshot, err := Backup(context.Background(), database, cfg, dir, "")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if _, err := database.Exec(`UPDATE ledger SET memo = 'modifie apres backup' WHERE id = 1`); err != nil {
		t.Fatalf("mutating: %v", err)
	}
	database.Close() // restore requires the server to be stopped

	prev, err := Restore(context.Background(), cfg, snapshot, dir, "", true)
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
	if _, err := Restore(context.Background(), cfg, bad, dir, "", true); err == nil {
		t.Error("Restore must refuse a snapshot that fails verification")
	}
}

func TestMaybeAutoBackupRespectsInterval(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()

	first, err := MaybeAutoBackup(context.Background(), database, cfg, dir, time.Hour, DefaultKeep, "")
	if err != nil {
		t.Fatalf("first MaybeAutoBackup: %v", err)
	}
	if first == "" {
		t.Fatal("expected a backup when none exists yet")
	}

	// A second call inside the interval must be a no-op.
	second, err := MaybeAutoBackup(context.Background(), database, cfg, dir, time.Hour, DefaultKeep, "")
	if err != nil {
		t.Fatalf("second MaybeAutoBackup: %v", err)
	}
	if second != "" {
		t.Errorf("expected no backup inside the interval, got %s", second)
	}

	// With a zero-length interval a fresh snapshot is due immediately.
	third, err := MaybeAutoBackup(context.Background(), database, cfg, dir, time.Nanosecond, DefaultKeep, "")
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
	if _, err := Backup(context.Background(), database, cfg, t.TempDir(), ""); err != ErrPostgresUnsupported {
		t.Errorf("Backup on PostgreSQL = %v, want ErrPostgresUnsupported", err)
	}
}

// ─── Sauvegardes chiffrées ────────────────────────────────────────────────────

// The whole round trip on a real database: encrypt, restore, and confirm the
// data survived. Unit tests on the cipher prove the bytes come back; this
// proves the ledger does.
func TestEncryptedBackupRestoresTheLedger(t *testing.T) {
	database, cfg := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO ledger (id, memo) VALUES (2, ?)`, "Client Chiffré SA"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	dir := t.TempDir()
	ctx := context.Background()

	path, err := Backup(ctx, database, cfg, dir, "phrase de passe distincte")
	if err != nil {
		t.Fatalf("encrypted backup: %v", err)
	}
	if enc, _ := IsEncrypted(path); !enc {
		t.Fatal("the snapshot was not encrypted")
	}
	// The plaintext must not have been left next to it.
	if _, err := os.Stat(strings.TrimSuffix(path, ".enc")); !os.IsNotExist(err) {
		t.Error("the plaintext snapshot is still on disk beside the encrypted one")
	}

	database.Close()
	if _, err := Restore(ctx, cfg, path, dir, "phrase de passe distincte", true); err != nil {
		t.Fatalf("restore: %v", err)
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	var memo string
	if err := reopened.QueryRow(`SELECT memo FROM ledger WHERE id = 2`).Scan(&memo); err != nil {
		t.Fatalf("the restored database lost its data: %v", err)
	}
	if memo != "Client Chiffré SA" {
		t.Errorf("restored memo = %q, want the original", memo)
	}
}

// A restore attempted with the wrong passphrase must leave the live database
// exactly as it was — this is the moment where a mistake destroys the ledger.
func TestFailedDecryptionLeavesTheLiveDatabaseAlone(t *testing.T) {
	database, cfg := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO ledger (id, memo) VALUES (2, ?)`, "Toujours là SA"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	dir := t.TempDir()
	ctx := context.Background()

	path, err := Backup(ctx, database, cfg, dir, "la bonne")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	database.Close()

	if _, err := Restore(ctx, cfg, path, dir, "la mauvaise", true); err == nil {
		t.Fatal("a restore with the wrong passphrase reported success")
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatalf("the live database no longer opens: %v", err)
	}
	defer reopened.Close()
	var memo string
	if err := reopened.QueryRow(`SELECT memo FROM ledger WHERE id = 2`).Scan(&memo); err != nil {
		t.Errorf("the live database was damaged by a failed restore: %v", err)
	}
}

// Restoring an encrypted snapshot without a passphrase must say so, rather than
// fail on an integrity check that suggests a corrupt backup.
func TestEncryptedRestoreWithoutPassphraseExplainsItself(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	path, err := Backup(ctx, database, cfg, dir, "pass")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	database.Close()

	_, err = Restore(ctx, cfg, path, dir, "", true)
	if err == nil || !strings.Contains(err.Error(), "passe") {
		t.Errorf("got %v, want a message naming the missing passphrase", err)
	}
}

// Passing a passphrase for a snapshot that is not encrypted is a
// misunderstanding worth naming: the user may believe their backups are
// protected when they are not.
func TestPassphraseOnAPlainSnapshotIsRefused(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	path, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	database.Close()

	if _, err := Restore(ctx, cfg, path, dir, "inutile", true); err == nil {
		t.Error("a passphrase was silently ignored on a plain snapshot")
	}
}

// An encrypted snapshot is named .db.enc, and the listing required .db — so it
// existed on disk and appeared nowhere. That is not only a display problem:
// the listing is how a restore resolves a chosen name, how pruning finds old
// copies, and how the startup check decides a backup is due. An encrypted
// backup was effectively unreachable.
func TestListBackupsIncludesEncryptedSnapshots(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	plain, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatalf("plain backup: %v", err)
	}
	encrypted, err := Backup(ctx, database, cfg, dir, "pass")
	if err != nil {
		t.Fatalf("encrypted backup: %v", err)
	}

	list, err := ListBackups(dir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	found := map[string]bool{}
	for _, b := range list {
		found[b.Name] = true
	}
	if !found[filepath.Base(plain)] {
		t.Error("the plaintext snapshot is missing from the listing")
	}
	if !found[filepath.Base(encrypted)] {
		t.Errorf("the encrypted snapshot %q is missing — it exists on disk but cannot be listed, pruned or restored",
			filepath.Base(encrypted))
	}
}

// Pruning must count encrypted snapshots too, or they accumulate forever while
// the plaintext ones rotate.
func TestPruneCountsEncryptedSnapshots(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, err := Backup(ctx, database, cfg, dir, "pass"); err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
	}
	if _, err := Prune(dir, 2); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	list, _ := ListBackups(dir)
	if len(list) != 2 {
		t.Errorf("after pruning to 2, %d snapshots remain", len(list))
	}
}

// The undo copy must inherit the protection the user chose. Someone who
// encrypts their backups did not ask for a clear copy of the whole ledger to
// appear beside them at restore time.
func TestUndoCopyIsEncryptedWhenThePassphraseIs(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "phrase de passe longue 2026")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	database.Close()

	previous, err := Restore(ctx, cfg, snapshot, dir, "phrase de passe longue 2026", true)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if previous == "" {
		t.Fatal("no undo copy was taken")
	}
	if enc, _ := IsEncrypted(previous); !enc {
		t.Errorf("the undo copy %q is in clear although the backup was encrypted", filepath.Base(previous))
	}
	// And no plaintext twin left behind.
	if _, err := os.Stat(strings.TrimSuffix(previous, ".enc")); !os.IsNotExist(err) {
		t.Error("a clear copy was left beside the encrypted undo copy")
	}
}

// Restoring a plaintext snapshot leaves a plaintext undo copy — consistent
// with the choice the user made, not a downgrade.
func TestUndoCopyStaysClearForAClearBackup(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	database.Close()

	previous, err := Restore(ctx, cfg, snapshot, dir, "", true)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if enc, _ := IsEncrypted(previous); enc {
		t.Error("an unexpected encryption was applied")
	}
}

// The staged path takes its undo copy earlier, so applying must not make a
// second one — which could only be written in clear.
func TestApplyDoesNotCreateAClearUndoCopy(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StageRestore(ctx, snapshot, dir, "", "u"); err != nil {
		t.Fatal(err)
	}
	database.Close()

	_, previous, err := ApplyPendingRestore(ctx, cfg, dir)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if previous != "" {
		t.Errorf("applying created an extra copy (%s); the undo was already taken at staging",
			filepath.Base(previous))
	}
}

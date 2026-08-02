package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The flow the Settings panel drives: stage now, apply at the next start.
// Staging must not touch the live database — the server is still using it.
func TestStageDoesNotTouchTheLiveDatabase(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	// Change the live database after the snapshot, so "restored" and "current"
	// are distinguishable.
	if _, err := database.Exec(`INSERT INTO ledger (id, memo) VALUES (2, 'après la sauvegarde')`); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	if _, err := StageRestore(ctx, snapshot, dir, "", "user-1"); err != nil {
		t.Fatalf("StageRestore: %v", err)
	}

	// Still the post-snapshot state: staging changed nothing.
	var memo string
	if err := database.QueryRow(`SELECT memo FROM ledger WHERE id = 2`).Scan(&memo); err != nil {
		t.Errorf("staging altered the live database: %v", err)
	}
	if p := ReadPendingRestore(dir); p == nil {
		t.Error("no pending restore was recorded")
	}
}

func TestApplyPendingRestoreReplacesTheDatabase(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO ledger (id, memo) VALUES (2, 'après la sauvegarde')`); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if _, err := StageRestore(ctx, snapshot, dir, "", "user-1"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	database.Close() // the server stops, as it must before a restore

	applied, previous, err := ApplyPendingRestore(ctx, cfg, dir)
	if err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}
	if applied == "" {
		t.Fatal("nothing was applied")
	}
	if previous == "" {
		t.Error("the replaced database was not snapshotted; a mistaken restore would be final")
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var count int
	if err := reopened.QueryRow(`SELECT COUNT(*) FROM ledger WHERE id = 2`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("the restore did not roll the database back to the snapshot")
	}

	// The marker and the staged plaintext must be gone, or the restore would
	// replay at every subsequent start.
	if p := ReadPendingRestore(dir); p != nil {
		t.Error("the pending restore survived being applied")
	}
	if _, err := os.Stat(filepath.Join(dir, "staged-restore.db")); !os.IsNotExist(err) {
		t.Error("the staged plaintext copy was left on disk")
	}
}

// Nothing staged must be a no-op, not an error: this runs at every start.
func TestApplyWithNothingStagedIsANoOp(t *testing.T) {
	_, cfg := newTestDB(t)
	applied, _, err := ApplyPendingRestore(context.Background(), cfg, t.TempDir())
	if err != nil || applied != "" {
		t.Errorf("got (%q, %v), want no-op", applied, err)
	}
}

// An encrypted snapshot is decrypted while the user is present to supply the
// passphrase. A wrong one must fail there and then — not at the next start,
// when the application would refuse to come back with no explanation.
func TestStageRejectsAWrongPassphraseImmediately(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "la bonne")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := StageRestore(ctx, snapshot, dir, "la mauvaise", "user-1"); err == nil {
		t.Fatal("a wrong passphrase was accepted at staging time")
	}
	if p := ReadPendingRestore(dir); p != nil {
		t.Error("a failed staging left a pending restore behind")
	}
}

func TestStageRequiresThePassphraseOfAnEncryptedSnapshot(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	snapshot, err := Backup(ctx, database, cfg, dir, "pass")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	_, err = StageRestore(ctx, snapshot, dir, "", "user-1")
	if err == nil || !strings.Contains(err.Error(), "passe") {
		t.Errorf("got %v, want a message naming the missing passphrase", err)
	}
}

// Staging twice must replace, not stack: only the last choice can be meant.
func TestStagingTwiceKeepsOnlyTheLast(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx := context.Background()

	first, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Backup(ctx, database, cfg, dir, "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := StageRestore(ctx, first, dir, "", "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := StageRestore(ctx, second, dir, "", "u"); err != nil {
		t.Fatal(err)
	}
	p := ReadPendingRestore(dir)
	if p == nil || p.SourceName != filepath.Base(second) {
		t.Errorf("pending restore = %v, want the second snapshot", p)
	}
}

// A marker pointing at a file that no longer exists must read as "nothing
// staged", not crash the start.
func TestMarkerWithoutItsStagedFileIsIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, pendingRestoreName),
		[]byte(`{"staged_path":"`+filepath.ToSlash(filepath.Join(dir, "absent.db"))+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := ReadPendingRestore(dir); p != nil {
		t.Error("a marker without its staged file was taken as valid")
	}
}

func TestMalformedMarkerIsIgnoredAndRemoved(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, pendingRestoreName)
	if err := os.WriteFile(marker, []byte("{ ce n'est pas du JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := ReadPendingRestore(dir); p != nil {
		t.Error("a malformed marker was accepted")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("a malformed marker was left in place; it would be re-read at every start")
	}
}

// A corrupt snapshot must be refused while the user is present, not at the
// next start.
func TestStageRefusesACorruptSnapshot(t *testing.T) {
	_, _ = newTestDB(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "ledgeralps-20260101-000000.db")
	if err := os.WriteFile(bad, []byte("ceci n'est pas une base"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := StageRestore(context.Background(), bad, dir, "", "u"); err == nil {
		t.Error("a corrupt snapshot was staged")
	}
}

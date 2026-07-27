package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

// LedgerAlps is local-first: the SQLite file on the user's machine is the only
// copy of records the Code des obligations (art. 958f) requires them to keep for
// ten years. Backups are therefore a compliance feature, not a convenience.
//
// Backups use SQLite's VACUUM INTO, which writes a consistent snapshot of the
// database while the server keeps serving requests — no downtime, no partially
// written file, and the result is compacted.

const (
	// BackupTimeFormat orders backups lexicographically by age.
	BackupTimeFormat = "2006-01-02T15-04-05"
	backupPrefix     = "ledgeralps-"
	backupSuffix     = ".db"

	// DefaultKeep is how many snapshots are retained by default.
	DefaultKeep = 14
	// DefaultInterval is the minimum age of the newest backup before the server
	// takes another one automatically at startup.
	DefaultInterval = 24 * time.Hour
)

// BackupDir returns the directory holding backup snapshots.
func BackupDir() string {
	return filepath.Join(config.AppDataDir(), "backups")
}

// ErrPostgresUnsupported is returned when a backup is requested for a
// PostgreSQL deployment, where pg_dump is the correct tool.
var ErrPostgresUnsupported = fmt.Errorf("automatic backup is only supported for SQLite; use pg_dump for PostgreSQL")

// Backup writes a consistent snapshot of the database into dir and returns the
// path of the file created. dir is created if it does not exist.
//
// VACUUM INTO refuses to overwrite an existing file, so a collision within the
// same second is resolved by adding a counter rather than clobbering a snapshot.
func Backup(ctx context.Context, database *sql.DB, cfg *config.Config, dir string) (string, error) {
	if cfg.UsePostgres() {
		return "", ErrPostgresUnsupported
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	dest := filepath.Join(dir, backupPrefix+time.Now().UTC().Format(BackupTimeFormat)+backupSuffix)
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			break
		}
		if i > 100 {
			return "", fmt.Errorf("could not find a free backup filename in %s", dir)
		}
		dest = filepath.Join(dir,
			fmt.Sprintf("%s%s-%d%s", backupPrefix, time.Now().UTC().Format(BackupTimeFormat), i, backupSuffix))
	}

	// VACUUM INTO takes a string literal, not a bind parameter; single quotes in
	// the path are escaped by doubling them per SQL string-literal rules.
	q := fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(dest, "'", "''"))
	if _, err := database.ExecContext(ctx, q); err != nil {
		return "", fmt.Errorf("writing backup snapshot: %w", err)
	}
	return dest, nil
}

// BackupInfo describes one snapshot on disk.
type BackupInfo struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// ListBackups returns the snapshots in dir, newest first. A missing directory
// yields an empty list rather than an error.
func ListBackups(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), backupPrefix) || !strings.HasSuffix(e.Name(), backupSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Path:      filepath.Join(dir, e.Name()),
			Name:      e.Name(),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Prune deletes the oldest snapshots, keeping at most keep of them.
// A non-positive keep is treated as DefaultKeep so a misconfiguration can never
// delete every backup.
func Prune(dir string, keep int) ([]string, error) {
	if keep <= 0 {
		keep = DefaultKeep
	}
	backups, err := ListBackups(dir)
	if err != nil {
		return nil, err
	}
	if len(backups) <= keep {
		return nil, nil
	}

	var removed []string
	for _, b := range backups[keep:] {
		if err := os.Remove(b.Path); err != nil {
			return removed, fmt.Errorf("pruning %s: %w", b.Name, err)
		}
		removed = append(removed, b.Name)
	}
	return removed, nil
}

// Verify opens a snapshot read-only and runs PRAGMA integrity_check, confirming
// the file is a usable SQLite database before it is ever restored over live data.
func Verify(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("backup file: %w", err)
	}
	handle, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return fmt.Errorf("opening backup: %w", err)
	}
	defer handle.Close()

	var result string
	if err := handle.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity check reported: %s", result)
	}
	return nil
}

// Restore replaces the live database at cfg.SQLitePath with the snapshot at src.
//
// The server must not be running: this swaps the file out from under any open
// connection. It is exposed only through the CLI for that reason. Before
// overwriting, the current database is snapshotted into dir so a mistaken
// restore is itself recoverable.
func Restore(ctx context.Context, cfg *config.Config, src, dir string) (backupOfCurrent string, err error) {
	if cfg.UsePostgres() {
		return "", ErrPostgresUnsupported
	}
	if err := Verify(ctx, src); err != nil {
		return "", fmt.Errorf("refusing to restore: %w", err)
	}

	// Snapshot the database being replaced, when one exists.
	if _, statErr := os.Stat(cfg.SQLitePath); statErr == nil {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("creating backup directory: %w", err)
		}
		backupOfCurrent = filepath.Join(dir,
			fmt.Sprintf("%spre-restore-%s%s", backupPrefix, time.Now().UTC().Format(BackupTimeFormat), backupSuffix))
		if err := copyFile(cfg.SQLitePath, backupOfCurrent); err != nil {
			return "", fmt.Errorf("snapshotting current database before restore: %w", err)
		}
	}

	if err := copyFile(src, cfg.SQLitePath); err != nil {
		return backupOfCurrent, fmt.Errorf("restoring snapshot: %w", err)
	}

	// Stale WAL/SHM sidecars describe the replaced database and would corrupt
	// the restored one; SQLite recreates them on next open.
	for _, sidecar := range []string{cfg.SQLitePath + "-wal", cfg.SQLitePath + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return backupOfCurrent, fmt.Errorf("removing stale %s: %w", filepath.Base(sidecar), err)
		}
	}
	return backupOfCurrent, nil
}

// MaybeAutoBackup takes a snapshot when the newest one is older than interval.
// Returns the created path, or "" when a backup was not due.
func MaybeAutoBackup(ctx context.Context, database *sql.DB, cfg *config.Config, dir string, interval time.Duration, keep int) (string, error) {
	if cfg.UsePostgres() {
		return "", nil // not an error: PostgreSQL deployments back up externally
	}
	if interval <= 0 {
		interval = DefaultInterval
	}

	backups, err := ListBackups(dir)
	if err != nil {
		return "", err
	}
	if len(backups) > 0 && time.Since(backups[0].CreatedAt) < interval {
		return "", nil
	}

	path, err := Backup(ctx, database, cfg, dir)
	if err != nil {
		return "", err
	}
	if _, err := Prune(dir, keep); err != nil {
		// The snapshot succeeded; a pruning failure must not mask that.
		return path, err
	}
	return path, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

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
	// Encrypted snapshots keep the .db name and gain a suffix, so a directory
	// listing shows at a glance which copies are protected and which are not.
	encryptedSuffix = ".enc"

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
//
// A non-empty passphrase encrypts the snapshot and leaves only the encrypted
// file behind. Backups are the copy that travels — a NAS, a USB stick — so it
// is the copy most likely to end up somewhere its owner does not control
// (nLPD art. 8).
func Backup(ctx context.Context, database *sql.DB, cfg *config.Config, dir, passphrase string) (string, error) {
	if cfg.UsePostgres() {
		return "", ErrPostgresUnsupported
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	// The counter has to consider the *final* name. When encrypting, the file
	// that survives is dest+".enc"; checking only dest let two encrypted
	// backups within the same second pick the same slot, and the second failed
	// on an already-existing ciphertext instead of taking the next number.
	dest := filepath.Join(dir, backupPrefix+time.Now().UTC().Format(BackupTimeFormat)+backupSuffix)
	for i := 1; ; i++ {
		if free(dest) && free(dest+encryptedSuffix) {
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

	if passphrase == "" {
		return dest, nil
	}
	return encryptInPlace(ctx, dest, passphrase)
}

// encryptInPlace replaces a freshly written snapshot with its encrypted form.
//
// The plaintext is only removed once the ciphertext has been decrypted back and
// the result checked with SQLite's own integrity check. An encrypted backup
// that cannot be decrypted is not a backup — and the moment you find out is
// the moment you needed it.
func encryptInPlace(ctx context.Context, plain, passphrase string) (string, error) {
	enc := plain + encryptedSuffix

	if err := EncryptFile(plain, enc, passphrase); err != nil {
		_ = os.Remove(plain)
		_ = os.Remove(enc)
		return "", fmt.Errorf("chiffrement de la sauvegarde: %w", err)
	}

	verifyPath := plain + ".verify"
	if err := DecryptFile(enc, verifyPath, passphrase); err != nil {
		_ = os.Remove(plain)
		_ = os.Remove(enc)
		_ = os.Remove(verifyPath)
		return "", fmt.Errorf("la sauvegarde chiffrée n'a pas pu être relue: %w", err)
	}
	verifyErr := Verify(ctx, verifyPath)
	_ = os.Remove(verifyPath)
	if verifyErr != nil {
		_ = os.Remove(plain)
		_ = os.Remove(enc)
		return "", fmt.Errorf("la sauvegarde chiffrée est illisible après déchiffrement: %w", verifyErr)
	}

	if err := os.Remove(plain); err != nil {
		return "", fmt.Errorf("suppression de la copie en clair: %w", err)
	}
	return enc, nil
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
		// Both plaintext (.db) and encrypted (.db.enc) snapshots belong here.
		// Requiring .db hid every encrypted backup: it existed on disk and
		// appeared nowhere — not in the interface, and not to the code that
		// resolves a name for a restore, prunes old copies, or decides at
		// startup whether a backup is due.
		if e.IsDir() || !strings.HasPrefix(e.Name(), backupPrefix) || !isSnapshotName(e.Name()) {
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
func Restore(ctx context.Context, cfg *config.Config, src, dir, passphrase string) (backupOfCurrent string, err error) {
	if cfg.UsePostgres() {
		return "", ErrPostgresUnsupported
	}

	// An encrypted snapshot is decrypted to a temporary file first: Verify and
	// the copy below both expect a real SQLite database. The temporary file is
	// removed on every path, including failures — it holds the whole ledger in
	// clear.
	encrypted, err := IsEncrypted(src)
	if err != nil {
		return "", fmt.Errorf("reading snapshot: %w", err)
	}
	if encrypted {
		if passphrase == "" {
			return "", fmt.Errorf("cette sauvegarde est chiffrée: indiquez la phrase de passe (--passphrase)")
		}
		tmp, tmpErr := os.CreateTemp(filepath.Dir(cfg.SQLitePath), "ledgeralps-restore-*.db")
		if tmpErr != nil {
			return "", fmt.Errorf("fichier temporaire: %w", tmpErr)
		}
		plain := tmp.Name()
		tmp.Close()
		// DecryptFile needs to create the file itself (O_EXCL).
		_ = os.Remove(plain)
		defer os.Remove(plain)

		if err := DecryptFile(src, plain, passphrase); err != nil {
			return "", fmt.Errorf("déchiffrement: %w", err)
		}
		src = plain
	} else if passphrase != "" {
		// Saying so beats silently ignoring it: the user may believe they are
		// restoring an encrypted copy when they are not.
		return "", fmt.Errorf("cette sauvegarde n'est pas chiffrée: retirez --passphrase")
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
func MaybeAutoBackup(ctx context.Context, database *sql.DB, cfg *config.Config, dir string, interval time.Duration, keep int, passphrase string) (string, error) {
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

	path, err := Backup(ctx, database, cfg, dir, passphrase)
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

// isSnapshotName reports whether a filename is one of our snapshots, plaintext
// or encrypted.
func isSnapshotName(name string) bool {
	return strings.HasSuffix(name, backupSuffix) ||
		strings.HasSuffix(name, backupSuffix+encryptedSuffix)
}

func free(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

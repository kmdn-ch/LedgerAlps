package db

// Deferred restore.
//
// Restoring swaps the database file out from under every open connection, so it
// cannot be done by a running server — which is exactly the server the user is
// clicking in. The UI therefore *stages* a restore and the swap happens at the
// next start, before the database is opened.
//
// The staged file is a decrypted, verified copy. Decryption happens while the
// user is present and can supply the passphrase; storing the passphrase on disk
// to decrypt later would undo the point of encrypting the backup at all.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

const pendingRestoreName = "pending-restore.json"

// PendingRestore describes a restore staged for the next start.
type PendingRestore struct {
	// StagedPath is a plaintext, integrity-checked copy of the snapshot.
	StagedPath string `json:"staged_path"`
	// SourceName is the backup the user picked, for the log line and the UI.
	SourceName  string    `json:"source_name"`
	RequestedAt time.Time `json:"requested_at"`
	RequestedBy string    `json:"requested_by"`
}

func pendingRestorePath(dir string) string { return filepath.Join(dir, pendingRestoreName) }

// StageRestore prepares a restore without performing it.
//
// The snapshot is decrypted if needed and verified here, while the user is
// still present to correct a wrong passphrase. Finding out at the next start
// that the file was unreadable would leave them with an application that
// refuses to come back and no way to know why.
func StageRestore(ctx context.Context, src, dir, passphrase, requestedBy string) (*PendingRestore, error) {
	if _, err := os.Stat(src); err != nil {
		return nil, fmt.Errorf("sauvegarde introuvable: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	// A previously staged restore that was never applied is replaced, not
	// stacked: only the last choice can be meant.
	if old := ReadPendingRestore(dir); old != nil {
		ClearPendingRestore(dir, old)
	}

	staged := filepath.Join(dir, "staged-restore.db")
	_ = os.Remove(staged)

	encrypted, err := IsEncrypted(src)
	if err != nil {
		return nil, fmt.Errorf("lecture de la sauvegarde: %w", err)
	}
	switch {
	case encrypted && passphrase == "":
		return nil, fmt.Errorf("cette sauvegarde est chiffrée: la phrase de passe est requise")
	case encrypted:
		if err := DecryptFile(src, staged, passphrase); err != nil {
			return nil, err
		}
	case passphrase != "":
		return nil, fmt.Errorf("cette sauvegarde n'est pas chiffrée: aucune phrase de passe n'est attendue")
	default:
		if err := copyFile(src, staged); err != nil {
			return nil, fmt.Errorf("copie de la sauvegarde: %w", err)
		}
	}

	if err := Verify(ctx, staged); err != nil {
		_ = os.Remove(staged)
		return nil, fmt.Errorf("cette sauvegarde est illisible, restauration refusée: %w", err)
	}

	p := &PendingRestore{
		StagedPath:  staged,
		SourceName:  filepath.Base(src),
		RequestedAt: time.Now().UTC(),
		RequestedBy: requestedBy,
	}
	if err := writePendingRestore(dir, p); err != nil {
		_ = os.Remove(staged)
		return nil, err
	}
	return p, nil
}

// ApplyPendingRestore performs a staged restore. It must run before the
// database is opened — that is the whole reason the restore was deferred.
//
// The database being replaced is snapshotted first, so a restore chosen by
// mistake is itself recoverable.
func ApplyPendingRestore(ctx context.Context, cfg *config.Config, dir string) (applied string, previous string, err error) {
	p := ReadPendingRestore(dir)
	if p == nil {
		return "", "", nil
	}
	// Whatever happens below, the marker and the plaintext copy go away: a
	// restore that failed must not be retried silently at every start.
	defer ClearPendingRestore(dir, p)

	// snapshotCurrent=false : la copie d'annulation a été prise au moment de
	// la préparation, quand l'utilisateur était là avec sa phrase de passe. En
	// reprendre une ici la produirait forcément en clair, faute de secret.
	previous, err = Restore(ctx, cfg, p.StagedPath, dir, "", false)
	if err != nil {
		return "", previous, fmt.Errorf("restauration de %s: %w", p.SourceName, err)
	}
	return p.SourceName, previous, nil
}

// ReadPendingRestore returns the staged restore, or nil when there is none.
// A malformed marker is treated as absent and removed: refusing to start
// because of an unreadable marker would strand the user completely.
func ReadPendingRestore(dir string) *PendingRestore {
	data, err := os.ReadFile(pendingRestorePath(dir))
	if err != nil {
		return nil
	}
	var p PendingRestore
	if err := json.Unmarshal(data, &p); err != nil || p.StagedPath == "" {
		_ = os.Remove(pendingRestorePath(dir))
		return nil
	}
	if _, err := os.Stat(p.StagedPath); err != nil {
		// The staged copy is gone; the marker is meaningless.
		_ = os.Remove(pendingRestorePath(dir))
		return nil
	}
	return &p
}

// ClearPendingRestore removes the marker and the staged plaintext copy. The
// copy holds the whole ledger in clear, so it does not outlive its purpose.
func ClearPendingRestore(dir string, p *PendingRestore) {
	if p != nil && p.StagedPath != "" {
		_ = os.Remove(p.StagedPath)
	}
	_ = os.Remove(pendingRestorePath(dir))
}

func writePendingRestore(dir string, p *PendingRestore) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending restore: %w", err)
	}
	return os.WriteFile(pendingRestorePath(dir), data, 0o600)
}

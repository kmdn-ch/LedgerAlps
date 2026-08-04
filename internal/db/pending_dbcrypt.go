package db

// Changement d'état de chiffrement de la base, appliqué au démarrage.
//
// Comme la restauration, la conversion remplace le fichier que chaque connexion
// ouverte utilise : le serveur qui reçoit le clic ne peut pas la faire lui-même.
// Elle est donc préparée, puis appliquée au démarrage suivant, avant que la base
// soit ouverte.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

const pendingEncryptionName = "pending-encryption.json"

// Actions possibles.
const (
	ActionEncrypt = "encrypt"
	ActionDecrypt = "decrypt"
)

// PendingEncryption describes a conversion staged for the next start.
type PendingEncryption struct {
	Action      string    `json:"action"`
	RequestedAt time.Time `json:"requested_at"`
	RequestedBy string    `json:"requested_by"`
}

func pendingEncryptionPath(dir string) string {
	return filepath.Join(dir, pendingEncryptionName)
}

// StageEncryption records the conversion to perform at the next start.
func StageEncryption(dir, action, requestedBy string) (*PendingEncryption, error) {
	if action != ActionEncrypt && action != ActionDecrypt {
		return nil, fmt.Errorf("action inconnue: %q", action)
	}
	p := &PendingEncryption{
		Action:      action,
		RequestedAt: time.Now().UTC(),
		RequestedBy: requestedBy,
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(pendingEncryptionPath(dir), data, 0o600); err != nil {
		return nil, err
	}
	return p, nil
}

// ReadPendingEncryption returns the staged conversion, or nil.
func ReadPendingEncryption(dir string) *PendingEncryption {
	data, err := os.ReadFile(pendingEncryptionPath(dir))
	if err != nil {
		return nil
	}
	var p PendingEncryption
	if err := json.Unmarshal(data, &p); err != nil || p.Action == "" {
		_ = os.Remove(pendingEncryptionPath(dir))
		return nil
	}
	return &p
}

// ClearPendingEncryption removes the marker.
func ClearPendingEncryption(dir string) { _ = os.Remove(pendingEncryptionPath(dir)) }

// ReconcileDatabaseEncryption brings the database file into the state this
// machine is configured for. It must run before the database is opened.
//
// It is deliberately a reconciliation and not just "apply the staged action".
// A restore writes a plaintext snapshot over the database file; without this,
// an installation whose owner had switched encryption on would silently come
// back in clear, and the interface would keep saying it was encrypted. The
// staged marker is one input, the key's existence is the other, and the file on
// disk is the thing being made to agree with both.
//
// Returns a short description of what was done, empty when nothing was needed.
func ReconcileDatabaseEncryption(ctx context.Context, cfg *config.Config, dir string) (string, error) {
	if cfg.UsePostgres() {
		return "", nil
	}
	keys := NewDatabaseKeys(dir)
	pending := ReadPendingEncryption(dir)

	if pending != nil && pending.Action == ActionDecrypt {
		defer ClearPendingEncryption(dir)
		key, err := keys.Key()
		if err != nil {
			return "", fmt.Errorf("déchiffrement impossible: %w", err)
		}
		if err := DecryptDatabaseFile(ctx, cfg.SQLitePath, key); err != nil {
			return "", err
		}
		// La clé ne part qu'après : l'effacer avant laisserait un fichier que
		// rien ne peut ouvrir si la conversion échoue en cours de route.
		if err := keys.Forget(); err != nil {
			return "", fmt.Errorf("base déchiffrée, mais la clé n'a pas pu être effacée: %w", err)
		}
		return "base de données déchiffrée", nil
	}

	if pending != nil {
		ClearPendingEncryption(dir)
	}
	if !keys.Configured() {
		return "", nil
	}

	encrypted, err := IsDatabaseEncrypted(cfg.SQLitePath)
	if err != nil {
		return "", err
	}
	if encrypted {
		return "", nil
	}
	if _, statErr := os.Stat(cfg.SQLitePath); statErr != nil {
		// Pas encore de base : la première ouverture en créera une, et le VFS
		// la chiffrera dès l'origine.
		return "", nil
	}

	key, err := keys.Key()
	if err != nil {
		return "", fmt.Errorf("chiffrement impossible: %w", err)
	}
	if err := EncryptDatabaseFile(ctx, cfg.SQLitePath, key); err != nil {
		return "", err
	}
	return "base de données chiffrée", nil
}

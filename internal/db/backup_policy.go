package db

// Whether automatic backups are encrypted used to depend on an environment
// variable, BACKUP_PASSPHRASE. In practice that meant: never.
//
// Measured on a real installation — the developer's own — the backups directory
// held plaintext SQLite files whose headers, VAT number, e-mail addresses and
// IBAN could be read with no key at all. Up to fourteen full copies of the
// ledger, in a directory people copy to a NAS or a USB stick precisely because
// it is the copy that is supposed to survive.
//
// So the passphrase now lives in the secret store, sealed to the Windows
// account, and every automatic snapshot uses it. The environment variable still
// works and still wins, because a server deployment may set it deliberately.
//
// It stays possible to have no passphrase at all. Someone who cannot produce
// their passphrase after a disk failure has lost ten years of records they are
// legally required to keep (CO art. 958f) — that is a worse outcome than an
// unencrypted file, and it is not a decision to make on the user's behalf. What
// changed is that the choice is now made in the open, and the interface says
// which one is in force.

import (
	"context"
	"fmt"
	"os"

	"github.com/kmdn-ch/ledgeralps/internal/core/secretstore"
)

// PassphraseSource says where the passphrase in force came from, so the
// interface can explain itself instead of showing a state with no cause.
type PassphraseSource string

const (
	// SourceNone — no passphrase: automatic snapshots are written in clear.
	SourceNone PassphraseSource = "none"
	// SourceStored — set in the interface, kept in the secret store.
	SourceStored PassphraseSource = "stored"
	// SourceEnv — BACKUP_PASSPHRASE, which a server deployment may set.
	SourceEnv PassphraseSource = "env"
	// SourceUnavailable — a passphrase is stored but could not be unsealed on
	// this machine or account. Distinct from "none": the difference decides
	// whether the user should be asked to set one or told why theirs is
	// unreadable.
	SourceUnavailable PassphraseSource = "unavailable"
)

// BackupPolicy resolves the passphrase automatic backups are taken with.
type BackupPolicy struct {
	store *secretstore.Store
}

// NewBackupPolicy reads and writes the passphrase kept in dir.
func NewBackupPolicy(dir string) *BackupPolicy {
	return &BackupPolicy{store: secretstore.New(dir)}
}

// Passphrase returns the passphrase in force and where it came from.
//
// The environment variable takes precedence over the stored one: an
// administrator who sets it on a server has made an explicit deployment
// decision, and silently preferring a value set months earlier in the interface
// would produce snapshots nobody at that site can open.
func (p *BackupPolicy) Passphrase() (string, PassphraseSource) {
	if v := os.Getenv("BACKUP_PASSPHRASE"); v != "" {
		return v, SourceEnv
	}
	if p == nil || p.store == nil {
		return "", SourceNone
	}
	if !p.store.Has(secretstore.NameBackupPassphrase) {
		return "", SourceNone
	}
	secret, err := p.store.Get(secretstore.NameBackupPassphrase)
	if err != nil {
		return "", SourceUnavailable
	}
	return string(secret), SourceStored
}

// Set records the passphrase used for every automatic snapshot from now on.
// Existing snapshots are untouched — see EncryptExisting.
func (p *BackupPolicy) Set(passphrase string) error {
	if err := ValidatePassphrase(passphrase); err != nil {
		return err
	}
	return p.store.Set(secretstore.NameBackupPassphrase, []byte(passphrase))
}

// Clear removes the stored passphrase. Automatic snapshots go back to being
// written in clear, which is why the caller must have said so out loud.
func (p *BackupPolicy) Clear() error {
	return p.store.Delete(secretstore.NameBackupPassphrase)
}

// PolicyStatus is what the interface needs to describe the current state
// without guessing.
type PolicyStatus struct {
	Source PassphraseSource `json:"source"`
	// Encrypting is the single fact that matters: will the next automatic
	// backup be encrypted? Derived here so no caller has to re-derive it and
	// get it backwards.
	Encrypting bool `json:"encrypting"`
	// Sealed reports whether the platform sealed the stored passphrase to the
	// account, or whether file permissions are the whole protection.
	Sealed    bool   `json:"sealed"`
	Mechanism string `json:"mechanism"`
	// PlaintextCount is how many snapshots on disk are readable with no key.
	// Setting a passphrase does not retroactively protect them, and a count is
	// the only honest way to say so.
	PlaintextCount int `json:"plaintext_count"`
	// EncryptedCount is how many snapshots would become unopenable if the
	// stored passphrase were removed and nobody had written it down.
	//
	// Compté pour que l'avertissement soit concret : « vos 7 sauvegardes
	// chiffrées » se lit, « vos sauvegardes chiffrées » se survole.
	EncryptedCount int `json:"encrypted_count"`
}

// Status describes the policy and what is actually on disk.
func (p *BackupPolicy) Status(dir string) PolicyStatus {
	_, source := p.Passphrase()
	st := PolicyStatus{
		Source:     source,
		Encrypting: source == SourceStored || source == SourceEnv,
		Sealed:     secretstore.Sealed(),
		Mechanism:  secretstore.Mechanism(),
	}
	if entries, err := ListBackups(dir); err == nil {
		for _, e := range entries {
			enc, encErr := IsEncrypted(e.Path)
			if encErr != nil {
				continue
			}
			if enc {
				st.EncryptedCount++
			} else {
				st.PlaintextCount++
			}
		}
	}
	return st
}

// EncryptExisting encrypts the snapshots already on disk that are still in
// clear, and reports which ones it converted.
//
// Setting a passphrase protects what comes next; it does nothing about the
// copies already written, and those are the ones that have had time to be
// copied onto a NAS. Each file is encrypted, decrypted back and integrity
// checked before the plaintext is removed — an encrypted backup that cannot be
// decrypted is not a backup, and the moment you find out is the moment you
// needed it.
func EncryptExisting(ctx context.Context, dir, passphrase string) ([]string, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("aucune phrase de passe: rien à chiffrer")
	}
	if err := ValidatePassphrase(passphrase); err != nil {
		return nil, err
	}
	entries, err := ListBackups(dir)
	if err != nil {
		return nil, err
	}

	var done []string
	for _, e := range entries {
		encrypted, encErr := IsEncrypted(e.Path)
		if encErr != nil || encrypted {
			continue
		}
		if _, err := encryptInPlace(ctx, e.Path, passphrase); err != nil {
			// Stop at the first failure and report what was converted. Carrying
			// on would leave the user unable to tell which copies are now
			// protected and which are not.
			return done, fmt.Errorf("%s: %w", e.Name, err)
		}
		done = append(done, e.Name)
	}
	return done, nil
}

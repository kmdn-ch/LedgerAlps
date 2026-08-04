// Package secretstore keeps small secrets on disk so LedgerAlps can use them
// without asking the user for anything at every start.
//
// Two secrets need this. The passphrase that encrypts automatic backups, and —
// when database encryption is switched on — the database key itself. Both have
// to be available before anyone has logged in, which rules out deriving them
// from the user's password: the server must open the database in order to read
// the users table at all.
//
// # What actually protects the file
//
// On Windows, DPAPI (CryptProtectData). The operating system seals the bytes to
// the current user account on this machine; another account cannot unseal them,
// and neither can the same file copied elsewhere. This is what Chrome uses for
// saved passwords. It needs no administrator rights and shows no prompt —
// measured, not assumed.
//
// Everywhere else there is no equivalent, so the file's permissions (0600) are
// the whole protection. That is weaker and the interface says so rather than
// implying a guarantee that is not there. Overstating protection is worse than
// having less of it: someone who believes their backups are sealed to their
// account will handle them accordingly.
//
// # What this does not protect against
//
// Anything running as the same user. A program with the user's rights can ask
// DPAPI to unseal exactly as LedgerAlps does. The threat this addresses is the
// file being read from somewhere else — another account, a copied profile, a
// disk pulled out of the machine — not malware already inside the session.
package secretstore

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound is returned when the named secret has never been stored.
var ErrNotFound = errors.New("secret absent")

// Names of the secrets LedgerAlps stores. Kept here so a typo cannot silently
// create a second, empty secret under a near-miss name.
const (
	// NameBackupPassphrase encrypts automatic backup snapshots.
	NameBackupPassphrase = "backup_passphrase"
	// NameDatabaseKey is the 32-byte key of an encrypted database.
	NameDatabaseKey = "database_key"
)

// sealNone marks a value the platform could not seal. It is still base64 in a
// 0600 file — protection by file permissions, and nothing more.
const (
	sealNone  = "none"
	sealDPAPI = "dpapi"
)

type entry struct {
	Seal string `json:"seal"`
	Data string `json:"data"`
}

type file struct {
	Version int              `json:"version"`
	Entries map[string]entry `json:"entries"`
}

// Store is a set of named secrets in one file.
type Store struct {
	path string
	mu   sync.Mutex
}

// New returns the store kept in dir. The file is created on first write.
func New(dir string) *Store {
	return &Store{path: filepath.Join(dir, "secrets.json")}
}

// Path is where the secrets live, for diagnostics and for the uninstaller.
func (s *Store) Path() string { return s.path }

// Mechanism names what protects the stored secrets, for the interface to show.
func Mechanism() string {
	if sealAvailable() {
		return "DPAPI — scellé au compte Windows"
	}
	return "droits du fichier uniquement"
}

// Sealed reports whether the platform can seal secrets to the account. When it
// cannot, callers should say so instead of implying a protection that is absent.
func Sealed() bool { return sealAvailable() }

// Set stores a secret under name, replacing any previous value.
func (s *Store) Set(name string, secret []byte) error {
	if len(secret) == 0 {
		return errors.New("secret vide")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.read()
	if err != nil {
		return err
	}

	e := entry{Seal: sealNone}
	if sealed, sealErr := seal(secret); sealErr == nil {
		e = entry{Seal: sealDPAPI, Data: base64.StdEncoding.EncodeToString(sealed)}
	} else if sealAvailable() {
		// The platform claims to support sealing and then failed. Storing the
		// secret in clear at that point would quietly downgrade the protection
		// the user was told they had.
		return fmt.Errorf("scellement du secret: %w", sealErr)
	} else {
		e.Data = base64.StdEncoding.EncodeToString(secret)
	}

	if f.Entries == nil {
		f.Entries = map[string]entry{}
	}
	f.Entries[name] = e
	return s.write(f)
}

// Get returns the secret stored under name.
func (s *Store) Get(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.read()
	if err != nil {
		return nil, err
	}
	e, ok := f.Entries[name]
	if !ok {
		return nil, ErrNotFound
	}
	raw, err := base64.StdEncoding.DecodeString(e.Data)
	if err != nil {
		return nil, fmt.Errorf("secret %q illisible: %w", name, err)
	}
	if e.Seal == sealDPAPI {
		out, unsealErr := unseal(raw)
		if unsealErr != nil {
			// This is the "new machine, restored profile, reinstalled Windows"
			// case. It has to be distinguishable from a corrupt file, because
			// the answer differs: one needs the recovery passphrase, the other
			// needs the file thrown away.
			return nil, fmt.Errorf("%w: ce secret a été scellé sur un autre compte ou une autre machine", unsealErr)
		}
		return out, nil
	}
	return raw, nil
}

// Has reports whether name is stored, without unsealing it. Used by the
// interface to show state on a machine where unsealing would fail.
func (s *Store) Has(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return false
	}
	_, ok := f.Entries[name]
	return ok
}

// Delete removes a secret. Removing one that is not there is not an error.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	if _, ok := f.Entries[name]; !ok {
		return nil
	}
	delete(f.Entries, name)
	return s.write(f)
}

func (s *Store) read() (file, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return file{Version: 1, Entries: map[string]entry{}}, nil
	}
	if err != nil {
		return file{}, fmt.Errorf("lecture de %s: %w", s.path, err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("%s est illisible: %w", s.path, err)
	}
	if f.Entries == nil {
		f.Entries = map[string]entry{}
	}
	return f, nil
}

// write replaces the file atomically. A half-written secrets file would lose
// both secrets at once, and the database key is not something to lose.
func (s *Store) write(f file) error {
	f.Version = 1
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("écriture de %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remplacement de %s: %w", s.path, err)
	}
	return nil
}

// NewKey returns cryptographically random bytes for use as a database key.
func NewKey(n int) ([]byte, error) {
	k := make([]byte, n)
	if _, err := rand.Read(k); err != nil {
		return nil, fmt.Errorf("génération de clé: %w", err)
	}
	return k, nil
}

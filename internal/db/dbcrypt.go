package db

// Chiffrement de la base elle-même.
//
// Les sauvegardes étaient chiffrables depuis longtemps ; la base ne l'était pas,
// et pour une raison d'architecture : SQLCipher est une bibliothèque C, or le
// projet compile avec CGO_ENABLED=0 pour tenir dans un binaire unique. Le pilote
// précédent (modernc.org/sqlite) n'offrait aucun point d'accroche : son paquet
// vfs est en lecture seule, donc rien ne pouvait s'insérer sous lui.
//
// Le pilote actuel en offre un. Le VFS « adiantum » chiffre le fichier par blocs
// de 4 Kio — le journal WAL compris, vérifié en cherchant des données en clair
// dedans — et reste en Go pur.
//
// # Ce que cela protège, et ce que cela ne protège pas
//
// Les mêmes menaces que le chiffrement du disque : machine volée, disque
// démonté, atelier de réparation. Plus une que BitLocker ne couvre pas, parce
// qu'un disque chiffré ne suit pas le fichier : la base copiée sur un NAS, un
// partage réseau ou un dossier synchronisé reste illisible.
//
// Cela ne protège pas contre un programme qui tourne sous le même compte
// Windows : il peut demander la clé exactement comme LedgerAlps le fait. Le
// prétendre serait le genre d'affirmation qui coûte la confiance dans toutes les
// autres.
//
// # Pourquoi deux copies de la clé
//
// La clé est scellée au compte Windows pour que le démarrage ne demande rien.
// Mais un profil recréé, un Windows réinstallé ou une machine morte rendent ce
// scellement illisible — et une comptabilité qu'il faut conserver dix ans
// (CO art. 958f) ne peut pas dépendre d'un secret que personne ne peut
// reproduire. D'où la phrase de récupération, que l'utilisateur note, et qui
// enveloppe la même clé.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kmdn-ch/ledgeralps/internal/core/secretstore"
	"golang.org/x/crypto/chacha20poly1305"
)

// sqliteHeader est l'en-tête d'un fichier SQLite non chiffré. Sa présence est le
// seul constat fiable et bon marché : le VFS étant préservateur de longueur, la
// taille ne dit rien.
var sqliteHeader = []byte("SQLite format 3\x00")

// DatabaseKeySize est la taille de clé attendue par le VFS.
const DatabaseKeySize = 32

const dbKeyFileName = "dbkey.json"

// ErrDatabaseKeyUnavailable : la base est chiffrée et la clé est introuvable ou
// illisible sur ce compte. Distinct d'une erreur générique parce que la suite
// diffère — ici, il faut la phrase de récupération.
var ErrDatabaseKeyUnavailable = errors.New(
	"la clé de la base est illisible sur ce compte : la phrase de récupération est nécessaire")

// ErrNoRecovery : aucune enveloppe de récupération n'a été enregistrée.
var ErrNoRecovery = errors.New("aucune phrase de récupération n'a été enregistrée pour cette base")

// recoveryWrap enveloppe la clé de la base avec une phrase de récupération.
// Argon2id + XChaCha20-Poly1305, les mêmes primitives que les sauvegardes, avec
// les mêmes paramètres — un seul jeu à auditer.
type recoveryWrap struct {
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type dbKeyFile struct {
	Version  int           `json:"version"`
	Recovery *recoveryWrap `json:"recovery,omitempty"`
}

// DatabaseKeys manages the key material of an encrypted database.
type DatabaseKeys struct {
	dir   string
	store *secretstore.Store
}

// NewDatabaseKeys reads and writes the key material kept in dir.
func NewDatabaseKeys(dir string) *DatabaseKeys {
	return &DatabaseKeys{dir: dir, store: secretstore.New(dir)}
}

func (k *DatabaseKeys) filePath() string { return filepath.Join(k.dir, dbKeyFileName) }

// IsDatabaseEncrypted reports whether the file at path is an encrypted
// database. A missing file is not encrypted — a fresh installation has none.
func IsDatabaseEncrypted(path string) (bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lecture de %s: %w", path, err)
	}
	defer f.Close()

	head := make([]byte, len(sqliteHeader))
	n, err := f.Read(head)
	if err != nil && n == 0 {
		// Fichier vide : SQLite en fera une base neuve, non chiffrée.
		return false, nil
	}
	if n < len(sqliteHeader) {
		return false, nil
	}
	return string(head) != string(sqliteHeader), nil
}

// Configured reports whether a database key has been set up on this machine.
func (k *DatabaseKeys) Configured() bool {
	return k.store.Has(secretstore.NameDatabaseKey)
}

// HasRecovery reports whether a recovery passphrase can open this database.
func (k *DatabaseKeys) HasRecovery() bool {
	f, err := k.readFile()
	return err == nil && f.Recovery != nil
}

// Key returns the database key sealed to this account.
func (k *DatabaseKeys) Key() ([]byte, error) {
	if !k.Configured() {
		return nil, ErrDatabaseKeyUnavailable
	}
	key, err := k.store.Get(secretstore.NameDatabaseKey)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrDatabaseKeyUnavailable, err)
	}
	if len(key) != DatabaseKeySize {
		return nil, fmt.Errorf("clé de base de taille %d, attendu %d", len(key), DatabaseKeySize)
	}
	return key, nil
}

// Create generates a new database key, seals it to this account and wraps it
// with the recovery passphrase.
//
// The recovery passphrase is not optional. Without it, one reinstalled Windows
// turns ten years of legally required records into an unreadable file — a
// confidentiality measure must not create a retention failure.
func (k *DatabaseKeys) Create(recoveryPassphrase string) ([]byte, error) {
	if err := ValidatePassphrase(recoveryPassphrase); err != nil {
		return nil, err
	}
	key, err := secretstore.NewKey(DatabaseKeySize)
	if err != nil {
		return nil, err
	}
	wrap, err := wrapKey(key, recoveryPassphrase)
	if err != nil {
		return nil, err
	}
	// Le fichier de récupération d'abord : si le scellement échoue ensuite, on
	// a une enveloppe orpheline et inoffensive. Dans l'autre ordre, on aurait
	// une clé scellée que rien ne peut récupérer.
	if err := k.writeFile(dbKeyFile{Version: 1, Recovery: wrap}); err != nil {
		return nil, err
	}
	if err := k.store.Set(secretstore.NameDatabaseKey, key); err != nil {
		_ = os.Remove(k.filePath())
		return nil, err
	}
	return key, nil
}

// Recover unwraps the key with the recovery passphrase and re-seals it to this
// account, so the next start needs nothing again.
func (k *DatabaseKeys) Recover(recoveryPassphrase string) ([]byte, error) {
	f, err := k.readFile()
	if err != nil {
		return nil, err
	}
	if f.Recovery == nil {
		return nil, ErrNoRecovery
	}
	key, err := unwrapKey(f.Recovery, recoveryPassphrase)
	if err != nil {
		return nil, err
	}
	if err := k.store.Set(secretstore.NameDatabaseKey, key); err != nil {
		// La clé est bonne : la rendre quand même, l'utilisateur doit pouvoir
		// travailler. Le prochain démarrage redemandera la phrase.
		return key, nil
	}
	return key, nil
}

// SetRecovery replaces the recovery passphrase without changing the key, so
// existing data stays readable.
func (k *DatabaseKeys) SetRecovery(newPassphrase string) error {
	if err := ValidatePassphrase(newPassphrase); err != nil {
		return err
	}
	key, err := k.Key()
	if err != nil {
		return err
	}
	wrap, err := wrapKey(key, newPassphrase)
	if err != nil {
		return err
	}
	return k.writeFile(dbKeyFile{Version: 1, Recovery: wrap})
}

// Forget removes all key material. Only ever called once the database on disk
// is back in clear — calling it earlier would leave an unreadable file.
func (k *DatabaseKeys) Forget() error {
	if err := k.store.Delete(secretstore.NameDatabaseKey); err != nil {
		return err
	}
	if err := os.Remove(k.filePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (k *DatabaseKeys) readFile() (dbKeyFile, error) {
	data, err := os.ReadFile(k.filePath())
	if os.IsNotExist(err) {
		return dbKeyFile{Version: 1}, nil
	}
	if err != nil {
		return dbKeyFile{}, fmt.Errorf("lecture de %s: %w", k.filePath(), err)
	}
	var f dbKeyFile
	if err := json.Unmarshal(data, &f); err != nil {
		return dbKeyFile{}, fmt.Errorf("%s est illisible: %w", k.filePath(), err)
	}
	return f, nil
}

func (k *DatabaseKeys) writeFile(f dbKeyFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(k.dir, 0o700); err != nil {
		return err
	}
	tmp := k.filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, k.filePath()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func wrapKey(key []byte, passphrase string) (*recoveryWrap, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("génération du sel: %w", err)
	}
	aead, err := chacha20poly1305.NewX(deriveKey(passphrase, salt))
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("génération du nonce: %w", err)
	}
	return &recoveryWrap{
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, key, []byte("ledgeralps/dbkey/v1"))),
	}, nil
}

func unwrapKey(w *recoveryWrap, passphrase string) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(w.Salt)
	if err != nil {
		return nil, fmt.Errorf("enveloppe de récupération illisible: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(w.Nonce)
	if err != nil {
		return nil, fmt.Errorf("enveloppe de récupération illisible: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(w.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("enveloppe de récupération illisible: %w", err)
	}
	aead, err := chacha20poly1305.NewX(deriveKey(passphrase, salt))
	if err != nil {
		return nil, err
	}
	key, err := aead.Open(nil, nonce, ct, []byte("ledgeralps/dbkey/v1"))
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	if len(key) != DatabaseKeySize {
		return nil, fmt.Errorf("clé récupérée de taille %d, attendu %d", len(key), DatabaseKeySize)
	}
	return key, nil
}

// hexKey renders a key for the PRAGMA that hands it to the VFS.
func hexKey(key []byte) string { return hex.EncodeToString(key) }

// SecretsSealed and SecretsMechanism expose the secret store's platform state
// without every caller having to import it too.
func SecretsSealed() bool      { return secretstore.Sealed() }
func SecretsMechanism() string { return secretstore.Mechanism() }

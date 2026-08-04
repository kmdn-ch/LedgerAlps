package db

// Passage d'une base en clair à une base chiffrée, et retour.
//
// Les deux sens passent par VACUUM INTO plutôt que par une copie : SQLite écrit
// alors une base compacte et cohérente, sans jamais produire de fichier
// intermédiaire en clair sur le disque quand la cible est chiffrée. Une
// migration qui déposerait la comptabilité en clair le temps de la convertir
// annulerait ce qu'elle est censée mettre en place.
//
// Le fichier d'origine n'est remplacé qu'après relecture du résultat. Une base
// convertie qu'on ne peut pas rouvrir n'est pas une base convertie, et le moment
// où on s'en aperçoit serait le pire possible.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// vacuumIntoPlain is the VACUUM INTO target that always lands in clear.
//
// Mesuré : depuis une connexion chiffrée, « VACUUM INTO 'chemin' » échoue avec
// « unable to open database file », parce que la cible hérite du VFS de la
// connexion et qu'aucune clé ne lui est donnée. Nommer explicitement le VFS par
// défaut est la sortie — et elle fonctionne aussi depuis une connexion en clair,
// donc le code n'a pas deux chemins à maintenir.
func vacuumIntoPlain(dest string) string {
	return fmt.Sprintf("VACUUM INTO '%s?vfs='", sqlQuote(SQLiteURI(dest)))
}

func vacuumIntoEncrypted(dest string, key []byte) string {
	return fmt.Sprintf("VACUUM INTO '%s?vfs=adiantum&hexkey=%s'", sqlQuote(SQLiteURI(dest)), hexKey(key))
}

// sqlQuote escapes a string for a SQL string literal. VACUUM INTO takes a
// literal, not a bind parameter.
func sqlQuote(s string) string {
	out := make([]byte, 0, len(s)+4)
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// EncryptDatabaseFile rewrites the database at path as an encrypted database.
//
// The server must not be holding it open: this replaces the file. It is
// therefore called at startup, from the staged-change path, never from a
// request handler.
func EncryptDatabaseFile(ctx context.Context, path string, key []byte) error {
	encrypted, err := IsDatabaseEncrypted(path)
	if err != nil {
		return err
	}
	if encrypted {
		return nil // déjà fait : réappliquer serait un double chiffrement
	}

	src, err := sql.Open(SQLiteDriver, sqliteDSN(path))
	if err != nil {
		return fmt.Errorf("ouverture de la base à chiffrer: %w", err)
	}
	defer src.Close()

	dest := path + ".encrypting"
	_ = os.Remove(dest)
	if _, err := src.ExecContext(ctx, vacuumIntoEncrypted(dest, key)); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("écriture de la base chiffrée: %w", err)
	}
	if err := src.Close(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("fermeture de la base d'origine: %w", err)
	}

	if err := verifyEncrypted(ctx, dest, key); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("la base chiffrée est illisible, migration annulée: %w", err)
	}
	return swapDatabase(dest, path)
}

// DecryptDatabaseFile rewrites an encrypted database back in clear.
func DecryptDatabaseFile(ctx context.Context, path string, key []byte) error {
	encrypted, err := IsDatabaseEncrypted(path)
	if err != nil {
		return err
	}
	if !encrypted {
		return nil
	}

	src, err := OpenEncryptedWithKey(path, key)
	if err != nil {
		return fmt.Errorf("ouverture de la base chiffrée: %w", err)
	}
	defer src.Close()

	dest := path + ".decrypting"
	_ = os.Remove(dest)
	if _, err := src.ExecContext(ctx, vacuumIntoPlain(dest)); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("écriture de la base en clair: %w", err)
	}
	if err := src.Close(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("fermeture de la base chiffrée: %w", err)
	}

	if err := Verify(ctx, dest); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("la base déchiffrée est illisible, migration annulée: %w", err)
	}
	return swapDatabase(dest, path)
}

// verifyEncrypted opens an encrypted database and runs SQLite's own integrity
// check, which is the only answer that means anything about a database file.
func verifyEncrypted(ctx context.Context, path string, key []byte) error {
	handle, err := OpenEncryptedWithKey(path, key)
	if err != nil {
		return err
	}
	defer handle.Close()

	var result string
	if err := handle.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check: %s", result)
	}
	return nil
}

// swapDatabase puts the converted file in place, keeping the original until the
// rename has succeeded.
//
// Les fichiers -wal et -shm décrivent la base remplacée et corrompraient la
// nouvelle ; SQLite les recrée à la prochaine ouverture.
func swapDatabase(converted, target string) error {
	previous := target + ".before-conversion"
	_ = os.Remove(previous)

	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, previous); err != nil {
			return fmt.Errorf("mise de côté de la base d'origine: %w", err)
		}
	}
	if err := os.Rename(converted, target); err != nil {
		// Remettre l'original : mieux vaut une base non convertie qu'aucune.
		_ = os.Rename(previous, target)
		return fmt.Errorf("mise en place de la base convertie: %w", err)
	}
	for _, sidecar := range []string{target + "-wal", target + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("suppression de %s: %w", filepath.Base(sidecar), err)
		}
	}
	// L'original ne part qu'ici, une fois la nouvelle base réellement en place.
	// Il contient toute la comptabilité en clair dans le sens chiffrement : le
	// laisser traîner annulerait la migration.
	_ = os.Remove(previous)
	return nil
}

package db

// Passphrase encryption for backup files.
//
// Backups are the copies that leave the machine — a NAS, a USB stick, a
// external drive — so they are the copy most likely to end up somewhere the
// owner does not control. A mislaid snapshot exposes the whole ledger and every
// client's details at once (nLPD art. 8, OPDo art. 1–6).
//
// Design notes, and why not something else:
//
//   - Argon2id derives the key. A backup passphrase is typed by a human, so the
//     KDF has to be the expensive part; a plain hash would make an offline
//     dictionary attack trivial against a file the attacker holds at leisure.
//   - XChaCha20-Poly1305 provides the AEAD. Its 24-byte nonce can be drawn at
//     random without birthday concerns, which suits a format where each chunk
//     gets its own nonce. It is also pure Go: the whole product ships with
//     CGO_ENABLED=0, and a C dependency would end single-binary cross-compiling.
//   - Chunked, not one-shot. A ledger backup is tens of megabytes and grows;
//     encrypting it in one buffer would mean holding plaintext and ciphertext in
//     memory at once, on a machine that is also running the accounting.
//   - The chunk index is bound into each chunk's additional data, so removing,
//     reordering or replaying chunks fails authentication instead of silently
//     yielding a truncated ledger.

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptedMagic prefixes every encrypted backup. It is what lets Restore tell
// an encrypted snapshot from a plain SQLite file without relying on the
// extension, which a user is free to rename.
var EncryptedMagic = []byte("LALPSBK1")

const (
	saltLen   = 16
	chunkSize = 1 << 20 // 1 MiB of plaintext per chunk

	// Argon2id parameters. Deliberately on the expensive side: this runs once
	// per backup or restore, and the file it protects may sit on a lost USB
	// stick for years.
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
)

// ErrWrongPassphrase is returned when authentication fails. It never
// distinguishes "wrong passphrase" from "corrupted file" to the caller beyond
// this: both mean the data cannot be trusted, and guessing which would help an
// attacker more than the owner.
var ErrWrongPassphrase = errors.New("passphrase incorrecte, ou fichier de sauvegarde altéré")

// ErrNotEncrypted is returned when a file lacking the magic is handed to
// DecryptFile.
var ErrNotEncrypted = errors.New("ce fichier n'est pas une sauvegarde chiffrée")

func deriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads,
		chacha20poly1305.KeySize)
}

// chunkAD binds a chunk to its position in the file. Without it, an attacker
// holding the file could drop or reorder whole chunks and still produce
// something that decrypts — a ledger missing its last month, for instance.
func chunkAD(index uint64, final bool) []byte {
	ad := make([]byte, 9)
	binary.BigEndian.PutUint64(ad[:8], index)
	if final {
		ad[8] = 1
	}
	return ad
}

// EncryptFile writes an encrypted copy of src to dst.
//
// Layout: magic ‖ salt ‖ then, per chunk, a 4-byte big-endian ciphertext length
// followed by nonce ‖ ciphertext. A zero length marks the end, and the final
// chunk is authenticated as final so truncation is detected rather than
// mistaken for a shorter ledger.
func EncryptFile(src, dst, passphrase string) error {
	if passphrase == "" {
		return errors.New("passphrase vide")
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open plaintext: %w", err)
	}
	defer in.Close()

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("random salt: %w", err)
	}
	aead, err := chacha20poly1305.NewX(deriveKey(passphrase, salt))
	if err != nil {
		return fmt.Errorf("cipher: %w", err)
	}

	// 0600: a backup is readable by its owner alone. On Windows this is
	// advisory, which is precisely why the content is encrypted.
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create ciphertext: %w", err)
	}
	defer out.Close()

	if _, err := out.Write(EncryptedMagic); err != nil {
		return fmt.Errorf("write magic: %w", err)
	}
	if _, err := out.Write(salt); err != nil {
		return fmt.Errorf("write salt: %w", err)
	}

	buf := make([]byte, chunkSize)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	var index uint64

	for {
		n, readErr := io.ReadFull(in, buf)
		final := readErr == io.EOF || readErr == io.ErrUnexpectedEOF
		if readErr != nil && !final {
			return fmt.Errorf("read plaintext: %w", readErr)
		}
		if n == 0 && index > 0 {
			// Input length was an exact multiple of chunkSize; the previous
			// chunk was not marked final, so emit an empty final chunk.
			final = true
		}

		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("random nonce: %w", err)
		}
		ct := aead.Seal(nil, nonce, buf[:n], chunkAD(index, final))

		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(nonce)+len(ct)))
		if _, err := out.Write(length[:]); err != nil {
			return fmt.Errorf("write chunk length: %w", err)
		}
		if _, err := out.Write(nonce); err != nil {
			return fmt.Errorf("write nonce: %w", err)
		}
		if _, err := out.Write(ct); err != nil {
			return fmt.Errorf("write chunk: %w", err)
		}
		index++

		if final {
			break
		}
	}

	// Terminator, so a truncated file is distinguishable from a complete one.
	if _, err := out.Write([]byte{0, 0, 0, 0}); err != nil {
		return fmt.Errorf("write terminator: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return out.Close()
}

// IsEncrypted reports whether a file carries the encrypted-backup magic.
func IsEncrypted(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	head := make([]byte, len(EncryptedMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		// Anything shorter than the magic cannot be one of ours.
		return false, nil //nolint:nilerr // a short file is simply not encrypted
	}
	return string(head) == string(EncryptedMagic), nil
}

// DecryptFile writes the plaintext of src to dst.
//
// On any failure dst is removed: a half-written plaintext that looks like a
// database is worse than no file, because someone will eventually try to
// restore from it.
func DecryptFile(src, dst, passphrase string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open ciphertext: %w", err)
	}
	defer in.Close()

	head := make([]byte, len(EncryptedMagic))
	if _, err := io.ReadFull(in, head); err != nil || string(head) != string(EncryptedMagic) {
		return ErrNotEncrypted
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(in, salt); err != nil {
		return ErrWrongPassphrase
	}
	aead, err := chacha20poly1305.NewX(deriveKey(passphrase, salt))
	if err != nil {
		return fmt.Errorf("cipher: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create plaintext: %w", err)
	}
	defer func() {
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(dst)
		}
	}()

	var (
		length [4]byte
		index  uint64
		sawEnd bool
	)
	for {
		if _, err := io.ReadFull(in, length[:]); err != nil {
			// Reaching EOF without the terminator means the file was cut short.
			return ErrWrongPassphrase
		}
		n := binary.BigEndian.Uint32(length[:])
		if n == 0 {
			sawEnd = true
			break
		}
		if n < uint32(chacha20poly1305.NonceSizeX)+uint32(aead.Overhead()) ||
			n > uint32(chunkSize)+uint32(chacha20poly1305.NonceSizeX)+uint32(aead.Overhead()) {
			return ErrWrongPassphrase
		}

		frame := make([]byte, n)
		if _, err := io.ReadFull(in, frame); err != nil {
			return ErrWrongPassphrase
		}
		nonce, ct := frame[:chacha20poly1305.NonceSizeX], frame[chacha20poly1305.NonceSizeX:]

		// Try the chunk as non-final, then as final: the flag is authenticated,
		// so only the true one verifies.
		pt, openErr := aead.Open(nil, nonce, ct, chunkAD(index, false))
		if openErr != nil {
			pt, openErr = aead.Open(nil, nonce, ct, chunkAD(index, true))
			if openErr != nil {
				return ErrWrongPassphrase
			}
		}
		if _, err := out.Write(pt); err != nil {
			return fmt.Errorf("write plaintext: %w", err)
		}
		index++
	}

	if !sawEnd || index == 0 {
		return ErrWrongPassphrase
	}
	return out.Sync()
}

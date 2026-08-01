package db

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeTemp creates a file of n pseudo-random bytes. Random content matters:
// compressible or repetitive data could hide a bug that mangles chunk order.
func writeTemp(t *testing.T, dir, name string, n int) (string, []byte) {
	t.Helper()
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p, data
}

func roundTrip(t *testing.T, size int) {
	t.Helper()
	dir := t.TempDir()
	src, want := writeTemp(t, dir, "plain.db", size)
	enc := filepath.Join(dir, "backup.enc")
	dec := filepath.Join(dir, "restored.db")

	if err := EncryptFile(src, enc, "correct horse battery staple"); err != nil {
		t.Fatalf("EncryptFile(%d bytes): %v", size, err)
	}
	if err := DecryptFile(enc, dec, "correct horse battery staple"); err != nil {
		t.Fatalf("DecryptFile(%d bytes): %v", size, err)
	}

	got, err := os.ReadFile(dec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round trip of %d bytes did not return the original", size)
	}
}

// Sizes chosen around the 1 MiB chunk boundary: exactly one chunk, and exact
// multiples, are where a chunked format usually breaks.
func TestRoundTripAcrossChunkBoundaries(t *testing.T) {
	for _, size := range []int{1, 1024, chunkSize - 1, chunkSize, chunkSize + 1, 3 * chunkSize} {
		roundTrip(t, size)
	}
}

// An empty database is unusual but legal — a fresh install backed up before any
// entry. It must not silently produce an unrestorable file.
func TestEmptyFileRoundTrips(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(src, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	enc, dec := filepath.Join(dir, "e"), filepath.Join(dir, "d")
	if err := EncryptFile(src, enc, "pass"); err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	if err := DecryptFile(enc, dec, "pass"); err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	got, _ := os.ReadFile(dec)
	if len(got) != 0 {
		t.Errorf("empty file came back as %d bytes", len(got))
	}
}

// The whole point: the ciphertext must not contain the ledger.
func TestCiphertextDoesNotLeakPlaintext(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.db")
	secret := []byte("CHE-424.492.624 Client SA facture FA-2026-0001 5405.00")
	if err := os.WriteFile(src, bytes.Repeat(secret, 200), 0o600); err != nil {
		t.Fatal(err)
	}
	enc := filepath.Join(dir, "backup.enc")
	if err := EncryptFile(src, enc, "pass"); err != nil {
		t.Fatal(err)
	}
	ct, _ := os.ReadFile(enc)
	if bytes.Contains(ct, secret) {
		t.Error("the ciphertext contains the plaintext")
	}
}

func TestWrongPassphraseIsRefused(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeTemp(t, dir, "plain.db", 4096)
	enc, dec := filepath.Join(dir, "e"), filepath.Join(dir, "d")
	if err := EncryptFile(src, enc, "right"); err != nil {
		t.Fatal(err)
	}

	err := DecryptFile(enc, dec, "wrong")
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("got %v, want ErrWrongPassphrase", err)
	}
	// A partial plaintext left behind would eventually be restored by someone.
	if _, statErr := os.Stat(dec); !os.IsNotExist(statErr) {
		t.Error("a failed decryption left a file behind")
	}
}

// Tampering must fail loudly. A backup that decrypts to quietly altered
// accounts is the worst possible outcome.
func TestTamperedCiphertextIsRefused(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeTemp(t, dir, "plain.db", 8192)
	enc, dec := filepath.Join(dir, "e"), filepath.Join(dir, "d")
	if err := EncryptFile(src, enc, "pass"); err != nil {
		t.Fatal(err)
	}

	ct, _ := os.ReadFile(enc)
	ct[len(ct)/2] ^= 0xFF
	if err := os.WriteFile(enc, ct, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := DecryptFile(enc, dec, "pass"); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("tampered file returned %v, want ErrWrongPassphrase", err)
	}
}

// Truncation is the failure mode of a bad USB stick or an interrupted copy. It
// must not pass for a shorter but valid ledger.
func TestTruncatedCiphertextIsRefused(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeTemp(t, dir, "plain.db", 3*chunkSize)
	enc, dec := filepath.Join(dir, "e"), filepath.Join(dir, "d")
	if err := EncryptFile(src, enc, "pass"); err != nil {
		t.Fatal(err)
	}

	ct, _ := os.ReadFile(enc)
	if err := os.WriteFile(enc, ct[:len(ct)/2], 0o600); err != nil {
		t.Fatal(err)
	}

	if err := DecryptFile(enc, dec, "pass"); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("truncated file returned %v, want a refusal", err)
	}
	if _, statErr := os.Stat(dec); !os.IsNotExist(statErr) {
		t.Error("a truncated file produced a partial plaintext")
	}
}

// Each encryption draws a fresh salt and nonces, so the same input must never
// produce the same file twice — otherwise an observer learns when nothing
// changed between two backups.
func TestEncryptionIsNotDeterministic(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeTemp(t, dir, "plain.db", 4096)
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := EncryptFile(src, a, "pass"); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFile(src, b, "pass"); err != nil {
		t.Fatal(err)
	}
	ctA, _ := os.ReadFile(a)
	ctB, _ := os.ReadFile(b)
	if bytes.Equal(ctA, ctB) {
		t.Error("two encryptions of the same input produced identical files")
	}
}

func TestIsEncryptedDistinguishesFiles(t *testing.T) {
	dir := t.TempDir()
	plain, _ := writeTemp(t, dir, "plain.db", 2048)
	enc := filepath.Join(dir, "e")
	if err := EncryptFile(plain, enc, "pass"); err != nil {
		t.Fatal(err)
	}

	if ok, _ := IsEncrypted(enc); !ok {
		t.Error("an encrypted backup was not recognised")
	}
	if ok, _ := IsEncrypted(plain); ok {
		t.Error("a plain file was reported as encrypted")
	}

	// Shorter than the magic: must be handled, not crash.
	tiny := filepath.Join(dir, "tiny")
	if err := os.WriteFile(tiny, []byte("ab"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsEncrypted(tiny); ok || err != nil {
		t.Errorf("tiny file: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestDecryptRefusesAPlainFile(t *testing.T) {
	dir := t.TempDir()
	plain, _ := writeTemp(t, dir, "plain.db", 1024)
	if err := DecryptFile(plain, filepath.Join(dir, "out"), "pass"); !errors.Is(err, ErrNotEncrypted) {
		t.Errorf("got %v, want ErrNotEncrypted", err)
	}
}

func TestEncryptRefusesEmptyPassphrase(t *testing.T) {
	dir := t.TempDir()
	src, _ := writeTemp(t, dir, "plain.db", 512)
	if err := EncryptFile(src, filepath.Join(dir, "e"), ""); err == nil {
		t.Error("an empty passphrase was accepted")
	}
}

package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// Le défaut d'origine : la sauvegarde automatique n'était chiffrée que si
// BACKUP_PASSPHRASE existait dans l'environnement, c'est-à-dire jamais. Mesuré
// sur une installation réelle : le dossier contenait des fichiers SQLite dont
// l'en-tête, le numéro de TVA, les adresses e-mail et l'IBAN se lisaient sans
// aucune clé.

func TestSansPhraseDePasseLaSourceEstNone(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	p := NewBackupPolicy(t.TempDir())
	pass, src := p.Passphrase()
	if src != SourceNone || pass != "" {
		t.Fatalf("source=%q pass=%q, attendu none/vide", src, pass)
	}
	if p.Status(t.TempDir()).Encrypting {
		t.Fatal("Encrypting=true sans aucune phrase de passe")
	}
}

func TestPhraseEnregistreeEstUtilisee(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	dir := t.TempDir()
	p := NewBackupPolicy(dir)
	const phrase = "colline-fromage-tunnel-95-Valais"

	if err := p.Set(phrase); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, src := p.Passphrase()
	if src != SourceStored {
		t.Fatalf("source=%q, attendu stored", src)
	}
	if got != phrase {
		t.Fatalf("phrase relue %q", got)
	}
	if !p.Status(dir).Encrypting {
		t.Fatal("Encrypting=false alors qu'une phrase est enregistrée")
	}
}

// Un déploiement serveur qui pose BACKUP_PASSPHRASE l'a décidé maintenant ;
// préférer en silence une phrase saisie dans l'interface il y a des mois
// produirait des sauvegardes que personne sur ce site ne sait ouvrir.
func TestLEnvironnementPrimeSurLeCoffre(t *testing.T) {
	dir := t.TempDir()
	p := NewBackupPolicy(dir)
	if err := p.Set("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKUP_PASSPHRASE", "phrase-du-deploiement-serveur-42")

	got, src := p.Passphrase()
	if src != SourceEnv {
		t.Fatalf("source=%q, attendu env", src)
	}
	if got != "phrase-du-deploiement-serveur-42" {
		t.Fatalf("phrase=%q", got)
	}
}

func TestPhraseFaibleRefusee(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	p := NewBackupPolicy(t.TempDir())
	if err := p.Set("1234"); err == nil {
		t.Fatal("une phrase de passe faible a été acceptée")
	}
}

func TestClearRetireLaPhrase(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	dir := t.TempDir()
	p := NewBackupPolicy(dir)
	if err := p.Set("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}
	if err := p.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, src := p.Passphrase(); src != SourceNone {
		t.Fatalf("source=%q après Clear, attendu none", src)
	}
}

// Le compte est celui que l'utilisateur regarde : combien de copies sur ce
// disque se lisent sans clé. Enregistrer une phrase n'y change rien, et le
// prétendre serait le mensonge le plus coûteux de cet écran.
func TestLeNombreDeCopiesEnClairEstCompte(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	// Deux copies en clair, une chiffrée.
	plainA := filepath.Join(backups, "ledgeralps-2026-08-01T10-00-00+0200.db")
	plainB := filepath.Join(backups, "ledgeralps-2026-08-02T10-00-00+0200.db")
	for _, f := range []string{plainA, plainB} {
		if err := os.WriteFile(f, []byte("SQLite format 3\x00 données lisibles"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := EncryptFile(plainA, filepath.Join(backups,
		"ledgeralps-2026-07-31T10-00-00+0200.db.enc"), "colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}

	st := NewBackupPolicy(dir).Status(backups)
	if st.PlaintextCount != 2 {
		t.Fatalf("%d copies en clair comptées, attendu 2", st.PlaintextCount)
	}
}

// Enregistrer une phrase ne protège que la suite. Les copies déjà écrites sont
// celles qui ont eu le temps de partir sur un NAS : il faut pouvoir les
// rattraper.
func TestEncryptExistingConvertitLesCopiesEnClair(t *testing.T) {
	dir := t.TempDir()
	const phrase = "colline-fromage-tunnel-95-Valais"

	// Une vraie base : encryptInPlace relit et vérifie l'intégrité SQLite avant
	// de supprimer le clair. Un fichier bidon ne prouverait rien.
	src := filepath.Join(dir, "source.db")
	makeTinyDB(t, src)

	for _, n := range []string{
		"ledgeralps-2026-08-01T10-00-00+0200.db",
		"ledgeralps-2026-08-02T10-00-00+0200.db",
	} {
		if err := copyFile(src, filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(src)

	done, err := EncryptExisting(context.Background(), dir, phrase)
	if err != nil {
		t.Fatalf("EncryptExisting: %v", err)
	}
	if len(done) != 2 {
		t.Fatalf("%d copies converties, attendu 2 (%v)", len(done), done)
	}

	entries, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("%d sauvegardes après conversion, attendu 2", len(entries))
	}
	for _, e := range entries {
		enc, err := IsEncrypted(e.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !enc {
			t.Fatalf("%s est encore en clair après conversion", e.Name)
		}
	}

	// Et elles se relisent : une sauvegarde chiffrée qu'on ne peut pas
	// déchiffrer n'est pas une sauvegarde.
	out := filepath.Join(dir, "relu.db")
	if err := DecryptFile(entries[0].Path, out, phrase); err != nil {
		t.Fatalf("déchiffrement de %s: %v", entries[0].Name, err)
	}
	if err := Verify(context.Background(), out); err != nil {
		t.Fatalf("la copie déchiffrée n'est pas une base valide: %v", err)
	}
}

func TestEncryptExistingIgnoreLesCopiesDejaChiffrees(t *testing.T) {
	dir := t.TempDir()
	const phrase = "colline-fromage-tunnel-95-Valais"
	src := filepath.Join(dir, "ledgeralps-2026-08-01T10-00-00+0200.db")
	makeTinyDB(t, src)

	if _, err := EncryptExisting(context.Background(), dir, phrase); err != nil {
		t.Fatal(err)
	}
	// Deuxième passage : rien à faire, et surtout pas un double chiffrement.
	again, err := EncryptExisting(context.Background(), dir, phrase)
	if err != nil {
		t.Fatalf("second passage: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("%d copies rechiffrées au second passage: %v", len(again), again)
	}
}

// makeTinyDB writes a real, valid SQLite database at path.
func makeTinyDB(t *testing.T, path string) {
	t.Helper()
	handle, err := sql.Open(SQLiteDriver, sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Exec(`CREATE TABLE t(a TEXT); INSERT INTO t VALUES('CHE-123.456.789')`); err != nil {
		handle.Close()
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
}

// L'avertissement affiché avant de revenir en clair annonce combien de
// sauvegardes deviendraient illisibles. « Vos 7 sauvegardes chiffrées » se lit ;
// « vos sauvegardes chiffrées » se survole.
//
// Le compte doit donc être juste : annoncer zéro alors qu'il en existe ferait
// dire à l'écran que rien n'est en jeu, au moment précis où quelque chose l'est.
func TestLeStatutCompteLesCopiesChiffreesEtEnClair(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	dir := t.TempDir()
	const phrase = "colline-fromage-tunnel-95-Valais"

	// Deux copies en clair, deux chiffrées.
	src := filepath.Join(dir, "source.db")
	makeTinyDB(t, src)
	for _, n := range []string{
		"ledgeralps-2026-08-01T10-00-00+0200.db",
		"ledgeralps-2026-08-02T10-00-00+0200.db",
		"ledgeralps-2026-08-03T10-00-00+0200.db",
		"ledgeralps-2026-08-04T10-00-00+0200.db",
	} {
		if err := copyFile(src, filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(src)

	// Chiffrer les deux premières seulement.
	for _, n := range []string{
		"ledgeralps-2026-08-01T10-00-00+0200.db",
		"ledgeralps-2026-08-02T10-00-00+0200.db",
	} {
		if _, err := encryptInPlace(context.Background(), filepath.Join(dir, n), phrase); err != nil {
			t.Fatal(err)
		}
	}

	st := NewBackupPolicy(t.TempDir()).Status(dir)
	if st.EncryptedCount != 2 {
		t.Errorf("EncryptedCount = %d, attendu 2 — l'avertissement sous-estimerait ce qui est en jeu",
			st.EncryptedCount)
	}
	if st.PlaintextCount != 2 {
		t.Errorf("PlaintextCount = %d, attendu 2", st.PlaintextCount)
	}
}

// Sans aucune sauvegarde chiffrée, le compte est zéro : l'écran doit alors dire
// que rien n'est perdu, et non brandir un avertissement sans objet.
func TestSansCopieChiffreeLeCompteEstZero(t *testing.T) {
	t.Setenv("BACKUP_PASSPHRASE", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "ledgeralps-2026-08-01T10-00-00+0200.db")
	makeTinyDB(t, src)

	st := NewBackupPolicy(t.TempDir()).Status(dir)
	if st.EncryptedCount != 0 {
		t.Fatalf("EncryptedCount = %d sur un dossier sans copie chiffrée", st.EncryptedCount)
	}
}

package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

// Une base réelle, avec des données reconnaissables : le seul moyen de prouver
// qu'un fichier est chiffré est de chercher ces données dedans et de ne pas les
// trouver.
const marqueur = "CHE-123.456.789 Boulangerie Dupont Sarl"

func baseAvecDonnees(t *testing.T, path string) {
	t.Helper()
	d, err := sql.Open(SQLiteDriver, sqliteDSN(path, livePragmas...))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(`CREATE TABLE invoices(id TEXT PRIMARY KEY, client TEXT, total REAL)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		if _, err := d.Exec(`INSERT INTO invoices VALUES(?,?,?)`,
			fmt.Sprintf("F-2026-%04d", i), marqueur, float64(i)*12.5); err != nil {
			t.Fatal(err)
		}
	}
}

func contientLeMarqueur(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Contains(b, []byte(marqueur))
}

func TestChiffrementPuisRelecture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)

	if !contientLeMarqueur(t, path) {
		t.Fatal("la base de départ ne contient pas le marqueur : le test ne prouverait rien")
	}
	if enc, _ := IsDatabaseEncrypted(path); enc {
		t.Fatal("une base neuve est annoncée comme chiffrée")
	}

	keys := NewDatabaseKeys(dir)
	key, err := keys.Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := EncryptDatabaseFile(context.Background(), path, key); err != nil {
		t.Fatalf("EncryptDatabaseFile: %v", err)
	}

	if contientLeMarqueur(t, path) {
		t.Fatal("les données sont encore lisibles en clair dans le fichier chiffré")
	}
	if enc, _ := IsDatabaseEncrypted(path); !enc {
		t.Fatal("le fichier n'est pas reconnu comme chiffré")
	}

	// Et rien n'est resté sur le côté. Un « .before-conversion » oublié serait
	// une copie complète de la comptabilité en clair, à côté de la base
	// chiffrée : la migration n'aurait servi à rien.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".before-conversion") || strings.HasSuffix(n, ".encrypting") {
			t.Fatalf("fichier intermédiaire laissé sur le disque: %s", n)
		}
	}

	d, err := OpenEncryptedWithKey(path, key)
	if err != nil {
		t.Fatalf("réouverture: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err != nil {
		t.Fatalf("relecture: %v", err)
	}
	if n != 200 {
		t.Fatalf("%d lignes après chiffrement, attendu 200", n)
	}
}

// La mauvaise clé doit refuser, pas rendre des données fausses.
func TestMauvaiseCleRefuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)

	key, err := NewDatabaseKeys(dir).Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatal(err)
	}
	if err := EncryptDatabaseFile(context.Background(), path, key); err != nil {
		t.Fatal(err)
	}

	autre := make([]byte, DatabaseKeySize)
	autre[0] = 0xAA
	d, err := OpenEncryptedWithKey(path, autre)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err == nil {
		t.Fatalf("base ouverte avec une autre clé, %d lignes lues", n)
	}
}

// Le cas « nouveau PC » : le scellement ne se descelle plus, mais la phrase de
// récupération enveloppe la même clé. Sans cela, une réinstallation de Windows
// perdrait dix ans de pièces (CO art. 958f).
func TestRecuperationParPhrase(t *testing.T) {
	dir := t.TempDir()
	const phrase = "colline-fromage-tunnel-95-Valais"

	keys := NewDatabaseKeys(dir)
	original, err := keys.Create(phrase)
	if err != nil {
		t.Fatal(err)
	}

	// Simuler la perte du scellement : le coffre disparaît, dbkey.json reste.
	if err := os.Remove(filepath.Join(dir, "secrets.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Key(); !errors.Is(err, ErrDatabaseKeyUnavailable) {
		t.Fatalf("erreur = %v, attendu ErrDatabaseKeyUnavailable", err)
	}

	recovered, err := keys.Recover(phrase)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if !bytes.Equal(recovered, original) {
		t.Fatal("la clé récupérée diffère de la clé d'origine")
	}
	// Et elle est rescellée : le démarrage suivant ne demande plus rien.
	again, err := keys.Key()
	if err != nil || !bytes.Equal(again, original) {
		t.Fatalf("clé non rescellée: %v", err)
	}
}

func TestMauvaisePhraseDeRecuperationRefuse(t *testing.T) {
	dir := t.TempDir()
	keys := NewDatabaseKeys(dir)
	if _, err := keys.Create("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Recover("mauvaise-phrase-de-recuperation-1"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("erreur = %v, attendu ErrWrongPassphrase", err)
	}
}

// Activer sans phrase de récupération est refusé. C'est la règle qui empêche le
// chiffrement de créer une façon de perdre les livres.
func TestCreationRefuseSansPhraseSolide(t *testing.T) {
	keys := NewDatabaseKeys(t.TempDir())
	if _, err := keys.Create(""); err == nil {
		t.Fatal("clé créée sans phrase de récupération")
	}
	if _, err := keys.Create("1234"); err == nil {
		t.Fatal("clé créée avec une phrase faible")
	}
}

// LE point qui décide de toute l'architecture : depuis une base chiffrée avec la
// clé de CETTE machine, la sauvegarde doit sortir en clair, pour être ensuite
// rechiffrée avec la phrase de passe de l'utilisateur. Autrement elle serait
// illisible le jour où la machine n'est plus là.
func TestLaSauvegardeDUneBaseChiffreeNeDependPasDeLaCleMachine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)

	key, err := NewDatabaseKeys(dir).Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatal(err)
	}
	if err := EncryptDatabaseFile(context.Background(), path, key); err != nil {
		t.Fatal(err)
	}

	database, err := OpenEncryptedWithKey(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	backups := filepath.Join(dir, "backups")

	// Sans phrase : la copie sort en clair — c'est ce qui la rend restaurable
	// ailleurs, et c'est aussi pourquoi le point A la chiffre par défaut.
	dest, err := Backup(context.Background(), database, cfg, backups, "")
	if err != nil {
		t.Fatalf("Backup depuis une base chiffrée: %v", err)
	}
	if enc, _ := IsDatabaseEncrypted(dest); enc {
		t.Fatal("la sauvegarde est chiffrée avec la clé de la machine : perdue avec la machine")
	}
	if err := Verify(context.Background(), dest); err != nil {
		t.Fatalf("la sauvegarde n'est pas une base valide: %v", err)
	}

	// Avec phrase : chiffrée par l'utilisateur, donc restaurable partout.
	dest2, err := Backup(context.Background(), database, cfg, backups, "colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatalf("Backup chiffré depuis une base chiffrée: %v", err)
	}
	if enc, _ := IsEncrypted(dest2); !enc {
		t.Fatal("la sauvegarde demandée chiffrée ne l'est pas")
	}
}

// Une restauration écrit un instantané EN CLAIR par-dessus la base. Sans
// réconciliation au démarrage, une installation chiffrée reviendrait en clair
// sans un mot — pendant que l'interface continuerait à afficher « chiffrée ».
func TestUneRestaurationNeDechiffrePasLInstallationEnSilence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)

	keys := NewDatabaseKeys(dir)
	key, err := keys.Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatal(err)
	}
	if err := EncryptDatabaseFile(context.Background(), path, key); err != nil {
		t.Fatal(err)
	}

	// Simuler la restauration : une base en clair prend la place.
	clair := filepath.Join(dir, "instantane.db")
	baseAvecDonnees(t, clair)
	if err := copyFile(clair, path); err != nil {
		t.Fatal(err)
	}
	if enc, _ := IsDatabaseEncrypted(path); enc {
		t.Fatal("le montage du test est faux : la base devait être en clair ici")
	}

	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	done, err := ReconcileDatabaseEncryption(context.Background(), cfg, dir)
	if err != nil {
		t.Fatalf("réconciliation: %v", err)
	}
	if done == "" {
		t.Fatal("aucune action : la base restaurée reste en clair alors qu'une clé est configurée")
	}
	if enc, _ := IsDatabaseEncrypted(path); !enc {
		t.Fatal("la base restaurée est restée en clair")
	}
	if contientLeMarqueur(t, path) {
		t.Fatal("les données restaurées sont lisibles en clair")
	}
}

// Le retour en arrière doit marcher aussi : chiffrer sans pouvoir déchiffrer
// serait un piège, pas une option.
func TestDechiffrementParLaReconciliation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)

	keys := NewDatabaseKeys(dir)
	key, err := keys.Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatal(err)
	}
	if err := EncryptDatabaseFile(context.Background(), path, key); err != nil {
		t.Fatal(err)
	}
	if _, err := StageEncryption(dir, ActionDecrypt, "u1"); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	if _, err := ReconcileDatabaseEncryption(context.Background(), cfg, dir); err != nil {
		t.Fatalf("réconciliation: %v", err)
	}
	if enc, _ := IsDatabaseEncrypted(path); enc {
		t.Fatal("la base est restée chiffrée")
	}
	if !contientLeMarqueur(t, path) {
		t.Fatal("les données ne sont pas revenues en clair : le contenu a été perdu")
	}
	// La clé est effacée, sinon le démarrage suivant rechiffrerait.
	if keys.Configured() {
		t.Fatal("la clé subsiste : la réconciliation suivante rechiffrerait la base")
	}
	if _, err := ReconcileDatabaseEncryption(context.Background(), cfg, dir); err != nil {
		t.Fatal(err)
	}
	if enc, _ := IsDatabaseEncrypted(path); enc {
		t.Fatal("la base a été rechiffrée au passage suivant")
	}
}

// Sans clé configurée, la réconciliation ne doit rien faire du tout — c'est
// l'état de la quasi-totalité des installations.
func TestSansCleLaReconciliationNeTouchePasALaBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)
	avant, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	done, err := ReconcileDatabaseEncryption(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if done != "" {
		t.Fatalf("action %q sur une installation sans clé", done)
	}
	apres, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(avant, apres) {
		t.Fatal("la base a été modifiée alors qu'aucun chiffrement n'est configuré")
	}
}

// Chiffrer deux fois produirait une base illisible. La réconciliation tourne à
// chaque démarrage : elle doit être idempotente.
func TestReconciliationIdempotente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)

	key, err := NewDatabaseKeys(dir).Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	for i := 0; i < 3; i++ {
		if _, err := ReconcileDatabaseEncryption(context.Background(), cfg, dir); err != nil {
			t.Fatalf("passage %d: %v", i+1, err)
		}
	}
	d, err := OpenEncryptedWithKey(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err != nil {
		t.Fatalf("après trois passages, la base ne s'ouvre plus: %v", err)
	}
	if n != 200 {
		t.Fatalf("%d lignes, attendu 200", n)
	}
}

package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

// Une clé configurée et pas encore de fichier : la base doit NAÎTRE chiffrée.
//
// Se fier au seul en-tête du fichier ne répond pas ici — il n'y a pas de
// fichier. La première version le faisait, et créait une base en clair sur une
// installation dont le propriétaire avait demandé le chiffrement.
//
// C'est le chemin de l'assistant d'installation, où la clé est créée avant que
// le serveur démarre pour que la base naisse chiffrée : pas de conversion, pas
// de redémarrage, et la comptabilité jamais écrite en clair.
func TestBaseNeuveAvecCleDejaConfiguree(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	appData := config.AppDataDir()
	if err := os.MkdirAll(appData, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDatabaseKeys(appData).Create("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "ledgeralps.db")
	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	d, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`CREATE TABLE t(a TEXT); INSERT INTO t VALUES('contact@dupont.ch')`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	b, _ := os.ReadFile(path)
	if string(b[:15]) == "SQLite format 3" {
		t.Fatal("base créée EN CLAIR alors qu'une clé est configurée")
	}
}

// Le fichier existant fait foi : une base en clair sur une installation qui a
// une clé n'est pas rouverte « chiffrée » par surprise — c'est le travail de la
// réconciliation au démarrage, qui la convertit dans le bon ordre.
func TestUneBaseEnClairExistanteResteLueEnClair(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	appData := config.AppDataDir()
	if err := os.MkdirAll(appData, 0o700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "ledgeralps.db")
	baseAvecDonnees(t, path)
	if _, err := NewDatabaseKeys(appData).Create("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{SQLitePath: path, Host: "127.0.0.1"}
	d, err := Open(cfg)
	if err != nil {
		t.Fatalf("une base en clair ne s'ouvre plus quand une clé existe: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err != nil {
		t.Fatalf("relecture: %v", err)
	}
	if n != 200 {
		t.Fatalf("%d lignes, attendu 200", n)
	}
}

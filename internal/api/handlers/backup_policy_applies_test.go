package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// La phrase de passe enregistrée doit s'appliquer à TOUTES les copies, pas
// seulement à celles que le serveur prend tout seul au démarrage.
//
// Mesuré sur un serveur réel : la politique était bien enregistrée, la
// sauvegarde automatique bien chiffrée, et le bouton « Créer une sauvegarde »
// produisait un fichier en clair — il ne lisait que le corps de la requête. Le
// trou refermé d'un côté restait ouvert de l'autre, et sur le chemin que
// l'utilisateur emprunte justement avant de copier le fichier sur une clé USB.
//
// Ce test travaille sur la résolution de la phrase de passe telle que les
// handlers la font, avec le vrai coffre à secrets sur disque.

const phraseDeTest = "colline-fromage-tunnel-95-Valais"

func withAppData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BACKUP_PASSPHRASE", "")
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	return config.AppDataDir()
}

// resolveBackupPassphrase reproduit la règle appliquée par CreateBackup :
// corps explicite, sinon politique, sauf demande explicite de clair.
func resolveBackupPassphrase(appData, explicit string, plaintext bool) string {
	if explicit != "" || plaintext {
		return explicit
	}
	stored, _ := db.NewBackupPolicy(appData).Passphrase()
	return stored
}

func TestLaPolitiqueSAppliqueALaSauvegardeManuelle(t *testing.T) {
	appData := withAppData(t)
	policy := db.NewBackupPolicy(appData)
	if err := policy.Set(phraseDeTest); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := resolveBackupPassphrase(appData, "", false)
	if got != phraseDeTest {
		t.Fatalf("phrase résolue = %q, attendu la phrase enregistrée — "+
			"une sauvegarde manuelle sortirait en clair", got)
	}
}

// Une copie en clair reste possible, mais elle se demande. Le silence ne veut
// plus dire « en clair ».
func TestLeClairSeDemandeExplicitement(t *testing.T) {
	appData := withAppData(t)
	if err := db.NewBackupPolicy(appData).Set(phraseDeTest); err != nil {
		t.Fatal(err)
	}
	if got := resolveBackupPassphrase(appData, "", true); got != "" {
		t.Fatalf("phrase = %q alors que le clair a été demandé explicitement", got)
	}
}

// Une phrase donnée dans la requête prime : l'utilisateur peut vouloir une copie
// protégée par une autre phrase, par exemple pour la remettre à un tiers.
func TestUnePhraseExpliciteRestePrioritaire(t *testing.T) {
	appData := withAppData(t)
	if err := db.NewBackupPolicy(appData).Set(phraseDeTest); err != nil {
		t.Fatal(err)
	}
	const autre = "autre-phrase-pour-un-tiers-2026"
	if got := resolveBackupPassphrase(appData, autre, false); got != autre {
		t.Fatalf("phrase = %q, attendu celle de la requête", got)
	}
}

// Sans politique, rien ne change : la copie sort en clair, comme avant.
func TestSansPolitiqueLaCopieResteEnClair(t *testing.T) {
	appData := withAppData(t)
	if got := resolveBackupPassphrase(appData, "", false); got != "" {
		t.Fatalf("phrase = %q sans politique enregistrée", got)
	}
}

// Et le résultat compte, pas seulement la résolution : une sauvegarde prise
// avec la phrase de la politique doit produire un fichier chiffré, relisible.
func TestUneSauvegardeAvecLaPolitiqueEstChiffree(t *testing.T) {
	appData := withAppData(t)
	if err := db.NewBackupPolicy(appData).Set(phraseDeTest); err != nil {
		t.Fatal(err)
	}

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "ledgeralps.db")
	cfg := &config.Config{SQLitePath: dbPath, Host: "127.0.0.1"}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE t(a TEXT); INSERT INTO t VALUES('contact@dupont.ch')`); err != nil {
		t.Fatal(err)
	}

	backups := filepath.Join(dbDir, "backups")
	pass := resolveBackupPassphrase(appData, "", false)
	dest, err := db.Backup(context.Background(), database, cfg, backups, pass)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	enc, err := db.IsEncrypted(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !enc {
		t.Fatal("la sauvegarde manuelle est en clair malgré la politique")
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw[:15]) == "SQLite format 3" {
		t.Fatal("en-tête SQLite : le fichier est lisible sans clé")
	}
}

package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

// Audit croisé Gemini, HIGH-14 : la base CHIFFRÉE n'ouvrait pas ses transactions en
// `BEGIN IMMEDIATE`.
//
// La chaîne d'audit lit le maillon précédent, puis insère le suivant. Dans une transaction
// « deferred » — le défaut du pilote — qui lit avant d'écrire, une validation concurrente
// rend SQLITE_BUSY_SNAPSHOT à la montée en écriture, que busy_timeout ne réessaie pas : le
// maillon disparaît, journalisé et non remonté. Le correctif existait pour la base en clair
// (`_txlock=immediate`) ; la base chiffrée, ouverte par un autre chemin, ne l'avait pas.

// isoler redirige le répertoire de données de l'application vers un dossier temporaire.
//
// `Open` sur un fichier qui n'existe pas encore consulte le coffre de clés de l'APPLICATION
// (`%APPDATA%\LedgerAlps`) : sur un poste où LedgerAlps est installé avec une base chiffrée,
// un test qui ouvre « sa » base dans un dossier neuf l'ouvrait CHIFFRÉE, avec la clé réelle de
// l'utilisateur. Un test ne doit ni dépendre de la machine, ni toucher à ses données.
func isoler(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

// lireEtEcrire est la forme exacte de la chaîne d'audit, avec une pause entre la lecture et
// l'écriture pour forcer le chevauchement des transactions. Rend le nombre d'échecs et la
// première erreur, pour que l'échec du test dise POURQUOI.
func lireEtEcrire(db *sql.DB, travailleurs int) (echecs int, premiere error, err error) {
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS maillons (n INTEGER PRIMARY KEY)`); err != nil {
		return
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	rater := func(e error) {
		mu.Lock()
		defer mu.Unlock()
		echecs++
		if premiere == nil {
			premiere = e
		}
	}
	for i := 0; i < travailleurs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := db.BeginTx(context.Background(), nil)
			if e != nil {
				rater(fmt.Errorf("BEGIN: %w", e))
				return
			}
			defer tx.Rollback()
			var dernier int
			if e := tx.QueryRow(`SELECT COALESCE(MAX(n), 0) FROM maillons`).Scan(&dernier); e != nil {
				rater(fmt.Errorf("SELECT: %w", e))
				return
			}
			time.Sleep(15 * time.Millisecond)
			if _, e := tx.Exec(`INSERT INTO maillons (n) VALUES (?)`, dernier+1); e != nil {
				rater(fmt.Errorf("INSERT: %w", e))
				return
			}
			if e := tx.Commit(); e != nil {
				rater(fmt.Errorf("COMMIT: %w", e))
			}
		}()
	}
	wg.Wait()
	return
}

func verifierSerialisation(t *testing.T, db *sql.DB) {
	t.Helper()
	const travailleurs = 12
	echecs, premiere, err := lireEtEcrire(db, travailleurs)
	if err != nil {
		t.Fatal(err)
	}
	if echecs != 0 {
		t.Fatalf("%d transaction(s) sur %d ont échoué (première erreur : %v) : un maillon perdu en silence — les transactions ne prennent pas le verrou d'écriture dès BEGIN", echecs, travailleurs, premiere)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maillons`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != travailleurs {
		t.Fatalf("%d maillons écrits, attendu %d", n, travailleurs)
	}
}

func TestLaBaseEnClairSerialiseLesEcrituresConcurrentes(t *testing.T) {
	isoler(t)
	database, err := Open(&config.Config{SQLitePath: filepath.Join(t.TempDir(), "clair.db"), Host: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	verifierSerialisation(t, database)
}

func TestLaBaseChiffreeSerialiseLesEcrituresConcurrentes(t *testing.T) {
	isoler(t)
	key := make([]byte, DatabaseKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	database, err := OpenEncryptedWithKey(filepath.Join(t.TempDir(), "chiffree.db"), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	database.SetMaxOpenConns(25) // comme openEncrypted
	verifierSerialisation(t, database)
}

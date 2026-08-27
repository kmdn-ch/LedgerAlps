package db

import (
	"strings"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

// PostgreSQL était documenté (« Définir POSTGRES_DSN bascule automatiquement le
// moteur ») et câblé dans ~30 constructeurs de handlers via `usePostgres bool`
// — mais il n'a jamais fonctionné une seule fois.
//
// Les migrations embarquées forment un jeu UNIQUE, appliqué verbatim aux deux
// moteurs (`tx.Exec(string(content))`, sans traduction de dialecte au-delà des
// `?` que gère rebind.go). Or la toute première utilise trois constructions
// que PostgreSQL ne connaît pas :
//
//   - `DEFAULT (lower(hex(randomblob(16))))` — fonctions SQLite ;
//   - `CREATE TRIGGER IF NOT EXISTS` — PostgreSQL n'a pas cette forme ;
//   - `SELECT RAISE(ABORT, '...')` en corps de déclencheur — PostgreSQL exige
//     une fonction PL/pgSQL (`CREATE FUNCTION ... RETURNS TRIGGER`).
//
// Le démarrage échouait donc sur une erreur SQL brute, après avoir ouvert une
// connexion et commencé à écrire. Mieux vaut refuser tôt et dire pourquoi.

func TestPostgresEstRefuseAvecUneRaisonLisible(t *testing.T) {
	cfg := &config.Config{
		PostgresDSN: "postgres://ledgeralps:motdepasse@localhost:5432/ledgeralps",
		SQLitePath:  "sans-objet.db",
		Host:        "127.0.0.1",
	}
	if !cfg.UsePostgres() {
		t.Fatal("le harnais est faux : UsePostgres() devrait être vrai avec un DSN posé")
	}

	database, err := Open(cfg)
	if database != nil {
		database.Close()
	}
	if err == nil {
		t.Fatal("Open a réussi avec POSTGRES_DSN : le moteur n'est pourtant pas porté")
	}

	// Le message doit nommer le problème ET la sortie, sinon il ne vaut pas
	// mieux que l'erreur SQL brute qu'il remplace.
	msg := err.Error()
	for _, attendu := range []string{"PostgreSQL", "SQLITE_PATH"} {
		if !strings.Contains(msg, attendu) {
			t.Errorf("le message de refus ne contient pas %q : %s", attendu, msg)
		}
	}
}

// SQLite — le seul moteur réellement supporté — ne doit pas être affecté.
func TestSQLiteResteOuvrableApresLeRefusDePostgres(t *testing.T) {
	database := newBackfillDB(t) // ouvre + migre une base SQLite complète
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("lecture des migrations: %v", err)
	}
	if n == 0 {
		t.Error("aucune migration appliquée : le chemin SQLite est cassé")
	}
}

package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path" // always forward-slash — required by embed.FS on all platforms
	"sort"
	"strings"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	_ "github.com/ncruces/go-sqlite3/driver"
	// L'ancien paquet .../embed n'est plus importe : il est deprecie, sa seule
	// action est d'ecrire « If you're reading this, you're unnecessarily
	// importing github.com/ncruces/go-sqlite3/embed » dans la sortie standard a
	// chaque demarrage. Le binaire WebAssembly de SQLite vient desormais du
	// module go-sqlite3-wasm, tire automatiquement par le pilote.
	//
	// Le message polluait server.log a chaque lancement, ce qui use l'attention
	// portee a ce fichier — le seul endroit ou l'utilisateur voit ce qui se
	// passe.
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

//go:embed migrations
var migrationsFS embed.FS

// Open returns an *sql.DB connected to SQLite (WAL mode), chiffrée ou non.
//
// PostgreSQL est refusé avec une raison lisible : le moteur a été documenté et
// câblé, mais jamais porté — voir le détail dans le corps de la fonction.
func Open(cfg *config.Config) (*sql.DB, error) {
	var driver, dsn string

	if cfg.UsePostgres() {
		// PostgreSQL n'a jamais fonctionné, et le prétendre coûtait plus cher
		// que de le dire.
		//
		// `POSTGRES_DSN` était documenté comme bascule de moteur, et
		// `usePostgres bool` traverse une trentaine de constructeurs — mais les
		// migrations embarquées forment un jeu UNIQUE, appliqué verbatim aux
		// deux moteurs (voir Migrate : `tx.Exec(string(content))`, sans
		// traduction de dialecte au-delà des `?` que gère rebind.go).
		//
		// La toute première migration utilise trois constructions que
		// PostgreSQL ne connaît pas : `randomblob()`/`hex()` en valeur par
		// défaut, `CREATE TRIGGER IF NOT EXISTS`, et `SELECT RAISE(ABORT, …)`
		// en corps de déclencheur — là où PostgreSQL exige une fonction
		// PL/pgSQL. Le démarrage échouait donc sur une erreur SQL brute, APRÈS
		// avoir ouvert une connexion et commencé à écrire dans la base.
		//
		// Refuser ici plutôt que là-bas ne retire aucune fonctionnalité : il
		// n'y en avait pas. Cela remplace une erreur de syntaxe illisible par
		// une phrase qui dit quoi faire, et cela évite de laisser un schéma à
		// moitié créé derrière soi.
		//
		// Porter le moteur pour de bon demande de réécrire les 28 migrations en
		// dialecte PostgreSQL — dont le déclencheur d'immuabilité (CO art.
		// 957a), le contrôle le plus critique du produit — et d'ajouter un job
		// CI qui les exécute réellement. C'est un chantier, pas un correctif.
		return nil, fmt.Errorf(
			"PostgreSQL n'est pas supporté par cette version : les migrations sont " +
				"écrites en SQLite et échoueraient à la première. Retirez POSTGRES_DSN " +
				"et utilisez SQLITE_PATH — le moteur prévu pour ce produit, qui tient " +
				"la comptabilité d'une PME sans serveur à administrer")
	}
	encrypted, encErr := shouldOpenEncrypted(cfg)
	if encErr != nil {
		return nil, encErr
	}
	if encrypted {
		return openEncrypted(cfg)
	}
	driver = SQLiteDriver
	dsn = sqliteDSN(cfg.SQLitePath, livePragmas...)

	database, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database (%s): %w", driver, err)
	}

	// SQLite WAL: concurrent readers, serialised writers.
	//
	// Le réglage PostgreSQL qui vivait ici est parti avec le moteur : il était
	// devenu inatteignable, et du code que rien n'exécute finit par mentir sur
	// ce que le programme sait faire.
	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(10 * time.Minute)

	// Ping with timeout to catch misconfigured DSNs at startup
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database (%s): %w", driver, err)
	}

	return database, nil
}

// Migrate applies all pending SQL migration files embedded in the binary.
// Each migration is applied atomically in a transaction (DDL is transactional
// in SQLite).
//
// Le paramètre usePostgres subsiste — il ne sert plus qu'à Rebind, et vaut
// toujours faux depuis qu'Open refuse ce moteur. Le retirer traverserait une
// trentaine de constructeurs de handlers : ce sera le premier geste du jour où
// PostgreSQL sera porté pour de bon, ou abandonné pour de bon.
//
// Files must follow the naming convention: NNNN_description.up.sql
func Migrate(database *sql.DB, usePostgres bool) error {
	// Ensure the migrations tracking table exists (outside the per-migration tx)
	createTable := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`
	if _, err := database.Exec(createTable); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".up.sql")

		// Check if already applied (use correct placeholder for DB dialect)
		checkQ := Rebind("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", usePostgres)
		var count int
		if err := database.QueryRow(checkQ, version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		// Apply migration atomically: DDL + schema_migrations record in one transaction
		tx, err := database.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %s: %w", name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		insertQ := Rebind("INSERT INTO schema_migrations(version) VALUES(?)", usePostgres)
		if _, err := tx.Exec(insertQ, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}

		fmt.Printf("  [OK] applied migration: %s\n", name)
	}
	return nil
}

// MigrationSQL returns the SQL of one embedded migration, by version.
//
// Exposé pour que les tests puissent rejouer une migration sur une base montée
// à la main. Une migration qui doit se comporter différemment selon ce qui
// existe déjà — comme l'extinction d'un réglage pour les installations
// antérieures — ne se vérifie pas autrement : la suite normale part toujours
// d'une base vide, où le cas n'existe pas.
func MigrationSQL(version string) (string, error) {
	content, err := migrationsFS.ReadFile(path.Join("migrations", version+".up.sql"))
	if err != nil {
		return "", fmt.Errorf("migration %s introuvable: %w", version, err)
	}
	return string(content), nil
}

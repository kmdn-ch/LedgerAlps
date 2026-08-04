package db

// Ouverture d'une base chiffrée.
//
// La clé passe par un PRAGMA dans le rappel d'initialisation de connexion, et
// non dans la DSN. Une DSN finit dans un message d'erreur ou une ligne de
// journal tôt ou tard ; ici, elle ne contient que le nom du VFS.
//
// L'ordre à l'intérieur du rappel n'est pas cosmétique, et il a été mesuré :
// tout pragma qui touche le fichier avant la clé échoue, journal_mode compris.
// C'est pour cette raison que les pragmas de la base en clair ne peuvent pas
// rester dans la DSN quand elle est chiffrée — première tentative, qui
// produisait « invalid _pragma: unable to open database file » au démarrage.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// encryptedPragmas are the same settings as livePragmas, expressed as SQL
// because they run after the key rather than as DSN parameters.
const encryptedPragmas = `
	PRAGMA journal_mode = WAL;
	PRAGMA foreign_keys = on;
	PRAGMA busy_timeout = 5000;
`

// OpenEncryptedWithKey opens the database at path with an explicit key.
// Exported for the migration paths and the tests, which hold the key directly.
func OpenEncryptedWithKey(path string, key []byte) (*sql.DB, error) {
	if len(key) != DatabaseKeySize {
		return nil, fmt.Errorf("clé de %d octets, attendu %d", len(key), DatabaseKeySize)
	}
	pragma := "PRAGMA hexkey='" + hexKey(key) + "';"
	return driver.Open(sqliteDSN(path, "vfs=adiantum"), func(c *sqlite3.Conn) error {
		if err := c.Exec(pragma); err != nil {
			return err
		}
		return c.Exec(encryptedPragmas)
	})
}

// openEncrypted opens the configured database with the key sealed to this
// account.
func openEncrypted(cfg *config.Config) (*sql.DB, error) {
	keys := NewDatabaseKeys(config.AppDataDir())
	key, err := keys.Key()
	if err != nil {
		return nil, err
	}

	database, err := OpenEncryptedWithKey(cfg.SQLitePath, key)
	if err != nil {
		return nil, fmt.Errorf("ouverture de la base chiffrée: %w", err)
	}
	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		// À ce stade la clé a été descellée : si l'ouverture échoue quand même,
		// c'est le fichier qui ne correspond pas à cette clé — typiquement une
		// base restaurée depuis une autre installation.
		return nil, fmt.Errorf("%w: la base ne s'ouvre pas avec la clé de cette machine (%v)",
			ErrDatabaseKeyUnavailable, err)
	}
	return database, nil
}

// shouldOpenEncrypted décide si la base doit être ouverte à travers le VFS
// chiffrant.
//
// L'en-tête du fichier répond dans le cas courant. Il ne répond PAS dans un cas
// qui compte : une clé est configurée et le fichier n'existe pas encore. Se fier
// au seul en-tête crée alors une base EN CLAIR sur une installation dont le
// propriétaire a demandé le chiffrement.
//
// C'est exactement le chemin de l'assistant d'installation, où la clé est créée
// avant que le serveur démarre pour que la base naisse chiffrée — sans
// conversion, sans redémarrage, et sans jamais écrire la comptabilité en clair.
// C'est aussi ce qui se passe si le fichier disparaît sur une installation
// chiffrée.
func shouldOpenEncrypted(cfg *config.Config) (bool, error) {
	encrypted, err := IsDatabaseEncrypted(cfg.SQLitePath)
	if err != nil {
		return false, err
	}
	if encrypted {
		return true, nil
	}
	if _, statErr := os.Stat(cfg.SQLitePath); statErr == nil {
		// Le fichier existe et n'est pas chiffré : il fait foi. Une clé
		// configurée en plus est le cas que ReconcileDatabaseEncryption traite
		// au démarrage, pas celui-ci.
		return false, nil
	}
	return NewDatabaseKeys(config.AppDataDir()).Configured(), nil
}

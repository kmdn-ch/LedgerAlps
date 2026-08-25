package db

import (
	"path/filepath"
	"strings"
)

// SQLiteDriver is the driver name registered by github.com/ncruces/go-sqlite3.
//
// The project moved off modernc.org/sqlite for one reason: encryption at rest.
// modernc has no write-capable VFS hook — its vfs package is read-only — so an
// encrypting layer cannot be inserted under it at all. ncruces ships one
// (vfs/adiantum) and stays CGO-free, which is what keeps the single binary and
// the cross-compilation.
const SQLiteDriver = "sqlite3"

// SQLiteURI turns an operating-system path into a SQLite URI filename.
//
// A path is not a URI, and on Windows the difference bites. Backslashes are not
// separators here, and three characters that are perfectly legal in a Windows
// filename mean something else in a URI:
//
//	?   starts the parameter list — everything after it would be read as options
//	#   starts a fragment
//	%   introduces an escape, so a literal one must escape itself
//
// %APPDATA% commonly contains a space (« C:\Users\Jean Dupont\… »); spaces are
// accepted as-is by SQLite's URI parser and are left alone. Measured on Windows:
// "file:" + forward slashes opens the intended file, while the file:/// form
// fails outright on a drive letter.
func SQLiteURI(path string) string {
	u := filepath.ToSlash(path)
	// The percent must be escaped first, or it would escape the escapes.
	u = strings.ReplaceAll(u, "%", "%25")
	u = strings.ReplaceAll(u, "?", "%3f")
	u = strings.ReplaceAll(u, "#", "%23")
	return "file:" + u
}

// sqliteDSN builds a data source name for one database file.
//
// Parameters are appended verbatim; callers pass already-formed "key=value"
// pairs. The encryption key is deliberately NOT passed this way — it goes
// through a per-connection PRAGMA instead, so it never sits in a string that
// could be copied into a log line or an error message.
func sqliteDSN(path string, params ...string) string {
	dsn := SQLiteURI(path)
	if len(params) > 0 {
		dsn += "?" + strings.Join(params, "&")
	}
	return dsn
}

// Pragmas applied to every connection of the live database.
//
// WAL lets readers work while a write is in flight, which is what makes the
// interface stay responsive while a backup runs. foreign_keys is off by default
// in SQLite and has to be asked for on each connection. busy_timeout replaces an
// immediate "database is locked" with a wait, which is almost always what a
// single-user desktop application wants.
var livePragmas = []string{
	"_pragma=journal_mode(WAL)",
	"_pragma=foreign_keys(on)",
	"_pragma=busy_timeout(5000)",
	// BEGIN IMMEDIATE plutôt que BEGIN.
	//
	// Une transaction « deferred » — le défaut du pilote — qui LIT avant
	// d'ÉCRIRE fige un instantané. Si une autre connexion valide entre les
	// deux, la montée en écriture rend SQLITE_BUSY_SNAPSHOT, et busy_timeout
	// ne le réessaie PAS : attendre ne rajeunit pas un instantané périmé.
	//
	// C'est exactement la forme de la chaîne d'audit — lire le maillon
	// précédent, puis insérer le suivant — et sur `traceFor` comme sur
	// `invoicing.record`, l'échec est journalisé et non remonté : le maillon
	// disparaissait en silence. Un journal à trous, que le produit lui-même
	// désigne comme pire qu'un journal absent. Avec SetMaxOpenConns(25), les
	// écritures concurrentes sont possibles.
	//
	// « immediate » prend le verrou d'écriture dès le BEGIN : le conflit
	// devient une ATTENTE, que busy_timeout absorbe.
	"_txlock=immediate",
}

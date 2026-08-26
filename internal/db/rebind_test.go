package db_test

import (
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/db"
)

func TestRebind(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		usePostgres bool
		want        string
	}{
		// ── SQLite (usePostgres=false) ──────────────────────────────────────────
		{
			name:        "SQLite: query returned unchanged with placeholders",
			query:       "SELECT * FROM t WHERE a=? AND b=?",
			usePostgres: false,
			want:        "SELECT * FROM t WHERE a=? AND b=?",
		},
		{
			name:        "SQLite: query without placeholders returned unchanged",
			query:       "SELECT 1",
			usePostgres: false,
			want:        "SELECT 1",
		},
		{
			name:        "SQLite: empty query returned unchanged",
			query:       "",
			usePostgres: false,
			want:        "",
		},

		// ── PostgreSQL (usePostgres=true) ───────────────────────────────────────
		{
			name:        "Postgres: single ? → $1",
			query:       "SELECT * FROM t WHERE id=?",
			usePostgres: true,
			want:        "SELECT * FROM t WHERE id=$1",
		},
		{
			name:        "Postgres: two placeholders → $1, $2",
			query:       "SELECT * FROM t WHERE a=? AND b=?",
			usePostgres: true,
			want:        "SELECT * FROM t WHERE a=$1 AND b=$2",
		},
		{
			name:        "Postgres: three placeholders → $1, $2, $3",
			query:       "INSERT INTO t (a, b, c) VALUES (?, ?, ?)",
			usePostgres: true,
			want:        "INSERT INTO t (a, b, c) VALUES ($1, $2, $3)",
		},
		{
			name:        "Postgres: complex query with LIMIT → $1, $2, $3",
			query:       "SELECT * FROM t WHERE a=? AND b=? LIMIT ?",
			usePostgres: true,
			want:        "SELECT * FROM t WHERE a=$1 AND b=$2 LIMIT $3",
		},
		{
			name:        "Postgres: query without ? unchanged",
			query:       "SELECT 1",
			usePostgres: true,
			want:        "SELECT 1",
		},
		{
			name:        "Postgres: empty query unchanged",
			query:       "",
			usePostgres: true,
			want:        "",
		},
		{
			name:        "Postgres: five placeholders sequentially numbered",
			query:       "INSERT INTO audit_logs (a,b,c,d,e) VALUES (?,?,?,?,?)",
			usePostgres: true,
			want:        "INSERT INTO audit_logs (a,b,c,d,e) VALUES ($1,$2,$3,$4,$5)",
		},
		{
			name:        "Postgres: deux emplacements ordinaires",
			query:       "UPDATE t SET col=? WHERE id=?",
			usePostgres: true,
			want:        "UPDATE t SET col=$1 WHERE id=$2",
		},

		// ── Ce qui n'est PAS un emplacement ────────────────────────────────────
		//
		// Rebind ignore les chaines litterales, les identifiants entre
		// guillemets et les commentaires. Un « ? » y est un CARACTERE. Aucune
		// requete du depot n'en contient aujourd'hui : le defaut etait latent,
		// et ces cas empechent qu'il redevienne possible.
		{
			name:        "Postgres: ? dans une chaine litterale n'est pas un emplacement",
			query:       "SELECT * FROM t WHERE note = 'et alors ?' AND id = ?",
			usePostgres: true,
			want:        "SELECT * FROM t WHERE note = 'et alors ?' AND id = $1",
		},
		{
			name:        "Postgres: apostrophe echappee dans la chaine",
			query:       "SELECT * FROM t WHERE note = 'l''heure ? oui' AND id = ?",
			usePostgres: true,
			want:        "SELECT * FROM t WHERE note = 'l''heure ? oui' AND id = $1",
		},
		{
			name:        "Postgres: motif LIKE contenant un ?",
			query:       "SELECT * FROM t WHERE nom LIKE '%?%' AND id = ?",
			usePostgres: true,
			want:        "SELECT * FROM t WHERE nom LIKE '%?%' AND id = $1",
		},
		{
			name:        "Postgres: ? dans un identifiant entre guillemets",
			query:       `SELECT "col?" FROM t WHERE id = ?`,
			usePostgres: true,
			want:        `SELECT "col?" FROM t WHERE id = $1`,
		},
		{
			name:        "Postgres: ? dans un commentaire de ligne",
			query:       "SELECT 1 -- pourquoi ?\n  WHERE id = ?",
			usePostgres: true,
			want:        "SELECT 1 -- pourquoi ?\n  WHERE id = $1",
		},
		{
			name:        "Postgres: ? dans un commentaire de bloc",
			query:       "SELECT /* est-ce clair ? */ 1 WHERE id = ?",
			usePostgres: true,
			want:        "SELECT /* est-ce clair ? */ 1 WHERE id = $1",
		},
		{
			name:        "Postgres: commentaires de bloc IMBRIQUES (PostgreSQL les imbrique)",
			query:       "SELECT /* a /* b ? */ c ? */ 1 WHERE id = ?",
			usePostgres: true,
			want:        "SELECT /* a /* b ? */ c ? */ 1 WHERE id = $1",
		},
		{
			name:        "Postgres: la numerotation ne saute pas apres un litteral",
			query:       "SELECT ? , 'a ? b' , ? , ?",
			usePostgres: true,
			want:        "SELECT $1 , 'a ? b' , $2 , $3",
		},
		{
			name:        "SQLite: rien n'est touche, litteraux compris",
			query:       "SELECT * FROM t WHERE note = 'et alors ?' AND id = ?",
			usePostgres: false,
			want:        "SELECT * FROM t WHERE note = 'et alors ?' AND id = ?",
		},

		// ── Both modes: no ? ───────────────────────────────────────────────────
		{
			name:        "No placeholder SQLite: untouched",
			query:       "SELECT id, name FROM accounts ORDER BY name",
			usePostgres: false,
			want:        "SELECT id, name FROM accounts ORDER BY name",
		},
		{
			name:        "No placeholder Postgres: untouched",
			query:       "SELECT id, name FROM accounts ORDER BY name",
			usePostgres: true,
			want:        "SELECT id, name FROM accounts ORDER BY name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := db.Rebind(tc.query, tc.usePostgres)
			if got != tc.want {
				t.Errorf("Rebind(%q, %v)\n  got  %q\n  want %q", tc.query, tc.usePostgres, got, tc.want)
			}
		})
	}
}

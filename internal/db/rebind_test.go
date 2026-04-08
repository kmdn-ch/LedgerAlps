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
			name:        "Postgres: ? inside string literal position is still replaced (Rebind is dumb-replace)",
			query:       "UPDATE t SET col=? WHERE id=?",
			usePostgres: true,
			want:        "UPDATE t SET col=$1 WHERE id=$2",
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

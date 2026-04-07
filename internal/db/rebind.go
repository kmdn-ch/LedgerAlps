package db

import (
	"fmt"
	"strings"
)

// Rebind converts ? placeholders to $1, $2, ... for PostgreSQL.
// Returns the query unchanged for SQLite.
func Rebind(query string, usePostgres bool) string {
	if !usePostgres {
		return query
	}
	idx := 0
	var b strings.Builder
	b.Grow(len(query) + 16)
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			idx++
			b.WriteString(fmt.Sprintf("$%d", idx))
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

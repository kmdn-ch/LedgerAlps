// Package dbtest regroupe ce dont les tests ont besoin pour simuler une ATTAQUE contre la base.
//
// Les déclencheurs d'immuabilité (journal_entries, journal_lines, audit_logs) ferment la porte à
// tout ce qui passe par la base. Un attaquant qui écrit directement dans LE FICHIER peut les
// retirer ; c'est contre lui que sert la chaîne d'empreintes, et c'est elle que ces tests
// éprouvent. Ils commencent donc par faire ce que ferait l'attaquant.
package dbtest

import (
	"database/sql"
	"testing"
)

// Desarmer retire les déclencheurs qui protègent le journal d'audit et les lignes des écritures
// comptabilisées, pour qu'un test puisse y réécrire ce qu'un attaquant réécrirait.
func Desarmer(t testing.TB, database *sql.DB) {
	t.Helper()
	for _, trigger := range []string{
		"trg_audit_logs_no_update",
		"trg_audit_logs_no_delete",
		"trg_journal_lines_no_insert_posted",
		"trg_journal_lines_no_update_posted",
		"trg_journal_lines_no_delete_posted",
	} {
		if _, err := database.Exec(`DROP TRIGGER IF EXISTS ` + trigger); err != nil {
			t.Fatalf("retirer %s : %v", trigger, err)
		}
	}
}

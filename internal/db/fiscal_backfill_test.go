package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/config"
)

func newBackfillDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-backfill-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := Open(&config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id, email, name, password_hash, is_admin) VALUES ('u1','b@t.ch','B','x',1)`,
	); err != nil {
		t.Fatal(err)
	}
	return database
}

// Reproduit une base d'avant la v1.4.6 : des écritures sans exercice.
func seedOrphan(t *testing.T, database *sql.DB, ref, date string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
		VALUES (?, ?, ?, 'ancienne', 'posted', 'u1')`, NewID(), ref, date); err != nil {
		t.Fatalf("seed %s: %v", ref, err)
	}
}

func TestBackfillAttachesOrphansAndCreatesMissingYears(t *testing.T) {
	database := newBackfillDB(t)
	seedOrphan(t, database, "JN-2024-001", "2024-03-15")
	seedOrphan(t, database, "JN-2024-002", "2024-11-02")
	seedOrphan(t, database, "JN-2025-001", "2025-01-09")

	if err := BackfillFiscalYears(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var orphans int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM journal_entries WHERE fiscal_year_id IS NULL`).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("%d écriture(s) restent sans exercice : elles seraient invisibles à la clôture", orphans)
	}

	rows, err := database.Query(`SELECT name FROM fiscal_years ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "2024" || names[1] != "2025" {
		t.Fatalf("exercices créés = %v, attendu [2024 2025]", names)
	}
}

// Le rattrapage ne doit pas poser une année civile par-dessus un exercice
// décalé déjà déclaré : le chevauchement rendrait tout rattachement arbitraire.
func TestBackfillDoesNotOverlapAnExistingPeriod(t *testing.T) {
	database := newBackfillDB(t)
	if _, err := database.Exec(
		`INSERT INTO fiscal_years (id,name,start_date,end_date) VALUES ('fy','2024/25','2024-07-01','2025-06-30')`,
	); err != nil {
		t.Fatal(err)
	}
	seedOrphan(t, database, "JN-2024-001", "2024-09-15")

	if err := BackfillFiscalYears(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM fiscal_years`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d exercices : une année civile chevauche l'exercice décalé", n)
	}
	var attached string
	if err := database.QueryRow(`SELECT fiscal_year_id FROM journal_entries`).Scan(&attached); err != nil {
		t.Fatal(err)
	}
	if attached != "fy" {
		t.Fatalf("rattachée à %q au lieu de l'exercice décalé", attached)
	}
}

// Deuxième exécution : rien ne doit bouger. Le rattrapage tourne à chaque
// démarrage, il ne peut pas se permettre d'avoir des effets cumulatifs.
func TestBackfillIsIdempotent(t *testing.T) {
	database := newBackfillDB(t)
	seedOrphan(t, database, "JN-2024-001", "2024-03-15")

	for i := 0; i < 3; i++ {
		if err := BackfillFiscalYears(database, false); err != nil {
			t.Fatalf("passe %d: %v", i+1, err)
		}
	}

	var years int
	if err := database.QueryRow(`SELECT COUNT(*) FROM fiscal_years`).Scan(&years); err != nil {
		t.Fatal(err)
	}
	if years != 1 {
		t.Fatalf("%d exercices après trois passes, attendu 1", years)
	}
}

// ─── L'exception au déclencheur d'immuabilité doit rester étroite ────────────
//
// La migration 0013 autorise une seule réparation sur une écriture
// comptabilisée : renseigner son exercice quand il manque. Tout le reste doit
// continuer d'être refusé, sans quoi on aurait troqué la garantie du
// CO art. 957a contre une commodité de migration.

func TestPostedEntriesRemainImmutableApartFromThePeriodRepair(t *testing.T) {
	database := newBackfillDB(t)
	seedOrphan(t, database, "JN-2024-001", "2024-03-15")
	if err := BackfillFiscalYears(database, false); err != nil {
		t.Fatal(err)
	}

	var id, fyID string
	if err := database.QueryRow(
		`SELECT id, fiscal_year_id FROM journal_entries`).Scan(&id, &fyID); err != nil {
		t.Fatal(err)
	}

	// Un second exercice, pour tenter le déplacement.
	if _, err := database.Exec(
		`INSERT INTO fiscal_years (id,name,start_date,end_date) VALUES ('other','2099','2099-01-01','2099-12-31')`,
	); err != nil {
		t.Fatal(err)
	}

	refused := []struct {
		what  string
		query string
		args  []any
	}{
		{"changer le montant via la description", `UPDATE journal_entries SET description = 'falsifiée' WHERE id = ?`, []any{id}},
		{"changer la date", `UPDATE journal_entries SET date = '2025-01-01' WHERE id = ?`, []any{id}},
		{"changer la référence", `UPDATE journal_entries SET reference = 'JN-9999-999' WHERE id = ?`, []any{id}},
		{"repasser en brouillon", `UPDATE journal_entries SET status = 'draft' WHERE id = ?`, []any{id}},
		{"effacer l'empreinte", `UPDATE journal_entries SET integrity_hash = NULL WHERE id = ?`, []any{id}},
		{"déplacer vers un autre exercice", `UPDATE journal_entries SET fiscal_year_id = 'other' WHERE id = ?`, []any{id}},
		{"détacher de son exercice", `UPDATE journal_entries SET fiscal_year_id = NULL WHERE id = ?`, []any{id}},
	}
	for _, tc := range refused {
		if _, err := database.Exec(tc.query, tc.args...); err == nil {
			t.Errorf("« %s » a été accepté sur une écriture comptabilisée", tc.what)
		}
	}
}

// La migration 0013 affirme que renseigner l'exercice ne change aucune
// empreinte. C'est l'argument qui autorise l'exception : il doit être vérifié,
// pas seulement écrit dans un commentaire.
func TestPeriodRepairLeavesEveryHashUntouched(t *testing.T) {
	database := newBackfillDB(t)

	const entryID = "e1"
	if _, err := database.Exec(`
		INSERT INTO journal_entries (id, reference, date, description, status, integrity_hash, created_by_id)
		VALUES (?, 'JN-2024-001', '2024-03-15', 'ancienne', 'posted', 'HASH-ENTREE', 'u1')`, entryID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO audit_logs (id, user_id, action, table_name, record_id, after_state,
		                        entry_hash, sequence_number, hash_version)
		VALUES ('a1', 'u1', 'post', 'journal_entries', ?, '{}', 'HASH-AUDIT', 1, 2)`, entryID); err != nil {
		t.Fatal(err)
	}

	if err := BackfillFiscalYears(database, false); err != nil {
		t.Fatal(err)
	}

	var entryHash, auditHash string
	var fyID sql.NullString
	if err := database.QueryRow(
		`SELECT integrity_hash, fiscal_year_id FROM journal_entries WHERE id = ?`, entryID,
	).Scan(&entryHash, &fyID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(
		`SELECT entry_hash FROM audit_logs WHERE record_id = ?`, entryID).Scan(&auditHash); err != nil {
		t.Fatal(err)
	}

	if !fyID.Valid {
		t.Fatal("la réparation n'a pas eu lieu : le test ne prouve rien")
	}
	if entryHash != "HASH-ENTREE" {
		t.Fatalf("integrity_hash modifié (%q) : la réparation n'est pas neutre", entryHash)
	}
	if auditHash != "HASH-AUDIT" {
		t.Fatalf("entry_hash d'audit modifié (%q) : la chaîne CO art. 957a a bougé", auditHash)
	}
}

// Une base vide ne doit rien créer : inventer un exercice sur une installation
// neuve reviendrait à décider à la place de l'utilisateur.
func TestBackfillLeavesAnEmptyDatabaseAlone(t *testing.T) {
	database := newBackfillDB(t)
	if err := BackfillFiscalYears(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var years int
	if err := database.QueryRow(`SELECT COUNT(*) FROM fiscal_years`).Scan(&years); err != nil {
		t.Fatal(err)
	}
	if years != 0 {
		t.Fatalf("%d exercice(s) créé(s) sur une base vide", years)
	}
}

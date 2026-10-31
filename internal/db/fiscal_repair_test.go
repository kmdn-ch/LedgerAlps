package db

import (
	"testing"
	"time"
)

// Audit 9, A-03 : un exercice créé pour l'année « 26 » portait `26-01-01`, que la
// base ne relisait plus — l'écran des exercices et l'archive légale répondaient
// 500. La réparation complète l'année sur quatre chiffres.
func TestRepairFiscalYearDatesRendLesBornesLisibles(t *testing.T) {
	database := newBackfillDB(t)
	if _, err := database.Exec(`
		INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed)
		VALUES ('fy-26', '26', '26-01-01', '26-12-31', 0),
		       ('fy-1',  '1',  '1-01-01',  '1-12-31',  0),
		       ('fy-ok', '2026', '2026-01-01', '2026-12-31', 0)`); err != nil {
		t.Fatal(err)
	}

	// Avant : l'exercice est illisible, comme dans le rapport.
	var start time.Time
	if err := database.QueryRow(`SELECT start_date FROM fiscal_years WHERE id = 'fy-26'`).Scan(&start); err == nil {
		t.Fatal("le défaut n'est pas reproduit : `26-01-01` se relit déjà")
	}

	if err := RepairFiscalYearDates(database, false); err != nil {
		t.Fatalf("réparation: %v", err)
	}
	for id, want := range map[string]string{"fy-26": "0026-01-01", "fy-1": "0001-01-01", "fy-ok": "2026-01-01"} {
		var got time.Time
		if err := database.QueryRow(`SELECT start_date FROM fiscal_years WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("%s reste illisible après la réparation: %v", id, err)
		}
		if got.Format("2006-01-02") != want {
			t.Errorf("%s : %s, attendu %s", id, got.Format("2006-01-02"), want)
		}
	}

	// Idempotente : un second passage ne change rien.
	if err := RepairFiscalYearDates(database, false); err != nil {
		t.Fatalf("second passage: %v", err)
	}
}

func TestPadYear(t *testing.T) {
	for in, want := range map[string]string{
		"26-01-01": "0026-01-01", "999-12-31": "0999-12-31", "2026-01-01": "2026-01-01",
		"": "", "pas une date": "pas une date",
	} {
		if got := padYear(in); got != want {
			t.Errorf("padYear(%q) = %q, attendu %q", in, got, want)
		}
	}
}

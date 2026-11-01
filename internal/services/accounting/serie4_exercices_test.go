package accounting

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// Audit croisé Gemini, série 4, MED-04 : deux exercices qui finissent la même année
// (un exercice court, un changement de date de clôture) et un exercice suivant que
// l'utilisateur a nommé autrement.

func creerExercice(t *testing.T, database *sql.DB, nom, debut, fin string) string {
	t.Helper()
	id := "fy-" + nom
	if _, err := database.Exec(
		`INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed) VALUES (?, ?, ?, ?, 0)`,
		id, nom, debut, fin); err != nil {
		t.Fatalf("exercice %s : %v", nom, err)
	}
	return id
}

func exercices(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.Query(`SELECT name || ' ' || start_date || ' ' || end_date FROM fiscal_years ORDER BY start_date, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// Rejoué sur le bac à sable : la clôture du premier exercice court CRÉAIT un exercice
// « 2032 » du 1.7.2031 au 30.6.2032, par-dessus le second ; et la clôture du second
// répondait 500 (`UNIQUE(reference)` : « CLOTURE-2031 » existait déjà).
func TestDeuxExercicesCourtsDeLaMemeAnneeSeClosentTousLesDeux(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	creerExercice(t, database, "Court-1", "2031-01-01", "2031-06-30")
	creerExercice(t, database, "Court-2", "2031-07-01", "2031-12-31")
	j := func(m, d int) time.Time { return time.Date(2031, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	ecritureSimple(t, s, database, j(3, 15), "1020", "3200", 1000)
	ecritureSimple(t, s, database, j(9, 15), "1020", "3200", 500)

	avant := len(exercices(t, database))
	fermerExercice(t, database, "Court-1")
	if got := exercices(t, database); len(got) != avant {
		t.Fatalf("la clôture du premier exercice a créé un exercice qui chevauche le second : %v", got)
	}
	fermerExercice(t, database, "Court-2") // échouait avec une contrainte d'unicité

	var refs []string
	rows, err := database.Query(`SELECT reference FROM journal_entries WHERE reference LIKE 'CLOTURE-%' ORDER BY reference`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		rows.Scan(&r)
		refs = append(refs, r)
	}
	if len(refs) != 2 || refs[0] != "CLOTURE-2031" || refs[1] != "CLOTURE-2031-2" {
		t.Fatalf("références de clôture = %v, attendu [CLOTURE-2031 CLOTURE-2031-2]", refs)
	}
	// Le compte de résultat est soldé par exercice : ni l'une ni l'autre n'est orpheline.
	if got := soldeCompte(t, database, "3200"); got != 0 {
		t.Errorf("solde du compte 3200 après les deux clôtures = %.2f, attendu 0", got)
	}
}

// L'exercice suivant est jugé sur les DATES : un exercice déjà là — même sous un autre
// nom — couvre la période, et rien n'est créé par-dessus.
func TestLaClotureNeCreePasUnExerciceSurUneperiodeDejaCouverte(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	creerExercice(t, database, "2027", "2027-01-01", "2027-12-31")
	creerExercice(t, database, "Exercice 2028", "2028-01-01", "2028-12-31")
	ecritureSimple(t, s, database, time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC), "1020", "3200", 100)

	avant := len(exercices(t, database))
	fermerExercice(t, database, "2027")
	if got := exercices(t, database); len(got) != avant {
		t.Fatalf("un exercice a été créé par-dessus « Exercice 2028 » : %v", got)
	}
}

// Sans exercice suivant, il est créé — avec le nom de l'année de sa FIN, et un autre nom
// si celui-là est pris par un exercice qui ne chevauche pas.
func TestLaClotureCreeLExerciceSuivantAvecUnNomLibre(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	creerExercice(t, database, "2029", "2029-01-01", "2029-12-31")
	// Un exercice lointain qui porte déjà le nom « 2030 » sans chevaucher 2030.
	creerExercice(t, database, "2030", "2040-01-01", "2040-12-31")
	ecritureSimple(t, s, database, time.Date(2029, 5, 1, 0, 0, 0, 0, time.UTC), "1020", "3200", 100)

	fermerExercice(t, database, "2029")
	var n int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM fiscal_years WHERE start_date = '2030-01-01' AND end_date = '2030-12-31'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("l'exercice 2030 n'a pas été créé (n=%d, err=%v) : %v", n, err, exercices(t, database))
	}
	var nom string
	database.QueryRow(`SELECT name FROM fiscal_years WHERE start_date = '2030-01-01'`).Scan(&nom)
	if nom == "2030" {
		t.Errorf("le nouvel exercice reprend le nom d'un autre exercice : %q", nom)
	}
}

// La réparation d'une clôture retrouve SA clôture par l'exercice, plus par l'année de fin :
// avec « CLOTURE-2031-2 », l'ancienne déduction visait la clôture de l'autre exercice.
func TestLaReparationRetrouveLaClotureDeSonExercice(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	f1 := creerExercice(t, database, "Court-1", "2031-01-01", "2031-06-30")
	f2 := creerExercice(t, database, "Court-2", "2031-07-01", "2031-12-31")
	j := func(m, d int) time.Time { return time.Date(2031, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	ecritureSimple(t, s, database, j(3, 15), "1020", "3200", 1000)
	ecritureSimple(t, s, database, j(9, 15), "1020", "3200", 500)
	fermerExercice(t, database, "Court-1")
	fermerExercice(t, database, "Court-2")

	fy := NewFiscalYearService(database, false)
	for id, ref := range map[string]string{f1: "CLOTURE-2031", f2: "CLOTURE-2031-2"} {
		// Les clôtures s'équilibrent : le refus attendu est « rien à réparer », et il doit
		// venir de l'écriture de CET exercice (une référence introuvable donnerait
		// « aucune écriture de clôture »).
		_, err := fy.PlanClosingRepair(context.Background(), id)
		if err != ErrReparationClotureEquilibree {
			t.Errorf("exercice %s (%s) : %v, attendu %v", id, ref, err, ErrReparationClotureEquilibree)
		}
	}
}

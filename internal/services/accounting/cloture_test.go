package accounting

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"
)

// Audit croisé Gemini, CRIT-04 : la clôture scellait une écriture déséquilibrée dès
// qu'un produit était débiteur net (rabais, avoirs) ou une charge créditrice nette
// (ristournes) : la somme de ces comptes entrait dans le résultat, mais ils
// n'avaient pas de ligne de clôture. Avec 5 000 de rabais et 2 000 de ristourne :
// 100 000 au débit pour 97 000 au crédit, scellés, et les deux comptes restaient
// ouverts sur l'exercice suivant.

// ecritureSimple passe une écriture à deux lignes entre deux comptes désignés par code.
func ecritureSimple(t *testing.T, s *Service, database *sql.DB, date time.Time, debit, credit string, montant float64) {
	t.Helper()
	id := func(code string) string {
		var v string
		if err := database.QueryRow(`SELECT id FROM accounts WHERE code = ?`, code).Scan(&v); err != nil {
			t.Fatalf("compte %s: %v", code, err)
		}
		return v
	}
	d, c := montant, montant
	e, err := s.CreateEntry(context.Background(), "u1", CreateEntryRequest{
		Date: date, Description: "essai " + debit + "/" + credit,
		Lines: []LineInput{
			{AccountID: id(debit), DebitAmount: &d, Sequence: 1},
			{AccountID: id(credit), CreditAmount: &c, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatalf("créer %s/%s: %v", debit, credit, err)
	}
	if err := s.PostEntry(context.Background(), "u1", e.ID, "127.0.0.1"); err != nil {
		t.Fatalf("comptabiliser %s/%s: %v", debit, credit, err)
	}
}

func fermerExercice(t *testing.T, database *sql.DB, name string) {
	t.Helper()
	var fyID string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = ?`, name).Scan(&fyID); err != nil {
		t.Fatal(err)
	}
	if err := NewFiscalYearService(database, false).CloseYear(context.Background(), fyID, "u1"); err != nil {
		t.Fatalf("clôture %s: %v", name, err)
	}
}

// totauxCloture rend (débit, crédit) de l'écriture de clôture, en centimes.
func totauxCloture(t *testing.T, database *sql.DB, ref string) (int64, int64) {
	t.Helper()
	var d, c sql.NullFloat64
	if err := database.QueryRow(`
		SELECT COALESCE(SUM(l.debit_amount),0), COALESCE(SUM(l.credit_amount),0)
		FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id WHERE e.reference = ?`, ref).Scan(&d, &c); err != nil {
		t.Fatal(err)
	}
	return int64(math.Round(d.Float64 * 100)), int64(math.Round(c.Float64 * 100))
}

func soldeCompte(t *testing.T, database *sql.DB, code string) float64 {
	t.Helper()
	var d, c sql.NullFloat64
	if err := database.QueryRow(`
		SELECT COALESCE(SUM(l.debit_amount),0), COALESCE(SUM(l.credit_amount),0)
		FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id JOIN accounts a ON a.id = l.account_id
		WHERE a.code = ? AND e.status = 'posted'`, code).Scan(&d, &c); err != nil {
		t.Fatal(err)
	}
	return math.Round((d.Float64-c.Float64)*100) / 100
}

func TestLaClotureEquilibreUnProduitDebiteurEtUneChargeCreditrice(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	j := func(m, d int) time.Time { return time.Date(2024, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	ecritureSimple(t, s, database, j(6, 1), "1020", "3200", 100000) // vente
	ecritureSimple(t, s, database, j(6, 2), "3600", "1020", 5000)   // produit DÉBITEUR (rabais)
	ecritureSimple(t, s, database, j(6, 3), "6000", "1020", 40000)  // loyer
	ecritureSimple(t, s, database, j(6, 4), "1020", "4200", 2000)   // charge CRÉDITRICE (ristourne)

	fermerExercice(t, database, "2024")

	d, c := totauxCloture(t, database, "CLOTURE-2024")
	if d == 0 || d != c {
		t.Fatalf("l'écriture de clôture est scellée déséquilibrée : débit %.2f, crédit %.2f", float64(d)/100, float64(c)/100)
	}
	// Plus aucun compte de produits ou de charges ne reste ouvert.
	for _, code := range []string{"3200", "3600", "6000", "4200"} {
		if v := soldeCompte(t, database, code); v != 0 {
			t.Errorf("le compte %s garde un solde de %.2f après la clôture", code, v)
		}
	}
	// Résultat : 100 000 − 5 000 − 40 000 + 2 000 = 57 000 au crédit de 5900.
	if v := soldeCompte(t, database, "5900"); v != -57000 {
		t.Errorf("compte 5900 : solde %.2f, attendu -57000.00 (bénéfice de 57 000 au crédit)", v)
	}
}

func TestLaClotureVireUnePerteAuDebitDuResultat(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	ecritureSimple(t, s, database, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), "1020", "3200", 1000)
	ecritureSimple(t, s, database, time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC), "6000", "1020", 4000)
	fermerExercice(t, database, "2024")

	d, c := totauxCloture(t, database, "CLOTURE-2024")
	if d != c {
		t.Fatalf("clôture d'une perte déséquilibrée : débit %.2f, crédit %.2f", float64(d)/100, float64(c)/100)
	}
	if v := soldeCompte(t, database, "5900"); v != 3000 {
		t.Errorf("compte 5900 : solde %.2f, attendu 3000.00 (perte au débit)", v)
	}
}

// Les centimes : des montants dont la somme flottante dérive ne doivent pas
// déséquilibrer l'écriture.
func TestLaClotureSEquilibreAuCentime(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	for i, m := range []float64{0.1, 0.2, 33.33, 12.34, 99.99, 0.07, 1234.56} {
		ecritureSimple(t, s, database, time.Date(2024, 4, i+1, 0, 0, 0, 0, time.UTC), "1020", "3200", m)
	}
	ecritureSimple(t, s, database, time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), "6000", "1020", 0.3)
	fermerExercice(t, database, "2024")
	if d, c := totauxCloture(t, database, "CLOTURE-2024"); d != c {
		t.Fatalf("débit %d ≠ crédit %d (centimes)", d, c)
	}
}

// La trace d'audit disait deux fois le débit : « credit » portait closingDebit.
func TestLaTraceDeCloturePorteLeDebitEtLeCreditReels(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	ecritureSimple(t, s, database, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "1020", "3200", 100000)
	ecritureSimple(t, s, database, time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC), "6000", "1020", 40000)
	fermerExercice(t, database, "2024")

	var after string
	if err := database.QueryRow(`SELECT after_state FROM audit_logs WHERE action = 'close' ORDER BY rowid DESC LIMIT 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	var p struct{ Debit, Credit float64 }
	if err := json.Unmarshal([]byte(after), &p); err != nil {
		t.Fatalf("trace illisible %q: %v", after, err)
	}
	d, c := totauxCloture(t, database, "CLOTURE-2024")
	if int64(math.Round(p.Debit*100)) != d || int64(math.Round(p.Credit*100)) != c {
		t.Errorf("la trace dit débit %.2f / crédit %.2f, l'écriture porte %.2f / %.2f",
			p.Debit, p.Credit, float64(d)/100, float64(c)/100)
	}
}

// Deux clôtures simultanées : une seule aboutit, et il n'y a qu'une écriture de
// clôture. Les vérifications précédaient la transaction.
func TestDeuxCloturesSimultaneesNEnProduisentQuUne(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	ecritureSimple(t, s, database, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "1020", "3200", 1000)
	var fyID string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2024'`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}
	fy := NewFiscalYearService(database, false)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fy.CloseYear(context.Background(), fyID, "u1")
		}()
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("%d clôtures ont abouti, attendu exactement 1 (%v)", ok, errs)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE reference = 'CLOTURE-2024'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d écriture(s) de clôture (%v), attendu 1", n, err)
	}
}

package accounting

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"
)

// Audit croisé Gemini, CRIT-04 : les bases dont l'exercice a été clos par une version antérieure
// à la 1.6.6 portent une clôture scellée déséquilibrée. Une écriture ordinaire ne la rétablit
// pas ; la réparation passe le COMPLÉMENT — les lignes qui manquaient — et rien d'autre.

// cloturerALAncienne reproduit EXACTEMENT ce que l'ancienne clôture scellait : une ligne par
// compte « du bon côté », et le compte de résultat calculé sur TOUS les comptes. Les comptes à
// solde inversé (3600 produit débiteur, 4200 charge créditrice) n'ont pas de ligne.
func cloturerALAncienne(t *testing.T, database *sql.DB, annee int) {
	t.Helper()
	id := func(code string) string {
		var v string
		if err := database.QueryRow(`SELECT id FROM accounts WHERE code = ?`, code).Scan(&v); err != nil {
			t.Fatalf("compte %s: %v", code, err)
		}
		return v
	}
	var fy string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = ?`, itoa(annee)).Scan(&fy); err != nil {
		t.Fatal(err)
	}
	ent := "ancienne-cloture"
	ref := "CLOTURE-" + itoa(annee)
	if _, err := database.Exec(`INSERT INTO journal_entries (id, reference, date, description, status, fiscal_year_id, created_by_id)
		VALUES (?, ?, ?, 'Clôture exercice', 'draft', ?, 'u1')`, ent, ref, itoa(annee)+"-12-31", fy); err != nil {
		t.Fatal(err)
	}
	ligne := func(n int, code string, debit, credit any) {
		t.Helper()
		if _, err := database.Exec(`INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount, sequence)
			VALUES (?, ?, ?, ?, ?, ?)`, ent+"-"+itoa(n), ent, id(code), debit, credit, n); err != nil {
			t.Fatal(err)
		}
	}
	// Produits 3200 : 100 000 au crédit → débité. Charge 6000 : 40 000 au débit → créditée.
	// Résultat sur TOUS les comptes : 100 000 − 5 000 (3600) − 40 000 + 2 000 (4200) = 57 000.
	ligne(1, "3200", 100000.0, nil)
	ligne(2, "6000", nil, 40000.0)
	ligne(3, "5900", nil, 57000.0)
	if _, err := database.Exec(`UPDATE journal_entries SET status = 'posted' WHERE id = ?`, ent); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE fiscal_years SET is_closed = 1 WHERE id = ?`, fy); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// exerciceDefectueux monte la situation de l'audit : 5 000 de rabais, 2 000 de ristourne, clôture
// de l'ancienne version.
func exerciceDefectueux(t *testing.T) (*FiscalYearService, *sql.DB, string) {
	t.Helper()
	s, database, _, _ := newAccountingDB(t)
	j := func(m, d int) time.Time { return time.Date(2024, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	ecritureSimple(t, s, database, j(6, 1), "1020", "3200", 100000)
	ecritureSimple(t, s, database, j(6, 2), "3600", "1020", 5000)
	ecritureSimple(t, s, database, j(6, 3), "6000", "1020", 40000)
	ecritureSimple(t, s, database, j(6, 4), "1020", "4200", 2000)
	cloturerALAncienne(t, database, 2024)
	var fy string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2024'`).Scan(&fy); err != nil {
		t.Fatal(err)
	}
	return NewFiscalYearService(database, false), database, fy
}

// totauxGeneraux : débit et crédit de TOUT le journal comptabilisé, en centimes.
func totauxGeneraux(t *testing.T, database *sql.DB) (int64, int64) {
	t.Helper()
	var d, c sql.NullFloat64
	if err := database.QueryRow(`
		SELECT COALESCE(SUM(l.debit_amount),0), COALESCE(SUM(l.credit_amount),0)
		FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id WHERE e.status = 'posted'`).Scan(&d, &c); err != nil {
		t.Fatal(err)
	}
	return int64(math.Round(d.Float64 * 100)), int64(math.Round(c.Float64 * 100))
}

func TestLaReparationRetablitLaBalanceDeVerification(t *testing.T) {
	fys, database, fy := exerciceDefectueux(t)
	ctx := context.Background()

	// Préambule : la base est bien dans l'état de l'audit.
	if d, c := totauxGeneraux(t, database); d == c {
		t.Fatalf("préambule : la balance s'équilibre déjà (%d / %d) — la situation de l'audit n'est pas reproduite", d, c)
	}
	if solde := soldeCompte(t, database, "3600"); solde != 5000 {
		t.Fatalf("préambule : 3600 vaut %.2f, attendu 5000 (resté ouvert)", solde)
	}

	// L'aperçu ne passe rien.
	plan, err := fys.PlanClosingRepair(ctx, fy)
	if err != nil {
		t.Fatalf("aperçu : %v", err)
	}
	if plan.Gap != 3000 || plan.Reference != "CLOTURE-2024-C" || plan.Date != "2024-12-31" {
		t.Errorf("aperçu : écart %.2f, référence %s, date %s — attendu 3000.00 / CLOTURE-2024-C / 2024-12-31", plan.Gap, plan.Reference, plan.Date)
	}
	lignes := map[string]ClosingRepairLine{}
	for _, l := range plan.Lines {
		lignes[l.AccountCode] = l
	}
	if lignes["3600"].Credit != 5000 || lignes["4200"].Debit != 2000 || len(plan.Lines) != 2 {
		t.Errorf("lignes du complément : %+v, attendu 3600 au crédit de 5000 et 4200 au débit de 2000, rien d'autre", plan.Lines)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM journal_entries WHERE reference = 'CLOTURE-2024-C'`); n != 0 {
		t.Fatal("l'aperçu a écrit une écriture")
	}

	// La réparation.
	fait, err := fys.RepairClosing(ctx, fy, "u1")
	if err != nil {
		t.Fatalf("réparation : %v", err)
	}
	if fait.EntryID == "" {
		t.Fatal("aucun identifiant d'écriture rendu")
	}
	if d, c := totauxGeneraux(t, database); d != c {
		t.Errorf("après la réparation, la balance ne s'équilibre pas : %d / %d", d, c)
	}
	if solde := soldeCompte(t, database, "3600"); solde != 0 {
		t.Errorf("3600 reste ouvert : %.2f", solde)
	}
	if solde := soldeCompte(t, database, "4200"); solde != 0 {
		t.Errorf("4200 reste ouvert : %.2f", solde)
	}
	// La clôture d'origine n'a pas bougé.
	if d, c := totauxCloture(t, database, "CLOTURE-2024"); d != 10000000 || c != 9700000 {
		t.Errorf("la clôture d'origine a été modifiée : %d / %d", d, c)
	}
	// Chaînée au journal d'audit, lignes scellées.
	if err := New(database, false).VerifyEntryIntegrity(ctx, fait.EntryID); err != nil {
		t.Errorf("le complément n'est pas vérifiable dans la chaîne : %v", err)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM audit_logs WHERE action = 'close_repair' AND record_id = ?`, fait.EntryID); n != 1 {
		t.Errorf("%d maillon(s) close_repair, attendu 1", n)
	}

	// Une seconde réparation est refusée : le complément existe.
	if _, err := fys.RepairClosing(ctx, fy, "u1"); !errors.Is(err, ErrReparationDejaFaite) {
		t.Errorf("seconde réparation : %v, attendu ErrReparationDejaFaite", err)
	}
}

func TestUneClotureQuiSEquilibreNeSeRepareesPas(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	j := func(m, d int) time.Time { return time.Date(2024, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	ecritureSimple(t, s, database, j(6, 1), "1020", "3200", 100000)
	ecritureSimple(t, s, database, j(6, 2), "3600", "1020", 5000)
	fermerExercice(t, database, "2024") // la clôture de cette version, équilibrée
	var fy string
	_ = database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2024'`).Scan(&fy)
	if _, err := NewFiscalYearService(database, false).RepairClosing(context.Background(), fy, "u1"); !errors.Is(err, ErrReparationClotureEquilibree) {
		t.Fatalf("clôture équilibrée : %v, attendu ErrReparationClotureEquilibree", err)
	}
}

func TestUnExerciceOuvertNeSeRepareesPas(t *testing.T) {
	s, database, _, _ := newAccountingDB(t)
	ecritureSimple(t, s, database, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "1020", "3200", 100)
	var fy string
	_ = database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2024'`).Scan(&fy)
	if _, err := NewFiscalYearService(database, false).RepairClosing(context.Background(), fy, "u1"); !errors.Is(err, ErrReparationExerciceOuvert) {
		t.Fatalf("exercice ouvert : %v, attendu ErrReparationExerciceOuvert", err)
	}
}

// Un écart que les comptes restés ouverts n'expliquent pas n'est pas « réparé » : on ne l'enjambe pas.
func TestUnEcartInexpliqueEstRefuse(t *testing.T) {
	fys, database, fy := exerciceDefectueux(t)
	// On fausse la ligne du compte de résultat de la clôture d'origine : l'écart devient 2 000 alors que
	// les comptes restés ouverts en expliqueraient 3 000 — il a donc une autre cause.
	if _, err := database.Exec(`DROP TRIGGER IF EXISTS trg_journal_lines_no_update_posted`); err != nil {
		t.Fatal(err)
	}
	res, err := database.Exec(`UPDATE journal_lines SET credit_amount = 58000 WHERE id = 'ancienne-cloture-3'`)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("préambule : la falsification a touché %d ligne(s), attendu 1", n)
	}
	if _, err := fys.RepairClosing(context.Background(), fy, "u1"); !errors.Is(err, ErrReparationInexpliquee) {
		t.Fatalf("écart inexpliqué : %v, attendu ErrReparationInexpliquee", err)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM journal_entries WHERE reference = 'CLOTURE-2024-C'`); n != 0 {
		t.Fatal("une écriture a été passée malgré le refus")
	}
}

func compte(t *testing.T, database *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

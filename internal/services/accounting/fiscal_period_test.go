package accounting

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// Ces tests couvrent un défaut silencieux : `fiscal_year_id` n'était renseigné
// nulle part avant la v1.4.6. `CloseYear` sélectionnant les soldes à virer avec
// `WHERE je.fiscal_year_id = ?`, il ne voyait aucune écriture et clôturait
// l'exercice **sans produire d'écriture de clôture**, en répondant « closed ».
// Vérifié sur un serveur réel avant correction : 10 000.- de produits n'avaient
// jamais été virés au résultat, et l'exercice était marqué clos.

func postAt(t *testing.T, s *Service, bank, sales string, date time.Time, amount float64) string {
	t.Helper()
	debit, credit := amount, amount
	entry, err := s.CreateEntry(context.Background(), "u1", CreateEntryRequest{
		Date:        date,
		Description: "Vente",
		Lines: []LineInput{
			{AccountID: bank, DebitAmount: &debit, Sequence: 1},
			{AccountID: sales, CreditAmount: &credit, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatalf("créer l'écriture au %s: %v", date.Format("2006-01-02"), err)
	}
	if err := s.PostEntry(context.Background(), "u1", entry.ID, "127.0.0.1"); err != nil {
		t.Fatalf("comptabiliser: %v", err)
	}
	return entry.ID
}

// ─── Rattachement ────────────────────────────────────────────────────────────

func TestCreateEntryAttachesToAFiscalPeriod(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postAt(t, s, bank, sales, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), 1000)

	var fyID sql.NullString
	if err := database.QueryRow(
		`SELECT fiscal_year_id FROM journal_entries LIMIT 1`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}
	if !fyID.Valid || fyID.String == "" {
		t.Fatal("l'écriture n'est rattachée à aucun exercice : elle est invisible à la clôture")
	}

	var name, start, end string
	if err := database.QueryRow(
		`SELECT name, start_date, end_date FROM fiscal_years WHERE id = ?`, fyID.String,
	).Scan(&name, &start, &end); err != nil {
		t.Fatal(err)
	}
	if name != "2026" || start[:10] != "2026-01-01" || end[:10] != "2026-12-31" {
		t.Fatalf("exercice créé = %s (%s → %s), attendu l'année civile 2026", name, start[:10], end[:10])
	}
}

// Un exercice décalé déclaré à l'avance doit être respecté, pas doublé d'une
// année civile — c'est le cas d'usage de la route POST /fiscal-years.
func TestCreateEntryRespectsAnExplicitOffsetPeriod(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	if _, err := database.Exec(
		`INSERT INTO fiscal_years (id, name, start_date, end_date) VALUES ('fy', '2026/27', '2026-07-01', '2027-06-30')`,
	); err != nil {
		t.Fatal(err)
	}

	postAt(t, s, bank, sales, time.Date(2027, 2, 10, 0, 0, 0, 0, time.UTC), 500)

	var fyID string
	if err := database.QueryRow(`SELECT fiscal_year_id FROM journal_entries LIMIT 1`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}
	if fyID != "fy" {
		t.Fatalf("rattachée à %q au lieu de l'exercice décalé déclaré", fyID)
	}

	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM fiscal_years`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d exercices : une année civile a été créée par-dessus l'exercice décalé", n)
	}
}

// ─── Verrouillage de période ─────────────────────────────────────────────────

func TestCreateEntryRefusesAClosedPeriod(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postAt(t, s, bank, sales, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), 1000)
	if _, err := database.Exec(`UPDATE fiscal_years SET is_closed = 1`); err != nil {
		t.Fatal(err)
	}

	debit, credit := 50.0, 50.0
	_, err := s.CreateEntry(context.Background(), "u1", CreateEntryRequest{
		Date:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Description: "Antidatée",
		Lines: []LineInput{
			{AccountID: bank, DebitAmount: &debit, Sequence: 1},
			{AccountID: sales, CreditAmount: &credit, Sequence: 2},
		},
	})
	var closed ErrPeriodClosed
	if !errors.As(err, &closed) {
		t.Fatalf("erreur = %v (%T), attendu ErrPeriodClosed : un exercice bouclé ne doit plus bouger (CO art. 958f)", err, err)
	}
}

// Le chemin qui compte vraiment : un brouillon créé AVANT la clôture, puis
// comptabilisé APRÈS. Contrôler seulement à la création le laisserait passer.
func TestPostEntryRefusesAPeriodClosedAfterTheDraft(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)

	debit, credit := 100.0, 100.0
	entry, err := s.CreateEntry(context.Background(), "u1", CreateEntryRequest{
		Date:        time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
		Description: "Brouillon d'avant clôture",
		Lines: []LineInput{
			{AccountID: bank, DebitAmount: &debit, Sequence: 1},
			{AccountID: sales, CreditAmount: &credit, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := database.Exec(`UPDATE fiscal_years SET is_closed = 1`); err != nil {
		t.Fatal(err)
	}

	err = s.PostEntry(context.Background(), "u1", entry.ID, "127.0.0.1")
	var closed ErrPeriodClosed
	if !errors.As(err, &closed) {
		t.Fatalf("erreur = %v (%T), attendu ErrPeriodClosed", err, err)
	}
}

// L'exercice ouvert suivant reste écrivable : le verrouillage ne doit pas
// bloquer la correction, il doit la déplacer là où elle est légale.
func TestOpenPeriodStaysWritableAfterAClosure(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postAt(t, s, bank, sales, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), 1000)
	if _, err := database.Exec(`UPDATE fiscal_years SET is_closed = 1 WHERE name = '2026'`); err != nil {
		t.Fatal(err)
	}

	postAt(t, s, bank, sales, time.Date(2027, 1, 8, 0, 0, 0, 0, time.UTC), 250)
}

// ─── Clôture ─────────────────────────────────────────────────────────────────

// La régression centrale : la clôture doit virer les produits au résultat.
func TestCloseYearProducesTheClosingEntry(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postAt(t, s, bank, sales, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), 5000)
	postAt(t, s, bank, sales, time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), 5000)

	var fyID string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2026'`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}

	fy := NewFiscalYearService(database, false)
	if err := fy.CloseYear(context.Background(), fyID, "u1"); err != nil {
		t.Fatalf("clôture: %v", err)
	}

	var lines int
	var debitTotal sql.NullFloat64
	err := database.QueryRow(`
		SELECT COUNT(*), SUM(l.debit_amount)
		FROM journal_lines l
		JOIN journal_entries e ON e.id = l.entry_id
		WHERE e.reference LIKE 'CLOTURE%'`).Scan(&lines, &debitTotal)
	if err != nil {
		t.Fatal(err)
	}
	if lines == 0 {
		t.Fatal("la clôture n'a produit AUCUNE écriture : les produits n'ont jamais été virés au résultat, " +
			"alors que l'exercice est marqué clos")
	}
	if !debitTotal.Valid || debitTotal.Float64 != 10000 {
		t.Fatalf("total débité par la clôture = %v, attendu 10000 (les deux ventes)", debitTotal.Float64)
	}
}

// L'écriture de clôture doit entrer dans la chaîne comme les autres. Elle était
// insérée directement en 'posted', sans empreinte ni maillon d'audit : la
// pièce qui vire le résultat de l'exercice était la seule hors chaîne
// (CO art. 957a), et le contrôle de cohérence la signalait déjà.
func TestClosingEntryJoinsTheAuditChain(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postAt(t, s, bank, sales, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), 5000)

	var fyID string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2026'`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}
	if err := NewFiscalYearService(database, false).CloseYear(context.Background(), fyID, "u1"); err != nil {
		t.Fatalf("clôture: %v", err)
	}

	var closingID, status string
	var hash sql.NullString
	if err := database.QueryRow(
		`SELECT id, status, integrity_hash FROM journal_entries WHERE reference LIKE 'CLOTURE%'`,
	).Scan(&closingID, &status, &hash); err != nil {
		t.Fatal(err)
	}
	if status != "posted" {
		t.Fatalf("écriture de clôture au statut %q, attendu posted", status)
	}
	if !hash.Valid || hash.String == "" {
		t.Fatal("l'écriture de clôture n'a pas d'empreinte d'intégrité : elle échappe au CO art. 957a")
	}

	var audits int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE record_id = ?`, closingID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("%d maillon(s) d'audit pour l'écriture de clôture, attendu 1", audits)
	}

	// Et la chaîne doit rester continue : deux écritures, deux maillons.
	var maxSeq, total int64
	if err := database.QueryRow(
		`SELECT COALESCE(MAX(sequence_number),0), COUNT(*) FROM audit_logs`).Scan(&maxSeq, &total); err != nil {
		t.Fatal(err)
	}
	if total != 2 || maxSeq != 2 {
		t.Fatalf("chaîne = %d maillon(s), séquence max %d ; attendu 2 et 2", total, maxSeq)
	}
}

// La clôture doit refuser un exercice contenant des brouillons. Ce contrôle
// filtrait lui aussi sur fiscal_year_id, donc il ne voyait rien.
func TestCloseYearRefusesRemainingDrafts(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postAt(t, s, bank, sales, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), 5000)

	debit, credit := 300.0, 300.0
	if _, err := s.CreateEntry(context.Background(), "u1", CreateEntryRequest{
		Date:        time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		Description: "Resté en brouillon",
		Lines: []LineInput{
			{AccountID: bank, DebitAmount: &debit, Sequence: 1},
			{AccountID: sales, CreditAmount: &credit, Sequence: 2},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var fyID string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2026'`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}

	fy := NewFiscalYearService(database, false)
	if err := fy.CloseYear(context.Background(), fyID, "u1"); err == nil {
		t.Fatal("un exercice contenant un brouillon a été clôturé sans protestation")
	}
}

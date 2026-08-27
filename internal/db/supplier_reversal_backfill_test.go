package db

// Le troisième audit a établi qu'une extourne fournisseur passée avant sa
// correction se décrivait comme telle dans son libellé, tout en portant
// is_reversal = 0 et aucun reversal_of_id — ce qui partait ainsi dans
// l'archive légale. Ces tests reproduisent EXACTEMENT ce qu'écrivait
// l'ancien code, sans passer par lui : une écriture de comptabilisation, une
// facture fournisseur annulée qui la référence, et une écriture d'extourne
// au libellé conforme mais aux colonnes non renseignées.

import (
	"database/sql"
	"testing"
)

// seedContact insère un contact minimal.
func seedContact(t *testing.T, database *sql.DB, id, name string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO contacts (id, contact_type, name, country, is_active)
		 VALUES (?, 'supplier', ?, 'CH', 1)`, id, name); err != nil {
		t.Fatalf("seed contact %s: %v", id, err)
	}
}

// seedPostedEntry insère une écriture déjà comptabilisée, sans passer par le
// service — pour simuler une base ANTÉRIEURE à la correction, où le
// déclencheur d'immuabilité n'a jamais vu ces lignes s'écrire une par une.
func seedPostedEntry(t *testing.T, database *sql.DB, id, reference, description string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
		VALUES (?, ?, '2026-01-15', ?, 'posted', 'u1')`, id, reference, description,
	); err != nil {
		t.Fatalf("seed entry %s: %v", id, err)
	}
}

// seedCancelledSupplierInvoice insère une facture fournisseur ANNULÉE dont
// journal_entry_id pointe vers l'écriture d'ORIGINE — exactement ce que
// laisse le code actuel, qui ne réécrit jamais cette colonne à l'annulation.
func seedCancelledSupplierInvoice(t *testing.T, database *sql.DB, id, supplierID, reference, origineID string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO supplier_invoices
		    (id, supplier_id, supplier_reference, status, issue_date, due_date,
		     currency, subtotal_amount, vat_amount, total_amount, amount_paid,
		     journal_entry_id, created_by_id)
		VALUES (?, ?, ?, 'cancelled', '2026-01-10', '2026-02-09',
		        'CHF', 100, 0, 100, 0, ?, 'u1')`,
		id, supplierID, reference, origineID); err != nil {
		t.Fatalf("seed supplier invoice %s: %v", id, err)
	}
}

func journalEntryFlags(t *testing.T, database *sql.DB, id string) (isReversal int, reversalOf sql.NullString) {
	t.Helper()
	if err := database.QueryRow(
		`SELECT is_reversal, reversal_of_id FROM journal_entries WHERE id = ?`, id,
	).Scan(&isReversal, &reversalOf); err != nil {
		t.Fatalf("lecture %s: %v", id, err)
	}
	return
}

// LE cas nominal : une extourne fournisseur historique, non marquée, avec une
// seule origine possible. Elle doit être marquée et rattachée.
func TestBackfillMarqueUneExtourneFournisseurHistorique(t *testing.T) {
	database := newBackfillDB(t)
	seedContact(t, database, "sup1", "Fournisseur Historique SA")

	seedPostedEntry(t, database, "orig1", "JN-2026-001", "Facture fournisseur FA-100")
	seedPostedEntry(t, database, "ext1", "JN-2026-002",
		"Extourne facture fournisseur FA-100 — saisie d'essai")
	seedCancelledSupplierInvoice(t, database, "inv1", "sup1", "FA-100", "orig1")

	if err := BackfillSupplierReversalMarkers(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	isRev, revOf := journalEntryFlags(t, database, "ext1")
	if isRev != 1 {
		t.Errorf("is_reversal = %d, attendu 1", isRev)
	}
	if !revOf.Valid || revOf.String != "orig1" {
		t.Errorf("reversal_of_id = %v, attendu orig1", revOf)
	}

	// L'écriture d'ORIGINE, elle, ne doit pas bouger : ce n'est pas elle
	// l'extourne.
	isRevOrig, _ := journalEntryFlags(t, database, "orig1")
	if isRevOrig != 0 {
		t.Error("l'écriture d'origine a été marquée comme extourne")
	}
}

// Idempotence : rejouer le rattrapage sur une base déjà corrigée ne doit rien
// changer, et surtout ne pas échouer — la sélection ne doit plus rien
// trouver.
func TestBackfillEstIdempotent(t *testing.T) {
	database := newBackfillDB(t)
	seedContact(t, database, "sup1", "Fournisseur SA")
	seedPostedEntry(t, database, "orig1", "JN-2026-001", "Facture fournisseur FA-200")
	seedPostedEntry(t, database, "ext1", "JN-2026-002",
		"Extourne facture fournisseur FA-200 — annulation")
	seedCancelledSupplierInvoice(t, database, "inv1", "sup1", "FA-200", "orig1")

	if err := BackfillSupplierReversalMarkers(database, false); err != nil {
		t.Fatalf("premier passage: %v", err)
	}
	if err := BackfillSupplierReversalMarkers(database, false); err != nil {
		t.Fatalf("second passage: %v", err)
	}

	isRev, revOf := journalEntryFlags(t, database, "ext1")
	if isRev != 1 || !revOf.Valid || revOf.String != "orig1" {
		t.Errorf("état après deux passages: is_reversal=%d reversal_of_id=%v", isRev, revOf)
	}
}

// Une extourne CLIENTE, déjà marquée par le chemin qui fonctionnait, ne doit
// pas être retouchée : le motif de sélection est le libellé fournisseur,
// jamais rencontré ici.
func TestBackfillNeTouchePasAuxExtournesClientesDejaMarquees(t *testing.T) {
	database := newBackfillDB(t)
	seedPostedEntry(t, database, "origc1", "JN-2026-009", "Facture FC-2026-001")
	if _, err := database.Exec(`
		INSERT INTO journal_entries
		    (id, reference, date, description, status, is_reversal, reversal_of_id, created_by_id)
		VALUES ('extc1', 'JN-2026-010', '2026-01-15',
		        'Contrepassation facture FC-2026-001', 'posted', 1, 'origc1', 'u1')`,
	); err != nil {
		t.Fatal(err)
	}

	if err := BackfillSupplierReversalMarkers(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	isRev, revOf := journalEntryFlags(t, database, "extc1")
	if isRev != 1 || !revOf.Valid || revOf.String != "origc1" {
		t.Errorf("l'extourne cliente a été altérée : is_reversal=%d reversal_of_id=%v", isRev, revOf)
	}
}

// Deux fournisseurs distincts ayant utilisé la même référence ET le même
// motif produisent un libellé d'extourne IDENTIQUE. Deviner lequel des deux
// est la bonne origine serait pire que ne rien faire : les deux candidates
// restent non marquées, et le fait est journalisé.
func TestBackfillNeDevinePasSurUneReferenceAmbigue(t *testing.T) {
	database := newBackfillDB(t)
	seedContact(t, database, "sup1", "Premier Fournisseur SA")
	seedContact(t, database, "sup2", "Second Fournisseur SA")

	seedPostedEntry(t, database, "origA", "JN-2026-001", "Facture fournisseur A")
	seedPostedEntry(t, database, "origB", "JN-2026-002", "Facture fournisseur B")
	seedPostedEntry(t, database, "extAmbigu", "JN-2026-003",
		"Extourne facture fournisseur DOUBLON — annulation")
	seedCancelledSupplierInvoice(t, database, "invA", "sup1", "DOUBLON", "origA")
	seedCancelledSupplierInvoice(t, database, "invB", "sup2", "DOUBLON", "origB")

	if err := BackfillSupplierReversalMarkers(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	isRev, revOf := journalEntryFlags(t, database, "extAmbigu")
	if isRev != 0 || revOf.Valid {
		t.Errorf("une extourne ambiguë a été marquée au hasard : is_reversal=%d reversal_of_id=%v",
			isRev, revOf)
	}
}

// Une extourne dont l'origine a été purgée (aucune facture annulée ne la
// référence) reste non marquée plutôt que de faire échouer le démarrage.
func TestBackfillLaisseUneExtourneSansOrigineTrouvee(t *testing.T) {
	database := newBackfillDB(t)
	seedPostedEntry(t, database, "extOrpheline", "JN-2026-099",
		"Extourne facture fournisseur INTROUVABLE — annulation")

	if err := BackfillSupplierReversalMarkers(database, false); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	isRev, revOf := journalEntryFlags(t, database, "extOrpheline")
	if isRev != 0 || revOf.Valid {
		t.Error("une extourne sans origine trouvée a été marquée")
	}
}

// Le déclencheur d'immuabilité doit refuser toute autre modification que
// celle-ci, même après la migration 0028 : marquer l'extourne ne doit pas
// ouvrir la porte à changer, par exemple, sa description.
func TestLeDeclencheurRefuseToujoursLesAutresModifications(t *testing.T) {
	database := newBackfillDB(t)
	seedPostedEntry(t, database, "e1", "JN-2026-050", "Description d'origine")

	_, err := database.Exec(
		`UPDATE journal_entries SET description = 'Modifiée' WHERE id = ?`, "e1")
	if err == nil {
		t.Fatal("une modification de description sur une écriture comptabilisée a été acceptée")
	}
}

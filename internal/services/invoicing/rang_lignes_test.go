package invoicing

// Le rang des lignes (retour sur la 1.6.6-rc2).
//
// L'écran d'une facture affichait « 0, 0, 0 » dans la colonne « # » : la création écrivait le rang tel que
// le client l'envoyait — et l'interface n'en envoie pas, donc 0 pour toutes les lignes. `ORDER BY sequence`
// ne départageait plus rien. La modification, elle, posait déjà la position.

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// rangsEnBase lit les rangs écrits, dans l'ordre où la facture les rend.
func (f *fixture) rangsEnBase(t *testing.T, invoiceID string) []int {
	t.Helper()
	rows, err := f.db.Query(`SELECT sequence FROM invoice_lines WHERE invoice_id = ? ORDER BY sequence, description`, invoiceID)
	if err != nil {
		t.Fatalf("lecture des rangs: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func (f *fixture) creerSansRang(t *testing.T, docType string, lignes ...LineInput) *models.Invoice {
	t.Helper()
	issued := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	inv, err := f.svc.CreateInvoice(context.Background(), f.userID, CreateInvoiceRequest{
		DocumentType: docType, ContactID: f.contactID,
		IssueDate: issued, DueDate: issued.AddDate(0, 0, 30), Currency: "CHF", Lines: lignes,
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	return inv
}

func troisLignesSansRang() []LineInput {
	return []LineInput{
		{Description: "Ligne A", Quantity: 1, UnitPrice: 100, VATRate: 8.1},
		{Description: "Ligne B", Quantity: 1, UnitPrice: 200, VATRate: 8.1},
		{Description: "Ligne C", Quantity: 1, UnitPrice: 300, VATRate: 8.1},
	}
}

// Des lignes envoyées sans rang prennent leur position, à partir de 1 — et non 0 pour toutes.
func TestLesLignesSansRangPrennentLeurPosition(t *testing.T) {
	f := newFixture(t)
	inv := f.creerSansRang(t, DocumentTypeInvoice, troisLignesSansRang()...)
	if got, want := f.rangsEnBase(t, inv.ID), []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rangs écrits = %v, attendu %v", got, want)
	}
}

// Un rang donné par le client est respecté.
func TestUnRangDonneParLeClientEstRespecte(t *testing.T) {
	f := newFixture(t)
	inv := f.creerSansRang(t, DocumentTypeInvoice,
		LineInput{Description: "Ligne A", Quantity: 1, UnitPrice: 100, VATRate: 8.1, Sequence: 10},
		LineInput{Description: "Ligne B", Quantity: 1, UnitPrice: 200, VATRate: 8.1, Sequence: 20},
	)
	if got, want := f.rangsEnBase(t, inv.ID), []int{10, 20}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rangs écrits = %v, attendu %v", got, want)
	}
}

// Une offre convertie en facture, et une note de crédit, recopient les lignes lues : si l'original porte des 0
// (une facture d'avant ce correctif), la copie ne doit pas les reprendre.
func TestLaConversionEtLaNoteDeCreditNeRecopientPasDesRangsNuls(t *testing.T) {
	f := newFixture(t)

	offre := f.creerSansRang(t, DocumentTypeQuote, troisLignesSansRang()...)
	// Simule une offre d'avant le correctif : tous les rangs à 0.
	if _, err := f.db.Exec(`UPDATE invoice_lines SET sequence = 0 WHERE invoice_id = ?`, offre.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Transition(context.Background(), offre.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("envoi de l'offre: %v", err)
	}
	conv, err := f.svc.ConvertQuote(context.Background(), offre.ID, f.userID, ConvertQuoteRequest{
		IssueDate: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ConvertQuote: %v", err)
	}
	if got, want := f.rangsEnBase(t, conv.ID), []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("facture issue de l'offre : rangs %v, attendu %v", got, want)
	}

	// Note de crédit sur une facture dont les rangs sont nuls.
	fact := f.creerSansRang(t, DocumentTypeInvoice, troisLignesSansRang()...)
	if _, err := f.db.Exec(`UPDATE invoice_lines SET sequence = 0 WHERE invoice_id = ?`, fact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE invoices SET status = 'sent' WHERE id = ?`, fact.ID); err != nil {
		t.Fatal(err)
	}
	note, err := f.svc.CreateCreditNote(context.Background(), fact.ID, f.userID, CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	if got, want := f.rangsEnBase(t, note.ID), []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("note de crédit : rangs %v, attendu %v", got, want)
	}
}

func TestRangDeLigne(t *testing.T) {
	for _, c := range []struct{ seq, pos, want int }{
		{0, 0, 1}, {0, 4, 5}, {7, 0, 7}, {3, 9, 3}, {-1, 2, 3},
	} {
		if got := rangDeLigne(c.seq, c.pos); got != c.want {
			t.Errorf("rangDeLigne(%d, %d) = %d, attendu %d", c.seq, c.pos, got, c.want)
		}
	}
}

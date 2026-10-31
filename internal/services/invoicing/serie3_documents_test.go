package invoicing

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// Audit croisé Gemini, série 3 — les documents dérivés (avoirs, factures nées d'une offre).
//
//   - CRIT-11 : ils naissaient sans exercice (`fiscal_year_id` NULL), et l'archive légale
//     filtrée par exercice les omettait.
//   - HIGH-02 : leur taux d'en-tête valait 8.1 dès que la première ligne était à 0 %, et la
//     déclaration TVA, qui groupe par ce taux, comptait de l'exonéré comme du taxable.
//   - HIGH-13 : une facture archivée n'était plus créditable, et la tolérance du plafond de
//     crédit valait cinq centimes à la création, un centime à la modification.

func exercice(t *testing.T, database *sql.DB, id string) (fy sql.NullString, vatRate float64) {
	t.Helper()
	if err := database.QueryRow(`SELECT fiscal_year_id, vat_rate FROM invoices WHERE id = ?`, id).Scan(&fy, &vatRate); err != nil {
		t.Fatal(err)
	}
	return
}

func TestUnAvoirEtUneFactureNeeDUneOffreSontRattachesAUnExercice(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()

	inv := facture1000(t, s, contactID)
	note, err := s.CreateCreditNote(ctx, inv, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if fy, _ := exercice(t, database, note.ID); !fy.Valid || fy.String == "" {
		t.Error("l'avoir n'a pas d'exercice : l'archive légale filtrée par exercice l'omet")
	}

	offre, err := s.CreateInvoice(ctx, "u1", CreateInvoiceRequest{
		ContactID: contactID, DocumentType: DocumentTypeQuote,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), DueDate: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency: "CHF", Lines: []LineInput{{Description: "Étude", Quantity: 1, UnitPrice: 500}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(ctx, offre.ID, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	conv, err := s.ConvertQuote(ctx, offre.ID, "u1", ConvertQuoteRequest{IssueDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if fy, _ := exercice(t, database, conv.ID); !fy.Valid || fy.String == "" {
		t.Error("la facture née de l'offre n'a pas d'exercice")
	}
}

// Une facture exonérée (première ligne à 0 %) donnait un avoir et une facture convertie à 8.1.
func TestLeTauxDEnteteDesDocumentsDerivesSuitLesLignes(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()

	inv := facture1000(t, s, contactID) // 0 %
	note, err := s.CreateCreditNote(ctx, inv, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, rate := exercice(t, database, note.ID); rate != 0 {
		t.Errorf("avoir d'une facture à 0 %% : taux d'en-tête %.1f, attendu 0", rate)
	}

	offre, err := s.CreateInvoice(ctx, "u1", CreateInvoiceRequest{
		ContactID: contactID, DocumentType: DocumentTypeQuote,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), DueDate: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency: "CHF", Lines: []LineInput{{Description: "Export", Quantity: 1, UnitPrice: 500, VATRate: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(ctx, offre.ID, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	conv, err := s.ConvertQuote(ctx, offre.ID, "u1", ConvertQuoteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, rate := exercice(t, database, conv.ID); rate != 0 {
		t.Errorf("facture née d'une offre à 0 %% : taux d'en-tête %.1f, attendu 0", rate)
	}
}

func TestUneFactureArchiveeSeCredite(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	inv := facture1000(t, s, contactID)
	if _, err := s.RecordPayment(ctx, acteur, paiement(inv, 1000)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE invoices SET status = 'archived' WHERE id = ?`, inv); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCreditNote(ctx, inv, "u1", CreateCreditNoteRequest{
		Lines: []LineInput{{Description: "Remboursement", Quantity: 1, UnitPrice: 200}},
	}); err != nil {
		t.Fatalf("créditer une facture archivée : %v", err)
	}
}

// Les brouillons et les factures annulées restent, eux, non créditables.
func TestUnBrouillonNeSeCreditePas(t *testing.T) {
	s, _, contactID := postingService(t, true)
	draft := makeInvoice(t, s, contactID, 100, 0)
	if _, err := s.CreateCreditNote(context.Background(), draft, "u1", CreateCreditNoteRequest{}); !errors.Is(err, ErrInvoiceNotCreditable) {
		t.Fatalf("avoir sur un brouillon : %v, attendu ErrInvoiceNotCreditable", err)
	}
}

// Un avoir créé dans la tolérance de la création doit pouvoir être enregistré à nouveau
// tel quel : le contrôle de modification valait un centime, celui de création cinq.
func TestLaToleranceDuPlafondDeCreditEstLaMemePartout(t *testing.T) {
	s, _, contactID := postingService(t, true)
	ctx := context.Background()
	inv := makeInvoice(t, s, contactID, 100, 0)
	if err := s.TransitionBy(ctx, inv, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	// 100.04 : dans les 5 centimes de tolérance.
	note, err := s.CreateCreditNote(ctx, inv, "u1", CreateCreditNoteRequest{
		Lines: []LineInput{{Description: "Reprise", Quantity: 1, UnitPrice: 100.04}},
	})
	if err != nil {
		t.Fatalf("création dans la tolérance : %v", err)
	}
	if _, err := s.UpdateInvoiceBy(ctx, note.ID, CreateInvoiceRequest{
		ContactID: contactID, IssueDate: note.IssueDate, DueDate: note.DueDate, Currency: "CHF",
		Lines: []LineInput{{Description: "Reprise", Quantity: 1, UnitPrice: 100.04}},
	}, acteur); err != nil {
		t.Fatalf("réenregistrer le même avoir : %v — la modification est plus stricte que la création", err)
	}
	// Au-delà de la tolérance, c'est refusé.
	if _, err := s.CreateCreditNote(ctx, inv, "u1", CreateCreditNoteRequest{
		Lines: []LineInput{{Description: "Trop", Quantity: 1, UnitPrice: 0.10}},
	}); !errors.Is(err, ErrCreditExceedsInvoice) {
		t.Fatalf("dépassement : %v, attendu ErrCreditExceedsInvoice", err)
	}
}

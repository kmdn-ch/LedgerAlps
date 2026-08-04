package invoicing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Deux garde-fous, et le trou par lequel le premier fuyait.
//
// Le plafond de crédit était vérifié à la CRÉATION d'une note de crédit et
// nulle part ailleurs : modifier ses lignes ensuite le contournait entièrement.
// Une note de 500 sur une facture de 500 passait à 1500 en HTTP 200,
// sous-évaluant d'autant la TVA due. Mesuré sur un serveur réel.
//
// En le corrigeant, un défaut plus grave est apparu : le champ `document_type`
// omis dans une modification retombait sur « invoice ». Modifier une note de
// crédit la transformait donc en facture — le lien vers la pièce corrigée
// survivait mais le document changeait de nature, ce que la LTVA art. 27 al. 4
// ne prévoit pas.

func newGuardDB(t *testing.T, vatNumber string) (*Service, *sql.DB, string) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-guard-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,is_admin) VALUES ('u1','a@t.ch','A','x',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO company_settings (id, company_name, vat_number, address_country)
		 VALUES ('cs','Test SA',?,'CH')`, vatNumber); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO contacts (id,contact_type,name,country,payment_term_days,is_active)
		 VALUES ('c1','customer','Client','CH',30,1)`); err != nil {
		t.Fatal(err)
	}
	return New(database, false), database, "c1"
}

func makeInvoice(t *testing.T, s *Service, contactID string, price, vatRate float64) string {
	t.Helper()
	inv, err := s.CreateInvoice(context.Background(), "u1", CreateInvoiceRequest{
		ContactID: contactID,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF",
		Lines:     []LineInput{{Description: "Prestation", Quantity: 1, UnitPrice: price, VATRate: vatRate}},
	})
	if err != nil {
		t.Fatalf("créer la facture: %v", err)
	}
	return inv.ID
}

// ─── Plafond de crédit ───────────────────────────────────────────────────────

func TestUpdateCannotInflateACreditNoteBeyondTheInvoice(t *testing.T) {
	s, database, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	invID := makeInvoice(t, s, contactID, 500, 0)
	if _, err := database.Exec(`UPDATE invoices SET status='sent' WHERE id=?`, invID); err != nil {
		t.Fatal(err)
	}

	note, err := s.CreateCreditNote(context.Background(), invID, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("créer la note de crédit: %v", err)
	}

	_, err = s.UpdateInvoice(context.Background(), note.ID, CreateInvoiceRequest{
		ContactID: contactID,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF",
		Lines:     []LineInput{{Description: "Gonflée", Quantity: 1, UnitPrice: 1500, VATRate: 0}},
	})
	if !errors.Is(err, ErrCreditExceedsInvoice) {
		t.Fatalf("erreur = %v, attendu ErrCreditExceedsInvoice — sinon la TVA due est sous-évaluée", err)
	}

	var total float64
	if err := database.QueryRow(`SELECT total_amount FROM invoices WHERE id=?`, note.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total > 500.01 {
		t.Fatalf("la note a été gonflée malgré le refus : %.2f", total)
	}
}

// Réduire reste possible : un crédit partiel est légitime, et bloquer toute
// modification reviendrait à traiter le garde-fou comme une interdiction.
func TestUpdateCanReduceACreditNote(t *testing.T) {
	s, database, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	invID := makeInvoice(t, s, contactID, 500, 0)
	if _, err := database.Exec(`UPDATE invoices SET status='sent' WHERE id=?`, invID); err != nil {
		t.Fatal(err)
	}
	note, err := s.CreateCreditNote(context.Background(), invID, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpdateInvoice(context.Background(), note.ID, CreateInvoiceRequest{
		ContactID: contactID,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF",
		Lines:     []LineInput{{Description: "Partielle", Quantity: 1, UnitPrice: 200, VATRate: 0}},
	}); err != nil {
		t.Fatalf("réduire une note de crédit est refusé : %v", err)
	}
}

// ─── Le type de document ne change pas ──────────────────────────────────────

func TestUpdateKeepsTheDocumentType(t *testing.T) {
	s, database, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	invID := makeInvoice(t, s, contactID, 500, 0)
	if _, err := database.Exec(`UPDATE invoices SET status='sent' WHERE id=?`, invID); err != nil {
		t.Fatal(err)
	}
	note, err := s.CreateCreditNote(context.Background(), invID, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Champ omis : « inchangé », et non « facture ».
	if _, err := s.UpdateInvoice(context.Background(), note.ID, CreateInvoiceRequest{
		ContactID: contactID,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF",
		Lines:     []LineInput{{Description: "Corrigée", Quantity: 1, UnitPrice: 100, VATRate: 0}},
	}); err != nil {
		t.Fatalf("modification refusée: %v", err)
	}

	var docType string
	if err := database.QueryRow(`SELECT document_type FROM invoices WHERE id=?`, note.ID).Scan(&docType); err != nil {
		t.Fatal(err)
	}
	if docType != DocumentTypeCreditNote {
		t.Fatalf("la note de crédit est devenue %q : le lien LTVA art. 27 al. 4 ne tient plus", docType)
	}
}

func TestUpdateRefusesAnExplicitTypeChange(t *testing.T) {
	s, _, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	invID := makeInvoice(t, s, contactID, 500, 0)

	_, err := s.UpdateInvoice(context.Background(), invID, CreateInvoiceRequest{
		DocumentType: DocumentTypeQuote,
		ContactID:    contactID,
		IssueDate:    time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:      time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:     "CHF",
		Lines:        []LineInput{{Description: "X", Quantity: 1, UnitPrice: 100, VATRate: 0}},
	})
	if !errors.Is(err, ErrDocumentTypeImmutable) {
		t.Fatalf("erreur = %v, attendu ErrDocumentTypeImmutable", err)
	}
}

// ─── TVA sans numéro de TVA ──────────────────────────────────────────────────

func TestCannotChargeVATWithoutAVATNumber(t *testing.T) {
	s, _, contactID := newGuardDB(t, "")

	_, err := s.CreateInvoice(context.Background(), "u1", CreateInvoiceRequest{
		ContactID: contactID,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF",
		Lines:     []LineInput{{Description: "Avec TVA", Quantity: 1, UnitPrice: 100, VATRate: 8.1}},
	})
	if !errors.Is(err, ErrVATWithoutNumber) {
		t.Fatalf("erreur = %v, attendu ErrVATWithoutNumber : la LTVA art. 27 al. 2 rend redevable de la TVA mentionnée à tort", err)
	}
}

func TestZeroVATIsAllowedWithoutAVATNumber(t *testing.T) {
	s, _, contactID := newGuardDB(t, "")
	makeInvoice(t, s, contactID, 100, 0) // échoue via t.Fatalf si refusé
}

func TestVATIsAllowedWithAVATNumber(t *testing.T) {
	s, _, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	makeInvoice(t, s, contactID, 100, 8.1)
}

// Une installation neuve n'a pas encore de fiche société : refuser sa première
// facture ressemblerait à une panne.
func TestVATIsAllowedWhenNoCompanySettingsRowExists(t *testing.T) {
	s, database, contactID := newGuardDB(t, "")
	if _, err := database.Exec(`DELETE FROM company_settings`); err != nil {
		t.Fatal(err)
	}
	makeInvoice(t, s, contactID, 100, 8.1)
}

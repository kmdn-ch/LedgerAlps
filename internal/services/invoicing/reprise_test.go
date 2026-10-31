package invoicing

import (
	"context"
	"errors"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// La reprise rattrape un document envoyé dont l'écriture manque (audit 9, A-01),
// sans jamais écrire deux fois.

// envoyeeSansEcriture produit l'état que laissait la 1.6.5 : document envoyé,
// un encaissement saisi, aucune écriture — la comptabilisation automatique étant
// éteinte au moment des faits, puis allumée.
func envoyeeSansEcriture(t *testing.T) (*Service, string, func(query string, args ...any) int) {
	t.Helper()
	s, database, contactID := postingService(t, false)
	ctx := context.Background()
	id := makeInvoice(t, s, contactID, 1000, 0)
	if err := s.TransitionBy(ctx, id, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatalf("émission: %v", err)
	}
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 300)); err != nil {
		t.Fatalf("encaissement: %v", err)
	}
	if _, err := database.Exec(`UPDATE company_settings SET auto_post_invoices = 1`); err != nil {
		t.Fatal(err)
	}
	return s, id, func(q string, a ...any) int { return compte(t, database, q, a...) }
}

func TestLaRepriseComptabiliseLEmissionEtLesEncaissementsOrphelins(t *testing.T) {
	s, id, n := envoyeeSansEcriture(t)
	database := s.db
	ctx := context.Background()

	res, err := s.PostPending(ctx, acteur, id)
	if err != nil {
		t.Fatalf("reprise: %v", err)
	}
	if res.JournalEntryID == "" || res.PaymentEntries != 1 {
		t.Fatalf("reprise = %+v, attendu l'écriture d'émission et un encaissement", res)
	}
	// 1000 à l'émission, 300 encaissés : la créance restante est de 700.
	if d, c := balance(t, database, accountReceivables); d != 1000 || c != 300 {
		t.Fatalf("débiteurs débit %.2f / crédit %.2f, attendu 1000 / 300", d, c)
	}
	if d, _ := balance(t, database, accountBank); d != 300 {
		t.Fatalf("banque au débit = %.2f, attendu 300", d)
	}
	if n(`SELECT COUNT(*) FROM payments WHERE COALESCE(journal_entry_id,'') = ''`) != 0 {
		t.Fatal("un encaissement reste sans écriture")
	}

	// Un second appel ne repasse rien.
	if _, err := s.PostPending(ctx, acteur, id); !errors.Is(err, ErrNothingToPost) {
		t.Fatalf("second appel : %v, attendu ErrNothingToPost", err)
	}
	if n(`SELECT COUNT(*) FROM journal_entries WHERE status='posted'`) != 2 {
		t.Fatal("la reprise a écrit plus que l'émission et l'encaissement")
	}

	// La reprise conduit à l'état qu'aurait donné un envoi réussi : l'encaissement
	// suivant se comptabilise.
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 700)); err != nil {
		t.Fatalf("encaissement après reprise: %v", err)
	}
	if d, c := balance(t, database, accountReceivables); d != c {
		t.Fatalf("la créance n'est pas soldée au journal : débit %.2f, crédit %.2f", d, c)
	}
}

func TestLaRepriseRefuseCeQuiNEstPasUnDocumentEnvoye(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()

	brouillon := makeInvoice(t, s, contactID, 100, 0)
	if _, err := s.PostPending(ctx, acteur, brouillon); !errors.Is(err, ErrPostingNotApplicable) {
		t.Errorf("brouillon : %v, attendu ErrPostingNotApplicable", err)
	}
	if _, err := s.PostPending(ctx, acteur, "inexistant"); !errors.Is(err, ErrInvoiceNotFound) {
		t.Errorf("inconnu : %v, attendu ErrInvoiceNotFound", err)
	}

	// Comptabilisation éteinte : la comptabilité est tenue à la main.
	envoyee := makeInvoice(t, s, contactID, 100, 0)
	if _, err := database.Exec(`UPDATE company_settings SET auto_post_invoices = 0`); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(ctx, envoyee, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostPending(ctx, acteur, envoyee); !errors.Is(err, ErrAutoPostOff) {
		t.Errorf("réglage éteint : %v, attendu ErrAutoPostOff", err)
	}
}

// Une note de crédit contrepasse le produit de sa facture : passée la première,
// elle créerait un produit négatif sans contrepartie.
func TestLaRepriseDUneNoteDeCreditExigeLaFactureDAbord(t *testing.T) {
	s, database, contactID := postingService(t, false)
	ctx := context.Background()
	id := makeInvoice(t, s, contactID, 500, 0)
	if err := s.TransitionBy(ctx, id, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	note, err := s.CreateCreditNote(ctx, id, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(ctx, note.ID, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatalf("envoi de la note: %v", err)
	}
	if _, err := database.Exec(`UPDATE company_settings SET auto_post_invoices = 1`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.PostPending(ctx, acteur, note.ID); !errors.Is(err, ErrOriginalNotPosted) {
		t.Fatalf("note avant sa facture : %v, attendu ErrOriginalNotPosted", err)
	}
	if _, err := s.PostPending(ctx, acteur, id); err != nil {
		t.Fatalf("reprise de la facture: %v", err)
	}
	if _, err := s.PostPending(ctx, acteur, note.ID); err != nil {
		t.Fatalf("reprise de la note après sa facture: %v", err)
	}
	// Facture et note se compensent au journal.
	if d, c := balance(t, database, accountReceivables); d != c {
		t.Fatalf("débiteurs débit %.2f ≠ crédit %.2f après facture + note", d, c)
	}
}

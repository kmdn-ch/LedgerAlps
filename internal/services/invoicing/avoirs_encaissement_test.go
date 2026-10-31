package invoicing

import (
	"context"
	"errors"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// Audit croisé Gemini, CRIT-05 : les encaissements ignoraient les notes de crédit.
//
//   - Après un avoir de 300 sur 1 000, le client réglait ses 700 : le cumul (700)
//     n'atteignait pas le total (1 000), la facture restait « envoyée » à vie et
//     passait « en retard » alors que la créance au grand livre était à zéro.
//   - « Marquer payée » après un avoir intégral encaissait la totalité de la
//     facture : banque +2 000, débiteurs −2 000 — de la trésorerie fictive.

func avoir(t *testing.T, s *Service, invID string, montant float64) string {
	t.Helper()
	ctx := context.Background()
	req := CreateCreditNoteRequest{}
	if montant > 0 {
		req.Lines = []LineInput{{Description: "Rabais", Quantity: 1, UnitPrice: montant}}
	}
	note, err := s.CreateCreditNote(ctx, invID, "u1", req)
	if err != nil {
		t.Fatalf("avoir: %v", err)
	}
	if err := s.TransitionBy(ctx, note.ID, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatalf("envoi de l'avoir: %v", err)
	}
	return note.ID
}

func TestLeSoldeDUneFactureEstNetDesAvoirs(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)
	avoir(t, s, id, 300)

	// Plus que 700 à régler : refus au-delà, acceptation exacte.
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 800)); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("800 sur 1000 − 300 : %v, attendu ErrPaymentExceedsBalance", err)
	}
	res, err := s.RecordPayment(ctx, acteur, paiement(id, 700))
	if err != nil {
		t.Fatalf("versement de 700 : %v", err)
	}
	if res.Status != models.InvoiceStatusPaid || res.Balance != 0 {
		t.Fatalf("après 700 : statut %s, solde %.2f — attendu paid / 0 (l'avoir de 300 solde le reste)", res.Status, res.Balance)
	}
	if status, paid := etat(t, database, id); status != "paid" || paid != 700 {
		t.Fatalf("en base : %s / %.2f, attendu paid / 700.00", status, paid)
	}
	// Au grand livre, la créance est soldée : 1000 − 300 − 700.
	if d, c := balance(t, database, accountReceivables); d-c != 0 {
		t.Fatalf("créance 1100 = %.2f, attendu 0", d-c)
	}
}

func TestMarquerPayeeApresUnAvoirIntegralNEncaisseRien(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := makeInvoice(t, s, contactID, 2000, 0)
	if err := s.TransitionBy(ctx, id, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	avoir(t, s, id, 0) // avoir intégral

	banqueAvant, banqueCreditAvant := balance(t, database, accountBank)
	if err := s.TransitionBy(ctx, id, models.InvoiceStatusPaid, acteur); err != nil {
		t.Fatalf("« Marquer payée » : %v", err)
	}
	d, c := balance(t, database, accountBank)
	if d != banqueAvant || c != banqueCreditAvant {
		t.Fatalf("la banque a bougé de %.2f : un avoir intégral ne laisse rien à encaisser", d-banqueAvant)
	}
	if d, c := balance(t, database, accountReceivables); d-c != 0 {
		t.Fatalf("créance 1100 = %.2f, attendu 0 (facture 2000 − avoir 2000)", d-c)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments WHERE invoice_id = ?`, id); n != 0 {
		t.Fatalf("%d encaissement(s) enregistré(s), attendu 0", n)
	}
}

// Un avoir en brouillon n'engage personne : il ne réduit pas le dû.
func TestUnAvoirEnBrouillonNeReduitPasLeDu(t *testing.T) {
	s, _, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)
	if _, err := s.CreateCreditNote(ctx, id, "u1", CreateCreditNoteRequest{
		Lines: []LineInput{{Description: "Rabais", Quantity: 1, UnitPrice: 300}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 1000)); err != nil {
		t.Fatalf("1000 sur une facture dont l'avoir n'est pas émis : %v", err)
	}
}

package invoicing

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// Audit 9, A-01 : une facture dont le HT n'était pas un multiple de 0,05 partait
// au client, puis son écriture d'émission était refusée (débit = total arrondi à
// 5 centimes, crédits = HT + TVA). Statut « envoyée », aucune écriture, aucun
// moyen de reprise. 80 % des montants au centime près étaient concernés ; les
// tests n'utilisaient que des montants ronds.

// Le total est HT + TVA, exactement, pour n'importe quel montant au centime.
func TestLeTotalEstHTPlusTVAAuCentime(t *testing.T) {
	rng := rand.New(rand.NewSource(20261030))
	rates := []float64{0, 2.6, 3.8, 8.1}
	for i := 0; i < 20000; i++ {
		ht := float64(rng.Intn(5_000_000)) / 100
		rate := rates[rng.Intn(len(rates))]
		sub, vat, total := computeTotals([]LineInput{{Quantity: 1, UnitPrice: ht, VATRate: rate}})
		if want := compliance.RoundCents(sub + vat); total != want {
			t.Fatalf("HT %.2f à %.1f %% : total %.2f ≠ HT %.2f + TVA %.2f = %.2f", ht, rate, total, sub, vat, want)
		}
		if sub != ht {
			t.Fatalf("HT %.2f altéré en %.2f : le HT d'une ligne est conservé au centime", ht, sub)
		}
	}
}

// Plusieurs lignes, plusieurs taux, quantités décimales et rabais : le total
// reste la somme de ce que le document imprime.
func TestLesLignesSAdditionnentAuCentime(t *testing.T) {
	lines := []LineInput{
		{Quantity: 3, UnitPrice: 0.126, VATRate: 8.1},                    // 0.378 → 0.38
		{Quantity: 1.5, UnitPrice: 19.99, DiscountPct: 10, VATRate: 8.1}, // 26.9865 → 26.99
		{Quantity: 7, UnitPrice: 3.33, VATRate: 2.6},                     // 23.31
		{Quantity: 1, UnitPrice: 100, VATRate: 0},
	}
	sub, vat, total := computeTotals(lines)
	var sum float64
	for _, l := range lines {
		sum += lineBase(l)
	}
	if want := compliance.RoundCents(sum); sub != want {
		t.Fatalf("HT = %.2f, la somme des lignes arrondies vaut %.2f", sub, want)
	}
	if total != compliance.RoundCents(sub+vat) {
		t.Fatalf("total %.2f ≠ HT %.2f + TVA %.2f", total, sub, vat)
	}
	if got := lineBase(lines[0]); got != 0.38 {
		t.Errorf("0.126 × 3 = %.4f, attendu 0.38", got)
	}
}

// Le cœur d'A-01 : chaque montant du rapport s'émet, s'équilibre et se lie.
func TestUneFactureAuCentimeSeComptabiliseALEmission(t *testing.T) {
	s, database, contactID := postingService(t, true)
	for _, ht := range []float64{12.34, 33.33, 99.99, 0.01, 1234.56, 0.07} {
		for _, rate := range []float64{0, 2.6, 8.1} {
			invID := makeInvoice(t, s, contactID, ht, rate)
			if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
				t.Fatalf("HT %.2f à %.1f %% : l'envoi échoue : %v", ht, rate, err)
			}
			var linked string
			if err := database.QueryRow(
				`SELECT COALESCE(journal_entry_id,'') FROM invoices WHERE id=?`, invID).Scan(&linked); err != nil {
				t.Fatal(err)
			}
			if linked == "" {
				t.Fatalf("HT %.2f à %.1f %% : envoyée mais sans écriture au journal", ht, rate)
			}
		}
	}
	var deb, cre float64
	if err := database.QueryRow(`
		SELECT COALESCE(SUM(jl.debit_amount),0), COALESCE(SUM(jl.credit_amount),0)
		FROM journal_lines jl JOIN journal_entries je ON je.id = jl.entry_id
		WHERE je.status = 'posted'`).Scan(&deb, &cre); err != nil {
		t.Fatal(err)
	}
	if math.Abs(deb-cre) > 1e-6 {
		t.Fatalf("le journal ne s'équilibre pas : débit %.4f, crédit %.4f", deb, cre)
	}
}

// Une facture établie AVANT ce correctif porte un total arrondi à 5 centimes, ce
// que le client a reçu. Son écriture doit porter ce montant, et l'écart rejoint
// le produit : sinon la reprise d'une facture restée sans écriture échouerait
// de la même façon que l'envoi.
func TestUneAncienneFactureArrondieA5CentimesSeComptabilise(t *testing.T) {
	s, database, contactID := postingService(t, true)
	invID := makeInvoice(t, s, contactID, 12.34, 0)
	if _, err := database.Exec(`UPDATE invoices SET total_amount = 12.35 WHERE id = ?`, invID); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("envoi d'une facture au total arrondi à 5 centimes : %v", err)
	}
	if d, _ := balance(t, database, accountReceivables); d != 12.35 {
		t.Errorf("débiteurs au débit = %.2f, attendu 12.35 (ce que le client doit)", d)
	}
	if _, c := balance(t, database, accountRevenue); c != 12.35 {
		t.Errorf("produits au crédit = %.2f, attendu 12.35 (HT 12.34 + 0.01 d'arrondi)", c)
	}
}

// Le crédit intégral reprend les montants de la facture : recalculés au centime,
// ils s'écarteraient de quelques centimes d'une ancienne facture arrondie.
func TestLeCreditIntegralReprendLesMontantsDeLaFacture(t *testing.T) {
	s, database, contactID := postingService(t, false)
	invID := makeInvoice(t, s, contactID, 12.34, 8.1)
	if _, err := database.Exec(
		`UPDATE invoices SET status='sent', total_amount = 13.35, vat_amount = 1.00 WHERE id = ?`, invID); err != nil {
		t.Fatal(err)
	}
	note, err := s.CreateCreditNote(context.Background(), invID, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("note de crédit intégrale : %v", err)
	}
	if note.TotalAmount != 13.35 || note.VATAmount != 1.00 || note.SubtotalAmount != 12.34 {
		t.Fatalf("la note porte HT %.2f / TVA %.2f / total %.2f, attendu 12.34 / 1.00 / 13.35",
			note.SubtotalAmount, note.VATAmount, note.TotalAmount)
	}
}

// ─── Bornes (audit 9, A-03, A-05, M-05) ──────────────────────────────────────

func demande(contactID string, issue, due time.Time, cur string, lines ...LineInput) CreateInvoiceRequest {
	return CreateInvoiceRequest{ContactID: contactID, IssueDate: issue, DueDate: due, Currency: cur, Lines: lines}
}

var (
	jourOK  = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	echeOK  = time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	ligneOK = LineInput{Description: "Prestation", Quantity: 1, UnitPrice: 100, VATRate: 8.1}
)

func TestUnDocumentRefuseLesValeursAbsurdes(t *testing.T) {
	s, _, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	ctx := context.Background()

	avec := func(f func(l *LineInput)) LineInput { l := ligneOK; f(&l); return l }
	lourd := make([]LineInput, 501)
	for i := range lourd {
		lourd[i] = ligneOK
	}
	cas := []struct {
		nom string
		req CreateInvoiceRequest
	}{
		{"quantité négative", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.Quantity = -3 }))},
		{"quantité zéro", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.Quantity = 0 }))},
		{"prix négatif", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.UnitPrice = -250 }))},
		{"rabais 150", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.DiscountPct = 150 }))},
		{"rabais négatif", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.DiscountPct = -50 }))},
		{"TVA négative", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.VATRate = -8.1 }))},
		{"TVA 1e9", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.VATRate = 1e9 }))},
		{"prix 1e300", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.UnitPrice = 1e300 }))},
		{"produit 1e15 x 1e15", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.Quantity, l.UnitPrice = 1e15, 1e15 }))},
		{"quantité x prix au-delà de la borne", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.Quantity, l.UnitPrice = 999_999, 999_999 }))},
		{"NaN", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.UnitPrice = math.NaN() }))},
		{"501 lignes", demande(contactID, jourOK, echeOK, "", lourd...)},
		{"libellé de 3000 caractères", demande(contactID, jourOK, echeOK, "", avec(func(l *LineInput) { l.Description = strings.Repeat("é", 3000) }))},
		{"émission en l'an 26", demande(contactID, time.Date(26, 10, 30, 0, 0, 0, 0, time.UTC), echeOK, "", ligneOK)},
		{"émission en 9999", demande(contactID, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), echeOK, "", ligneOK)},
		{"émission en 1900", demande(contactID, time.Date(1900, 5, 5, 0, 0, 0, 0, time.UTC), echeOK, "", ligneOK)},
		{"échéance en 2999", demande(contactID, jourOK, time.Date(2999, 6, 1, 0, 0, 0, 0, time.UTC), "", ligneOK)},
		{"devise EUR dans des livres CHF", demande(contactID, jourOK, echeOK, "EUR", ligneOK)},
		{"devise XXX", demande(contactID, jourOK, echeOK, "XXX", ligneOK)},
	}
	for _, c := range cas {
		if _, err := s.CreateInvoice(ctx, "u1", c.req); err == nil {
			t.Errorf("%s : accepté", c.nom)
		}
	}

	// Les valeurs ordinaires et les limites admises passent.
	for nom, req := range map[string]CreateInvoiceRequest{
		"ordinaire":      demande(contactID, jourOK, echeOK, "CHF", ligneOK),
		"prix zéro":      demande(contactID, jourOK, echeOK, "", LineInput{Description: "Offert", Quantity: 1, UnitPrice: 0}),
		"rabais 100":     demande(contactID, jourOK, echeOK, "", LineInput{Description: "Offert", Quantity: 2, UnitPrice: 10, DiscountPct: 100}),
		"0.126 x 3":      demande(contactID, jourOK, echeOK, "", LineInput{Description: "x", Quantity: 3, UnitPrice: 0.126}),
		"500 lignes":     demande(contactID, jourOK, echeOK, "", lourd[:500]...),
		"devise omise":   demande(contactID, jourOK, echeOK, "", ligneOK),
		"échéance sans":  demande(contactID, jourOK, time.Time{}, "", ligneOK),
		"année courante": demande(contactID, time.Now(), time.Now().AddDate(0, 0, 30), "", ligneOK),
	} {
		if _, err := s.CreateInvoice(ctx, "u1", req); err != nil {
			t.Errorf("%s : refusé à tort : %v", nom, err)
		}
	}
}

// La devise par défaut est celle de la comptabilité, et un document dans cette
// devise passe : une société tenue en EUR n'est pas forcée au franc.
func TestLaDeviseDuDocumentEstCelleDeLaComptabilite(t *testing.T) {
	s, database, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	ctx := context.Background()
	if _, err := database.Exec(`UPDATE company_settings SET currency = 'EUR'`); err != nil {
		t.Fatal(err)
	}
	inv, err := s.CreateInvoice(ctx, "u1", demande(contactID, jourOK, echeOK, "", ligneOK))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Currency != "EUR" {
		t.Errorf("devise par défaut = %s, attendu EUR (celle de la comptabilité)", inv.Currency)
	}
	if _, err := s.CreateInvoice(ctx, "u1", demande(contactID, jourOK, echeOK, "CHF", ligneOK)); err == nil {
		t.Error("une facture en CHF dans des livres EUR est acceptée")
	}
}

func TestUnPaiementRefuseUneDateAbsurde(t *testing.T) {
	s, _, contactID := postingService(t, true)
	id := makeInvoice(t, s, contactID, 1000, 0)
	if err := s.TransitionBy(context.Background(), id, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Time{
		time.Date(2, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		_, err := s.RecordPayment(context.Background(), acteur,
			PaymentRequest{InvoiceID: id, Amount: 1, Date: d, Method: "bank_transfer"})
		if err == nil {
			t.Errorf("paiement daté du %s accepté", d.Format("2006-01-02"))
		}
	}
}

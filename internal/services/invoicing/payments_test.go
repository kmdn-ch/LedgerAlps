package invoicing

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// Un encaissement est de l'argent qui entre : il doit entrer dans les livres,
// qu'il solde la facture ou non. Ces tests rejouent le défaut tel qu'il se
// produisait — deux versements de 600 et 400 sur 1000 ne laissaient ni écriture,
// ni cumul, ni changement de statut — et les chemins voisins qu'il fallait
// tenir pour que la correction ne déplace pas le trou.

var jourPaiement = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

var acteur = Actor{UserID: "u1", IP: "127.0.0.1"}

func paiement(invID string, montant float64) PaymentRequest {
	return PaymentRequest{InvoiceID: invID, Amount: montant, Date: jourPaiement, Method: "bank_transfer"}
}

// facture1000 crée, envoie (donc comptabilise) une facture de 1000.- sans TVA.
func facture1000(t *testing.T, s *Service, contactID string) string {
	t.Helper()
	id := makeInvoice(t, s, contactID, 1000, 0)
	if err := s.TransitionBy(context.Background(), id, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatalf("émission: %v", err)
	}
	return id
}

func etat(t *testing.T, database *sql.DB, invID string) (status string, paid float64) {
	t.Helper()
	if err := database.QueryRow(`SELECT status, amount_paid FROM invoices WHERE id = ?`, invID).Scan(&status, &paid); err != nil {
		t.Fatal(err)
	}
	return
}

func compte(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Le défaut du rapport : 600 puis 400 sur 1000.
func TestDeuxVersementsPartielsSoldentLaFactureEtLaCreance(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)

	r1, err := s.RecordPayment(ctx, acteur, paiement(id, 600))
	if err != nil {
		t.Fatalf("premier versement: %v", err)
	}
	if r1.Status != models.InvoiceStatusSent || r1.AmountPaid != 600 || r1.Balance != 400 {
		t.Fatalf("après 600 : statut %s, payé %.2f, solde %.2f — attendu sent / 600 / 400", r1.Status, r1.AmountPaid, r1.Balance)
	}
	if r1.JournalEntryID == "" {
		t.Fatal("le versement partiel n'a laissé aucune écriture au journal")
	}
	if d, c := balance(t, database, accountReceivables); d-c != 400 {
		t.Fatalf("créance après 600 = %.2f, attendu 400", d-c)
	}

	r2, err := s.RecordPayment(ctx, acteur, paiement(id, 400))
	if err != nil {
		t.Fatalf("second versement: %v", err)
	}
	if r2.Status != models.InvoiceStatusPaid || r2.AmountPaid != 1000 || r2.Balance != 0 {
		t.Fatalf("après 600+400 : statut %s, payé %.2f, solde %.2f — attendu paid / 1000 / 0", r2.Status, r2.AmountPaid, r2.Balance)
	}
	if d, c := balance(t, database, accountReceivables); d-c != 0 {
		t.Fatalf("créance après 600+400 = %.2f, attendu 0 : l'argent est entré, la créance doit être soldée", d-c)
	}
	if d, c := balance(t, database, accountBank); d-c != 1000 {
		t.Fatalf("banque = %.2f, attendu 1000", d-c)
	}
	if st, paid := etat(t, database, id); st != "paid" || paid != 1000 {
		t.Fatalf("facture = %s / %.2f, attendu paid / 1000", st, paid)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments WHERE invoice_id = ?`, id); n != 2 {
		t.Fatalf("%d paiement(s) enregistré(s), attendu 2", n)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments WHERE invoice_id = ? AND journal_entry_id IS NOT NULL`, id); n != 2 {
		t.Fatalf("%d paiement(s) lié(s) à leur écriture, attendu 2", n)
	}
}

// « Marquer payée » : l'écran promet « passé au journal (banque / débiteurs) ».
func TestMarquerPayeeSoldeLaCreanceAuJournal(t *testing.T) {
	s, database, contactID := postingService(t, true)
	id := facture1000(t, s, contactID)

	if err := s.TransitionBy(context.Background(), id, models.InvoiceStatusPaid, acteur); err != nil {
		t.Fatalf("marquer payée: %v", err)
	}
	if d, c := balance(t, database, accountReceivables); d-c != 0 {
		t.Fatalf("créance après « Marquer payée » = %.2f, attendu 0 : le statut changeait, le journal non", d-c)
	}
	if d, c := balance(t, database, accountBank); d-c != 1000 {
		t.Fatalf("banque = %.2f, attendu 1000", d-c)
	}
	if st, paid := etat(t, database, id); st != "paid" || paid != 1000 {
		t.Fatalf("facture = %s / %.2f, attendu paid / 1000 (amount_paid restait à 0)", st, paid)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments WHERE invoice_id = ?`, id); n != 1 {
		t.Fatalf("%d paiement(s), attendu 1", n)
	}
}

// Après un versement partiel, « Marquer payée » encaisse le RESTE, pas le total.
func TestMarquerPayeeApresUnVersementPartielEncaisseLeReste(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 700)); err != nil {
		t.Fatal(err)
	}

	if err := s.TransitionBy(ctx, id, models.InvoiceStatusPaid, acteur); err != nil {
		t.Fatalf("marquer payée: %v", err)
	}
	if d, c := balance(t, database, accountBank); d-c != 1000 {
		t.Fatalf("banque = %.2f, attendu 1000 (700 + le reste, 300) — un total encaissé deux fois ?", d-c)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments WHERE invoice_id = ?`, id); n != 2 {
		t.Fatalf("%d paiement(s), attendu 2", n)
	}
	var reste float64
	if err := database.QueryRow(
		`SELECT amount FROM payments WHERE invoice_id = ? AND amount <> 700`, id).Scan(&reste); err != nil {
		t.Fatal(err)
	}
	if reste != 300 {
		t.Fatalf("second encaissement = %.2f, attendu le reste, 300", reste)
	}
}

// Un trop-perçu ne solde pas « un peu plus » : il est refusé, et rien ne change.
func TestUnVersementAuDessusDuSoldeEstRefuseSansRienChanger(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 600)); err != nil {
		t.Fatal(err)
	}
	entriesAvant := compte(t, database, `SELECT COUNT(*) FROM journal_entries`)

	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 400.01)); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("erreur = %v, attendu ErrPaymentExceedsBalance", err)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments WHERE invoice_id = ?`, id); n != 1 {
		t.Fatalf("%d paiement(s) après un refus, attendu 1", n)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM journal_entries`); n != entriesAvant {
		t.Fatalf("une écriture a été créée par un paiement refusé (%d → %d)", entriesAvant, n)
	}
	if _, paid := etat(t, database, id); paid != 600 {
		t.Fatalf("amount_paid = %.2f après un refus, attendu 600", paid)
	}
}

// Une installation qui tient sa comptabilité à la main n'a pas d'écriture
// d'émission : passer l'encaissement seul porterait 1100 au crédit sans débit.
func TestSansEcritureDEmissionLEncaissementNePasseRienAuJournal(t *testing.T) {
	s, database, contactID := postingService(t, false)
	id := facture1000(t, s, contactID)

	res, err := s.RecordPayment(context.Background(), acteur, paiement(id, 1000))
	if err != nil {
		t.Fatalf("encaissement: %v", err)
	}
	if res.JournalEntryID != "" {
		t.Fatalf("écriture %s passée alors que l'émission ne l'était pas", res.JournalEntryID)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM journal_entries`); n != 0 {
		t.Fatalf("%d écriture(s) au journal, attendu 0", n)
	}
	if st, paid := etat(t, database, id); st != "paid" || paid != 1000 {
		t.Fatalf("facture = %s / %.2f, attendu paid / 1000", st, paid)
	}
}

// Une note de crédit se règle dans l'autre sens : c'est un remboursement.
func TestLeReglementDUneNoteDeCreditEstUnRemboursement(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)
	note, err := s.CreateCreditNote(ctx, id, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(ctx, note.ID, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	// Facture 1000 au débit, note 1000 au crédit : la créance est à zéro.
	if d, c := balance(t, database, accountReceivables); d-c != 0 {
		t.Fatalf("créance avant remboursement = %.2f, attendu 0", d-c)
	}

	if err := s.TransitionBy(ctx, note.ID, models.InvoiceStatusPaid, acteur); err != nil {
		t.Fatalf("règlement de la note: %v", err)
	}
	// Le remboursement rend l'argent : banque au crédit, créance au débit.
	if d, c := balance(t, database, accountBank); d-c != -1000 {
		t.Fatalf("banque = %.2f, attendu -1000 (l'argent est parti)", d-c)
	}
	if d, c := balance(t, database, accountReceivables); d-c != 1000 {
		t.Fatalf("créance = %.2f, attendu 1000 (rétablie par le remboursement)", d-c)
	}
}

func TestSeuleUneFactureEnvoyeeAccepteUnPaiement(t *testing.T) {
	s, _, contactID := postingService(t, true)
	ctx := context.Background()

	brouillon := makeInvoice(t, s, contactID, 100, 0)
	if _, err := s.RecordPayment(ctx, acteur, paiement(brouillon, 100)); !errors.Is(err, ErrInvoiceNotPayable) {
		t.Errorf("brouillon : %v, attendu ErrInvoiceNotPayable", err)
	}

	payee := facture1000(t, s, contactID)
	if _, err := s.RecordPayment(ctx, acteur, paiement(payee, 1000)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordPayment(ctx, acteur, paiement(payee, 1)); !errors.Is(err, ErrInvoiceNotPayable) {
		t.Errorf("facture payée : %v, attendu ErrInvoiceNotPayable", err)
	}

	if _, err := s.RecordPayment(ctx, acteur, paiement("inconnue", 1)); !errors.Is(err, ErrInvoiceNotFound) {
		t.Errorf("facture inconnue : %v, attendu ErrInvoiceNotFound", err)
	}
	if _, err := s.RecordPayment(ctx, acteur, paiement(payee, 0)); err == nil {
		t.Error("un paiement nul a été accepté")
	}
}

// Un exercice clos refuse l'écriture : le paiement ne doit alors PAS rester
// enregistré sans elle — c'était le défaut décrit en tête de payments.go.
func TestUnExerciceCloisRefuseLEncaissementEntierement(t *testing.T) {
	s, database, contactID := postingService(t, true)
	id := facture1000(t, s, contactID)
	if _, err := database.Exec(`UPDATE fiscal_years SET is_closed = 1`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.RecordPayment(context.Background(), acteur, paiement(id, 1000)); err == nil {
		t.Fatal("encaissement accepté dans un exercice clos")
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments`); n != 0 {
		t.Fatalf("%d paiement(s) restent enregistrés sans écriture", n)
	}
	if st, paid := etat(t, database, id); st != "sent" || paid != 0 {
		t.Fatalf("facture = %s / %.2f après un refus, attendu sent / 0", st, paid)
	}
}

// Des espèces entrent en caisse, pas à la banque.
func TestDesEspecesEntrentEnCaisse(t *testing.T) {
	s, database, contactID := postingService(t, true)
	id := facture1000(t, s, contactID)

	req := paiement(id, 250)
	req.Method = "cash"
	if _, err := s.RecordPayment(context.Background(), acteur, req); err != nil {
		t.Fatal(err)
	}
	if d, c := balance(t, database, accountCash); d-c != 250 {
		t.Fatalf("caisse = %.2f, attendu 250", d-c)
	}
	if d, c := balance(t, database, accountBank); d-c != 0 {
		t.Fatalf("banque = %.2f, attendu 0 pour des espèces", d-c)
	}
}

// Annuler une facture encaissée extournerait l'émission en laissant les
// encaissements : le refus renvoie vers la note de crédit.
func TestUneFactureEncaisseeNeSAnnulePas(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := facture1000(t, s, contactID)
	if _, err := s.RecordPayment(ctx, acteur, paiement(id, 100)); err != nil {
		t.Fatal(err)
	}
	avant := compte(t, database, `SELECT COUNT(*) FROM journal_entries`)

	if err := s.TransitionBy(ctx, id, models.InvoiceStatusCancelled, acteur); !errors.Is(err, ErrInvoiceHasPayments) {
		t.Fatalf("annulation = %v, attendu ErrInvoiceHasPayments", err)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM journal_entries`); n != avant {
		t.Fatalf("une extourne a été créée par une annulation refusée (%d → %d)", avant, n)
	}
}

// Deux encaissements simultanés qui dépasseraient ensemble le solde : un seul
// passe. Sans la sérialisation, les deux lisent le même solde.
func TestDeuxEncaissementsSimultanesNeDepassentPasLeSolde(t *testing.T) {
	s, database, contactID := postingService(t, true)
	id := facture1000(t, s, contactID)

	var wg sync.WaitGroup
	var mu sync.Mutex
	acceptes, refuses := 0, 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.RecordPayment(context.Background(), acteur, paiement(id, 600))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				acceptes++
			case errors.Is(err, ErrPaymentExceedsBalance):
				refuses++
			default:
				t.Errorf("erreur inattendue: %v", err)
			}
		}()
	}
	wg.Wait()
	if acceptes != 1 || refuses != 1 {
		t.Fatalf("%d accepté(s), %d refusé(s), attendu 1 et 1", acceptes, refuses)
	}
	if _, paid := etat(t, database, id); paid != 600 {
		t.Fatalf("amount_paid = %.2f, attendu 600", paid)
	}
}

// Une facture à zéro n'a rien à encaisser : « Marquer payée » ne change que le statut.
func TestUneFactureAZeroSeMarquePayeeSansPaiement(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	id := makeInvoice(t, s, contactID, 0, 0)
	if err := s.TransitionBy(ctx, id, models.InvoiceStatusSent, acteur); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(ctx, id, models.InvoiceStatusPaid, acteur); err != nil {
		t.Fatalf("marquer payée: %v", err)
	}
	if st, _ := etat(t, database, id); st != "paid" {
		t.Fatalf("statut = %s, attendu paid", st)
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM payments`); n != 0 {
		t.Fatalf("%d paiement(s) pour une facture à zéro", n)
	}
}

// Les encaissements entrent dans la chaîne d'audit, avec leur auteur.
func TestUnEncaissementEstTrace(t *testing.T) {
	s, database, contactID := postingService(t, true)
	id := facture1000(t, s, contactID)

	res, err := s.RecordPayment(context.Background(), acteur, paiement(id, 1000))
	if err != nil {
		t.Fatal(err)
	}
	if n := compte(t, database,
		`SELECT COUNT(*) FROM audit_logs WHERE table_name = 'payments' AND record_id = ?`, res.Payment.ID); n != 1 {
		t.Fatalf("%d maillon(s) d'audit pour le paiement, attendu 1", n)
	}
}

// La migration 0029 aligne `amount_paid` des factures déjà payées (« Marquer
// payée » le laissait à zéro) sans toucher à ce qui ne doit pas bouger.
func TestLaMigrationAligneLEncaisseDesFacturesPayeesDAvant(t *testing.T) {
	s, database, contactID := postingService(t, false)

	// Quatre documents d'avant la mise à jour, dans quatre situations.
	payee := makeInvoice(t, s, contactID, 500, 0)    // payée, encaissé resté à 0
	archivee := makeInvoice(t, s, contactID, 300, 0) // archivée : on ne sait pas
	deja := makeInvoice(t, s, contactID, 200, 0)     // payée, encaissé déjà renseigné
	offre := makeInvoice(t, s, contactID, 100, 0)    // jamais de paiement sur une offre
	for id, sets := range map[string]string{
		payee:    `status = 'paid', amount_paid = 0`,
		archivee: `status = 'archived', amount_paid = 0`,
		deja:     `status = 'paid', amount_paid = 120`,
		offre:    `status = 'sent', amount_paid = 0, document_type = 'quote'`,
	} {
		if _, err := database.Exec(`UPDATE invoices SET `+sets+` WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}

	migration, err := db.MigrationSQL("0029_amount_paid_des_factures_payees")
	if err != nil {
		t.Fatal(err)
	}
	// Deux passages : la migration est rejouable sans effet.
	for i := 0; i < 2; i++ {
		if _, err := database.Exec(migration); err != nil {
			t.Fatalf("migration (passage %d): %v", i+1, err)
		}
	}

	for id, want := range map[string]float64{payee: 500, archivee: 0, deja: 120, offre: 0} {
		if _, paid := etat(t, database, id); paid != want {
			t.Errorf("document %s : amount_paid = %.2f, attendu %.2f", id[:6], paid, want)
		}
	}
	if n := compte(t, database, `SELECT COUNT(*) FROM journal_entries`); n != 0 {
		t.Errorf("la migration a créé %d écriture(s) : le journal est immuable, elle n'y touche pas", n)
	}
}

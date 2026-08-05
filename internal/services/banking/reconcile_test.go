package banking

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/iso20022"
)

// Le rapprochement bancaire touche à des mouvements d'argent. Deux erreurs y
// coûtent cher : dupliquer une écriture à la réimportation du relevé, et
// proposer une facture sur une ressemblance qu'on présenterait comme une
// analyse.

func newDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-bank-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,is_admin) VALUES ('u1','a@t.ch','A','x',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO contacts (id,contact_type,name,country,payment_term_days,is_active)
		 VALUES ('c1','customer','Boulangerie Dupont','CH',30,1)`); err != nil {
		t.Fatal(err)
	}
	return New(database, false), database
}

func addInvoice(t *testing.T, database *sql.DB, id, number string, total float64, status, qrRef string) {
	t.Helper()
	_, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status,
		                      issue_date, due_date, currency,
		                      subtotal_amount, vat_amount, total_amount, vat_rate, qr_reference,
		                      created_by_id)
		VALUES (?,?,'invoice','c1',?, '2026-05-01','2026-05-31','CHF', ?, 0, ?, 0, ?, 'u1')`,
		id, number, status, total, total, qrRef)
	if err != nil {
		t.Fatal(err)
	}
}

func entry(amount float64, qrRef string) iso20022.BankEntry {
	return iso20022.BankEntry{
		Amount:          amount,
		Currency:        "CHF",
		IsCredit:        true,
		BookingDate:     time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
		BankRef:         "REF-" + qrRef,
		QRReference:     qrRef,
		CounterpartName: "Boulangerie Dupont",
	}
}

// Réimporter le relevé du mois est une opération courante. Elle ne doit rien
// dupliquer — un encaissement compté deux fois fausse le rapprochement de tout
// le mois.
func TestReimporterUnReleveNeDupliquePas(t *testing.T) {
	s, _ := newDB(t)
	entries := []iso20022.BankEntry{entry(100, "210000000003139471430009017"), entry(250, "")}

	first, err := s.Import(context.Background(), entries)
	if err != nil {
		t.Fatal(err)
	}
	if first.Imported != 2 || first.Duplicate != 0 {
		t.Fatalf("premier import: %+v", first)
	}

	second, err := s.Import(context.Background(), entries)
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 || second.Duplicate != 2 {
		t.Fatalf("second import: %+v — le relevé a été dupliqué", second)
	}
}

// Deux versements identiques le même jour existent réellement. Les fondre en un
// seul ferait disparaître un encaissement.
func TestDeuxVersementsIdentiquesRestentDeuxEcritures(t *testing.T) {
	s, _ := newDB(t)
	a := entry(100, "")
	a.BankRef = "REF-A"
	b := entry(100, "")
	b.BankRef = "REF-B"

	res, err := s.Import(context.Background(), []iso20022.BankEntry{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 {
		t.Fatalf("%d écriture(s) conservée(s), attendu 2 — un encaissement a disparu", res.Imported)
	}
}

// La référence du bulletin est recopiée par la banque : c'est une
// correspondance, pas une ressemblance.
func TestLaReferenceDuBulletinDesigneLaFacture(t *testing.T) {
	s, database := newDB(t)
	const ref = "210000000003139471430009017"
	addInvoice(t, database, "i1", "FA-2026-0001", 1081, "sent", ref)

	if _, err := s.Import(context.Background(), []iso20022.BankEntry{entry(1081, ref)}); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Suggestion == nil {
		t.Fatalf("aucune suggestion: %+v", list)
	}
	if list[0].Suggestion.InvoiceNumber != "FA-2026-0001" {
		t.Fatalf("facture proposée %q", list[0].Suggestion.InvoiceNumber)
	}
	if list[0].Suggestion.Confidence != "certaine" {
		t.Fatalf("confiance %q, attendu certaine", list[0].Suggestion.Confidence)
	}
}

// Plusieurs factures au même montant : ne rien proposer. Désigner la première
// serait un tirage au sort présenté comme une analyse.
func TestPlusieursFacturesAuMemeMontantAucuneSuggestion(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 500, "sent", "")
	addInvoice(t, database, "i2", "FA-2026-0002", 500, "sent", "")

	if _, err := s.Import(context.Background(), []iso20022.BankEntry{entry(500, "")}); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("%d écriture(s)", len(list))
	}
	if list[0].Suggestion != nil {
		t.Fatalf("une facture est proposée (%s) alors que deux correspondent",
			list[0].Suggestion.InvoiceNumber)
	}
}

func TestMontantExactUneSeuleFactureOuverte(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 750.50, "sent", "")
	addInvoice(t, database, "i2", "FA-2026-0002", 999, "sent", "")

	if _, err := s.Import(context.Background(), []iso20022.BankEntry{entry(750.50, "")}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(context.Background(), false)
	if list[0].Suggestion == nil || list[0].Suggestion.InvoiceNumber != "FA-2026-0001" {
		t.Fatalf("suggestion: %+v", list[0].Suggestion)
	}
	if list[0].Suggestion.Confidence != "probable" {
		t.Fatalf("confiance %q, attendu probable — le montant seul n'est pas une certitude",
			list[0].Suggestion.Confidence)
	}
}

// Une facture déjà payée ne doit pas être proposée : le versement qu'on regarde
// en est forcément un autre.
func TestUneFactureDejaPayeeNEstPasProposee(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 300, "paid", "")

	if _, err := s.Import(context.Background(), []iso20022.BankEntry{entry(300, "")}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(context.Background(), false)
	if list[0].Suggestion != nil {
		t.Fatal("une facture déjà payée a été proposée")
	}
}

// Une sortie d'argent n'encaisse rien : proposer une facture dessus n'a pas de
// sens, et encombrerait la liste des vraies décisions à prendre.
func TestUneSortieDArgentNAPasDeSuggestion(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 300, "sent", "")
	e := entry(300, "")
	e.IsCredit = false

	if _, err := s.Import(context.Background(), []iso20022.BankEntry{e}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(context.Background(), false)
	if list[0].Suggestion != nil {
		t.Fatal("une facture est proposée sur un débit")
	}
}

// Rapprocher n'encaisse pas. C'est la règle qui empêche de faire passer pour
// réglée une créance que personne n'a vérifiée.
func TestRapprocherNeCreeAucunPaiement(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 400, "sent", "")
	if _, err := s.Import(context.Background(), []iso20022.BankEntry{entry(400, "")}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(context.Background(), false)

	if err := s.Match(context.Background(), list[0].ID, "i1", "u1"); err != nil {
		t.Fatal(err)
	}

	var payments, entries int
	database.QueryRow(`SELECT COUNT(*) FROM payments`).Scan(&payments)
	database.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&entries)
	if payments != 0 {
		t.Fatalf("%d paiement(s) créé(s) par un rapprochement", payments)
	}
	if entries != 0 {
		t.Fatalf("%d écriture(s) au journal créée(s) par un rapprochement", entries)
	}
	var status string
	database.QueryRow(`SELECT status FROM invoices WHERE id='i1'`).Scan(&status)
	if status != "sent" {
		t.Fatalf("la facture est passée à %q : elle a été soldée sans encaissement vérifié", status)
	}
}

// Une écriture rapprochée sort de la liste des décisions à prendre, sinon la
// liste ne se vide jamais et on cesse de la lire.
func TestUneEcritureRapprocheeSortDeLaListe(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 400, "sent", "")
	s.Import(context.Background(), []iso20022.BankEntry{entry(400, "")})
	list, _ := s.List(context.Background(), false)

	if err := s.Match(context.Background(), list[0].ID, "i1", "u1"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.List(context.Background(), false)
	if len(after) != 0 {
		t.Fatalf("%d écriture(s) encore à traiter", len(after))
	}
	all, _ := s.List(context.Background(), true)
	if len(all) != 1 || all[0].InvoiceNumber != "FA-2026-0001" {
		t.Fatalf("l'écriture rapprochée est introuvable dans la vue complète: %+v", all)
	}
}

// Se tromper de facture arrive. Une décision qu'on ne peut pas reprendre pousse
// à ne pas la prendre.
func TestUnRapprochementSeDefait(t *testing.T) {
	s, database := newDB(t)
	addInvoice(t, database, "i1", "FA-2026-0001", 400, "sent", "")
	s.Import(context.Background(), []iso20022.BankEntry{entry(400, "")})
	list, _ := s.List(context.Background(), false)
	s.Match(context.Background(), list[0].ID, "i1", "u1")

	if err := s.Unmatch(context.Background(), list[0].ID); err != nil {
		t.Fatal(err)
	}
	back, _ := s.List(context.Background(), false)
	if len(back) != 1 {
		t.Fatal("l'écriture n'est pas revenue dans la liste à traiter")
	}
	_ = database
}

// Frais bancaires, virement interne : écarter doit être distinct de « pas
// encore regardé ».
func TestUneEcritureEcarteeSortDeLaListe(t *testing.T) {
	s, _ := newDB(t)
	s.Import(context.Background(), []iso20022.BankEntry{entry(12.50, "")})
	list, _ := s.List(context.Background(), false)

	if err := s.Ignore(context.Background(), list[0].ID, true); err != nil {
		t.Fatal(err)
	}
	after, _ := s.List(context.Background(), false)
	if len(after) != 0 {
		t.Fatalf("%d écriture(s) restante(s) après mise à l'écart", len(after))
	}
}

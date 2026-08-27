package vat_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/vat"
)

// newTestDB gives each test an isolated, fully migrated SQLite database.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-vat-*.db")
	if err != nil {
		t.Fatalf("temp db: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name()})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func seedSupplier(t *testing.T, database *sql.DB) string {
	t.Helper()
	id := db.NewID()
	_, err := database.Exec(
		`INSERT INTO contacts (id, name, contact_type) VALUES (?, ?, 'supplier')`,
		id, "Fournisseur SA")
	if err != nil {
		t.Fatalf("seed supplier: %v", err)
	}
	return id
}

// seedUser creates the user that owns seeded records (invoices.created_by_id
// is NOT NULL and references users).
func seedUser(t *testing.T, database *sql.DB) string {
	t.Helper()
	id := db.NewID()
	if _, err := database.Exec(
		`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, ?)`,
		id, "vat-test-"+id[:8]+"@example.ch", "VAT Test", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedSalesInvoice records revenue with collected VAT.
func seedSalesInvoice(t *testing.T, database *sql.DB, status string, ht, vatAmt, rate float64, issued string) {
	t.Helper()
	userID := seedUser(t, database)
	contactID := db.NewID()
	if _, err := database.Exec(
		`INSERT INTO contacts (id, name, contact_type) VALUES (?, ?, 'customer')`,
		contactID, "Client SA"); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	_, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date, currency,
		                      subtotal_amount, vat_amount, total_amount, vat_rate, created_by_id)
		VALUES (?, ?, ?, ?, ?, ?, 'CHF', ?, ?, ?, ?, ?)`,
		db.NewID(), "FA-"+db.NewID()[:8], contactID, status, issued, issued, ht, vatAmt, ht+vatAmt, rate, userID)
	if err != nil {
		t.Fatalf("seed sales invoice: %v", err)
	}
}

// seedSupplierInvoice records an expense carrying deductible input VAT.
func seedSupplierInvoice(t *testing.T, database *sql.DB, supplierID, status string, ht, vatAmt, rate float64, issued string) {
	t.Helper()
	_, err := database.Exec(`
		INSERT INTO supplier_invoices (id, supplier_id, supplier_reference, status, issue_date,
		                               currency, subtotal_amount, vat_amount, total_amount, vat_rate)
		VALUES (?, ?, ?, ?, ?, 'CHF', ?, ?, ?, ?)`,
		db.NewID(), supplierID, "SUP-"+db.NewID()[:8], status, issued, ht, vatAmt, ht+vatAmt, rate)
	if err != nil {
		t.Fatalf("seed supplier invoice: %v", err)
	}
}

func period() (time.Time, time.Time) {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
}

// The regression this feature exists for: before supplier invoices existed the
// declaration always reported zero deductible input tax, so the business
// over-declared what it owed the AFC.
func TestDeductibleInputVATReducesAmountPayable(t *testing.T) {
	database := newTestDB(t)
	supplier := seedSupplier(t, database)

	// Sales: 10'000 HT, 810.00 collected VAT at 8.1%.
	seedSalesInvoice(t, database, "sent", 10000, 810, 8.1, "2026-03-15")
	// Purchases: 2'000 HT, 162.00 input VAT at 8.1%.
	seedSupplierInvoice(t, database, supplier, "booked", 2000, 162, 0.081, "2026-03-20")

	start, end := period()
	decl, err := vat.New(database, false).GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}

	if decl.VATCollected.Total != 810 {
		t.Errorf("collected VAT = %.2f, want 810.00", decl.VATCollected.Total)
	}
	if decl.VATDeductible != 162 {
		t.Errorf("deductible input VAT = %.2f, want 162.00", decl.VATDeductible)
	}
	if want := 648.0; decl.VATPayable != want {
		t.Errorf("VAT payable = %.2f, want %.2f (810 collected − 162 deductible)", decl.VATPayable, want)
	}
}

func TestDraftAndCancelledSupplierInvoicesAreExcluded(t *testing.T) {
	database := newTestDB(t)
	supplier := seedSupplier(t, database)

	seedSupplierInvoice(t, database, supplier, "booked", 1000, 81, 0.081, "2026-04-01")
	seedSupplierInvoice(t, database, supplier, "draft", 5000, 405, 0.081, "2026-04-02")
	seedSupplierInvoice(t, database, supplier, "cancelled", 9000, 729, 0.081, "2026-04-03")

	start, end := period()
	decl, err := vat.New(database, false).GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}
	// Only the booked invoice may be claimed.
	if decl.VATDeductible != 81 {
		t.Errorf("deductible = %.2f, want 81.00 (drafts and cancelled excluded)", decl.VATDeductible)
	}
}

func TestPaidSupplierInvoicesAreDeductible(t *testing.T) {
	database := newTestDB(t)
	supplier := seedSupplier(t, database)
	seedSupplierInvoice(t, database, supplier, "paid", 1000, 81, 0.081, "2026-05-01")

	start, end := period()
	decl, err := vat.New(database, false).GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}
	if decl.VATDeductible != 81 {
		t.Errorf("deductible = %.2f, want 81.00", decl.VATDeductible)
	}
}

func TestSupplierInvoicesOutsidePeriodAreExcluded(t *testing.T) {
	database := newTestDB(t)
	supplier := seedSupplier(t, database)

	seedSupplierInvoice(t, database, supplier, "booked", 1000, 81, 0.081, "2026-06-01")  // inside
	seedSupplierInvoice(t, database, supplier, "booked", 4000, 324, 0.081, "2025-06-01") // before
	seedSupplierInvoice(t, database, supplier, "booked", 4000, 324, 0.081, "2027-06-01") // after

	start, end := period()
	decl, err := vat.New(database, false).GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}
	if decl.VATDeductible != 81 {
		t.Errorf("deductible = %.2f, want 81.00 (only the in-period invoice counts)", decl.VATDeductible)
	}
}

// Under TDFN the net rate already embeds an input-tax allowance, so claiming
// input VAT again would be double-counting (art. 37 LTVA).
func TestTDFNDoesNotClaimInputVAT(t *testing.T) {
	database := newTestDB(t)
	supplier := seedSupplier(t, database)

	seedSalesInvoice(t, database, "sent", 10000, 810, 8.1, "2026-03-15")
	seedSupplierInvoice(t, database, supplier, "booked", 2000, 162, 0.081, "2026-03-20")

	start, end := period()
	decl, err := vat.New(database, false).GenerateDeclaration(context.Background(), start, end, "tdfn")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}
	if decl.VATDeductible != 0 {
		t.Errorf("TDFN deductible = %.2f, want 0 (allowance already in the net rate)", decl.VATDeductible)
	}
}

func TestNoSupplierInvoicesYieldsZeroDeductible(t *testing.T) {
	database := newTestDB(t)
	seedSalesInvoice(t, database, "sent", 1000, 81, 8.1, "2026-02-01")

	start, end := period()
	decl, err := vat.New(database, false).GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}
	if decl.VATDeductible != 0 {
		t.Errorf("deductible = %.2f, want 0", decl.VATDeductible)
	}
	if decl.VATPayable != 81 {
		t.Errorf("payable = %.2f, want 81.00", decl.VATPayable)
	}
}

// seedQuote records a price offer — a document that commits nobody and creates
// no VAT liability until it is turned into an invoice.
func seedQuote(t *testing.T, database *sql.DB, status string, ht, vatAmt, rate float64, issued string) {
	t.Helper()
	userID := seedUser(t, database)
	contactID := db.NewID()
	if _, err := database.Exec(
		`INSERT INTO contacts (id, name, contact_type) VALUES (?, ?, 'customer')`,
		contactID, "Prospect SA"); err != nil {
		t.Fatalf("seed prospect: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate, created_by_id)
		VALUES (?, ?, 'quote', ?, ?, ?, ?, 'CHF', ?, ?, ?, ?, ?)`,
		db.NewID(), "OF-"+db.NewID()[:8], contactID, status, issued, issued,
		ht, vatAmt, ht+vatAmt, rate, userID); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
}

// A quote sent to a prospect must never reach the VAT declaration. Under
// LTVA art. 40 al. 1 the tax claim arises when the invoice is issued; a price
// offer is not an invoice, and the prospect may never accept it. Counting it
// would make the business declare — and pay — VAT on revenue it never earned.
func TestQuoteIsExcludedFromVATDeclaration(t *testing.T) {
	database := newTestDB(t)
	start, end := period()

	seedQuote(t, database, "sent", 10000, 810, 8.1, "2026-03-01")

	decl, err := vat.New(database, false).
		GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("generate declaration: %v", err)
	}

	if decl.TotalRevenue != 0 {
		t.Errorf("chiffre 200 = %.2f, want 0 — une offre de prix n'est pas un chiffre d'affaires imposable", decl.TotalRevenue)
	}
	if decl.VATCollected.Standard != 0 {
		t.Errorf("TVA collectée = %.2f, want 0 — aucune créance fiscale ne naît d'une offre (LTVA art. 40)", decl.VATCollected.Standard)
	}
}

// A credit note reduces the tax debt (LTVA art. 41). Amounts are stored
// unsigned so the document reads naturally, so the sign is applied when
// aggregating — summing it as-is used to add VAT where it should subtract.
func TestCreditNoteReducesVATOwed(t *testing.T) {
	database := newTestDB(t)
	start, end := period()

	seedSalesInvoice(t, database, "sent", 10000, 810, 8.1, "2026-03-01")
	seedCreditNote(t, database, 2000, 162, 8.1, "2026-03-15")

	decl, err := vat.New(database, false).
		GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("generate declaration: %v", err)
	}

	if want := 648.0; decl.VATCollected.Standard != want {
		t.Errorf("TVA collectée = %.2f, want %.2f (810 facturés − 162 crédités)",
			decl.VATCollected.Standard, want)
	}
	if want := 8000.0; decl.TotalRevenue != want {
		t.Errorf("chiffre 200 = %.2f, want %.2f", decl.TotalRevenue, want)
	}
}

// A credit note larger than the invoices in the period is unusual but legal —
// a big cancellation early in a quarter. It must not silently clamp to zero.
func TestCreditNoteCanExceedInvoicesInPeriod(t *testing.T) {
	database := newTestDB(t)
	start, end := period()

	seedSalesInvoice(t, database, "sent", 1000, 81, 8.1, "2026-03-01")
	seedCreditNote(t, database, 3000, 243, 8.1, "2026-03-15")

	decl, err := vat.New(database, false).
		GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("generate declaration: %v", err)
	}
	if decl.VATCollected.Standard >= 0 {
		t.Errorf("TVA collectée = %.2f — un crédit net doit rester négatif, pas être écrasé à zéro",
			decl.VATCollected.Standard)
	}
}

func seedCreditNote(t *testing.T, database *sql.DB, ht, vatAmt, rate float64, issued string) {
	t.Helper()
	userID := seedUser(t, database)
	contactID := db.NewID()
	if _, err := database.Exec(
		`INSERT INTO contacts (id, name, contact_type) VALUES (?, ?, 'customer')`,
		contactID, "Client SA"); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate, created_by_id)
		VALUES (?, ?, 'credit_note', ?, 'sent', ?, ?, 'CHF', ?, ?, ?, ?, ?)`,
		db.NewID(), "NC-"+db.NewID()[:8], contactID, issued, issued,
		ht, vatAmt, ht+vatAmt, rate, userID); err != nil {
		t.Fatalf("seed credit note: %v", err)
	}
}

// ─── Classement par taux (AFC 318, lignes 302 / 312 / 342) ───────────────────
//
// Le formulaire AFC 318 ne demande pas un total : il demande le chiffre
// d'affaires ET l'impôt VENTILÉS par taux, sur trois lignes distinctes. Une
// déclaration dont le total est juste mais dont les trois lignes sont fausses
// reste une déclaration fausse.
//
// Ce test existe parce que rien ne tenait cet invariant : les fixtures de ce
// fichier mélangeaient les deux notations (0.081 et 8.1) et n'assertionnaient
// que des totaux, insensibles au classement. La base, elle, stocke des
// POURCENTAGES depuis la migration 0005 (« new inserts always use
// percentages »), ce que confirme le calcul de invoicing/service.go
// (`base * l.VATRate / 100`).

// seedSalesInvoiceAt est un alias explicite : le taux passé ici est celui que
// la base contient réellement, en pourcentage.
func seedSalesInvoiceAt(t *testing.T, database *sql.DB, ht, vatAmt, ratePct float64, issued string) {
	t.Helper()
	seedSalesInvoice(t, database, "sent", ht, vatAmt, ratePct, issued)
}

func TestDeclarationVentileParTaux(t *testing.T) {
	database := newTestDB(t)
	svc := vat.New(database, false)

	// Un hôtel avec restauration : les trois taux suisses coexistent sur le
	// même exercice. C'est exactement le cas que la méthode effective doit
	// savoir ventiler.
	seedSalesInvoiceAt(t, database, 10000, 810, 8.1, "2026-03-15") // restauration
	seedSalesInvoiceAt(t, database, 5000, 190, 3.8, "2026-04-10")  // hébergement
	seedSalesInvoiceAt(t, database, 2000, 52, 2.6, "2026-05-20")   // denrées

	decl, err := svc.GenerateDeclaration(context.Background(),
		mustDate("2026-01-01"), mustDate("2026-12-31"), "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}

	if got, want := decl.VATCollected.Standard, 810.0; !nearly(got, want) {
		t.Errorf("ligne 302 (taux normal) = %.2f, attendu %.2f", got, want)
	}
	if got, want := decl.VATCollected.Special, 190.0; !nearly(got, want) {
		t.Errorf("ligne 342 (taux spécial) = %.2f, attendu %.2f — "+
			"un taux réel de la base ne doit pas retomber dans le seau « normal »", got, want)
	}
	if got, want := decl.VATCollected.Reduced, 52.0; !nearly(got, want) {
		t.Errorf("ligne 312 (taux réduit) = %.2f, attendu %.2f — "+
			"un taux réel de la base ne doit pas retomber dans le seau « normal »", got, want)
	}
	// Le total reste juste dans TOUS les cas, y compris quand le classement est
	// faux : c'est précisément ce qui a permis au défaut de passer inaperçu.
	if got, want := decl.VATCollected.Total, 1052.0; !nearly(got, want) {
		t.Errorf("total collecté = %.2f, attendu %.2f", got, want)
	}
}

// Un taux inconnu (une facture ancienne, un taux étranger) doit continuer de
// retomber dans le seau « normal » plutôt que d'être perdu : mieux vaut une
// ligne discutable qu'un montant qui disparaît de la déclaration.
func TestUnTauxInconnuRetombeSurLeTauxNormal(t *testing.T) {
	database := newTestDB(t)
	svc := vat.New(database, false)

	seedSalesInvoiceAt(t, database, 1000, 77, 7.7, "2026-03-15") // ancien taux 2023

	decl, err := svc.GenerateDeclaration(context.Background(),
		mustDate("2026-01-01"), mustDate("2026-12-31"), "effective")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}
	if got, want := decl.VATCollected.Standard, 77.0; !nearly(got, want) {
		t.Errorf("un taux inconnu doit rejoindre le seau normal : %.2f, attendu %.2f", got, want)
	}
	if got, want := decl.VATCollected.Total, 77.0; !nearly(got, want) {
		t.Errorf("total = %.2f, attendu %.2f — aucun montant ne doit être perdu", got, want)
	}
}

func mustDate(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func nearly(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.005
}

// Le montant TDFN lui-même doit rester pinné.
//
// La méthode TDFN multiplie un chiffre d'affaires par
// `compliance.VATRateStandard * 0.8`, donc elle a besoin des constantes sous
// leur forme FRACTIONNAIRE (0.081). Le classement par taux, lui, compare à des
// valeurs lues en POURCENTAGE. Les deux usages coexistent dans le même fichier.
//
// Sans ce test, « corriger » le classement en passant les constantes en
// pourcentage — le raccourci tentant — ferait calculer 648'000 au lieu de 648
// sans qu'aucun test ne bronche : l'ancien test TDFN ne vérifiait que le
// déductible à zéro.
func TestLeMontantTDFNResteCalculeSurLaFraction(t *testing.T) {
	database := newTestDB(t)
	svc := vat.New(database, false)

	seedSalesInvoiceAt(t, database, 10000, 810, 8.1, "2026-03-15")

	decl, err := svc.GenerateDeclaration(context.Background(),
		mustDate("2026-01-01"), mustDate("2026-12-31"), "tdfn")
	if err != nil {
		t.Fatalf("GenerateDeclaration: %v", err)
	}

	// 10'000 HT × (8.1 % × 0.8) = 10'000 × 0.0648 = 648.00
	if got, want := decl.VATCollected.Total, 648.0; !nearly(got, want) {
		t.Errorf("TVA due en TDFN = %.2f, attendu %.2f — "+
			"les constantes compliance.VATRate* doivent rester en fraction", got, want)
	}
}

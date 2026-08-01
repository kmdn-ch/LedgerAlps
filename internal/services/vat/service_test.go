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
	seedSalesInvoice(t, database, "sent", 10000, 810, 0.081, "2026-03-15")
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

	seedSalesInvoice(t, database, "sent", 10000, 810, 0.081, "2026-03-15")
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
	seedSalesInvoice(t, database, "sent", 1000, 81, 0.081, "2026-02-01")

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

// A credit note must not inflate the declaration either. Amounts are stored
// unsigned, so including it added VAT where it should reduce it (LTVA art. 41).
// Excluding it is the conservative reading until signed amounts exist; the
// wrong direction here would understate what is owed to the AFC.
func TestCreditNoteDoesNotIncreaseVATOwed(t *testing.T) {
	database := newTestDB(t)
	start, end := period()

	seedSalesInvoice(t, database, "sent", 10000, 810, 8.1, "2026-03-01")

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
		db.NewID(), "NC-"+db.NewID()[:8], contactID, "2026-03-15", "2026-03-15",
		2000.0, 162.0, 2162.0, 8.1, userID); err != nil {
		t.Fatalf("seed credit note: %v", err)
	}

	decl, err := vat.New(database, false).
		GenerateDeclaration(context.Background(), start, end, "effective")
	if err != nil {
		t.Fatalf("generate declaration: %v", err)
	}

	if decl.VATCollected.Standard > 810.0 {
		t.Errorf("TVA collectée = %.2f — une note de crédit ne doit jamais augmenter la TVA due", decl.VATCollected.Standard)
	}
}

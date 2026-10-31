package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

func newMaintenanceDB(t *testing.T) (*MaintenanceHandler, func(q string, args ...any)) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-maint-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	cfg := &config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := database.Exec(q, args...); err != nil {
			t.Fatalf("seed (%s): %v", q, err)
		}
	}
	return NewMaintenanceHandler(database, cfg), exec
}

func runIntegrity(t *testing.T, h *MaintenanceHandler) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/integrity", h.IntegrityCheck)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/integrity", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return body
}

func checksIn(body map[string]any) map[string]bool {
	found := map[string]bool{}
	items, _ := body["findings"].([]any)
	for _, it := range items {
		if f, ok := it.(map[string]any); ok {
			found[f["check"].(string)] = true
		}
	}
	return found
}

// A freshly migrated database has a seeded chart of accounts and nothing else.
// Reporting problems on it would make the page noise from the first use.
func TestCleanDatabaseReportsNothing(t *testing.T) {
	h, _ := newMaintenanceDB(t)
	body := runIntegrity(t, h)

	if clean, _ := body["clean"].(bool); !clean {
		t.Errorf("une base neuve est signalée comme problématique: %v", body["findings"])
	}
}

// The invariant that matters most: an entry whose debits and credits differ
// makes the balance sheet wrong.
func TestUnbalancedEntryIsReported(t *testing.T) {
	h, exec := newMaintenanceDB(t)

	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`,
		userID, "m@example.ch", "M")
	entryID := db.NewID()
	exec(`INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
	      VALUES (?, 'JE-1', '2026-01-01', 'déséquilibrée', 'draft', ?)`, entryID, userID)

	var accountID string
	if err := h.db.QueryRow(`SELECT id FROM accounts LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatalf("le plan comptable devrait être préchargé: %v", err)
	}
	// 100 au débit, 60 au crédit.
	exec(`INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, sequence)
	      VALUES (?, ?, ?, 100, 1)`, db.NewID(), entryID, accountID)
	exec(`INSERT INTO journal_lines (id, entry_id, account_id, credit_amount, sequence)
	      VALUES (?, ?, ?, 60, 2)`, db.NewID(), entryID, accountID)

	if !checksIn(runIntegrity(t, h))["journal_balance"] {
		t.Error("une écriture déséquilibrée n'a pas été signalée")
	}
}

// Rounding to 5 rappen is normal operation, not a defect. Flagging it would
// bury the real findings under noise on every invoice.
func TestRoundingDifferenceIsNotReported(t *testing.T) {
	h, exec := newMaintenanceDB(t)

	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`,
		userID, "r@example.ch", "R")
	contactID := db.NewID()
	exec(`INSERT INTO contacts (id, name, contact_type) VALUES (?, 'Client', 'customer')`, contactID)

	invID := db.NewID()
	// Sous-total 100.02 pour une ligne à 100.00 : 2 centimes d'écart.
	exec(`INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date,
	                            subtotal_amount, total_amount, created_by_id)
	      VALUES (?, 'FA-2026-9001', ?, 'sent', '2026-01-01', '2026-01-31', 100.02, 108.12, ?)`,
		invID, contactID, userID)
	exec(`INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price, line_total, sequence)
	      VALUES (?, ?, 'Prestation', 1, 100, 100, 1)`, db.NewID(), invID)

	if checksIn(runIntegrity(t, h))["invoice_totals"] {
		t.Error("un écart d'arrondi de 2 centimes a été signalé comme une erreur")
	}
}

// Beyond the rounding tolerance, the total really is wrong and must show.
func TestRealTotalMismatchIsReported(t *testing.T) {
	h, exec := newMaintenanceDB(t)

	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`,
		userID, "t@example.ch", "T")
	contactID := db.NewID()
	exec(`INSERT INTO contacts (id, name, contact_type) VALUES (?, 'Client', 'customer')`, contactID)

	invID := db.NewID()
	exec(`INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date,
	                            subtotal_amount, total_amount, created_by_id)
	      VALUES (?, 'FA-2026-9002', ?, 'sent', '2026-01-01', '2026-01-31', 500, 540, ?)`,
		invID, contactID, userID)
	exec(`INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price, line_total, sequence)
	      VALUES (?, ?, 'Prestation', 1, 100, 100, 1)`, db.NewID(), invID)

	if !checksIn(runIntegrity(t, h))["invoice_totals"] {
		t.Error("un sous-total de 500 pour 100 de lignes n'a pas été signalé")
	}
}

// Over-crediting became impossible in v1.4.4, but documents created before then
// still exist and understate the VAT owed.
func TestOverCreditedInvoiceIsReported(t *testing.T) {
	h, exec := newMaintenanceDB(t)

	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`,
		userID, "c@example.ch", "C")
	contactID := db.NewID()
	exec(`INSERT INTO contacts (id, name, contact_type) VALUES (?, 'Client', 'customer')`, contactID)

	invID := db.NewID()
	exec(`INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date,
	                            total_amount, created_by_id)
	      VALUES (?, 'FA-2026-9003', ?, 'sent', '2026-01-01', '2026-01-31', 1000, ?)`,
		invID, contactID, userID)
	exec(`INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date,
	                            due_date, total_amount, created_by_id, corrects_invoice_id)
	      VALUES (?, 'NC-2026-9001', 'credit_note', ?, 'sent', '2026-02-01', '2026-02-01', 1500, ?, ?)`,
		db.NewID(), contactID, userID, invID)

	if !checksIn(runIntegrity(t, h))["over_credited"] {
		t.Error("une facture créditée au-delà de son montant n'a pas été signalée")
	}
}

// Every finding must tell the user what to do; one they cannot act on is noise
// that teaches them to ignore the page.
func TestEveryFindingCarriesAnAction(t *testing.T) {
	h, exec := newMaintenanceDB(t)

	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`,
		userID, "a@example.ch", "A")
	entryID := db.NewID()
	exec(`INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
	      VALUES (?, 'JE-2', '2026-01-01', 'vide', 'draft', ?)`, entryID, userID)

	body := runIntegrity(t, h)
	items, _ := body["findings"].([]any)
	if len(items) == 0 {
		t.Fatal("aucun constat produit alors qu'une écriture est vide")
	}
	for _, it := range items {
		f := it.(map[string]any)
		if s, _ := f["action"].(string); s == "" {
			t.Errorf("le constat %q ne dit pas quoi faire", f["check"])
		}
		if s, _ := f["severity"].(string); s != "error" && s != "warning" && s != "info" {
			t.Errorf("le constat %q a une gravité inattendue: %q", f["check"], s)
		}
	}
}

// The health view is read-only and must never fail on a fresh install, where
// there are no backups and nothing has happened yet.
func TestHealthWorksOnAFreshInstall(t *testing.T) {
	h, _ := newMaintenanceDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", h.SystemHealth)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"version", "database", "counts", "network", "capabilities"} {
		if _, ok := body[key]; !ok {
			t.Errorf("la réponse ne contient pas %q", key)
		}
	}
	// Loopback must read as loopback, or the page would tell the user they are
	// exposed when they are not — or worse, the reverse.
	net, _ := body["network"].(map[string]any)
	if lb, _ := net["loopback"].(bool); !lb {
		t.Error("127.0.0.1 n'est pas reconnu comme loopback")
	}
}

// Audit 9, M-07 : le contrôle d'intégrité restait muet sur une facture envoyée
// sans écriture (A-01). Il la signale quand la comptabilisation automatique est
// active, et seulement alors : une installation qui tient ses écritures à la
// main n'a rien à se faire reprocher.
func TestUnSentInvoiceWithoutEntryIsReportedOnlyWhenAutoPostIsOn(t *testing.T) {
	h, exec := newMaintenanceDB(t)

	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`,
		userID, "u@example.ch", "U")
	contactID := db.NewID()
	exec(`INSERT INTO contacts (id, name, contact_type) VALUES (?, 'Client', 'customer')`, contactID)
	exec(`INSERT INTO company_settings (id, company_name, auto_post_invoices) VALUES (?, 'Test SA', 0)`, db.NewID())

	invID := db.NewID()
	exec(`INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date,
	                            subtotal_amount, total_amount, created_by_id)
	      VALUES (?, 'FA-2026-9100', ?, 'sent', '2026-01-01', '2026-01-31', 12.34, 12.35, ?)`,
		invID, contactID, userID)
	exec(`INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price, line_total, sequence)
	      VALUES (?, ?, 'Prestation', 1, 12.34, 12.34, 1)`, db.NewID(), invID)

	if checksIn(runIntegrity(t, h))["invoice_unposted"] {
		t.Error("signalé alors que la comptabilisation automatique est éteinte")
	}

	exec(`UPDATE company_settings SET auto_post_invoices = 1`)
	body := runIntegrity(t, h)
	if !checksIn(body)["invoice_unposted"] {
		t.Fatal("une facture envoyée sans écriture n'a pas été signalée")
	}
	items, _ := body["findings"].([]any)
	for _, it := range items {
		f := it.(map[string]any)
		if f["check"] == "invoice_unposted" {
			docs, _ := f["documents"].([]any)
			if len(docs) != 1 || docs[0].(map[string]any)["number"] != "FA-2026-9100" {
				t.Errorf("le constat doit nommer la facture : %v", f["documents"])
			}
		}
	}

	// Liée à une écriture : plus rien à signaler.
	entryID := db.NewID()
	exec(`INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
	      VALUES (?, 'JE-9', '2026-01-01', 'émission', 'draft', ?)`, entryID, userID)
	exec(`UPDATE invoices SET journal_entry_id = ? WHERE id = ?`, entryID, invID)
	if checksIn(runIntegrity(t, h))["invoice_unposted"] {
		t.Error("signalé alors que la facture est liée à son écriture")
	}
}

// Audit 9, M-07 : après les essais de l'audit, le contrôle d'intégrité ne rendait
// que l'avertissement IBAN. Il doit montrer ce que le service ne refuse plus
// d'accepter mais qui est déjà entré : montants négatifs ou à trois décimales,
// dates improbables, exercice illisible, document à total négatif.
func TestLeControleSignaleLesDonneesAbsurdesDejaEnBase(t *testing.T) {
	h, exec := newMaintenanceDB(t)
	userID := db.NewID()
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, 'x')`, userID, "a@example.ch", "A")
	contactID := db.NewID()
	exec(`INSERT INTO contacts (id, name, contact_type) VALUES (?, 'Client', 'customer')`, contactID)
	var accountID string
	if err := h.db.QueryRow(`SELECT id FROM accounts LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}

	// Écriture équilibrée, mais à trois décimales et datée de l'an 26.
	entryID := db.NewID()
	exec(`INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
	      VALUES (?, 'JN-0026-001', '0026-10-30', 'faute de frappe', 'posted', ?)`, entryID, userID)
	exec(`INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, sequence) VALUES (?, ?, ?, 100.123, 1)`,
		db.NewID(), entryID, accountID)
	exec(`INSERT INTO journal_lines (id, entry_id, account_id, credit_amount, sequence) VALUES (?, ?, ?, 100.123, 2)`,
		db.NewID(), entryID, accountID)

	// Facture à total négatif.
	invID := db.NewID()
	exec(`INSERT INTO invoices (id, invoice_number, contact_id, status, issue_date, due_date,
	                            subtotal_amount, total_amount, created_by_id)
	      VALUES (?, 'FA-2026-9200', ?, 'sent', '2026-01-01', '2026-01-31', -250, -250, ?)`, invID, contactID, userID)
	exec(`INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price, line_total, sequence)
	      VALUES (?, ?, 'x', 1, -250, -250, 1)`, db.NewID(), invID)

	// Exercice aux bornes illisibles.
	exec(`INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed)
	      VALUES ('fy-26', '26', '26-01-01', '26-12-31', 0)`)

	found := checksIn(runIntegrity(t, h))
	for _, check := range []string{"journal_amounts", "journal_dates", "fiscal_year_dates", "invoice_amounts"} {
		if !found[check] {
			t.Errorf("le contrôle %q n'a rien signalé", check)
		}
	}
}

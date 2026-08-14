package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Anonymiser un contact (nLPD art. 6 al. 4 et 32) sans amputer une pièce
// comptable (CO art. 958f, LTVA art. 26 qui exige que la facture nomme son
// destinataire). Ce n'est possible que depuis que la facture porte l'identité
// figée à l'émission — avant, le PDF relisait le contact vivant.

func newContactsDB(t *testing.T) (*ContactsHandler, *sql.DB) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-contacts-*.db")
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
		`INSERT INTO users (id, email, name, password_hash, is_admin) VALUES ('u1','a@t.ch','Admin','x',1)`,
	); err != nil {
		t.Fatal(err)
	}
	return NewContactsHandler(database, false), database
}

func seedContact(t *testing.T, database *sql.DB, id, name string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO contacts (id, contact_type, name, email, phone, address, city, postal_code,
		                      country, iban, vat_number, notes, payment_term_days, is_active)
		VALUES (?, 'customer', ?, 'client@example.ch', '+41 22 000 00 00', 'Rue du Test 1',
		        'Genève', '1201', 'CH', 'CH9300762011623852957', 'CHE-123.456.789', 'note privée', 30, 1)`,
		id, name); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
}

func seedInvoiceFor(t *testing.T, database *sql.DB, id, contactID, number string, frozen bool) {
	t.Helper()
	rcpName, backfilled := "", 0
	if frozen {
		rcpName, backfilled = "Client SA", 0
	}
	if _, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate,
		                      recipient_name, recipient_city, recipient_backfilled, created_by_id)
		VALUES (?, ?, 'invoice', ?, 'sent', '2026-03-01', '2026-03-31', 'CHF', 1000, 81, 1081, 8.1, ?, 'Genève', ?, 'u1')`,
		id, number, contactID, rcpName, backfilled); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
}

func runAnonymise(t *testing.T, h *ContactsHandler, id string, admin bool) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/contacts/:id/anonymise", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: admin})
		h.AnonymiseContact(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/contacts/"+id+"/anonymise", nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// ─── Cas nominal ─────────────────────────────────────────────────────────────

func TestAnonymiseErasesPersonalDataAndKeepsTheDocument(t *testing.T) {
	h, database := newContactsDB(t)
	seedContact(t, database, "c1", "Client SA")
	seedInvoiceFor(t, database, "i1", "c1", "FA-2026-001", true)

	code, body := runAnonymise(t, h, "c1", true)
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, body)
	}

	var name string
	var email, address, iban, vat, notes sql.NullString
	var anonymisedAt sql.NullTime
	var active int
	if err := database.QueryRow(`
		SELECT name, email, address, iban, vat_number, notes, anonymised_at, is_active
		FROM contacts WHERE id = 'c1'`).Scan(
		&name, &email, &address, &iban, &vat, &notes, &anonymisedAt, &active); err != nil {
		t.Fatal(err)
	}

	for label, v := range map[string]sql.NullString{
		"courriel": email, "adresse": address, "IBAN": iban,
		"numéro de TVA": vat, "notes": notes,
	} {
		if v.Valid && v.String != "" {
			t.Errorf("%s subsiste après anonymisation : %q", label, v.String)
		}
	}
	if name == "Client SA" {
		t.Error("le nom subsiste après anonymisation")
	}
	if !anonymisedAt.Valid {
		t.Error("aucune trace de l'opération : la nLPD demande de pouvoir démontrer sa conformité")
	}
	if active != 0 {
		t.Error("le contact reste actif : il réapparaîtrait dans les listes de sélection")
	}

	// Et la pièce comptable, elle, garde son destinataire.
	var invoices int
	var rcpName string
	if err := database.QueryRow(
		`SELECT COUNT(*), COALESCE(MAX(recipient_name),'') FROM invoices WHERE contact_id = 'c1'`,
	).Scan(&invoices, &rcpName); err != nil {
		t.Fatal(err)
	}
	if invoices != 1 {
		t.Fatalf("%d facture(s) : la pièce comptable a disparu (CO art. 958f)", invoices)
	}
	if rcpName != "Client SA" {
		t.Fatalf("destinataire de la facture = %q : la LTVA art. 26 exige qu'elle nomme son destinataire", rcpName)
	}
}

// ─── Refus ───────────────────────────────────────────────────────────────────

// Le garde-fou qui compte : anonymiser un contact dont une facture ne porte pas
// son destinataire figé effacerait une mention obligatoire, de façon
// irréversible.
func TestAnonymiseRefusesWhenAnInvoiceHasNoFrozenRecipient(t *testing.T) {
	h, database := newContactsDB(t)
	seedContact(t, database, "c1", "Client SA")
	seedInvoiceFor(t, database, "i1", "c1", "FA-2026-001", false)

	code, body := runAnonymise(t, h, "c1", true)
	if code != http.StatusConflict {
		t.Fatalf("status %d, attendu 409 : %v", code, body)
	}

	var name string
	if err := database.QueryRow(`SELECT name FROM contacts WHERE id = 'c1'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Client SA" {
		t.Fatal("le contact a été anonymisé malgré le refus — l'opération est irréversible")
	}
}

func TestAnonymiseRefusesASecondTime(t *testing.T) {
	h, database := newContactsDB(t)
	seedContact(t, database, "c1", "Client SA")

	if code, body := runAnonymise(t, h, "c1", true); code != http.StatusOK {
		t.Fatalf("première passe: %d %v", code, body)
	}
	if code, _ := runAnonymise(t, h, "c1", true); code != http.StatusConflict {
		t.Fatalf("seconde passe = %d, attendu 409 : réécrire la date ferait croire à un traitement récent", code)
	}
}

// L'effacement suit la permission de GESTION, declaree sur la route.
//
// La garde interne lisait le drapeau du jeton et ecartait le comptable —
// pourtant, repondre a une demande d'effacement (nLPD art. 6 al. 4) fait partie
// de son travail, pas de celui de l'administrateur du logiciel.
func TestLEffacementSuitLaPermissionDeGestion(t *testing.T) {
	if authz.Can(authz.RoleViewer, authz.PermManage) {
		t.Fatal("la lecture seule pourrait effacer des donnees personnelles")
	}
	if !authz.Can(authz.RoleAccountant, authz.PermManage) {
		t.Fatal("le comptable ne peut pas repondre a une demande d'effacement")
	}

	src, err := os.ReadFile(filepath.Join("..", "..", "..", "cmd", "server", "main.go"))
	if err != nil {
		t.Skipf("source des routes illisible: %v", err)
	}
	route := `api.POST("/contacts/:id/anonymise", authorizer.Require(authz.PermManage)`
	if !strings.Contains(string(src), route) {
		t.Errorf("route sans permission declaree : %s", route)
	}
}

func TestAnonymiseUnknownContact(t *testing.T) {
	h, _ := newContactsDB(t)
	if code, _ := runAnonymise(t, h, "inconnu", true); code != http.StatusNotFound {
		t.Fatalf("status %d, attendu 404", code)
	}
}

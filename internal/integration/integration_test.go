// Package integration contains end-to-end tests that spin up a full HTTP server
// backed by an in-memory SQLite database and exercise the API via httptest.
package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	_ "modernc.org/sqlite"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/api/handlers"
	"github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"

	_ "modernc.org/sqlite"
)

// ─── Test server setup ────────────────────────────────────────────────────────

func newTestServer(t *testing.T) (*gin.Engine, *sql.DB, *config.Config) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Use a temporary file per test so each test gets an isolated DB.
	// db.Open wraps SQLitePath in file:...?_journal_mode=WAL so we just supply the path.
	tmpFile, err := os.CreateTemp("", "ledgeralps-test-*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	cfg := &config.Config{
		SQLitePath:       tmpFile.Name(),
		JWTSecret:        "integration-test-secret-32chars-ok",
		JWTAccessMinutes: 60,
		JWTRefreshDays:   30,
		AllowedOrigins:   "http://localhost:5173",
	}

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	v1 := r.Group("/api/v1")

	// Auth
	authH := handlers.NewAuthHandler(database, cfg)
	v1.POST("/auth/bootstrap", authH.Bootstrap)
	v1.POST("/auth/register", authH.Register)
	v1.POST("/auth/login", authH.Login)
	v1.POST("/auth/refresh", authH.Refresh)
	v1.POST("/auth/logout", authH.Logout)

	// Protected
	api := v1.Group("")
	api.Use(middleware.RequireAuth(cfg.JWTSecret))

	accountingSvc := accounting.New(database, false)

	jh := handlers.NewJournalHandler(database, false)
	jwh := handlers.NewJournalWriteHandler(accountingSvc)
	api.GET("/journal", jh.ListJournal)
	api.POST("/journal", jwh.CreateEntry)
	api.POST("/journal/:id/post", jwh.PostEntry)

	ah := handlers.NewAccountsHandler(database, false)
	api.GET("/accounts", ah.ListAccounts)
	api.GET("/accounts/trial-balance", ah.TrialBalance)
	api.GET("/accounts/:code/balance", ah.AccountBalance)
	api.POST("/accounts", ah.CreateAccount)

	ch := handlers.NewContactsHandler(database, false)
	api.GET("/contacts", ch.ListContacts)
	api.POST("/contacts", ch.CreateContact)

	ih := handlers.NewInvoicesHandler(database, false, accountingSvc)
	api.GET("/invoices", ih.ListInvoices)
	api.POST("/invoices", ih.CreateInvoice)
	api.POST("/invoices/:id/transition", ih.TransitionInvoice)

	return r, database, cfg
}

// helpers ─────────────────────────────────────────────────────────────────────

func doJSON(t *testing.T, r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func parseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode response body: %v — body was: %s", err, w.Body.String())
	}
	return m
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d — body: %s", w.Code, want, w.Body.String())
	}
}

// ─── Auth flow ────────────────────────────────────────────────────────────────

func TestBootstrap(t *testing.T) {
	r, _, _ := newTestServer(t)

	// First bootstrap succeeds
	w := doJSON(t, r, "POST", "/api/v1/auth/bootstrap", mustJSON(t, map[string]any{
		"email":    "admin@example.com",
		"name":     "Admin",
		"password": "strongpassword123",
	}), "")
	assertStatus(t, w, http.StatusCreated)

	body := parseBody(t, w)
	if body["is_admin"] != true {
		t.Errorf("bootstrap user should be admin, got is_admin=%v", body["is_admin"])
	}

	// Second bootstrap is rejected
	w2 := doJSON(t, r, "POST", "/api/v1/auth/bootstrap", mustJSON(t, map[string]any{
		"email":    "admin2@example.com",
		"name":     "Admin2",
		"password": "strongpassword123",
	}), "")
	assertStatus(t, w2, http.StatusConflict)
}

func TestRegister(t *testing.T) {
	r, _, _ := newTestServer(t)

	// Bootstrap admin first
	doJSON(t, r, "POST", "/api/v1/auth/bootstrap", mustJSON(t, map[string]any{
		"email": "admin@test.com", "name": "Admin", "password": "adminpass1234",
	}), "")

	// Register a new user
	w := doJSON(t, r, "POST", "/api/v1/auth/register", mustJSON(t, map[string]any{
		"email":    "user@test.com",
		"name":     "Regular User",
		"password": "userpass1234",
	}), "")
	assertStatus(t, w, http.StatusCreated)

	body := parseBody(t, w)
	if body["is_admin"] != false {
		t.Errorf("registered user should not be admin, got is_admin=%v", body["is_admin"])
	}

	// Duplicate email → 409
	w2 := doJSON(t, r, "POST", "/api/v1/auth/register", mustJSON(t, map[string]any{
		"email":    "user@test.com",
		"name":     "Dup",
		"password": "userpass1234",
	}), "")
	assertStatus(t, w2, http.StatusConflict)
}

func TestLogin(t *testing.T) {
	r, _, _ := newTestServer(t)

	// Bootstrap
	doJSON(t, r, "POST", "/api/v1/auth/bootstrap", mustJSON(t, map[string]any{
		"email": "admin@test.com", "name": "Admin", "password": "adminpass1234",
	}), "")

	// Successful login
	w := doJSON(t, r, "POST", "/api/v1/auth/login", mustJSON(t, map[string]any{
		"email":    "admin@test.com",
		"password": "adminpass1234",
	}), "")
	assertStatus(t, w, http.StatusOK)

	body := parseBody(t, w)
	if body["access_token"] == nil || body["access_token"] == "" {
		t.Error("login response missing access_token")
	}
	if body["refresh_token"] == nil || body["refresh_token"] == "" {
		t.Error("login response missing refresh_token — refresh_tokens table not populated")
	}

	// Wrong password → 401
	w2 := doJSON(t, r, "POST", "/api/v1/auth/login", mustJSON(t, map[string]any{
		"email":    "admin@test.com",
		"password": "wrongpassword",
	}), "")
	assertStatus(t, w2, http.StatusUnauthorized)

	// Unknown email → 401 (same response, no enumeration)
	w3 := doJSON(t, r, "POST", "/api/v1/auth/login", mustJSON(t, map[string]any{
		"email":    "nobody@test.com",
		"password": "anything",
	}), "")
	assertStatus(t, w3, http.StatusUnauthorized)
}

func TestRefreshAndLogout(t *testing.T) {
	r, _, _ := newTestServer(t)

	// Bootstrap + login
	doJSON(t, r, "POST", "/api/v1/auth/bootstrap", mustJSON(t, map[string]any{
		"email": "admin@test.com", "name": "Admin", "password": "adminpass1234",
	}), "")
	loginW := doJSON(t, r, "POST", "/api/v1/auth/login", mustJSON(t, map[string]any{
		"email":    "admin@test.com",
		"password": "adminpass1234",
	}), "")
	assertStatus(t, loginW, http.StatusOK)

	loginBody := parseBody(t, loginW)
	refreshToken, _ := loginBody["refresh_token"].(string)
	if refreshToken == "" {
		t.Fatal("no refresh_token in login response")
	}

	// Use refresh token to get a new access token
	refreshW := doJSON(t, r, "POST", "/api/v1/auth/refresh", "", refreshToken)
	assertStatus(t, refreshW, http.StatusOK)

	refreshBody := parseBody(t, refreshW)
	if refreshBody["access_token"] == nil {
		t.Error("refresh response missing access_token")
	}

	// Logout (revoke refresh token)
	logoutW := doJSON(t, r, "POST", "/api/v1/auth/logout", "", refreshToken)
	assertStatus(t, logoutW, http.StatusNoContent)

	// Refresh after logout must fail
	refreshW2 := doJSON(t, r, "POST", "/api/v1/auth/refresh", "", refreshToken)
	assertStatus(t, refreshW2, http.StatusUnauthorized)
}

// ─── Journal flow ─────────────────────────────────────────────────────────────

func loginAdmin(t *testing.T, r *gin.Engine) string {
	t.Helper()
	doJSON(t, r, "POST", "/api/v1/auth/bootstrap", mustJSON(t, map[string]any{
		"email": "admin@test.com", "name": "Admin", "password": "adminpass1234",
	}), "")
	w := doJSON(t, r, "POST", "/api/v1/auth/login", mustJSON(t, map[string]any{
		"email":    "admin@test.com",
		"password": "adminpass1234",
	}), "")
	body := parseBody(t, w)
	token, _ := body["access_token"].(string)
	if token == "" {
		t.Fatal("no access_token")
	}
	return token
}

func TestJournalCreateAndPost(t *testing.T) {
	r, _, _ := newTestServer(t)
	token := loginAdmin(t, r)

	// Fetch seeded accounts to get real account IDs (lines use account_id not account_code)
	accW := doJSON(t, r, "GET", "/api/v1/accounts", "", token)
	assertStatus(t, accW, http.StatusOK)
	var accounts []map[string]any
	if err := json.NewDecoder(bytes.NewBufferString(accW.Body.String())).Decode(&accounts); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	// Find account IDs for codes 1000 (Caisse) and 2000 (Dettes)
	var acc1000ID, acc2000ID string
	for _, acc := range accounts {
		code, _ := acc["Code"].(string)
		id, _ := acc["ID"].(string)
		if code == "1000" {
			acc1000ID = id
		}
		if code == "2000" {
			acc2000ID = id
		}
	}
	if acc1000ID == "" || acc2000ID == "" {
		t.Fatalf("could not find seeded accounts 1000/2000 — got %d accounts", len(accounts))
	}

	// Create a draft journal entry: debit 1000 (Caisse) / credit 2000
	today := time.Now().Format("2006-01-02")
	debit := 500.0
	credit := 500.0
	entry := map[string]any{
		"date":        today,
		"description": "Test double-entry",
		"lines": []map[string]any{
			{"account_id": acc1000ID, "debit_amount": debit, "description": "Caisse"},
			{"account_id": acc2000ID, "credit_amount": credit, "description": "Capital"},
		},
	}

	createW := doJSON(t, r, "POST", "/api/v1/journal", mustJSON(t, entry), token)
	assertStatus(t, createW, http.StatusCreated)

	createBody := parseBody(t, createW)
	// JournalEntry model serializes ID as uppercase (no json tags)
	entryID, _ := createBody["ID"].(string)
	if entryID == "" {
		t.Fatalf("created entry has no ID — body: %s", createW.Body.String())
	}
	if createBody["Status"] != "draft" {
		t.Errorf("new entry should be draft, got %v", createBody["Status"])
	}

	// Post the entry — returns {"status": "posted"}
	postW := doJSON(t, r, "POST", "/api/v1/journal/"+entryID+"/post", "{}", token)
	assertStatus(t, postW, http.StatusOK)

	postBody := parseBody(t, postW)
	if postBody["status"] != "posted" {
		t.Errorf("after post, status should be 'posted', got %v", postBody["status"])
	}
}

func TestJournalDoubleEntryValidation(t *testing.T) {
	r, _, _ := newTestServer(t)
	token := loginAdmin(t, r)

	today := time.Now().Format("2006-01-02")
	// Unbalanced entry: debit 500 ≠ credit 300
	entry := map[string]any{
		"date":        today,
		"description": "Unbalanced",
		"lines": []map[string]any{
			{"account_code": "1000", "debit_amount": 500.0, "credit_amount": 0.0, "description": "dr"},
			{"account_code": "2000", "debit_amount": 0.0, "credit_amount": 300.0, "description": "cr"},
		},
	}
	w := doJSON(t, r, "POST", "/api/v1/journal", mustJSON(t, entry), token)
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Errorf("unbalanced entry should be rejected, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ─── Accounts ─────────────────────────────────────────────────────────────────

func TestAccountsListAndTrialBalance(t *testing.T) {
	r, _, _ := newTestServer(t)
	token := loginAdmin(t, r)

	// Seed plan comptable should give us accounts
	w := doJSON(t, r, "GET", "/api/v1/accounts", "", token)
	assertStatus(t, w, http.StatusOK)

	var accounts []any
	if err := json.NewDecoder(bytes.NewBufferString(w.Body.String())).Decode(&accounts); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	if len(accounts) == 0 {
		t.Error("expected seeded accounts from plan comptable migration, got none")
	}

	// Trial balance
	tbW := doJSON(t, r, "GET", "/api/v1/accounts/trial-balance", "", token)
	assertStatus(t, tbW, http.StatusOK)
}

// ─── Contacts + Invoices ──────────────────────────────────────────────────────

func TestContactCreate(t *testing.T) {
	r, _, _ := newTestServer(t)
	token := loginAdmin(t, r)

	w := doJSON(t, r, "POST", "/api/v1/contacts", mustJSON(t, map[string]any{
		"contact_type": "customer",
		"name":         "Test AG",
		"country":      "CH",
	}), token)
	assertStatus(t, w, http.StatusCreated)

	body := parseBody(t, w)
	// models.Contact has no json tags — Go serializes ID as "ID" (uppercase)
	if body["ID"] == nil || body["ID"] == "" {
		t.Errorf("created contact has no ID — body: %s", w.Body.String())
	}
}

func TestInvoiceCreateAndTransition(t *testing.T) {
	r, _, _ := newTestServer(t)
	token := loginAdmin(t, r)

	// Create a contact first
	contactW := doJSON(t, r, "POST", "/api/v1/contacts", mustJSON(t, map[string]any{
		"contact_type": "customer",
		"name":         "Test AG",
		"country":      "CH",
	}), token)
	assertStatus(t, contactW, http.StatusCreated)
	contactBody := parseBody(t, contactW)
	// models.Contact has no json tags — Go serializes ID as "ID" (uppercase)
	contactID, _ := contactBody["ID"].(string)
	if contactID == "" {
		t.Fatalf("contact has no ID — body: %s", contactW.Body.String())
	}

	today := time.Now().Format("2006-01-02")
	due := time.Now().AddDate(0, 0, 30).Format("2006-01-02")

	invW := doJSON(t, r, "POST", "/api/v1/invoices", mustJSON(t, map[string]any{
		"contact_id": contactID,
		"issue_date": today,
		"due_date":   due,
		"currency":   "CHF",
		"vat_rate":   8.1,
		"lines": []map[string]any{
			{"description": "Service", "quantity": 1.0, "unit_price": 1000.0, "vat_rate": 8.1},
		},
	}), token)
	assertStatus(t, invW, http.StatusCreated)

	invBody := parseBody(t, invW)
	// models.Invoice has no json tags — Go serializes ID as "ID" (uppercase)
	invoiceID, _ := invBody["ID"].(string)
	if invoiceID == "" {
		t.Fatalf("invoice has no ID — body: %s", invW.Body.String())
	}
	if invBody["Status"] != "draft" {
		t.Errorf("new invoice should be draft, got %v", invBody["Status"])
	}

	// DRAFT → SENT
	sentW := doJSON(t, r, "POST", "/api/v1/invoices/"+invoiceID+"/transition",
		mustJSON(t, map[string]any{"status": "sent"}), token)
	assertStatus(t, sentW, http.StatusOK)

	sentBody := parseBody(t, sentW)
	// TransitionInvoice calls GetInvoice internally → returns models.Invoice (uppercase keys)
	if sentBody["Status"] != "sent" {
		t.Errorf("after transition, status should be sent, got %v", sentBody["Status"])
	}
}

// ─── Protected route enforcement ──────────────────────────────────────────────

func TestProtectedRoutesRequireAuth(t *testing.T) {
	r, _, _ := newTestServer(t)

	routes := []struct{ method, path string }{
		{"GET", "/api/v1/accounts"},
		{"GET", "/api/v1/accounts/trial-balance"},
		{"GET", "/api/v1/contacts"},
		{"GET", "/api/v1/invoices"},
		{"GET", "/api/v1/journal"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := doJSON(t, r, rt.method, rt.path, "", "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without token, got %d", w.Code)
			}
		})
	}
}

// ─── Main (needed for test binary in standalone package) ─────────────────────

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// Audit croisé Gemini, série 4 : exercices et comptes (MED-11).

// routeurAuthentifie monte les routes données derrière un auteur authentifié, sans quoi la
// trace d'audit ne s'écrit pas (une trace sans auteur ne trace rien).
func routeurAuthentifie(t *testing.T, auteur string, monter func(r *gin.Engine)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		claims, err := security.ParseToken(usersSecret, mustToken(t, auteur))
		if err != nil {
			t.Fatal(err)
		}
		c.Set("claims", claims)
		c.Next()
	})
	monter(r)
	return r
}

// Un code de compte déjà pris est une erreur de saisie : 409 avec la phrase qui dit quoi faire,
// pas un 500 « erreur de base de données ». Et une création réussie laisse une trace.
func TestUnCompteEnDoubleRepond409EtUneCreationEstTracee(t *testing.T) {
	database, _ := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	h := NewAccountsHandler(database, false)
	r := routeurAuthentifie(t, "admin1", func(r *gin.Engine) { r.POST("/accounts", h.CreateAccount) })

	nouveau := map[string]string{"code": "1099", "name": "Compte d'essai", "account_type": "asset"}
	if w := do(r, http.MethodPost, "/accounts", "", nouveau); w.Code != http.StatusCreated {
		t.Fatalf("création : %d — %s", w.Code, w.Body.String())
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE table_name = 'accounts' AND action = 'account_created'`).Scan(&n)
	if n != 1 {
		t.Errorf("%d ligne(s) d'audit pour la création du compte, attendu 1", n)
	}

	w := do(r, http.MethodPost, "/accounts", "", nouveau)
	if w.Code != http.StatusConflict {
		t.Fatalf("code en double : %d, attendu 409 — %s", w.Code, w.Body.String())
	}
	database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE table_name = 'accounts'`).Scan(&n)
	if n != 1 {
		t.Errorf("le refus a laissé une trace : %d ligne(s), attendu 1", n)
	}
}

// Deux créations simultanées d'exercices qui se chevauchent : une seule passe. Le contrôle et
// l'insertion vivent dans une seule transaction.
func TestDeuxExercicesQuiSeChevauchentNePassentPasEnParallele(t *testing.T) {
	database, _ := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	h := NewFiscalYearHandler(database, false)
	r := routeurAuthentifie(t, "admin1", func(r *gin.Engine) { r.POST("/fiscal-years", h.CreateFiscalYear) })

	const n = 8
	var (
		mu    sync.Mutex
		codes []int
		wg    sync.WaitGroup
		start = make(chan struct{})
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			w := do(r, http.MethodPost, "/fiscal-years", "", map[string]string{
				"name": "Exercice " + string(rune('A'+i)), "start_date": "2030-01-01", "end_date": "2030-12-31"})
			mu.Lock()
			codes = append(codes, w.Code)
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()

	crees := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			crees++
		}
	}
	var total int
	database.QueryRow(`SELECT COUNT(*) FROM fiscal_years WHERE start_date = '2030-01-01'`).Scan(&total)
	if crees != 1 || total != 1 {
		t.Fatalf("%d création(s) acceptée(s), %d exercice(s) en base sur %d requêtes simultanées (statuts %v), attendu 1 et 1",
			crees, total, n, codes)
	}
}

// 409 pour le seul cas qu'il décrit — le nom est pris — et pas pour n'importe quelle erreur.
func TestUnNomDExerciceDejaPrisRepond409(t *testing.T) {
	database, _ := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	h := NewFiscalYearHandler(database, false)
	r := routeurAuthentifie(t, "admin1", func(r *gin.Engine) { r.POST("/fiscal-years", h.CreateFiscalYear) })

	if w := do(r, http.MethodPost, "/fiscal-years", "", map[string]string{
		"name": "2030", "start_date": "2030-01-01", "end_date": "2030-12-31"}); w.Code != http.StatusCreated {
		t.Fatalf("premier exercice : %d — %s", w.Code, w.Body.String())
	}
	var w *httptest.ResponseRecorder = do(r, http.MethodPost, "/fiscal-years", "", map[string]string{
		"name": "2030", "start_date": "2031-01-01", "end_date": "2031-12-31"})
	if w.Code != http.StatusConflict {
		t.Fatalf("nom en double : %d, attendu 409 — %s", w.Code, w.Body.String())
	}
}

// MED-05. Les montants sont des REAL : 0,10 + 0,20 au débit contre 0,30 au crédit laissent
// 5,55e-17 sur un compte soldé. La balance l'affichait comme solde, le compte de résultat
// listait une ligne « 6000 » à 5,55e-17, et le CSV écrivait « -0.00 ».
func TestUnCompteSoldeEstAZeroDansLesEtats(t *testing.T) {
	database, _ := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)

	id := func(code string) string {
		var v string
		if err := database.QueryRow(`SELECT id FROM accounts WHERE code = ?`, code).Scan(&v); err != nil {
			t.Fatalf("compte %s : %v", code, err)
		}
		return v
	}
	// Une écriture à deux lignes, comptabilisée (les lignes AVANT le statut : les déclencheurs
	// d'immuabilité refusent d'y toucher ensuite).
	ecriture := func(n int, date, debit, credit string, montant float64) {
		t.Helper()
		e := "e" + string(rune('0'+n))
		if _, err := database.Exec(`INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
			VALUES (?, ?, ?, 'essai', 'draft', 'admin1')`, e, "JN-2031-00"+string(rune('0'+n)), date); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount, sequence)
			VALUES (?, ?, ?, ?, NULL, 1), (?, ?, ?, NULL, ?, 2)`,
			e+"a", e, id(debit), montant, e+"b", e, id(credit), montant); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE journal_entries SET status = 'posted' WHERE id = ?`, e); err != nil {
			t.Fatal(err)
		}
	}
	ecriture(1, "2031-10-10", "6000", "1020", 0.1)
	ecriture(2, "2031-10-11", "6000", "1020", 0.2)
	ecriture(3, "2031-10-12", "1020", "6000", 0.3)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ah := NewAccountsHandler(database, false)
	rh := NewReportsHandler(database, false)
	xh := NewAccountingExportHandler(database, false)
	r.GET("/accounts/trial-balance", ah.TrialBalance)
	r.GET("/reports/income-statement", rh.IncomeStatement)
	r.GET("/reports/general-ledger", rh.GeneralLedger)
	r.GET("/exports/trial-balance.csv", xh.ExportTrialBalanceCSV)

	get := func(chemin string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, chemin, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s : %d — %s", chemin, w.Code, w.Body.String())
		}
		return w
	}

	var tb []struct {
		Code   string  `json:"code"`
		Debit  float64 `json:"total_debit"`
		Credit float64 `json:"total_credit"`
		Solde  float64 `json:"balance"`
	}
	if err := json.Unmarshal(get("/accounts/trial-balance").Body.Bytes(), &tb); err != nil {
		t.Fatal(err)
	}
	vu := false
	for _, l := range tb {
		if l.Code == "6000" {
			vu = true
			if l.Solde != 0 || l.Debit != 0.3 || l.Credit != 0.3 {
				t.Errorf("balance du 6000 : débit %v crédit %v solde %v — attendu 0.3 / 0.3 / 0", l.Debit, l.Credit, l.Solde)
			}
		}
	}
	if !vu {
		t.Fatal("le compte 6000 manque à la balance")
	}

	var cr struct {
		Expenses      []map[string]any `json:"expenses"`
		TotalExpenses float64          `json:"total_expenses"`
		NetIncome     float64          `json:"net_income"`
	}
	if err := json.Unmarshal(get("/reports/income-statement?from=2031-10-01&to=2031-10-31").Body.Bytes(), &cr); err != nil {
		t.Fatal(err)
	}
	if len(cr.Expenses) != 0 || cr.TotalExpenses != 0 || cr.NetIncome != 0 {
		t.Errorf("compte de résultat d'un compte soldé : %d ligne(s) de charges, total %v, net %v — attendu 0, 0, 0",
			len(cr.Expenses), cr.TotalExpenses, cr.NetIncome)
	}

	var gl struct {
		Closing float64 `json:"closing_balance"`
	}
	if err := json.Unmarshal(get("/reports/general-ledger?account_code=6000&from=2031-10-01&to=2031-10-31").Body.Bytes(), &gl); err != nil {
		t.Fatal(err)
	}
	if gl.Closing != 0 {
		t.Errorf("solde de clôture du grand livre = %v, attendu 0", gl.Closing)
	}

	csv := get("/exports/trial-balance.csv?to=2031-12-31").Body.String()
	if strings.Contains(csv, "-0.00") || strings.Contains(csv, "e-") {
		t.Errorf("le CSV de la balance porte un résidu : %s", csv)
	}
}

// MED-10. Le type de média se lit sans ses paramètres, et un relevé trop gros est refusé AU LIEU
// d'être coupé en silence.
func TestLImportDUnReleveAccepteLeCharsetEtRefuseLesFichiersTropGros(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/bank-statements/import", (&ISO20022Handler{}).ImportCamt053)

	xml := `<?xml version="1.0"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08"><BkToCstmrStmt><GrpHdr><MsgId>X</MsgId></GrpHdr></BkToCstmrStmt></Document>`
	post := func(contentType, corps string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/bank-statements/import", strings.NewReader(corps))
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, ct := range []string{"application/xml", "application/xml; charset=utf-8", "text/xml; charset=UTF-8", "Application/XML;charset=utf-8"} {
		if w := post(ct, xml); w.Code != http.StatusOK {
			t.Errorf("Content-Type %q : %d, attendu 200 — %s", ct, w.Code, w.Body.String())
		}
	}
	// Un relevé de plus de 10 Mo : 413, et un message qui le dit.
	gros := xml + strings.Repeat(" ", maxRelevePoids)
	w := post("application/xml; charset=utf-8", gros)
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "10 Mo") {
		t.Errorf("relevé de plus de 10 Mo : %d — %s, attendu 413 et un message qui nomme la limite", w.Code, w.Body.String())
	}
	// Pile à la limite : accepté (la limite est inclusive).
	exact := xml + strings.Repeat(" ", maxRelevePoids-len(xml))
	if w := post("application/xml", exact); w.Code != http.StatusOK {
		t.Errorf("relevé de 10 Mo exactement : %d — %s", w.Code, w.Body.String())
	}
}

// MED-08. L'archive du dossier SIX s'appelait « facture-FA-….pdf-payload.txt » — le nom de FICHIER
// du PDF pris pour le numéro de la facture — et une facture inconnue répondait 422 (le PDF seul : 404).
func TestLeDossierSIXPorteLeNumeroDeLaFactureEtUneFactureInconnueRepond404(t *testing.T) {
	database, r := serie3Env(t)
	seedContact(t, database, "c1", "Client SA")
	seedDoc(t, database, "i1", "c1", "FA-2026-001", "invoice", "sent", 1000, 0, "", "2026-03-31")
	if _, err := database.Exec(`UPDATE invoices SET recipient_address = 'Rue 3', recipient_postal_code = '1000',
		recipient_city = 'Lausanne', recipient_country = 'CH' WHERE id = 'i1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO company_settings (id, company_name, legal_form, address_street, address_postal_code,
		       address_city, address_country, che_number, vat_number, iban, fiscal_year_start_month, currency)
		VALUES ('co1', 'Alpes SA', '', 'Rue 1', '1000', 'Lausanne', 'CH', '', '', 'CH9300762011623852957', 1, 'CHF')`); err != nil {
		t.Fatal(err)
	}
	ih := NewInvoicesHandler(database, false, nil)
	r.GET("/invoices/:id/six-validation", func(c *gin.Context) { ih.SixValidationDossier(c) })

	w := hit(r, http.MethodGet, "/invoices/i1/six-validation", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("dossier : %d — %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var noms []string
	for _, f := range zr.File {
		noms = append(noms, f.Name)
	}
	for _, attendu := range []string{"FA-2026-001-payload.txt", "FA-2026-001-bulletin.pdf"} {
		trouve := false
		for _, n := range noms {
			trouve = trouve || n == attendu
		}
		if !trouve {
			t.Errorf("l'archive ne contient pas %q : %v", attendu, noms)
		}
	}
	if cd := w.Header().Get("Content-Disposition"); strings.Contains(cd, ".pdf-") {
		t.Errorf("nom de l'archive %q : le nom de fichier du PDF a été pris pour le numéro", cd)
	}

	if w = hit(r, http.MethodGet, "/invoices/inexistante/six-validation", nil); w.Code != http.StatusNotFound {
		t.Errorf("facture inconnue : %d, attendu 404 — %s", w.Code, w.Body.String())
	}
}

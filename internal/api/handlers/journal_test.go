package handlers

// Le journal, le plan comptable et les états financiers, exercés par HTTP.
//
// Ce qui est vérifié ici tient en une phrase : un BROUILLON n'est pas une
// écriture comptable. Il ne compte ni à la balance, ni au bilan, ni au compte de
// résultat. C'est exactement ce qui ne tenait plus : la condition
// « status = 'posted' » était portée par une jointure externe, qui décide si
// l'écriture est rattachée et non si la ligne est retenue — les lignes de
// brouillon survivaient et leurs montants entraient dans les états.
//
// Trouvé en interrogeant un serveur réel, pas en relisant le SQL : la requête se
// lit comme si elle filtrait.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

const journalSecret = "secret-de-test-journal-0123456789"

func journalEnv(t *testing.T) (*sql.DB, *gin.Engine) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-journal-*.db")
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
		`INSERT INTO users (id,email,name,password_hash,role,is_admin,is_active)
		 VALUES ('u1','u1@t.ch','Comptable','x','accountant',0,1)`); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := accounting.New(database, false)
	jh := NewJournalHandler(database, false)
	jwh := NewJournalWriteHandler(svc, database, false)
	ah := NewAccountsHandler(database, false)
	rh := NewReportsHandler(database, false)

	tok, err := security.GenerateAccessToken(journalSecret, "u1", false, 3600_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	journalToken = tok

	api := r.Group("")
	api.Use(func(c *gin.Context) {
		claims, err := security.ParseToken(journalSecret, tok)
		if err != nil {
			t.Fatal(err)
		}
		c.Set("claims", claims)
		c.Next()
	})
	api.GET("/journal", jh.ListJournal)
	api.GET("/journal/:id", jh.GetJournalEntry)
	api.POST("/journal", jwh.CreateEntry)
	api.POST("/journal/:id/post", jwh.PostEntry)
	api.GET("/accounts/trial-balance", ah.TrialBalance)
	api.GET("/reports/balance-sheet", rh.BalanceSheet)
	api.GET("/reports/income-statement", rh.IncomeStatement)
	return database, r
}

var journalToken string

func hit(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func asMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("réponse illisible (%d): %s", w.Code, w.Body.String())
	}
	return m
}

// venteComptant crée une écriture 1000 / 3200 et rend son identifiant.
func venteComptant(t *testing.T, r *gin.Engine, montant float64) string {
	t.Helper()
	w := hit(r, http.MethodPost, "/journal", map[string]any{
		"date":        "2026-08-05",
		"description": "Vente comptant",
		"lines": []map[string]any{
			{"account_code": "1000", "debit_amount": montant},
			{"account_code": "3200", "credit_amount": montant},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("création: statut %d — %s", w.Code, w.Body.String())
	}
	id, _ := asMap(t, w)["id"].(string)
	if id == "" {
		t.Fatalf("aucun identifiant rendu: %s", w.Body.String())
	}
	return id
}

func soldeBalance(t *testing.T, r *gin.Engine, code string) (debit, credit float64) {
	t.Helper()
	w := hit(r, http.MethodGet, "/accounts/trial-balance", nil)
	var lignes []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &lignes); err != nil {
		t.Fatalf("balance illisible: %s", w.Body.String())
	}
	for _, l := range lignes {
		if l["code"] == code {
			d, _ := l["total_debit"].(float64)
			cr, _ := l["total_credit"].(float64)
			return d, cr
		}
	}
	t.Fatalf("compte %s absent de la balance", code)
	return 0, 0
}

// LE test. Un brouillon ne compte nulle part.
func TestUnBrouillonNeCompteNiALaBalanceNiAuBilanNiAuResultat(t *testing.T) {
	_, r := journalEnv(t)
	venteComptant(t, r, 100)

	if d, c := soldeBalance(t, r, "1000"); d != 0 || c != 0 {
		t.Errorf("balance : le brouillon compte (débit %.2f, crédit %.2f)", d, c)
	}

	bilan := asMap(t, hit(r, http.MethodGet, "/reports/balance-sheet?as_of=2026-12-31", nil))
	if total, _ := bilan["total_assets"].(float64); total != 0 {
		t.Errorf("bilan : actif = %.2f pour une écriture jamais comptabilisée", total)
	}

	res := asMap(t, hit(r, http.MethodGet,
		"/reports/income-statement?from=2026-01-01&to=2026-12-31", nil))
	if total, _ := res["total_revenue"].(float64); total != 0 {
		t.Errorf("résultat : produits = %.2f pour une écriture jamais comptabilisée", total)
	}
}

// Comptabilisée, elle compte partout — sinon la correction ci-dessus aurait
// simplement tout mis à zéro.
func TestUneEcritureComptabiliseeCompte(t *testing.T) {
	_, r := journalEnv(t)
	id := venteComptant(t, r, 100)

	if w := hit(r, http.MethodPost, "/journal/"+id+"/post", nil); w.Code != http.StatusOK {
		t.Fatalf("comptabilisation: %d — %s", w.Code, w.Body.String())
	}

	if d, _ := soldeBalance(t, r, "1000"); d != 100 {
		t.Errorf("balance : débit 1000 = %.2f, attendu 100", d)
	}
	if _, c := soldeBalance(t, r, "3200"); c != 100 {
		t.Errorf("balance : crédit 3200 = %.2f, attendu 100", c)
	}

	bilan := asMap(t, hit(r, http.MethodGet, "/reports/balance-sheet?as_of=2026-12-31", nil))
	if total, _ := bilan["total_assets"].(float64); total != 100 {
		t.Errorf("bilan : actif = %.2f, attendu 100", total)
	}
	res := asMap(t, hit(r, http.MethodGet,
		"/reports/income-statement?from=2026-01-01&to=2026-12-31", nil))
	if total, _ := res["total_revenue"].(float64); total != 100 {
		t.Errorf("résultat : produits = %.2f, attendu 100", total)
	}
}

// La liste rend le montant. Sans lui, la colonne « Montant CHF » du journal
// restait vide — et c'est la seule grandeur qui permet de repérer une écriture.
func TestLaListeRendLeMontantEtLAuteur(t *testing.T) {
	_, r := journalEnv(t)
	venteComptant(t, r, 250.50)

	body := asMap(t, hit(r, http.MethodGet, "/journal", nil))
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("%d écriture(s) dans la liste, attendu 1 : %s", len(items), body)
	}
	it := items[0].(map[string]any)
	if total, _ := it["total"].(float64); total != 250.50 {
		t.Errorf("total = %v, attendu 250.50", it["total"])
	}
	if auteur, _ := it["author"].(string); auteur != "Comptable" {
		t.Errorf("auteur = %q — la traçabilité du CO art. 957a al. 2 ch. 5 doit être lisible", auteur)
	}
	if ref, _ := it["reference"].(string); ref == "" {
		t.Error("aucune référence")
	}
}

// Le journal est UN registre : il ne se filtre pas sur l'auteur.
//
// Le filtre retiré restreignait la liste aux écritures du compte connecté, si
// bien que deux personnes travaillant sur les mêmes livres voyaient deux
// journaux différents, tous deux en désaccord avec la balance.
func TestLeJournalMontreLesEcrituresDeTousLesComptes(t *testing.T) {
	database, r := journalEnv(t)
	venteComptant(t, r, 100)

	// Une écriture d'un autre compte, écrite directement pour ne pas dépendre
	// d'une seconde session.
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,role,is_admin,is_active)
		 VALUES ('u2','u2@t.ch','Autre','x','accountant',0,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
		VALUES ('e2','JN-2026-999','2026-08-04','Écriture d''un autre','draft','u2')`); err != nil {
		t.Fatal(err)
	}

	body := asMap(t, hit(r, http.MethodGet, "/journal", nil))
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("%d écriture(s) visible(s), attendu 2 — le journal doit être complet "+
			"(CO art. 957a al. 2 ch. 2)", len(items))
	}
}

// ─── Les refus, et ce qu'ils disent ──────────────────────────────────────────

func TestUnNumeroDeCompteInconnuEstRefuseEtEstNomme(t *testing.T) {
	_, r := journalEnv(t)
	w := hit(r, http.MethodPost, "/journal", map[string]any{
		"date":        "2026-08-05",
		"description": "Test",
		"lines": []map[string]any{
			{"account_code": "10", "debit_amount": 10},
			{"account_code": "3200", "credit_amount": 10},
		},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d pour un compte inexistant", w.Code)
	}
	msg, _ := asMap(t, w)["error"].(string)
	// Le message doit nommer le numéro fautif : sur une écriture de quatre
	// lignes, « compte introuvable » oblige à deviner lequel.
	for _, want := range []string{"10", "ligne 1"} {
		if !contains(msg, want) {
			t.Errorf("le message ne dit pas %q : %s", want, msg)
		}
	}
}

func TestUneEcritureDeseequilibreeDitLEcart(t *testing.T) {
	_, r := journalEnv(t)
	w := hit(r, http.MethodPost, "/journal", map[string]any{
		"date":        "2026-08-05",
		"description": "Test",
		"lines": []map[string]any{
			{"account_code": "1000", "debit_amount": 100},
			{"account_code": "3200", "credit_amount": 10},
		},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d pour une écriture déséquilibrée", w.Code)
	}
	msg, _ := asMap(t, w)["error"].(string)
	// L'écart désigne presque toujours la faute de frappe : 90.00 sur 100.00,
	// c'est un zéro oublié.
	if !contains(msg, "90.00") {
		t.Errorf("le message ne donne pas l'écart : %s", msg)
	}
}

func TestUneSeuleLigneEstRefuseeAvecUnMessageLisible(t *testing.T) {
	_, r := journalEnv(t)
	w := hit(r, http.MethodPost, "/journal", map[string]any{
		"date":        "2026-08-05",
		"description": "Test",
		"lines": []map[string]any{
			{"account_code": "1000", "debit_amount": 10},
		},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d", w.Code)
	}
	msg, _ := asMap(t, w)["error"].(string)
	if contains(msg, "validation for") || contains(msg, "'min' tag") {
		t.Errorf("le message du validateur sort tel quel : %s", msg)
	}
	if !contains(msg, "deux lignes") {
		t.Errorf("le message n'explique pas la règle : %s", msg)
	}
}

func TestUnCompteDebiteEtCrediteSurLaMemeLigneEstRefuse(t *testing.T) {
	_, r := journalEnv(t)
	w := hit(r, http.MethodPost, "/journal", map[string]any{
		"date":        "2026-08-05",
		"description": "Test",
		"lines": []map[string]any{
			{"account_code": "1000", "debit_amount": 50, "credit_amount": 50},
			{"account_code": "3200", "credit_amount": 10},
			{"account_code": "1020", "debit_amount": 10},
		},
	})
	// L'écriture est équilibrée — la ligne s'annule elle-même — et passerait
	// donc le contrôle de partie double. C'est précisément pourquoi le refus
	// doit être ailleurs.
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d : une ligne débitée ET créditée a été acceptée", w.Code)
	}
}

func TestUnMontantNegatifEstRefuse(t *testing.T) {
	_, r := journalEnv(t)
	w := hit(r, http.MethodPost, "/journal", map[string]any{
		"date":        "2026-08-05",
		"description": "Test",
		"lines": []map[string]any{
			{"account_code": "1000", "debit_amount": -10},
			{"account_code": "3200", "credit_amount": -10},
		},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d pour des montants négatifs", w.Code)
	}
}

// ─── Le détail ───────────────────────────────────────────────────────────────

// Le détail rend le NUMÉRO et le NOM du compte. L'identifiant interne ne dit
// rien à personne, et une écriture qu'on ne peut pas relire ne se contrôle pas.
func TestLeDetailRendLesComptesEnClair(t *testing.T) {
	_, r := journalEnv(t)
	id := venteComptant(t, r, 100)

	body := asMap(t, hit(r, http.MethodGet, "/journal/"+id, nil))
	lines, _ := body["lines"].([]any)
	if len(lines) != 2 {
		t.Fatalf("%d ligne(s), attendu 2", len(lines))
	}
	vus := map[string]bool{}
	for _, l := range lines {
		m := l.(map[string]any)
		code, _ := m["account_code"].(string)
		name, _ := m["account_name"].(string)
		if code == "" || name == "" {
			t.Fatalf("ligne sans compte lisible : %v", m)
		}
		vus[code] = true
	}
	if !vus["1000"] || !vus["3200"] {
		t.Fatalf("comptes attendus absents : %v", vus)
	}

	// Un brouillon n'est scellé par rien, et cela doit se voir.
	if h, _ := body["integrity_hash"].(string); h != "" {
		t.Errorf("un brouillon porte une empreinte : %q", h)
	}
}

func TestLEmpreinteApparaitApresComptabilisation(t *testing.T) {
	_, r := journalEnv(t)
	id := venteComptant(t, r, 100)
	if w := hit(r, http.MethodPost, "/journal/"+id+"/post", nil); w.Code != http.StatusOK {
		t.Fatalf("comptabilisation: %d — %s", w.Code, w.Body.String())
	}
	body := asMap(t, hit(r, http.MethodGet, "/journal/"+id, nil))
	h, _ := body["integrity_hash"].(string)
	if len(h) != 64 {
		t.Fatalf("empreinte = %q — une écriture comptabilisée est scellée (CO art. 957a)", h)
	}
}

func TestUneEcritureInconnueRendUnQuatreCentQuatre(t *testing.T) {
	_, r := journalEnv(t)
	if w := hit(r, http.MethodGet, "/journal/inexistante", nil); w.Code != http.StatusNotFound {
		t.Fatalf("statut = %d", w.Code)
	}
}

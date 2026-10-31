package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db/dbtest"
	"github.com/kmdn-ch/ledgeralps/internal/i18n"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Audit croisé Gemini, série 3 — comportements de bout en bout.
//
//   - HIGH-03 : un corps mal formé sur la note de crédit émettait un avoir de 100 %.
//   - HIGH-10 : la balance âgée additionnait le TOTAL des factures, sans les acomptes ni les avoirs.
//   - CRIT-12 : anonymiser un fournisseur effaçait son nom de la pièce de l'impôt préalable.

// serie3Env : l'environnement du journal, plus les routes de cette série.
func serie3Env(t *testing.T) (*sql.DB, *gin.Engine) {
	t.Helper()
	database, r := journalEnv(t)
	ih := NewInvoicesHandler(database, false, nil)
	rh := NewReportsHandler(database, false)
	avecClaims := func(h gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			claims, err := security.ParseToken(journalSecret, journalToken)
			if err != nil {
				t.Fatal(err)
			}
			c.Set("claims", claims)
			h(c)
		}
	}
	r.POST("/invoices/:id/credit-note", avecClaims(ih.CreateCreditNote))
	r.GET("/reports/ar-aging", avecClaims(rh.ARaging))
	r.GET("/stats", avecClaims(NewStatsHandler(database, false).GetStats))
	fyh := NewFiscalYearHandler(database, false)
	r.GET("/fiscal-years/:id/closing-repair", avecClaims(fyh.PlanClosingRepair))
	r.POST("/fiscal-years/:id/closing-repair", avecClaims(fyh.RepairClosing))
	return database, r
}

// seedDoc insère un document directement, avec le minimum que les lectures exigent.
func seedDoc(t *testing.T, database *sql.DB, id, contactID, number, docType, status string, total, paid float64, corrects, due string) {
	t.Helper()
	var c any
	if corrects != "" {
		c = corrects
	}
	if _, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, vat_rate, amount_paid,
		                      recipient_name, corrects_invoice_id, created_by_id)
		VALUES (?, ?, ?, ?, ?, '2026-03-01', ?, 'CHF', ?, 0, ?, 0, ?, 'Client SA', ?, 'u1')`,
		id, number, docType, contactID, status, due, total, total, paid, c); err != nil {
		t.Fatalf("seed %s: %v", number, err)
	}
	if docType == "invoice" {
		if _, err := database.Exec(`
			INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price, discount_pct, vat_rate, line_total, sequence)
			VALUES (?, ?, 'Prestation', 1, ?, 0, 0, ?, 0)`, "l-"+id, id, total, total); err != nil {
			t.Fatalf("seed ligne %s: %v", number, err)
		}
	}
}

func nbAvoirs(t *testing.T, database *sql.DB, invoiceID string) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM invoices WHERE corrects_invoice_id = ?`, invoiceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUnCorpsMalFormeNEmetPasUnAvoirIntegral(t *testing.T) {
	database, r := serie3Env(t)
	seedContact(t, database, "c1", "Client SA")
	seedDoc(t, database, "i1", "c1", "FA-2026-001", "invoice", "sent", 1000, 0, "", "2026-03-31")

	// Une virgule en trop dans les lignes à créditer : ce n'est pas « pas de corps ».
	for _, corps := range []string{`{"lines": [{"description": "Rabais",}]}`, `pas du json`, `{"lines": 5}`} {
		w := hitRaw(r, http.MethodPost, "/invoices/i1/credit-note", corps)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("corps %q : %d, attendu 422 — %s", corps, w.Code, w.Body.String())
		}
	}
	if n := nbAvoirs(t, database, "i1"); n != 0 {
		t.Fatalf("%d avoir(s) émis par des corps mal formés — c'était l'avoir intégral silencieux", n)
	}

	// Un corps VIDE crédite toujours la facture en entier : c'est le cas courant.
	w := hit(r, http.MethodPost, "/invoices/i1/credit-note", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("corps vide : %d, attendu 201 — %s", w.Code, w.Body.String())
	}
	if n := nbAvoirs(t, database, "i1"); n != 1 {
		t.Fatalf("%d avoir(s) après un corps vide, attendu 1", n)
	}
}

func TestLaBalanceAgeeEstNetteDesAcomptesEtDesAvoirs(t *testing.T) {
	database, r := serie3Env(t)
	seedContact(t, database, "c1", "Client SA")
	echeance := time.Now().AddDate(0, 0, -10).Format("2006-01-02") // 10 jours de retard

	seedDoc(t, database, "plein", "c1", "FA-1", "invoice", "sent", 1000, 0, "", echeance)     // 1000 dus
	seedDoc(t, database, "acompte", "c1", "FA-2", "invoice", "sent", 1000, 400, "", echeance) // 600 dus
	seedDoc(t, database, "credite", "c1", "FA-3", "invoice", "sent", 1000, 0, "", echeance)   // 700 dus
	seedDoc(t, database, "cn1", "c1", "NC-3", "credit_note", "sent", 300, 0, "credite", echeance)
	seedDoc(t, database, "solde", "c1", "FA-4", "invoice", "sent", 500, 0, "", echeance) // entièrement crédité : 0 dû
	seedDoc(t, database, "cn2", "c1", "NC-4", "credit_note", "sent", 500, 0, "solde", echeance)
	seedDoc(t, database, "brouillon", "c1", "NC-5", "credit_note", "draft", 400, 0, "plein", echeance) // ne compte pas

	rep := asMap(t, hit(r, http.MethodGet, "/reports/ar-aging", nil))
	attendu := 1000.0 + 600 + 700
	if got, _ := rep["grand_total"].(float64); math.Abs(got-attendu) > 0.005 {
		t.Errorf("total général %.2f, attendu %.2f (1000 + 600 d'acompte + 700 après avoir)", got, attendu)
	}
	lignes, _ := rep["days_1_to_30"].([]any)
	if len(lignes) != 3 {
		t.Fatalf("%d facture(s) classées, attendu 3 (la facture entièrement créditée n'est plus due) : %v", len(lignes), lignes)
	}
	dus := map[string]float64{}
	for _, l := range lignes {
		m := l.(map[string]any)
		dus[m["invoice_number"].(string)], _ = m["balance_due"].(float64)
	}
	for n, v := range map[string]float64{"FA-1": 1000, "FA-2": 600, "FA-3": 700} {
		if math.Abs(dus[n]-v) > 0.005 {
			t.Errorf("%s : reste dû %.2f, attendu %.2f", n, dus[n], v)
		}
	}
}

func TestOnNAnonymisePasUnFournisseurDontLaPieceEstConservee(t *testing.T) {
	database, r := serie3Env(t)
	// Une écriture comptabilisée, pour que la facture fournisseur puisse la citer.
	ecrire(t, r, "2026-03-15", debit("6500", 100), credit("2000", 100))
	var entry string
	if err := database.QueryRow(`SELECT id FROM journal_entries LIMIT 1`).Scan(&entry); err != nil {
		t.Fatal(err)
	}
	h := NewContactsHandler(database, false)

	insererPiece := func(id, ref, statut string, ecriture any, date string) {
		t.Helper()
		if _, err := database.Exec(`
			INSERT INTO supplier_invoices (id, supplier_id, supplier_reference, status, issue_date, subtotal_amount,
			                               vat_amount, total_amount, journal_entry_id)
			VALUES (?, 'fourn', ?, ?, ?, 100, 0, 100, ?)`, id, ref, statut, date, ecriture); err != nil {
			t.Fatalf("seed pièce %s: %v", ref, err)
		}
	}
	seedContact(t, database, "fourn", "Fournisseur SA")

	// Un brouillon n'est pas une pièce comptable : rien ne s'oppose à l'anonymisation.
	insererPiece("p-brouillon", "B-1", "draft", nil, "2026-03-01")
	// Une pièce comptabilisée il y a plus de dix ans n'est plus soumise à la conservation.
	insererPiece("p-ancienne", "A-1", "paid", entry, fmt.Sprintf("%d-03-01", time.Now().Year()-12))
	if code, body := runAnonymise(t, h, "fourn", true); code != http.StatusOK {
		t.Fatalf("avec un brouillon et une pièce prescrite : %d, attendu 200 — %v", code, body)
	}

	// Une pièce comptabilisée récente, elle, doit rester lisible : refus, et rien d'effacé.
	seedContact(t, database, "fourn2", "Autre Fournisseur SA")
	if _, err := database.Exec(`
		INSERT INTO supplier_invoices (id, supplier_id, supplier_reference, status, issue_date, subtotal_amount,
		                               vat_amount, total_amount, journal_entry_id)
		VALUES ('p-recente', 'fourn2', 'R-1', 'booked', '2026-03-01', 100, 0, 100, ?)`, entry); err != nil {
		t.Fatal(err)
	}
	code, body := runAnonymise(t, h, "fourn2", true)
	if code != http.StatusConflict || body["reason"] != "supplier_documents_retained" {
		t.Fatalf("pièce comptabilisée récente : %d %v, attendu 409 supplier_documents_retained", code, body)
	}
	var name string
	if err := database.QueryRow(`SELECT name FROM contacts WHERE id = 'fourn2'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Autre Fournisseur SA" {
		t.Fatalf("le fournisseur a été anonymisé malgré le refus : %q", name)
	}
}

// hitRaw envoie un corps tel quel, sans l'encoder : pour les corps qui ne sont pas du JSON valide.
func hitRaw(r *gin.Engine, method, path, corps string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(corps))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─── CRIT-10 : le contrôle d'intégrité voit des lignes réécrites ──────────────

func TestLeControleSignaleDesLignesReecritesApresComptabilisation(t *testing.T) {
	h, exec := newMaintenanceDB(t)
	exec(`INSERT INTO users (id, email, name, password_hash) VALUES ('u1', 'a@t.ch', 'A', 'x')`)

	var banque, ventes string
	if err := h.db.QueryRow(`SELECT id FROM accounts WHERE code = '1020'`).Scan(&banque); err != nil {
		t.Fatal(err)
	}
	if err := h.db.QueryRow(`SELECT id FROM accounts WHERE code = '3000'`).Scan(&ventes); err != nil {
		t.Fatal(err)
	}
	svc := accounting.New(h.db, false)
	montant := 1000.0
	entry, err := svc.CreateEntry(context.Background(), "u1", accounting.CreateEntryRequest{
		Date: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), Description: "Vente",
		Lines: []accounting.LineInput{
			{AccountID: banque, DebitAmount: &montant, Sequence: 1},
			{AccountID: ventes, CreditAmount: &montant, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.PostEntry(context.Background(), "u1", entry.ID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if checksIn(runIntegrity(t, h))["journal_lines_altered"] {
		t.Fatal("une écriture intacte est signalée")
	}

	// Réécriture par quelqu'un qui écrit dans le fichier : les déclencheurs retirés, un compte
	// change, le montant non — l'équilibre ne voit rien.
	dbtest.Desarmer(t, h.db)
	exec(`UPDATE journal_lines SET account_id = (SELECT id FROM accounts WHERE code = '6200')
	      WHERE entry_id = ? AND credit_amount IS NOT NULL`, entry.ID)
	rep := runIntegrity(t, h)
	if !checksIn(rep)["journal_lines_altered"] {
		t.Fatalf("des lignes réécrites ne sont pas signalées : %v", rep)
	}
	if checksIn(rep)["journal_balance"] {
		t.Error("préambule : l'écriture reste équilibrée, le contrôle d'équilibre ne doit rien dire")
	}
}

// ─── CRIT-10 b : la marche à suivre de l'attestation se reproduit ─────────────

// Le texte disait de RETIRER la ligne « self_hash » : retirée, le SHA-256 du reste ne correspond
// jamais — le sceau se calcule sur le document dont la valeur est VIDE, la ligne restant en place
// (audit croisé Gemini, CRIT-10 b). Ce test suit la marche à suivre telle qu'elle est écrite,
// sans logiciel : il vide la valeur, calcule le SHA-256, et compare.
func TestLaMarcheASuivreDeLAttestationSeReproduit(t *testing.T) {
	h, _ := newAuditDB(t)
	doc, err := h.BuildAttestation(context.Background(), "u1", i18n.FR)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`("self_hash":\s*")([0-9a-f]{64})(")`)
	m := re.FindSubmatch(doc)
	if m == nil {
		t.Fatalf("self_hash introuvable dans l'attestation : %s", doc)
	}
	valeur := string(m[2])

	// Marche à suivre : vider la valeur, garder la ligne et les guillemets.
	vide := re.ReplaceAll(doc, []byte(`${1}${3}`))
	somme := sha256.Sum256(vide)
	if got := hex.EncodeToString(somme[:]); got != valeur {
		t.Fatalf("le SHA-256 du fichier à valeur vidée est %s, attendu %s : la marche à suivre ne se reproduit pas", got, valeur)
	}

	// Et la fausse marche — retirer la ligne — ne doit PAS correspondre : c'est ce que le texte
	// prescrivait à tort.
	sansLigne := regexp.MustCompile(`(?m)^\s*"self_hash":\s*"[0-9a-f]{64}"\s*\n?`).ReplaceAll(doc, nil)
	somme = sha256.Sum256(sansLigne)
	if hex.EncodeToString(somme[:]) == valeur {
		t.Error("retirer la ligne donne aussi le bon condensé : le test ne distingue plus les deux marches")
	}

	// Le texte livré décrit la bonne marche.
	var att struct {
		HowToVerify []string `json:"how_to_verify"`
	}
	if err := json.Unmarshal(doc, &att); err != nil {
		t.Fatal(err)
	}
	if len(att.HowToVerify) == 0 || !strings.Contains(att.HowToVerify[0], "videz") {
		t.Errorf("le texte ne décrit pas la marche qui fonctionne : %v", att.HowToVerify)
	}
}

// ─── CRIT-16 b : la rotation d'urgence ne ment pas ───────────────────────────

// Quand la clé vient de JWT_SECRET, régénérer config.json ne change rien : la variable reprend
// la main au démarrage suivant. Répondre « secret régénéré » alors que la clé en service reste
// la même — peut-être compromise — est pire que refuser.
func TestLaRotationDUrgenceEstRefuseeQuandLaCleVientDeLEnvironnement(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("HOME", appData)
	h := &SystemHandler{cfg: &config.Config{JWTSecretDepuisEnv: true}}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/rotate", h.RotateJWTSecret)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/rotate", nil))

	if w.Code != http.StatusConflict {
		t.Fatalf("statut %d, attendu 409 — %s", w.Code, w.Body.String())
	}
	var rep map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rep)
	if rep["reason"] != "secret_from_environment" {
		t.Errorf("raison %v, attendu secret_from_environment", rep["reason"])
	}
	if _, err := os.Stat(config.ConfigFilePath()); err == nil {
		t.Error("config.json a été écrit malgré le refus")
	}
}

// ─── HIGH-22 : les créances du tableau de bord viennent du serveur ───────────

func TestLesCreancesDuTableauDeBordSontNettesEtCalculeesSurToutesLesFactures(t *testing.T) {
	database, r := serie3Env(t)
	seedContact(t, database, "c1", "Client SA")
	echu := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	futur := time.Now().AddDate(0, 0, 20).Format("2006-01-02")

	// Plus de vingt factures : la première page de la liste n'en montre que vingt.
	for i := 0; i < 30; i++ {
		seedDoc(t, database, fmt.Sprintf("f%02d", i), "c1", fmt.Sprintf("FA-%02d", i), "invoice", "sent", 100, 0, "", futur)
	}
	seedDoc(t, database, "acompte", "c1", "FA-A", "invoice", "sent", 1000, 400, "", echu)    // 600 dus, en retard
	seedDoc(t, database, "credite", "c1", "FA-C", "invoice", "sent", 1000, 0, "", echu)      // 700 dus après l'avoir, en retard
	seedDoc(t, database, "cn", "c1", "NC-C", "credit_note", "sent", 300, 0, "credite", echu) // un avoir n'est PAS une créance
	seedDoc(t, database, "offre", "c1", "OF-1", "quote", "sent", 5000, 0, "", echu)          // une offre non plus

	st := asMap(t, hit(r, http.MethodGet, "/stats", nil))
	inv, _ := st["invoices"].(map[string]any)
	if got := inv["total_receivable"]; got != "4300.00" { // 30 × 100 + 600 + 700
		t.Errorf("créances = %v, attendu 4300.00 (30 factures + 600 + 700, sans l'avoir ni l'offre)", got)
	}
	if got := inv["total_overdue"]; got != "1300.00" { // 600 + 700
		t.Errorf("en retard = %v, attendu 1300.00", got)
	}
}

// ─── CRIT-04 : l'assainissement d'une clôture déséquilibrée, par l'API ───────

// cloturerALAncienne fabrique ce que l'ancienne version scellait : les comptes à solde inversé
// (3600 produit débiteur, 4200 charge créditrice) n'ont pas de ligne de clôture.
func cloturerALAncienne(t *testing.T, database *sql.DB, r *gin.Engine) string {
	t.Helper()
	ecrire(t, r, "2024-06-01", debit("1020", 100000), credit("3200", 100000))
	ecrire(t, r, "2024-06-02", debit("3600", 5000), credit("1020", 5000))
	ecrire(t, r, "2024-06-03", debit("6000", 40000), credit("1020", 40000))
	ecrire(t, r, "2024-06-04", debit("1020", 2000), credit("4200", 2000))

	var fy string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2024'`).Scan(&fy); err != nil {
		t.Fatal(err)
	}
	compte := func(code string) string {
		var id string
		if err := database.QueryRow(`SELECT id FROM accounts WHERE code = ?`, code).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := database.Exec(q, args...); err != nil {
			t.Fatalf("%s : %v", q, err)
		}
	}
	must(`INSERT INTO journal_entries (id, reference, date, description, status, fiscal_year_id, created_by_id)
	      VALUES ('cl', 'CLOTURE-2024', '2024-12-31', 'Clôture exercice', 'draft', ?, 'u1')`, fy)
	must(`INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, sequence) VALUES ('cl1', 'cl', ?, 100000, 1)`, compte("3200"))
	must(`INSERT INTO journal_lines (id, entry_id, account_id, credit_amount, sequence) VALUES ('cl2', 'cl', ?, 40000, 2)`, compte("6000"))
	must(`INSERT INTO journal_lines (id, entry_id, account_id, credit_amount, sequence) VALUES ('cl3', 'cl', ?, 57000, 3)`, compte("5900"))
	must(`UPDATE journal_entries SET status = 'posted' WHERE id = 'cl'`)
	must(`UPDATE fiscal_years SET is_closed = 1 WHERE id = ?`, fy)
	return fy
}

func TestLaReparationDUneClotureSeFaitParLAPI(t *testing.T) {
	database, r := serie3Env(t)
	fy := cloturerALAncienne(t, database, r)

	// Le constat d'intégrité propose la réparation.
	h := NewMaintenanceHandler(database, &config.Config{})
	rep := runIntegrity(t, h)
	var proposee bool
	for _, f := range rep["findings"].([]any) {
		m := f.(map[string]any)
		if m["check"] != "journal_balance" {
			continue
		}
		reps, _ := m["repairs"].([]any)
		for _, x := range reps {
			if x.(map[string]any)["fiscal_year_id"] == fy {
				proposee = true
			}
		}
	}
	if !proposee {
		t.Fatalf("le constat « écritures déséquilibrées » ne propose pas la réparation : %v", rep)
	}

	// L'aperçu ne passe rien.
	w := hit(r, http.MethodGet, "/fiscal-years/"+fy+"/closing-repair", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("aperçu : %d — %s", w.Code, w.Body.String())
	}
	if plan := asMap(t, w); plan["gap"] != float64(3000) || plan["reference"] != "CLOTURE-2024-C" {
		t.Errorf("aperçu : %v", plan)
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE reference = 'CLOTURE-2024-C'`).Scan(&n)
	if n != 0 {
		t.Fatal("l'aperçu a écrit une écriture")
	}

	// Sans confirmation, rien ne part.
	if w := hit(r, http.MethodPost, "/fiscal-years/"+fy+"/closing-repair", nil); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("sans confirmation : %d, attendu 422", w.Code)
	}
	if w := hit(r, http.MethodPost, "/fiscal-years/"+fy+"/closing-repair", map[string]any{"confirm": false}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("confirm false : %d, attendu 422", w.Code)
	}

	// Confirmée : passée, 201.
	if w := hit(r, http.MethodPost, "/fiscal-years/"+fy+"/closing-repair", map[string]any{"confirm": true}); w.Code != http.StatusCreated {
		t.Fatalf("réparation : %d — %s", w.Code, w.Body.String())
	}
	// Le contrôle d'intégrité ne signale plus aucune écriture déséquilibrée : la clôture et son
	// complément s'équilibrent ensemble.
	if checksIn(runIntegrity(t, h))["journal_balance"] {
		t.Error("le contrôle signale encore des écritures déséquilibrées après la réparation")
	}
	// Une seconde réparation : 409.
	if w := hit(r, http.MethodPost, "/fiscal-years/"+fy+"/closing-repair", map[string]any{"confirm": true}); w.Code != http.StatusConflict {
		t.Errorf("seconde réparation : %d, attendu 409", w.Code)
	}
}

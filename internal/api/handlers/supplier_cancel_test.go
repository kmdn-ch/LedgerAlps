package handlers

// Annuler une facture fournisseur doit vider les livres, pas seulement la liste.
//
// Le défaut d'origine : passer une facture comptabilisée à « annulée » changeait
// son statut et rien d'autre. La charge et la TVA déductible restaient dans les
// livres et continuaient d'alimenter le résultat et la déclaration, pendant que
// l'écran affichait « annulée ». Un tel écart ne se découvre qu'au décompte
// trimestriel, des mois plus tard, sans rien qui pointe vers sa cause.
//
// Ces tests portent donc sur les SOLDES, pas sur le statut : c'est le seul
// endroit où le défaut se voyait.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

const cancelSecret = "secret-de-test-annulation-0123456789"

func cancelEnv(t *testing.T) (*sql.DB, *gin.Engine, *SupplierInvoicesHandler) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-cancel-*.db")
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
	if _, err := database.Exec(
		`INSERT INTO contacts (id,contact_type,name,country,is_active)
		 VALUES ('c1','supplier','Fournisseur SA','CH',1)`); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := accounting.New(database, false)
	h := NewSupplierInvoicesHandler(database, false).WithAccounting(svc)

	tok, err := security.GenerateAccessToken(cancelSecret, "u1", false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r.Use(func(c *gin.Context) {
		claims, err := security.ParseToken(cancelSecret, tok)
		if err != nil {
			t.Fatal(err)
		}
		c.Set("claims", claims)
		c.Next()
	})
	r.POST("/supplier-invoices/cancel", h.CancelSupplierInvoices)
	r.POST("/supplier-invoices/:id/transition", h.TransitionSupplierInvoice)
	return database, r, h
}

// creerFacture insère une facture fournisseur et la comptabilise via l'API,
// pour que l'écriture soit passée exactement comme en production.
func creerFacture(t *testing.T, database *sql.DB, r *gin.Engine, id, ref string,
	ht, tva float64) {
	t.Helper()
	_, err := database.Exec(`
		INSERT INTO supplier_invoices
		    (id, supplier_id, supplier_reference, status, issue_date, due_date,
		     currency, subtotal_amount, vat_amount, total_amount, amount_paid,
		     expense_account_code, created_by_id)
		VALUES (?, 'c1', ?, 'draft', ?, ?, 'CHF', ?, ?, ?, 0, '6500', 'u1')`,
		id, ref, time.Now().UTC(), time.Now().UTC(), ht, tva, ht+tva)
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"status":"booked"}`)
	req := httptest.NewRequest(http.MethodPost, "/supplier-invoices/"+id+"/transition", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("comptabilisation de %s : code %d — %s", id, w.Code, w.Body.String())
	}
}

// soldeCompte rend débit − crédit pour un compte, sur les écritures passées.
func soldeCompte(t *testing.T, database *sql.DB, code string) float64 {
	t.Helper()
	var solde sql.NullFloat64
	err := database.QueryRow(`
		SELECT SUM(COALESCE(jl.debit_amount,0) - COALESCE(jl.credit_amount,0))
		FROM journal_lines jl
		JOIN accounts a       ON a.id = jl.account_id
		JOIN journal_entries e ON e.id = jl.entry_id
		WHERE a.code = ? AND e.status = 'posted'`, code).Scan(&solde)
	if err != nil {
		t.Fatal(err)
	}
	return solde.Float64
}

func annuler(t *testing.T, r *gin.Engine, ids ...string) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"ids": ids, "reason": "saisie d'essai"})
	req := httptest.NewRequest(http.MethodPost, "/supplier-invoices/cancel",
		bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("annulation : code %d — %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// LE test : après annulation, les comptes reviennent à zéro.
//
// Le statut ne prouve rien — c'est lui qui mentait. Ce sont les soldes qui
// disent si la charge a réellement quitté les livres.
func TestUneAnnulationNeutraliseLaChargeEtLaTVA(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 1000, 81)

	if got := soldeCompte(t, database, "6500"); round2(got) != 1000 {
		t.Fatalf("charge après comptabilisation = %.2f, attendu 1000", got)
	}
	if got := soldeCompte(t, database, "2262"); round2(got) != 81 {
		t.Fatalf("TVA déductible = %.2f, attendu 81", got)
	}
	if got := soldeCompte(t, database, "2000"); round2(got) != -1081 {
		t.Fatalf("créanciers = %.2f, attendu -1081", got)
	}

	annuler(t, r, "f1")

	for _, cas := range []struct {
		compte string
		nom    string
	}{
		{"6500", "charge"},
		{"2262", "TVA déductible"},
		{"2000", "créanciers"},
	} {
		if got := round2(soldeCompte(t, database, cas.compte)); got != 0 {
			t.Errorf("%s (%s) = %.2f après extourne, attendu 0 — "+
				"l'annulation n'a pas vidé les livres", cas.nom, cas.compte, got)
		}
	}
}

// L'écriture d'origine RESTE : on corrige, on n'efface pas (CO art. 958f).
func TestLEcritureDOrigineEtLaFactureSontConservees(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 1000, 81)
	annuler(t, r, "f1")

	var entrees int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM journal_entries WHERE status = 'posted'`).Scan(&entrees); err != nil {
		t.Fatal(err)
	}
	if entrees != 2 {
		t.Errorf("%d écriture(s) comptabilisée(s), attendu 2 — l'origine et son extourne", entrees)
	}

	var statut string
	if err := database.QueryRow(
		`SELECT status FROM supplier_invoices WHERE id = 'f1'`).Scan(&statut); err != nil {
		t.Fatal(err)
	}
	if statut != "cancelled" {
		t.Errorf("statut %q, attendu cancelled", statut)
	}
}

// Un brouillon n'a rien dans les livres : il disparaît vraiment.
func TestUnBrouillonEstSupprimeSansEcriture(t *testing.T) {
	database, r, _ := cancelEnv(t)
	if _, err := database.Exec(`
		INSERT INTO supplier_invoices
		    (id, supplier_id, supplier_reference, status, issue_date, currency,
		     subtotal_amount, vat_amount, total_amount, amount_paid, created_by_id)
		VALUES ('b1','c1','BR-001','draft', ?, 'CHF', 100, 8.1, 108.1, 0, 'u1')`,
		time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	annuler(t, r, "b1")

	var reste int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM supplier_invoices WHERE id = 'b1'`).Scan(&reste); err != nil {
		t.Fatal(err)
	}
	if reste != 0 {
		t.Error("le brouillon n'a pas été supprimé")
	}
	if got := soldeCompte(t, database, "6500"); round2(got) != 0 {
		t.Errorf("un brouillon a écrit %.2f dans les livres", got)
	}
}

// Une facture déjà réglée est refusée : l'argent est parti.
func TestUneFacturePayeeNEstPasAnnulable(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 1000, 81)
	if _, err := database.Exec(
		`UPDATE supplier_invoices SET amount_paid = 1081 WHERE id = 'f1'`); err != nil {
		t.Fatal(err)
	}

	out := annuler(t, r, "f1")
	res := out["results"].([]any)[0].(map[string]any)
	if res["outcome"] != "refused" {
		t.Fatalf("verdict %q, attendu refused", res["outcome"])
	}
	if got := round2(soldeCompte(t, database, "6500")); got != 1000 {
		t.Errorf("charge = %.2f — un refus ne doit toucher à rien", got)
	}
}

// Un lot partiel traite ce qu'il peut et dit ligne par ligne ce qu'il a fait.
func TestUnLotPartielTraiteCeQuIlPeut(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 100, 8.1)
	creerFacture(t, database, r, "f2", "FA-002", 200, 16.2)
	if _, err := database.Exec(
		`UPDATE supplier_invoices SET amount_paid = 216.2 WHERE id = 'f2'`); err != nil {
		t.Fatal(err)
	}

	out := annuler(t, r, "f1", "f2", "inexistante")
	if int(out["processed"].(float64)) != 1 {
		t.Errorf("%v traitée(s), attendu 1", out["processed"])
	}
	if int(out["total"].(float64)) != 3 {
		t.Errorf("%v lignes rendues, attendu 3", out["total"])
	}
	if got := round2(soldeCompte(t, database, "6500")); got != 200 {
		t.Errorf("charge restante = %.2f, attendu 200 — seule f1 devait être extournée", got)
	}
}

// Recocher une ligne déjà annulée ne repasse pas d'extourne.
//
// Sans cette garde, un double clic ou un rechargement doublerait la correction
// et laisserait la charge en négatif.
func TestUneSecondeAnnulationNeDoublePasLExtourne(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 1000, 81)
	annuler(t, r, "f1")
	out := annuler(t, r, "f1")

	res := out["results"].([]any)[0].(map[string]any)
	if res["outcome"] != "skipped" {
		t.Errorf("verdict %q, attendu skipped", res["outcome"])
	}
	if got := round2(soldeCompte(t, database, "6500")); got != 0 {
		t.Errorf("charge = %.2f après double annulation, attendu 0", got)
	}
}

// La trace suit l'action — c'est tout l'objet du lot de couverture.
//
// Le journal chaîné ne portait que trois actions ; les factures fournisseurs
// n'y laissaient rien. Un journal à trous est pire qu'un journal absent : on le
// consulte en croyant qu'il dit tout, et l'absence d'une ligne se lit comme
// « cela n'a pas eu lieu » alors qu'elle veut dire « cela n'a jamais été
// écrit ».
func TestChaqueAnnulationLaisseUnMaillonDAudit(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 1000, 81)
	annuler(t, r, "f1")

	var n int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM audit_logs
		WHERE table_name = 'supplier_invoices' AND action = ? AND record_id = 'f1'`,
		ActionSupplierInvoiceCancelled).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d maillon(s) d'audit pour l'annulation, attendu 1", n)
	}

	// L'auteur est obligatoire : une trace anonyme ne trace rien.
	var auteur string
	if err := database.QueryRow(`
		SELECT user_id FROM audit_logs
		WHERE table_name = 'supplier_invoices' AND action = ? LIMIT 1`,
		ActionSupplierInvoiceCancelled).Scan(&auteur); err != nil {
		t.Fatal(err)
	}
	if auteur != "u1" {
		t.Errorf("auteur %q, attendu u1", auteur)
	}
}

// Un REFUS se trace aussi.
//
// Sans cela, la question qui se pose après coup reste sans réponse : « on a
// bien essayé de la retirer, pourquoi est-elle encore là ? »
func TestUnRefusLaisseAussiUneTrace(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 1000, 81)
	if _, err := database.Exec(
		`UPDATE supplier_invoices SET amount_paid = 1081 WHERE id = 'f1'`); err != nil {
		t.Fatal(err)
	}
	annuler(t, r, "f1")

	var etat string
	if err := database.QueryRow(`
		SELECT COALESCE(after_state, '') FROM audit_logs
		WHERE record_id = 'f1' AND action = ? LIMIT 1`,
		ActionSupplierInvoiceCancelled).Scan(&etat); err != nil {
		t.Fatal(err)
	}
	if etat == "" {
		t.Fatal("aucune trace pour un refus")
	}
}

// La comptabilisation laisse elle aussi son maillon.
func TestLaComptabilisationEstTracee(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "FA-001", 100, 8.1)

	var n int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM audit_logs
		WHERE table_name = 'supplier_invoices' AND action = ?`,
		ActionSupplierInvoiceBooked).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d maillon(s) pour la comptabilisation, attendu 1", n)
	}
}

package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Audit croisé Gemini, CRIT-06 : la route de transition d'une facture fournisseur ne
// validait aucune transition et n'écrivait rien au règlement.
//
//   - comptabilisée → brouillon était permis, puis la pièce se modifiait et se supprimait,
//     son écriture restant au journal ;
//   - comptabilisée → annulée ne passait pas l'extourne ;
//   - brouillon → payée faisait entrer sa TVA préalable dans la déclaration sans la
//     moindre écriture ;
//   - comptabilisée → payée ne passait aucun règlement : la dette (2000) gonflait au bilan.

func transitionner(t *testing.T, r *gin.Engine, id string, corps map[string]any) (int, map[string]any) {
	t.Helper()
	payload, _ := json.Marshal(corps)
	req := httptest.NewRequest(http.MethodPost, "/supplier-invoices/"+id+"/transition", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func statutFournisseur(t *testing.T, database *sql.DB, id string) (status string, paid float64, entry sql.NullString) {
	t.Helper()
	if err := database.QueryRow(`SELECT status, amount_paid, payment_entry_id FROM supplier_invoices WHERE id = ?`, id).
		Scan(&status, &paid, &entry); err != nil {
		t.Fatal(err)
	}
	return
}

func brouillonFournisseur(t *testing.T, database *sql.DB, id, ref string, ht, tva float64) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO supplier_invoices
		    (id, supplier_id, supplier_reference, status, issue_date, due_date, currency,
		     subtotal_amount, vat_amount, total_amount, amount_paid, expense_account_code, created_by_id)
		VALUES (?, 'c1', ?, 'draft', '2026-05-01', '2026-05-31', 'CHF', ?, ?, ?, 0, '6500', 'u1')`,
		id, ref, ht, tva, ht+tva); err != nil {
		t.Fatal(err)
	}
}

func TestUnePieceComptabiliseeNeRedevientPasUnBrouillon(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "F-1", 1000, 81)

	for _, cible := range []string{"draft"} {
		code, body := transitionner(t, r, "f1", map[string]any{"status": cible})
		if code != http.StatusConflict || body["reason"] != "transition_not_allowed" {
			t.Errorf("comptabilisée → %s : %d %v, attendu 409 transition_not_allowed", cible, code, body)
		}
	}
	if s, _, _ := statutFournisseur(t, database, "f1"); s != "booked" {
		t.Errorf("statut = %s, attendu booked", s)
	}
}

func TestUnBrouillonNePasseJamaisDirectementPayee(t *testing.T) {
	database, r, _ := cancelEnv(t)
	brouillonFournisseur(t, database, "f1", "F-1", 1000, 81)

	code, body := transitionner(t, r, "f1", map[string]any{"status": "paid"})
	if code != http.StatusConflict {
		t.Fatalf("brouillon → payée : %d %v, attendu 409", code, body)
	}
	if s, _, _ := statutFournisseur(t, database, "f1"); s != "draft" {
		t.Errorf("statut = %s : la TVA d'une pièce jamais comptabilisée entrerait dans la déclaration", s)
	}
}

func TestAnnulerUnePieceComptabiliseePasseLExtourne(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "F-1", 1000, 81)
	if v := soldeCompte(t, database, "6500"); v != 1000 {
		t.Fatalf("charge avant annulation = %.2f", v)
	}

	code, body := transitionner(t, r, "f1", map[string]any{"status": "cancelled"})
	if code != http.StatusOK {
		t.Fatalf("annulation : %d %v", code, body)
	}
	for _, c := range []string{"6500", "2262", "2000"} {
		if v := soldeCompte(t, database, c); v != 0 {
			t.Errorf("compte %s = %.2f après annulation, attendu 0 : l'extourne n'a pas été passée", c, v)
		}
	}
	var n int
	_ = database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE is_reversal = 1`).Scan(&n)
	if n != 1 {
		t.Errorf("%d extourne(s), attendu 1", n)
	}
}

func TestPayerUneFactureFournisseurPasseLeReglement(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "F-1", 1000, 81)
	// La comptabilisation a créé la dette : 1081 au crédit de 2000.
	if v := soldeCompte(t, database, "2000"); v != -1081 {
		t.Fatalf("dette avant règlement = %.2f, attendu -1081.00", v)
	}

	code, body := transitionner(t, r, "f1", map[string]any{"status": "paid", "payment_date": "2026-06-15"})
	if code != http.StatusOK {
		t.Fatalf("règlement : %d %v", code, body)
	}
	if v := soldeCompte(t, database, "2000"); v != 0 {
		t.Errorf("dette après règlement = %.2f, attendu 0 : le règlement n'est pas passé", v)
	}
	if v := soldeCompte(t, database, "1020"); v != -1081 {
		t.Errorf("banque = %.2f, attendu -1081.00 (sortie du règlement)", v)
	}
	status, paid, entry := statutFournisseur(t, database, "f1")
	if status != "paid" || paid != 1081 || !entry.Valid {
		t.Errorf("en base : %s / %.2f / écriture %v — attendu paid / 1081.00 / liée", status, paid, entry)
	}

	// Redemander « payée » ne repasse rien ; une payée ne s'annule ni ne se rouvre.
	if code, _ := transitionner(t, r, "f1", map[string]any{"status": "paid"}); code != http.StatusOK {
		t.Errorf("second appel : %d, attendu 200 sans effet", code)
	}
	if v := soldeCompte(t, database, "1020"); v != -1081 {
		t.Errorf("banque = %.2f après un second appel : le règlement est passé deux fois", v)
	}
	for _, cible := range []string{"cancelled", "draft", "booked"} {
		if code, _ := transitionner(t, r, "f1", map[string]any{"status": cible}); code != http.StatusConflict {
			t.Errorf("payée → %s : %d, attendu 409", cible, code)
		}
	}
}

func TestReglerEnEspecesDebiteLaCaisse(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "F-1", 200, 0)
	if code, body := transitionner(t, r, "f1", map[string]any{"status": "paid", "method": "cash"}); code != http.StatusOK {
		t.Fatalf("%d %v", code, body)
	}
	if v := soldeCompte(t, database, "1000"); v != -200 {
		t.Errorf("caisse = %.2f, attendu -200.00", v)
	}
	if v := soldeCompte(t, database, "1020"); v != 0 {
		t.Errorf("banque = %.2f, attendu 0 pour un règlement en espèces", v)
	}
}

func TestUnReglementRefuseUneDateAbsurdeOuUnModeInconnu(t *testing.T) {
	database, r, _ := cancelEnv(t)
	creerFacture(t, database, r, "f1", "F-1", 200, 0)
	for _, corps := range []map[string]any{
		{"status": "paid", "payment_date": "0026-06-15"},
		{"status": "paid", "payment_date": "pas une date"},
		{"status": "paid", "method": "bitcoin"},
	} {
		if code, _ := transitionner(t, r, "f1", corps); code < 400 || code >= 500 {
			t.Errorf("%v : %d, attendu un refus 4xx", corps, code)
		}
	}
	if s, _, _ := statutFournisseur(t, database, "f1"); s != "booked" {
		t.Errorf("statut = %s après des refus, attendu booked", s)
	}
}

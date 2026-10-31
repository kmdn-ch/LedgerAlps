package handlers

import (
	"context"
	"math"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Audit croisé Gemini, CRIT-01 et CRIT-03.
//
// CRIT-03 : le bilan ne lisait que l'actif, le passif et les fonds propres. Le
// résultat de l'exercice — produits moins charges, pas encore virés — n'y figurait
// nulle part : une facture de 1 000 + 81 de TVA donnait 1 081 à l'actif contre 81 au
// passif, tous les jours d'un exercice ouvert.
//
// CRIT-01 : le compte de résultat comptait l'écriture de clôture, qui remet les
// comptes à zéro : produits, charges et résultat tombaient à 0.00 sur tout exercice
// clôturé.

func ecrire(t *testing.T, r *gin.Engine, date string, lignes ...map[string]any) {
	t.Helper()
	w := hit(r, http.MethodPost, "/journal", map[string]any{"date": date, "description": "essai", "lines": lignes})
	if w.Code != http.StatusCreated {
		t.Fatalf("création %s: %d — %s", date, w.Code, w.Body.String())
	}
	id, _ := asMap(t, w)["id"].(string)
	if w := hit(r, http.MethodPost, "/journal/"+id+"/post", nil); w.Code != http.StatusOK {
		t.Fatalf("comptabilisation: %d — %s", w.Code, w.Body.String())
	}
}

func debit(code string, v float64) map[string]any {
	return map[string]any{"account_code": code, "debit_amount": v}
}
func credit(code string, v float64) map[string]any {
	return map[string]any{"account_code": code, "credit_amount": v}
}

func bilanAu(t *testing.T, r *gin.Engine, date string) (actif, passifEtFP, resultat float64) {
	t.Helper()
	b := asMap(t, hit(r, http.MethodGet, "/reports/balance-sheet?date="+date, nil))
	actif, _ = b["total_assets"].(float64)
	passifEtFP, _ = b["total_liabilities_equity"].(float64)
	resultat, _ = b["unclosed_result"].(float64)
	return
}

func TestLeBilanDUnExerciceOuvertSEquilibre(t *testing.T) {
	_, r := journalEnv(t)
	// Facture de 1 000 + 8,1 % : débiteurs 1 081 / produits 1 000 / TVA due 81.
	ecrire(t, r, "2026-03-15", debit("1100", 1081), credit("3200", 1000), credit("2261", 81))

	actif, passif, resultat := bilanAu(t, r, "2026-03-15")
	if actif != 1081 || passif != 1081 {
		t.Fatalf("bilan : actif %.2f, passif + fonds propres %.2f — écart de %.2f", actif, passif, actif-passif)
	}
	if resultat != 1000 {
		t.Errorf("résultat non clôturé = %.2f, attendu 1000.00", resultat)
	}
}

func TestLeCompteDeResultatSurvitALaCloture(t *testing.T) {
	database, r := journalEnv(t)
	ecrire(t, r, "2025-06-01", debit("1020", 250000), credit("3200", 250000))
	ecrire(t, r, "2025-06-02", debit("6000", 90000), credit("1020", 90000))

	q := "/reports/income-statement?from=2025-01-01&to=2025-12-31"
	avant := asMap(t, hit(r, http.MethodGet, q, nil))

	var fyID string
	if err := database.QueryRow(`SELECT id FROM fiscal_years WHERE name = '2025'`).Scan(&fyID); err != nil {
		t.Fatal(err)
	}
	if err := accounting.NewFiscalYearService(database, false).CloseYear(context.Background(), fyID, "u1"); err != nil {
		t.Fatalf("clôture: %v", err)
	}

	apres := asMap(t, hit(r, http.MethodGet, q, nil))
	for _, k := range []string{"total_revenue", "total_expenses", "net_income"} {
		a, _ := avant[k].(float64)
		b, _ := apres[k].(float64)
		if b != a || a == 0 {
			t.Errorf("%s : %.2f avant la clôture, %.2f après — le compte de résultat doit être inchangé", k, a, b)
		}
	}
	if n, _ := apres["net_income"].(float64); n != 160000 {
		t.Errorf("résultat net = %.2f, attendu 160000.00", n)
	}

	// Le bilan reste équilibré avant, pendant et après la clôture.
	for _, date := range []string{"2025-06-30", "2025-12-31", "2026-03-01"} {
		a, p, _ := bilanAu(t, r, date)
		if math.Abs(a-p) > 0.005 || a != 160000 {
			t.Errorf("bilan au %s : actif %.2f, passif + fonds propres %.2f (attendu 160000 des deux côtés)", date, a, p)
		}
	}
}

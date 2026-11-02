package handlers

// Audit croisé Gemini, CRIT-07 (décision du propriétaire) : le taux de la dette fiscale nette est celui
// que l'AFC a ACCORDÉ à l'entreprise. Il se saisit dans la fiche entreprise ; LedgerAlps ne le devine pas.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/models"
)

func TestLeTauxTDFNSeSaisitSeConserveEtSEfface(t *testing.T) {
	_, r := nouveauxRéglages(t)
	base := func(extra map[string]any) map[string]any {
		m := map[string]any{"company_name": "Test SA", "vat_status": models.VatLiable}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	// Pas de taux tant qu'on n'en a pas saisi.
	if w := enregistrer(t, r, base(nil)); w.Code != http.StatusOK {
		t.Fatalf("enregistrement : %d %s", w.Code, w.Body.String())
	}
	if got := lireFiche(t, r)["tdfn_rate"]; got != nil {
		t.Errorf("taux = %v sans saisie, attendu null", got)
	}

	// Saisi, il est relu tel quel.
	if w := enregistrer(t, r, base(map[string]any{"tdfn_rate": 6.1})); w.Code != http.StatusOK {
		t.Fatalf("taux 6,1 : %d %s", w.Code, w.Body.String())
	}
	if got := lireFiche(t, r)["tdfn_rate"]; got != 6.1 {
		t.Errorf("taux = %v, attendu 6.1", got)
	}

	// Un enregistrement qui ne porte PAS le champ ne l'efface pas : le formulaire d'un autre écran
	// ne doit pas faire disparaître un taux que l'AFC a accordé.
	if w := enregistrer(t, r, base(nil)); w.Code != http.StatusOK {
		t.Fatalf("enregistrement sans taux : %d %s", w.Code, w.Body.String())
	}
	if got := lireFiche(t, r)["tdfn_rate"]; got != 6.1 {
		t.Errorf("taux = %v après un enregistrement sans le champ, attendu 6.1 conservé", got)
	}

	// 0 l'efface.
	if w := enregistrer(t, r, base(map[string]any{"tdfn_rate": 0})); w.Code != http.StatusOK {
		t.Fatalf("effacement : %d %s", w.Code, w.Body.String())
	}
	if got := lireFiche(t, r)["tdfn_rate"]; got != nil {
		t.Errorf("taux = %v après effacement, attendu null", got)
	}
}

// Un taux hors bornes est refusé AVANT toute écriture : la fiche entière reste celle d'avant. Aucun taux
// de la dette fiscale nette ne dépasse le taux normal (8,1 %), ni n'est négatif.
func TestUnTauxTDFNImpossibleEstRefuseSansRienEcrire(t *testing.T) {
	_, r := nouveauxRéglages(t)
	if w := enregistrer(t, r, map[string]any{"company_name": "Avant SA", "tdfn_rate": 6.1}); w.Code != http.StatusOK {
		t.Fatalf("mise en place : %d %s", w.Code, w.Body.String())
	}
	for _, taux := range []float64{8.5, 100, -1, 648} {
		w := enregistrer(t, r, map[string]any{"company_name": "Apres SA", "tdfn_rate": taux})
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("taux %v : %d %s, attendu 422", taux, w.Code, w.Body.String())
		}
	}
	fiche := lireFiche(t, r)
	if fiche["company_name"] != "Avant SA" || fiche["tdfn_rate"] != 6.1 {
		t.Errorf("la fiche a changé malgré les refus : %v / %v", fiche["company_name"], fiche["tdfn_rate"])
	}
}

// Sur la route : sans taux saisi, la déclaration TDFN répond 422 et dit où le saisir ; avec le taux, le
// montant est celui du taux accordé sur la base TTC.
func TestLaRouteDeDeclarationTDFNExigeLeTaux(t *testing.T) {
	database, r, _ := cancelEnv(t)
	r.POST("/vat/declaration", NewFiscalYearHandler(database, false).GenerateVATDeclaration)

	if _, err := database.Exec(`INSERT INTO company_settings (id, company_name) VALUES ('s1', 'Test SA')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date, currency,
		                      subtotal_amount, vat_amount, total_amount, vat_rate, created_by_id)
		VALUES ('i1', 'FA-1', 'invoice', 'c1', 'sent', '2026-03-01', '2026-03-31', 'CHF', 10000, 810, 10810, 8.1, 'u1')`); err != nil {
		t.Fatal(err)
	}

	corps := `{"period_start":"2026-01-01","period_end":"2026-12-31","method":"tdfn"}`
	w := hitRaw(r, http.MethodPost, "/vat/declaration", corps)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "taux TDFN") ||
		!strings.Contains(w.Body.String(), "Paramètres → Banque") {
		t.Fatalf("sans taux : %d %s, attendu 422 qui dit où le saisir", w.Code, w.Body.String())
	}

	if _, err := database.Exec(`UPDATE company_settings SET tdfn_rate = 6`); err != nil {
		t.Fatal(err)
	}
	w = hitRaw(r, http.MethodPost, "/vat/declaration", corps)
	if w.Code != http.StatusOK {
		t.Fatalf("avec taux : %d %s", w.Code, w.Body.String())
	}
	res := asMap(t, w)
	if res["tdfn_base"] != 10810.0 || res["tdfn_rate"] != 6.0 {
		t.Errorf("base / taux = %v / %v, attendu 10810 / 6", res["tdfn_base"], res["tdfn_rate"])
	}
	if vc, _ := res["vat_collected"].(map[string]any); vc["total"] != 648.6 {
		t.Errorf("TVA due = %v, attendu 648.6", vc["total"])
	}
}

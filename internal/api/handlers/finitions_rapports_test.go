package handlers

// Audit croisé Gemini, finitions (P3) : rapport de chiffre d'affaires (MED-02).

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
)

func semerFacture(t *testing.T, database *sql.DB, id, statut, devise string, total, paye float64) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date, due_date,
		                      currency, subtotal_amount, vat_amount, total_amount, amount_paid, vat_rate,
		                      created_by_id)
		VALUES (?, ?, 'invoice', 'k1', ?, '2026-03-01', '2026-03-31', ?, ?, 0, ?, ?, 0, 'u1')`,
		id, "FA-"+id, statut, devise, total, total, paye); err != nil {
		t.Fatalf("facture %s : %v", id, err)
	}
}

func devisesDuRapport(t *testing.T, res map[string]any) []string {
	t.Helper()
	brut, ok := res["currencies"].([]any)
	if !ok {
		t.Fatalf("le rapport ne porte pas de liste de devises : %v", res)
	}
	out := make([]string, len(brut))
	for i, d := range brut {
		out[i], _ = d.(string)
	}
	return out
}

// MED-02. Les montants de devises différentes s'additionnent sans conversion : le total n'est alors
// pas un montant en francs. Le rapport nomme les devises qu'il additionne — celles des seules pièces
// COMPTÉES, pas celles d'un brouillon — et son texte dit que « Encaissé » suit la date d'émission.
func TestLeRapportDeCANommeLesDevisesAdditionneesEtDitCeQueEstEncaisse(t *testing.T) {
	database, r := journalEnv(t)
	r.GET("/reports/revenue", NewReportsHandler(database, false).Revenue)
	if _, err := database.Exec(
		`INSERT INTO contacts (id,contact_type,name,country,is_active) VALUES ('k1','customer','Client SA','CH',1)`); err != nil {
		t.Fatal(err)
	}

	semerFacture(t, database, "a", "sent", "CHF", 1000, 400)
	res := asMap(t, hit(r, http.MethodGet, "/reports/revenue?group_by=year", nil))
	if d := devisesDuRapport(t, res); len(d) != 1 || d[0] != "CHF" {
		t.Errorf("devises = %v, attendu [CHF]", d)
	}
	basis, _ := res["basis"].(string)
	if strings.Contains(basis, "additionnés sans conversion") {
		t.Errorf("avertissement de devises sur un rapport en une seule devise : %s", basis)
	}
	if !strings.Contains(basis, "date d'émission") || !strings.Contains(basis, "flux de trésorerie") {
		t.Errorf("le texte ne dit pas que « Encaissé » suit la date d'émission : %s", basis)
	}

	// Un brouillon en livres sterling n'entre pas dans le total : il ne doit pas en annoncer la devise.
	semerFacture(t, database, "b", "draft", "GBP", 500, 0)
	res = asMap(t, hit(r, http.MethodGet, "/reports/revenue?group_by=year", nil))
	if d := devisesDuRapport(t, res); len(d) != 1 {
		t.Errorf("devises = %v : un brouillon annonce sa devise", d)
	}

	// Une facture en euros émise : deux devises additionnées, le rapport le dit.
	semerFacture(t, database, "c", "sent", "EUR", 200, 0)
	res = asMap(t, hit(r, http.MethodGet, "/reports/revenue?group_by=year", nil))
	d := devisesDuRapport(t, res)
	if len(d) != 2 || d[0] != "CHF" || d[1] != "EUR" {
		t.Errorf("devises = %v, attendu [CHF EUR]", d)
	}
	basis, _ = res["basis"].(string)
	if !strings.Contains(basis, "CHF, EUR") || !strings.Contains(basis, "sans conversion") {
		t.Errorf("le texte n'avertit pas de l'addition de CHF et d'EUR : %s", basis)
	}
	if total, _ := res["total_invoiced"].(float64); total != 1200 {
		t.Errorf("total facturé = %v, attendu 1200 (le brouillon est exclu)", total)
	}
	if paye, _ := res["total_paid"].(float64); paye != 400 {
		t.Errorf("total encaissé = %v, attendu 400", paye)
	}
}

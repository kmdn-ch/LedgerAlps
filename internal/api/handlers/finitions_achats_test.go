package handlers

// Audit croisé Gemini, finitions (P3) : achats, audit et validation (MED-07).

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// envAchats monte les routes de saisie d'une facture fournisseur sur l'environnement des tests
// d'annulation : un fournisseur « c1 », un auteur authentifié « u1 ». Un client « c2 » s'ajoute.
func envAchats(t *testing.T) (*sql.DB, *gin.Engine) {
	t.Helper()
	database, r, h := cancelEnv(t)
	r.POST("/supplier-invoices", h.CreateSupplierInvoice)
	r.PUT("/supplier-invoices/:id", h.UpdateSupplierInvoice)
	r.GET("/supplier-invoices", h.ListSupplierInvoices)
	if _, err := database.Exec(
		`INSERT INTO contacts (id,contact_type,name,country,is_active)
		 VALUES ('c2','customer','Client SA','CH',1), ('c3','both','Mixte SA','CH',1)`); err != nil {
		t.Fatal(err)
	}
	return database, r
}

func appeler(t *testing.T, r *gin.Engine, methode, chemin string, corps any) (int, string) {
	t.Helper()
	var lecteur *bytes.Reader
	if corps != nil {
		raw, _ := json.Marshal(corps)
		lecteur = bytes.NewReader(raw)
	} else {
		lecteur = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(methode, chemin, lecteur)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// corpsAchat est une facture reçue valide ; `modif` en change ce qui sert au cas.
func corpsAchat(fournisseur, reference string, prix float64) map[string]any {
	return map[string]any{
		"supplier_id": fournisseur, "supplier_reference": reference, "issue_date": "2026-05-02",
		"currency": "CHF",
		"lines": []map[string]any{
			{"description": "Achat", "quantity": 1, "unit_price": prix, "vat_rate": 0.081},
		},
	}
}

// MED-07. Modifier un brouillon ne laissait aucune trace, alors que la création et les
// transitions en laissent : la référence, la devise, le montant et le fournisseur d'une pièce
// sont ce qu'un virement versera plus tard.
func TestModifierUnBrouillonLaisseUneTrace(t *testing.T) {
	database, r := envAchats(t)
	brouillonFournisseur(t, database, "f1", "F-1", 100, 8.1)

	code, corps := appeler(t, r, http.MethodPut, "/supplier-invoices/f1", corpsAchat("c1", "F-1-corrigee", 250))
	if code != http.StatusOK {
		t.Fatalf("modification : %d %s", code, corps)
	}

	var n int
	var avant, apres sql.NullString
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE record_id = 'f1' AND action = 'supplier_invoice_updated'`).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d trace(s) de modification, attendu 1", n)
	}
	if err := database.QueryRow(
		`SELECT before_state, after_state FROM audit_logs
		 WHERE record_id = 'f1' AND action = 'supplier_invoice_updated'`).Scan(&avant, &apres); err != nil {
		t.Fatal(err)
	}
	// L'avant et l'après portent la référence et le montant ; le nom du fournisseur, lui, n'y est pas.
	for nom, etat := range map[string]string{"avant": avant.String, "après": apres.String} {
		if !strings.Contains(etat, `"reference"`) || !strings.Contains(etat, `"total"`) {
			t.Errorf("état %s sans référence ni montant : %s", nom, etat)
		}
	}
	if !strings.Contains(avant.String, "F-1") || !strings.Contains(apres.String, "F-1-corrigee") {
		t.Errorf("la trace ne distingue pas l'ancienne et la nouvelle référence : %s → %s", avant.String, apres.String)
	}
}

// MED-07. La création refusait un contact qui n'est pas fournisseur ; la MODIFICATION ne
// contrôlait rien : un brouillon se rattachait après coup à un client, ou à un identifiant qui
// n'existe pas.
func TestModifierUnBrouillonRefuseUnContactQuiNEstPasFournisseur(t *testing.T) {
	database, r := envAchats(t)
	brouillonFournisseur(t, database, "f1", "F-1", 100, 8.1)

	for nom, cible := range map[string]string{"un client": "c2", "un identifiant inconnu": "fantome"} {
		code, corps := appeler(t, r, http.MethodPut, "/supplier-invoices/f1", corpsAchat(cible, "F-1", 100))
		if code != http.StatusUnprocessableEntity {
			t.Errorf("rattacher à %s : %d %s, attendu 422", nom, code, corps)
		}
	}
	var fournisseur string
	if err := database.QueryRow(`SELECT supplier_id FROM supplier_invoices WHERE id = 'f1'`).Scan(&fournisseur); err != nil {
		t.Fatal(err)
	}
	if fournisseur != "c1" {
		t.Errorf("fournisseur = %q après deux refus, attendu c1", fournisseur)
	}
	var n int
	_ = database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'supplier_invoice_updated'`).Scan(&n)
	if n != 0 {
		t.Errorf("%d trace(s) pour des modifications refusées", n)
	}

	// Un contact « les deux » est un fournisseur valable.
	if code, corps := appeler(t, r, http.MethodPut, "/supplier-invoices/f1", corpsAchat("c3", "F-1", 100)); code != http.StatusOK {
		t.Errorf("rattacher à un contact « les deux » : %d %s, attendu 200", code, corps)
	}
}

// Un brouillon dont le fournisseur est devenu client depuis la saisie reste modifiable (montant,
// date) tant qu'on ne change pas de fournisseur : le contrôle ne porte que sur un CHANGEMENT.
func TestUnBrouillonDontLeContactADeviClientResteModifiable(t *testing.T) {
	database, r := envAchats(t)
	brouillonFournisseur(t, database, "f1", "F-1", 100, 8.1)
	if _, err := database.Exec(`UPDATE contacts SET contact_type = 'customer' WHERE id = 'c1'`); err != nil {
		t.Fatal(err)
	}
	if code, corps := appeler(t, r, http.MethodPut, "/supplier-invoices/f1", corpsAchat("c1", "F-1", 120)); code != http.StatusOK {
		t.Errorf("modifier le montant sans changer de fournisseur : %d %s, attendu 200", code, corps)
	}
}

// La phrase du refus est en français (elle passe par le catalogue des langues), et dit quoi faire.
func TestLeRefusDeFournisseurEstEnFrancaisEtDitQuoiFaire(t *testing.T) {
	_, r := envAchats(t)
	code, corps := appeler(t, r, http.MethodPost, "/supplier-invoices", corpsAchat("c2", "X-1", 50))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("créer sur un client : %d %s, attendu 422", code, corps)
	}
	if strings.Contains(corps, "expected 'supplier'") || strings.Contains(corps, "contact is of type") {
		t.Errorf("le refus est resté en anglais : %s", corps)
	}
	if !strings.Contains(corps, "pas un fournisseur") || !strings.Contains(corps, "changez son type") {
		t.Errorf("le refus ne dit pas quoi faire : %s", corps)
	}
}

// MED-07. Une erreur de lecture de la liste ne renvoie plus le texte du pilote (noms de colonnes,
// types de la base) : il va au journal du serveur, la réponse porte une phrase fixe.
func TestUneErreurDeLectureNeRenvoiePasLeTexteDuPilote(t *testing.T) {
	database, r := envAchats(t)
	brouillonFournisseur(t, database, "f1", "F-1", 100, 8.1)
	// Une date de création illisible fait échouer le Scan vers time.Time.
	if _, err := database.Exec(`UPDATE supplier_invoices SET created_at = 'pas une date' WHERE id = 'f1'`); err != nil {
		t.Fatal(err)
	}
	code, corps := appeler(t, r, http.MethodGet, "/supplier-invoices", nil)
	if code != http.StatusInternalServerError {
		t.Fatalf("liste avec une ligne illisible : %d %s, attendu 500", code, corps)
	}
	for _, fuite := range []string{"Scan", "sql:", "column", "created_at", "time.Time", "converting"} {
		if strings.Contains(corps, fuite) {
			t.Errorf("la réponse laisse passer %q du pilote : %s", fuite, corps)
		}
	}
	if !strings.Contains(corps, "n'ont pas pu être lues") {
		t.Errorf("phrase fixe attendue, reçu : %s", corps)
	}
}

// MED-07. Le taux « dominant » de l'en-tête se choisissait en parcourant une carte : à bases égales,
// il changeait d'un appel à l'autre. À égalité, le plus haut l'emporte — toujours.
func TestLeTauxDominantNEstPasAleatoire(t *testing.T) {
	for i := 0; i < 200; i++ {
		lignes := []supplierInvoiceLine{
			{Quantity: 1, UnitPrice: 100, VATRate: 0.026},
			{Quantity: 1, UnitPrice: 100, VATRate: 0.081},
			{Quantity: 1, UnitPrice: 100, VATRate: 0.038},
		}
		if _, _, _, taux := computeTotals(lignes); taux != 0.081 {
			t.Fatalf("passage %d : taux dominant %.3f, attendu 0.081 (bases égales → le plus haut)", i, taux)
		}
	}
	// Une base plus grosse l'emporte, même à un taux plus bas.
	lignes := []supplierInvoiceLine{
		{Quantity: 1, UnitPrice: 300, VATRate: 0.026},
		{Quantity: 1, UnitPrice: 100, VATRate: 0.081},
	}
	if _, _, _, taux := computeTotals(lignes); taux != 0.026 {
		t.Errorf("taux dominant %.3f, attendu 0.026 (la plus grosse base)", taux)
	}
	// Aucune base : aucun taux.
	if _, _, _, taux := computeTotals([]supplierInvoiceLine{{Quantity: 1, UnitPrice: 0, VATRate: 0.081}}); taux != 0 {
		t.Errorf("pièce sans base : taux %.3f, attendu 0", taux)
	}
}

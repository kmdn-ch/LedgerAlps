package handlers

// Audit croisé Gemini, HIGH-06 (reliquat) : l'adresse postale du bénéficiaire et du débiteur, et le
// compte à débiter.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func exporterPain001(t *testing.T, r *gin.Engine, corps string) *httptest.ResponseRecorder {
	t.Helper()
	return hitRaw(r, http.MethodPost, "/payments/export", corps)
}

func routeurPain001(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/payments/export", (&ISO20022Handler{}).ExportPain001)
	return r
}

const lotHistorique = `{"execution_date":"2026-11-05","debtor_name":"Debiteur SA","debtor_iban":"%s",
"transactions":[{"end_to_end_id":"A","creditor_name":"Papeterie Alpes","creditor_iban":"` + ibanOrdinaire + `","amount":10,
"creditor_street":"Rue 2","creditor_post_code":"1000","creditor_town":"Lausanne","creditor_country":"CH"}]}`

func lot(debiteur string) string { return strings.Replace(lotHistorique, "%s", debiteur, 1) }

// Un QR-IBAN identifie un compte pour RECEVOIR des paiements à référence QR : il ne sert pas de compte à
// débiter. Mis dans le champ IBAN de l'entreprise, il donnait un ordre que la banque refuse APRÈS son
// dépôt.
func TestUnQRIBANNeSertPasDeCompteADebiter(t *testing.T) {
	r := routeurPain001(t)

	w := exporterPain001(t, r, lot(qrIBAN))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("débiteur en QR-IBAN : %d %s, attendu 422", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "QR-IBAN") || !strings.Contains(w.Body.String(), "IBAN ordinaire") {
		t.Errorf("le refus ne dit pas quoi faire : %s", w.Body.String())
	}

	// Un IBAN ordinaire passe, et l'adresse du bénéficiaire part dans le fichier.
	w = exporterPain001(t, r, lot("CH9300762011623852957"))
	if w.Code != http.StatusOK {
		t.Fatalf("débiteur ordinaire : %d %s, attendu 200", w.Code, w.Body.String())
	}
	xml := w.Body.String()
	if !strings.Contains(xml, "<TwnNm>Lausanne</TwnNm>") || !strings.Contains(xml, "<Ctry>CH</Ctry>") ||
		!strings.Contains(xml, "<StrtNm>Rue 2</StrtNm>") {
		t.Errorf("l'adresse du bénéficiaire n'est pas dans le fichier :\n%s", xml)
	}
}

// La forme de l'interface ne transmet que des identifiants de factures : le serveur relit lui-même le
// bénéficiaire, et son adresse, sur la fiche du fournisseur ; le débiteur, sur la fiche de l'entreprise.
func TestLOrdreDePaiementPorteLesAdressesDesFiches(t *testing.T) {
	database, r, _ := cancelEnv(t)
	run := NewPaymentRunHandler(database, false)
	r.POST("/payments/export", (&ISO20022Handler{}).WithAudit(database, false).WithPaymentRun(run).ExportPain001)

	if _, err := database.Exec(`UPDATE contacts SET address = 'Rue des Alpes 12', postal_code = '1700',
		city = 'Fribourg', country = 'CH', iban = ?, name = 'Fournisseur SA' WHERE id = 'c1'`, ibanOrdinaire); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO company_settings (company_name, iban, address_street, address_postal_code,
		address_city, address_country) VALUES ('Gemini SA', 'CH9300762011623852957', 'Rue 1', '1000', 'Lausanne', 'CH')`); err != nil {
		t.Fatal(err)
	}
	creerFacture(t, database, r, "f1", "F-1", 1000, 81)

	w := exporterPain001(t, r, `{"execution_date":"2026-11-05","supplier_invoice_ids":["f1"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("export : %d %s", w.Code, w.Body.String())
	}
	xml := w.Body.String()
	for _, attendu := range []string{
		"<Nm>Gemini SA</Nm>", "<TwnNm>Lausanne</TwnNm>", // le débiteur : l'entreprise
		"<Nm>Fournisseur SA</Nm>", "<StrtNm>Rue des Alpes 12</StrtNm>", "<PstCd>1700</PstCd>", "<TwnNm>Fribourg</TwnNm>", // le bénéficiaire
	} {
		if !strings.Contains(xml, attendu) {
			t.Errorf("fichier sans %s :\n%s", attendu, xml)
		}
	}
	if strings.Count(xml, "<PstlAdr>") != 2 {
		t.Errorf("%d adresse(s) émise(s), attendu 2 (débiteur et bénéficiaire) :\n%s", strings.Count(xml, "<PstlAdr>"), xml)
	}
}

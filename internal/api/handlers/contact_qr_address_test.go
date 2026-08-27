package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// Un client (ou un contact "both") devient le débiteur d'une facture QR : sans
// adresse structurée complète, le bulletin de versement suisse ne peut pas
// s'imprimer (SPC 0200 §4.2.2). Ces tests reproduisent le refus, et vérifient
// qu'un fournisseur pur — jamais débiteur d'une facture émise par LedgerAlps —
// n'est pas concerné.

func postContact(t *testing.T, h *ContactsHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/contacts", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.CreateContact(c)
	})

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/contacts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestCreateContactRefuseUnClientSansAdresseComplete(t *testing.T) {
	h, _ := newContactsDB(t)

	cases := []map[string]any{
		{"contact_type": "customer", "name": "Client Sans Rue", "postal_code": "1000", "city": "Lausanne"},
		{"contact_type": "customer", "name": "Client Sans NPA", "address": "Rue du Lac 1", "city": "Lausanne"},
		{"contact_type": "customer", "name": "Client Sans Ville", "address": "Rue du Lac 1", "postal_code": "1000"},
		{"contact_type": "both", "name": "Mixte Sans Rue", "postal_code": "1000", "city": "Lausanne"},
	}
	for _, body := range cases {
		w := postContact(t, h, body)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%v : status %d, attendu 422 — %s", body["name"], w.Code, w.Body.String())
		}
	}
}

func TestCreateContactAccepteUnClientAvecAdresseComplete(t *testing.T) {
	h, _ := newContactsDB(t)

	w := postContact(t, h, map[string]any{
		"contact_type": "customer", "name": "Client Complet",
		"address": "Rue du Lac 1", "postal_code": "1000", "city": "Lausanne",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, attendu 201 — %s", w.Code, w.Body.String())
	}
}

// Un fournisseur pur n'est jamais débiteur d'une facture émise par
// LedgerAlps — sa fiche reste allégée, comme le fait déjà la saisie rapide de
// l'écran des achats (souvent seuls le nom et l'IBAN sont connus à ce stade).
func TestCreateContactNExigePasDAdressePourUnFournisseurPur(t *testing.T) {
	h, _ := newContactsDB(t)

	w := postContact(t, h, map[string]any{
		"contact_type": "supplier", "name": "Fournisseur Minimal",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, attendu 201 — %s", w.Code, w.Body.String())
	}
}

// ─── La règle doit tenir aussi à la MODIFICATION (audit 4, C-2) ──────────────
//
// `requireQRAddress` n'avait qu'un seul site d'appel : `CreateContact`. La
// porte d'entrée était fermée, la fenêtre restait ouverte — un simple PATCH
// vidait l'adresse d'un client déjà complet, et rien ne l'empêchait. Le PDF
// se générait alors sans destinataire : `validateQRBillData` traite un
// débiteur sans nom comme « non identifié » (cas légitime du SPC 0200 pour une
// facture générique) et saute tous ses contrôles d'adresse.

func patchContact(t *testing.T, h *ContactsHandler, id string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PATCH("/contacts/:id", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.UpdateContact(c)
	})

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/contacts/"+id, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// createdContactID crée un client complet et rend son identifiant.
func createdContactID(t *testing.T, h *ContactsHandler) string {
	t.Helper()
	w := postContact(t, h, map[string]any{
		"contact_type": "customer", "name": "Client Complet",
		"address": "Rue du Lac 1", "postal_code": "1000", "city": "Lausanne",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("création: status %d — %s", w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("réponse illisible: %v", err)
	}
	return out.ID
}

func TestUpdateContactRefuseDeViderLAdresseDUnClient(t *testing.T) {
	h, _ := newContactsDB(t)
	id := createdContactID(t, h)

	cases := []map[string]any{
		{"address": ""},
		{"postal_code": ""},
		{"city": ""},
		{"address": "", "postal_code": "", "city": ""},
	}
	for _, body := range cases {
		w := patchContact(t, h, id, body)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %v : status %d, attendu 422 — %s", body, w.Code, w.Body.String())
		}
	}
}

// Le nom aussi : `createContactRequest.Name` porte `binding:"required"`,
// `updateContactRequest.Name` n'avait aucune contrainte. Un nom vide produit
// une facture QR sans destinataire nulle part sur le document.
func TestUpdateContactRefuseDeViderLeNom(t *testing.T) {
	h, _ := newContactsDB(t)
	id := createdContactID(t, h)

	w := patchContact(t, h, id, map[string]any{"name": "   "})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status %d, attendu 422 — %s", w.Code, w.Body.String())
	}
}

func TestUpdateContactAccepteUneAdresseComplete(t *testing.T) {
	h, _ := newContactsDB(t)
	id := createdContactID(t, h)

	w := patchContact(t, h, id, map[string]any{
		"address": "Avenue de la Gare 5", "postal_code": "1003", "city": "Lausanne",
	})
	if w.Code != http.StatusOK {
		t.Errorf("status %d, attendu 200 — %s", w.Code, w.Body.String())
	}
}

// Un fournisseur pur n'est jamais débiteur d'une facture émise par
// LedgerAlps : la règle ne s'applique pas à lui, à la modification comme à la
// création.
func TestUpdateContactNExigePasDAdressePourUnFournisseur(t *testing.T) {
	h, _ := newContactsDB(t)
	w := postContact(t, h, map[string]any{"contact_type": "supplier", "name": "Fournisseur SA"})
	if w.Code != http.StatusCreated {
		t.Fatalf("création: %d — %s", w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if w := patchContact(t, h, out.ID, map[string]any{"address": ""}); w.Code != http.StatusOK {
		t.Errorf("status %d, attendu 200 — un fournisseur n'a pas besoin d'adresse QR: %s",
			w.Code, w.Body.String())
	}
}

// LE cas qui interdit une règle plus stricte : un client créé AVANT la v1.5.9,
// donc sans adresse, doit rester modifiable. Refuser un changement de
// téléphone parce qu'une adresse manque depuis des mois enfermerait la fiche
// sans rien corriger — on refuse de DÉGRADER, on n'exige pas de réparer.
func TestUpdateContactLaisseModifierUnClientHistoriqueIncomplet(t *testing.T) {
	h, database := newContactsDB(t)
	if _, err := database.Exec(`
		INSERT INTO contacts (id, contact_type, name, country, payment_term_days, is_active)
		VALUES ('legacy1', 'customer', 'Client Historique', 'CH', 30, 1)`); err != nil {
		t.Fatal(err)
	}

	w := patchContact(t, h, "legacy1", map[string]any{"phone": "+41 21 000 00 00"})
	if w.Code != http.StatusOK {
		t.Errorf("status %d, attendu 200 — une fiche héritée doit rester modifiable "+
			"sur les champs qui ne touchent ni l'identité ni l'adresse: %s", w.Code, w.Body.String())
	}
}

// Le type était jeté silencieusement par Gin (absent de
// `updateContactRequest`) alors que l'écran d'édition offre un menu pour le
// changer. Le rétablir doit se voir en base.
func TestUpdateContactChangeReellementLeType(t *testing.T) {
	h, database := newContactsDB(t)
	w := postContact(t, h, map[string]any{"contact_type": "supplier", "name": "Fournisseur SA"})
	if w.Code != http.StatusCreated {
		t.Fatalf("création: %d — %s", w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	// Fournisseur → « les deux » : le contact devient débiteur possible, donc
	// l'adresse devient exigible au moment de la bascule.
	if w := patchContact(t, h, out.ID, map[string]any{"contact_type": "both"}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("bascule sans adresse : status %d, attendu 422 — %s", w.Code, w.Body.String())
	}

	// Avec l'adresse, la bascule passe ET s'écrit vraiment.
	w = patchContact(t, h, out.ID, map[string]any{
		"contact_type": "both",
		"address":      "Rue du Lac 1", "postal_code": "1000", "city": "Lausanne",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bascule avec adresse : status %d, attendu 200 — %s", w.Code, w.Body.String())
	}
	var stocke string
	if err := database.QueryRow(`SELECT contact_type FROM contacts WHERE id = ?`, out.ID).Scan(&stocke); err != nil {
		t.Fatal(err)
	}
	if stocke != "both" {
		t.Errorf("contact_type en base = %q, attendu \"both\" — le champ était ignoré", stocke)
	}
}

// La porte dérobée que le rétablissement du type doit fermer : passer un
// client en fournisseur, vider l'adresse, puis revenir en client. Aucune des
// trois modifications ne devait pouvoir aboutir à un client sans adresse.
func TestUpdateContactFermeLaPorteDeroboeeParLeType(t *testing.T) {
	h, _ := newContactsDB(t)
	id := createdContactID(t, h)

	if w := patchContact(t, h, id, map[string]any{"contact_type": "supplier"}); w.Code != http.StatusOK {
		t.Fatalf("client → fournisseur : %d — %s", w.Code, w.Body.String())
	}
	if w := patchContact(t, h, id, map[string]any{"address": "", "postal_code": "", "city": ""}); w.Code != http.StatusOK {
		t.Fatalf("vidage sur un fournisseur : %d — %s", w.Code, w.Body.String())
	}
	// Le retour en client doit être refusé : la fiche n'a plus d'adresse.
	if w := patchContact(t, h, id, map[string]any{"contact_type": "customer"}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("retour en client sans adresse : status %d, attendu 422 — %s", w.Code, w.Body.String())
	}
}

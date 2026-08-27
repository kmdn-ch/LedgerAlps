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

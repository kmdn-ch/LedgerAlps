package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// Le filtre par type était envoyé par l'interface et ignoré par le serveur :
// cliquer « Clients » ou « Fournisseurs » ne changeait rien à la liste. Un
// filtre qui ne filtre pas est pire qu'un filtre absent — on croit avoir
// restreint la vue, et on lit la mauvaise.

func seedTyped(t *testing.T, database *sql.DB, id, name, kind string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO contacts (id, contact_type, name, country, payment_term_days, is_active)
		VALUES (?, ?, ?, 'CH', 30, 1)`, id, kind, name); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
}

func listContacts(t *testing.T, h *ContactsHandler, query string) []models.Contact {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/contacts", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.ListContacts(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts"+query, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	// La route rend un tableau nu, pas un objet paginé. Le sélecteur
	// d'anonymisation lisait `data.items` et n'affichait donc jamais personne.
	var out []models.Contact
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("la réponse n'est pas un tableau JSON : %v — %s", err, w.Body.String())
	}
	return out
}

func namesOf(cs []models.Contact) map[string]bool {
	out := map[string]bool{}
	for _, c := range cs {
		out[c.Name] = true
	}
	return out
}

func TestListContactsFiltersByType(t *testing.T) {
	h, database := newContactsDB(t)
	seedTyped(t, database, "c1", "Client", "customer")
	seedTyped(t, database, "c2", "Fournisseur", "supplier")
	seedTyped(t, database, "c3", "Mixte", "both")

	if got := namesOf(listContacts(t, h, "")); len(got) != 3 {
		t.Fatalf("sans filtre = %v, attendu les trois", got)
	}

	customers := namesOf(listContacts(t, h, "?contact_type=customer"))
	if !customers["Client"] || customers["Fournisseur"] {
		t.Errorf("filtre client = %v", customers)
	}
	suppliers := namesOf(listContacts(t, h, "?contact_type=supplier"))
	if !suppliers["Fournisseur"] || suppliers["Client"] {
		t.Errorf("filtre fournisseur = %v", suppliers)
	}

	// Un contact « both » est client ET fournisseur : l'exclure d'un des deux
	// filtres le ferait disparaître de la vue où on le cherche.
	if !customers["Mixte"] {
		t.Error("un contact « both » n'apparaît pas dans le filtre client")
	}
	if !suppliers["Mixte"] {
		t.Error("un contact « both » n'apparaît pas dans le filtre fournisseur")
	}
}

// Une valeur de filtre inconnue ne doit pas vider la liste en silence : mieux
// vaut tout montrer que faire croire à une base vide.
func TestListContactsIgnoresAnUnknownTypeFilter(t *testing.T) {
	h, database := newContactsDB(t)
	seedTyped(t, database, "c1", "Client", "customer")

	if got := namesOf(listContacts(t, h, "?contact_type=n_importe_quoi")); len(got) != 1 {
		t.Fatalf("filtre inconnu = %v, attendu la liste complète", got)
	}
}

// Un contact anonymisé est désactivé : il ne doit plus encombrer les listes de
// sélection, ni être proposé une seconde fois à l'anonymisation.
func TestListContactsHidesAnonymisedContacts(t *testing.T) {
	h, database := newContactsDB(t)
	seedTyped(t, database, "c1", "Client", "customer")
	seedContact(t, database, "c2", "À anonymiser")

	if code, _ := runAnonymise(t, h, "c2", true); code != http.StatusOK {
		t.Fatalf("anonymisation: %d", code)
	}
	got := namesOf(listContacts(t, h, ""))
	if got["À anonymiser"] {
		t.Error("un contact anonymisé reste proposé dans les listes")
	}
	if !got["Client"] {
		t.Error("les autres contacts ont disparu")
	}
}

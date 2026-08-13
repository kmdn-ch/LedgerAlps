package handlers

// Ce que ce test protège, et pourquoi il vaut la peine.
//
// La liste de mise en route est la première chose qu'un débutant voit. Une
// étape qui reste cochée alors que le champ est vide envoie quelqu'un facturer
// avec un bulletin que sa banque refusera — et l'écran lui aura dit que tout
// allait bien. C'est le sens de l'erreur qui compte ici : se tromper en
// affichant une étape déjà faite est agaçant, se tromper en cochant une étape
// qui ne l'est pas est trompeur.
//
// Le compteur est vérifié séparément parce qu'il se déduit des étapes : une
// étape ajoutée demain doit le faire bouger toute seule.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

func nouvelleMiseEnRoute(t *testing.T) (*OnboardingHandler, func(q string, args ...any)) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-mr-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	cfg := &config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := database.Exec(q, args...); err != nil {
			t.Fatalf("amorçage (%s): %v", q, err)
		}
	}
	return NewOnboardingHandler(database, false), exec
}

func lireMiseEnRoute(t *testing.T, h *OnboardingHandler) MiseEnRoute {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/onboarding", h.GetOnboarding)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/onboarding", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("statut = %d : %s", w.Code, w.Body.String())
	}
	var m MiseEnRoute
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("JSON invalide : %v", err)
	}
	return m
}

func étape(t *testing.T, m MiseEnRoute, clé string) ÉtapeMiseEnRoute {
	t.Helper()
	for _, e := range m.Étapes {
		if e.Clé == clé {
			return e
		}
	}
	t.Fatalf("étape %q absente de la liste", clé)
	return ÉtapeMiseEnRoute{}
}

// Une base vierge : rien n'est fait, et rien ne plante faute de fiche société.
func TestMiseEnRouteSurBaseVierge(t *testing.T) {
	h, _ := nouvelleMiseEnRoute(t)
	m := lireMiseEnRoute(t, h)

	if m.Faites != 0 {
		t.Errorf("faites = %d, attendu 0", m.Faites)
	}
	if m.Terminé {
		t.Error("terminé = true sur une base vierge")
	}
	if m.Total != len(m.Étapes) || m.Total == 0 {
		t.Errorf("total = %d pour %d étapes", m.Total, len(m.Étapes))
	}
	for _, e := range m.Étapes {
		if e.Fait {
			t.Errorf("étape %q cochée alors que rien n'est saisi", e.Clé)
		}
	}
}

// L'IBAN présent mais faux ne coche pas. C'est le cas qui compte : un IBAN
// invalide produit un bulletin d'apparence normale que la banque refuse, et
// une étape cochée dirait le contraire.
func TestMiseEnRouteRefuseUnIBANInvalide(t *testing.T) {
	h, exec := nouvelleMiseEnRoute(t)
	exec(`INSERT INTO company_settings (company_name, address_postal_code,
	        address_city, address_country, che_number, iban)
	      VALUES ('Test SA', '1000', 'Lausanne', 'CH', 'CHE-123.456.789',
	              'CH00 0000 0000 0000 0000 0')`)

	m := lireMiseEnRoute(t, h)

	e := étape(t, m, ÉtapeIBAN)
	if e.Fait {
		t.Error("l'étape IBAN est cochée alors que la clé de contrôle est fausse")
	}
	if len(e.Manquants) != 1 || e.Manquants[0] != "iban_invalid" {
		t.Errorf("manquants = %v, attendu [iban_invalid]", e.Manquants)
	}
	// Les autres étapes de la fiche, elles, passent.
	if !étape(t, m, ÉtapeIdentité).Fait {
		t.Error("l'identité est complète et devrait être cochée")
	}
	if !étape(t, m, ÉtapeIDE).Fait {
		t.Error("l'IDE est au bon format et devrait être coché")
	}
}

// L'adresse incomplète nomme les champs qui manquent, pas « quelque chose ».
func TestMiseEnRouteNommeLesChampsManquants(t *testing.T) {
	h, exec := nouvelleMiseEnRoute(t)
	exec(`INSERT INTO company_settings (company_name, address_postal_code,
	        address_city, address_country) VALUES ('Test SA', '', '', 'CH')`)

	e := étape(t, lireMiseEnRoute(t, h), ÉtapeIdentité)
	if e.Fait {
		t.Fatal("étape cochée alors que NPA et localité manquent")
	}
	attendus := map[string]bool{"postal_code": true, "city": true}
	if len(e.Manquants) != 2 {
		t.Fatalf("manquants = %v, attendu deux champs", e.Manquants)
	}
	for _, c := range e.Manquants {
		if !attendus[c] {
			t.Errorf("champ inattendu %q dans %v", c, e.Manquants)
		}
	}
}

// Un IDE mal formé se distingue d'un IDE absent : ce ne sont pas les mêmes
// gestes — l'un se saisit, l'autre se corrige.
func TestMiseEnRouteDistingueIDEAbsentEtIDEMalForme(t *testing.T) {
	h, exec := nouvelleMiseEnRoute(t)
	exec(`INSERT INTO company_settings (che_number) VALUES ('123456789')`)

	e := étape(t, lireMiseEnRoute(t, h), ÉtapeIDE)
	if len(e.Manquants) != 1 || e.Manquants[0] != "uid_invalid" {
		t.Errorf("manquants = %v, attendu [uid_invalid]", e.Manquants)
	}
}

// Tout réglé : la liste se déclare terminée, et c'est ce qui la fait
// disparaître de l'écran.
func TestMiseEnRouteSeTermine(t *testing.T) {
	h, exec := nouvelleMiseEnRoute(t)
	exec(`INSERT INTO company_settings (company_name, address_postal_code,
	        address_city, address_country, che_number, iban)
	      VALUES ('Test SA', '1000', 'Lausanne', 'CH', 'CHE-123.456.789',
	              'CH9300762011623852957')`)
	exec(`INSERT INTO users (id, email, name, password_hash)
	      VALUES ('u1', 'a@t.ch', 'A', 'x')`)
	exec(`INSERT INTO contacts (id, name, contact_type) VALUES ('c1', 'Client', 'customer')`)
	exec(`INSERT INTO invoices (id, invoice_number, contact_id, issue_date, due_date, status,
	        created_by_id)
	      VALUES ('i1', 'FA-2026-0001', 'c1', '2026-01-01', '2026-01-31', 'draft',
	              'u1')`)

	m := lireMiseEnRoute(t, h)
	for _, e := range m.Étapes {
		if !e.Fait {
			t.Errorf("étape %q non cochée : manquants = %v", e.Clé, e.Manquants)
		}
	}
	if !m.Terminé {
		t.Errorf("terminé = false alors que %d/%d étapes sont faites", m.Faites, m.Total)
	}
	if m.Faites != m.Total {
		t.Errorf("faites = %d, total = %d", m.Faites, m.Total)
	}
}

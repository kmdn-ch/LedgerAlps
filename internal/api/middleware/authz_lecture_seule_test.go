package middleware

// Ce qu'un compte en lecture seule ne doit atteindre par AUCUN chemin.
//
// L'écran cache déjà ces commandes, mais cacher n'est pas interdire : le code
// d'une page web est lisible, modifiable, et une requête se forge sans elle.
// Ces tests portent donc sur le seul endroit qui décide — le serveur — et
// nomment un par un les gestes qui feraient entrer quelque chose dans les
// livres ou changeraient ce que porte une facture émise.
//
// Le filtre global les couvre tous par construction, puisqu'il refuse toute
// méthode autre que GET, HEAD et OPTIONS. Les nommer quand même a un intérêt
// précis : si quelqu'un ajoute demain une exemption à ce filtre — pour un cas
// qui semblait inoffensif — ces tests disent lesquels ne peuvent pas en être.

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
)

// routeurDesDepots monte les routes réellement visées par la demande : dépôt
// d'une facture fournisseur, import d'un relevé bancaire, logo de société,
// écriture au journal, facture, contact, ordre de paiement.
//
// Aucune n'a de garde déclarée : c'est délibéré. On vérifie que le filtre
// global suffit, précisément parce qu'une garde par route peut être oubliée.
func routeurDesDepots(a *Authorizer) (*gin.Engine, map[string]*bool) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(RequireAuth(secret))
	api.Use(a.DenyWritesWithoutPermission())

	atteintes := map[string]*bool{}
	monter := func(methode, chemin string) {
		atteint := false
		atteintes[methode+" "+chemin] = &atteint
		api.Handle(methode, chemin, func(c *gin.Context) {
			atteint = true
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	}

	monter(http.MethodPost, "/supplier-invoices/read-qr") // dépôt d'un PDF fournisseur
	monter(http.MethodPost, "/supplier-invoices")         // saisie d'une facture reçue
	monter(http.MethodPost, "/bank-statements/import")    // camt.053
	monter(http.MethodPost, "/payments/export")           // pain.001
	monter(http.MethodPost, "/settings/logo")             // logo de société
	monter(http.MethodDelete, "/settings/logo")           //
	monter(http.MethodPut, "/settings/company")           // coordonnées, IBAN, n° TVA
	monter(http.MethodPost, "/journal-entries")           // écriture comptable
	monter(http.MethodPost, "/invoices")                  // facture émise
	monter(http.MethodPatch, "/invoices/:id")             //
	monter(http.MethodPost, "/contacts")                  // client ou fournisseur
	monter(http.MethodPut, "/contacts/:id")               //
	monter(http.MethodPost, "/bank-entries/:id/match")    // rapprochement
	return r, atteintes
}

// LE test de cette demande : rien de ce qui écrit n'est atteint, et le
// gestionnaire n'est même pas exécuté — le refus tombe avant.
func TestUnLecteurNAtteintAucuneRouteDEcriture(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "lecteur", authz.RoleViewer, true)
	r, atteintes := routeurDesDepots(a)

	for route, atteint := range atteintes {
		methode, chemin := decouper(route)
		w := call(r, methode, chemin, tokenFor(t, "lecteur"))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s : code %d, attendu 403", route, w.Code)
		}
		if *atteint {
			t.Errorf("%s : le gestionnaire a tourné — le refus est arrivé trop tard", route)
		}
	}
}

// Le pendant : un comptable les atteint toutes. Sans ce test, refuser tout le
// monde ferait passer le précédent.
func TestUnComptableAtteintCesMemesRoutes(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "comptable", authz.RoleAccountant, true)
	r, atteintes := routeurDesDepots(a)

	for route, atteint := range atteintes {
		methode, chemin := decouper(route)
		if w := call(r, methode, chemin, tokenFor(t, "comptable")); w.Code != http.StatusOK {
			t.Errorf("%s : code %d, attendu 200 pour un comptable", route, w.Code)
		}
		if !*atteint {
			t.Errorf("%s : le gestionnaire n'a pas tourné pour un comptable", route)
		}
	}
}

// Et ce qui reste ouvert : consulter et EXPORTER.
//
// Une fiduciaire à qui l'on ouvre les livres vient chercher exactement cela.
// L'archive légale des dix ans (CO art. 958f) et les exports comptables sont
// des GET : leur fermer la porte viderait le rôle de son sens.
func TestUnLecteurExporteEtTelechargeLArchive(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "lecteur", authz.RoleViewer, true)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(RequireAuth(secret))
	api.Use(a.DenyWritesWithoutPermission())
	for _, chemin := range []string{
		"/exports/legal-archive",
		"/exports/journal.csv",
		"/exports/ledger.csv",
		"/exports/trial-balance.csv",
		"/invoices",
		"/journal-entries",
	} {
		api.GET(chemin, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	}

	for _, chemin := range []string{
		"/api/v1/exports/legal-archive",
		"/api/v1/exports/journal.csv",
		"/api/v1/exports/ledger.csv",
		"/api/v1/exports/trial-balance.csv",
		"/api/v1/invoices",
		"/api/v1/journal-entries",
	} {
		if w := call(r, http.MethodGet, chemin, tokenFor(t, "lecteur")); w.Code != http.StatusOK {
			t.Errorf("%s : code %d — un lecteur doit pouvoir consulter et exporter", chemin, w.Code)
		}
	}
}

func decouper(route string) (methode, chemin string) {
	for i := 0; i < len(route); i++ {
		if route[i] == ' ' {
			return route[:i], "/api/v1" + route[i+1:]
		}
	}
	return route, ""
}

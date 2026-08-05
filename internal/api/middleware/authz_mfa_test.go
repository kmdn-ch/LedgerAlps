package middleware

// L'obligation de second facteur pour les comptes administrateurs.
//
// Ce qui est vérifié ici : que le blocage soit TECHNIQUE. Cacher les écrans ne
// ferme aucune porte — l'adresse reste tapable, l'appel réseau reste faisable.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
)

func mfaRouter(a *Authorizer) (*gin.Engine, *bool) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ran := false
	api := r.Group("/api/v1")
	api.Use(RequireAuth(secret))
	api.Use(a.RequireMFAEnrolled())
	api.GET("/quelconque", func(c *gin.Context) {
		ran = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r, &ran
}

func confirmMFA(t *testing.T, a *Authorizer, userID string) {
	t.Helper()
	if _, err := a.db.Exec(
		`INSERT INTO user_mfa (user_id, secret, confirmed_at) VALUES (?, 'ABCDEFGH', ?)`,
		userID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

// Un administrateur sans second facteur ne peut RIEN faire, pas même lire.
func TestUnAdministrateurSansSecondFacteurEstBloque(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "admin1", authz.RoleAdmin, true)

	r, ran := mfaRouter(a)
	w := call(r, http.MethodGet, "/api/v1/quelconque", tokenFor(t, "admin1"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("statut = %d, attendu 403", w.Code)
	}
	if *ran {
		t.Fatal("le handler a tourné : le blocage est cosmétique, pas technique")
	}
	// L'interface doit savoir où conduire ; le serveur, lui, refuse de toute façon.
	if !strings.Contains(w.Body.String(), "mfa_enrolment_required") {
		t.Fatalf("la réponse ne dit pas quoi faire: %s", w.Body.String())
	}
}

func TestUnAdministrateurInscritPasse(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "admin1", authz.RoleAdmin, true)
	confirmMFA(t, a, "admin1")

	r, ran := mfaRouter(a)
	if w := call(r, http.MethodGet, "/api/v1/quelconque", tokenFor(t, "admin1")); w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	if !*ran {
		t.Fatal("le handler n'a pas tourné")
	}
}

// Une inscription COMMENCÉE mais jamais confirmée ne compte pas : quelqu'un qui
// ferme l'assistant en cours de route doit pouvoir le reprendre, pas se
// retrouver à devoir fournir un code qu'aucun téléphone ne calcule.
func TestUneInscriptionNonConfirmeeNeCompteToujoursPas(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "admin1", authz.RoleAdmin, true)
	if _, err := database.Exec(
		`INSERT INTO user_mfa (user_id, secret, confirmed_at) VALUES ('admin1','ABCDEFGH',NULL)`); err != nil {
		t.Fatal(err)
	}

	r, _ := mfaRouter(a)
	if w := call(r, http.MethodGet, "/api/v1/quelconque", tokenFor(t, "admin1")); w.Code != http.StatusForbidden {
		t.Fatalf("statut = %d — une inscription abandonnée a été comptée comme faite", w.Code)
	}
}

// Les autres rôles ne sont pas concernés : un comptable écrit dans un journal
// chaîné et tracé, et n'a pas les clés de l'installation. Lui imposer un
// téléphone coûterait plus qu'il ne protège.
func TestLesAutresRolesNeSontPasConcernes(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "compta", authz.RoleAccountant, true)
	addUser(t, database, "lecteur", authz.RoleViewer, true)

	r, _ := mfaRouter(a)
	for _, id := range []string{"compta", "lecteur"} {
		if w := call(r, http.MethodGet, "/api/v1/quelconque", tokenFor(t, id)); w.Code != http.StatusOK {
			t.Fatalf("%s: statut = %d", id, w.Code)
		}
	}
}

// La promotion en administrateur s'applique TOUT DE SUITE, y compris pour
// l'obligation de second facteur : le rôle est relu à chaque requête, jamais
// pris dans le jeton.
func TestUnePromotionImposeLeSecondFacteurImmediatement(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "compta", authz.RoleAccountant, true)

	r, _ := mfaRouter(a)
	tok := tokenFor(t, "compta")
	if w := call(r, http.MethodGet, "/api/v1/quelconque", tok); w.Code != http.StatusOK {
		t.Fatalf("avant promotion: statut %d", w.Code)
	}
	if _, err := database.Exec(`UPDATE users SET role='admin', is_admin=1 WHERE id='compta'`); err != nil {
		t.Fatal(err)
	}
	if w := call(r, http.MethodGet, "/api/v1/quelconque", tok); w.Code != http.StatusForbidden {
		t.Fatalf("après promotion: statut %d — l'obligation attend l'expiration du jeton", w.Code)
	}
}

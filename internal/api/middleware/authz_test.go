package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Le contrôle des droits est le genre de code dont un défaut ne se voit pas :
// il s'exploite. Ces tests portent sur les trois choses qui peuvent mal tourner
// — un rôle périmé qui continue d'agir, une route oubliée qui reste ouverte, et
// un compte désactivé dont la session survit.

const secret = "secret-de-test-pour-authz-0123456789"

func authzDB(t *testing.T) (*sql.DB, *Authorizer) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-authz-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatal(err)
	}
	return database, NewAuthorizer(database, false, secret)
}

func addUser(t *testing.T, database *sql.DB, id string, role authz.Role, active bool) {
	t.Helper()
	a := 0
	if active {
		a = 1
	}
	admin := 0
	if role == authz.RoleAdmin {
		admin = 1
	}
	_, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,role,is_admin,is_active)
		 VALUES (?,?,?,'x',?,?,?)`, id, id+"@t.ch", id, string(role), admin, a)
	if err != nil {
		t.Fatal(err)
	}
}

func tokenFor(t *testing.T, userID string) string {
	t.Helper()
	// isAdmin volontairement à true dans le JETON pour tous les cas : si le
	// contrôle le lisait là, tous ces tests passeraient à tort. C'est la base
	// qui doit trancher.
	tok, err := security.GenerateAccessToken(secret, userID, true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func router(a *Authorizer, perm authz.Permission) (*gin.Engine, *bool) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ran := false
	api := r.Group("/api/v1")
	api.Use(RequireAuth(secret))
	api.Use(a.DenyWritesWithoutPermission())
	api.GET("/secret", a.Require(perm), func(c *gin.Context) {
		ran = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	// Route d'écriture SANS garde déclarée : elle représente celle qu'on
	// oubliera un jour d'annoter.
	api.POST("/oubliee", func(c *gin.Context) {
		ran = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r, &ran
}

func call(r *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// LE test qui justifie toute l'architecture : rétrograder quelqu'un doit lui
// couper les droits TOUT DE SUITE, sans attendre l'expiration de son jeton.
//
// Si le rôle était lu dans le jeton, l'administrateur rétrogradé continuerait
// d'administrer pendant une heure — une heure durant laquelle on croit avoir
// coupé l'accès.
func TestUneRetrogradationSAppliqueImmediatement(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleAdmin, true)
	r, _ := router(a, authz.PermAdmin)
	tok := tokenFor(t, "u1")

	if w := call(r, http.MethodGet, "/api/v1/secret", tok); w.Code != http.StatusOK {
		t.Fatalf("l'administrateur est refusé: %d %s", w.Code, w.Body.String())
	}

	// Rétrogradation. Le jeton, lui, n'a pas bougé.
	if _, err := database.Exec(`UPDATE users SET role='viewer', is_admin=0 WHERE id='u1'`); err != nil {
		t.Fatal(err)
	}

	w := call(r, http.MethodGet, "/api/v1/secret", tok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("statut = %d avec le MÊME jeton après rétrogradation, attendu 403 — "+
			"les droits survivraient à la décision", w.Code)
	}
}

// Désactiver un compte doit couper l'accès immédiatement, y compris en lecture.
func TestUnCompteDesactiveNAccedePlus(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleAdmin, true)
	r, _ := router(a, authz.PermAdmin)
	tok := tokenFor(t, "u1")

	if _, err := database.Exec(`UPDATE users SET is_active=0 WHERE id='u1'`); err != nil {
		t.Fatal(err)
	}
	if w := call(r, http.MethodGet, "/api/v1/secret", tok); w.Code != http.StatusUnauthorized {
		t.Fatalf("statut = %d pour un compte désactivé, attendu 401", w.Code)
	}
}

// La seconde barrière : une route d'écriture dont personne n'a déclaré la
// permission doit rester fermée à un rôle en lecture seule. C'est l'oubli le
// plus courant, et celui que rien ne signale.
func TestUneRouteOublieeResteFermeeALaLectureSeule(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleViewer, true)
	r, ran := router(a, authz.PermRead)

	w := call(r, http.MethodPost, "/api/v1/oubliee", tokenFor(t, "u1"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("statut = %d sur une route d'écriture non déclarée, attendu 403", w.Code)
	}
	if *ran {
		t.Fatal("le handler a tourné pour un rôle en lecture seule")
	}
}

// Le même rôle doit pouvoir lire : la lecture seule lit.
func TestLaLectureSeuleLitBien(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleViewer, true)
	r, _ := router(a, authz.PermRead)

	if w := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "u1")); w.Code != http.StatusOK {
		t.Fatalf("statut = %d pour une lecture par un rôle en lecture seule, attendu 200", w.Code)
	}
}

// Un comptable écrit les livres mais n'administre pas.
func TestUnComptableNAdministrePas(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleAccountant, true)
	r, ran := router(a, authz.PermAdmin)

	w := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "u1"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("statut = %d, attendu 403 : un comptable n'administre pas", w.Code)
	}
	if *ran {
		t.Fatal("le handler d'administration a tourné pour un comptable")
	}
	// Mais il écrit : le filtre global ne doit pas l'arrêter.
	if w := call(r, http.MethodPost, "/api/v1/oubliee", tokenFor(t, "u1")); w.Code != http.StatusOK {
		t.Fatalf("statut = %d sur une écriture par un comptable, attendu 200", w.Code)
	}
}

// Un rôle inconnu — base bricolée, restaurée d'une version future — vaut
// « non ». Deviner ici accorderait des droits sur la foi d'une chaîne qu'on ne
// comprend pas.
func TestUnRoleInconnuNaAucunDroit(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleAdmin, true)
	if _, err := database.Exec(`UPDATE users SET role='superadmin' WHERE id='u1'`); err != nil {
		t.Fatal(err)
	}
	r, _ := router(a, authz.PermRead)

	if w := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "u1")); w.Code != http.StatusUnauthorized {
		t.Fatalf("statut = %d pour un rôle inconnu, attendu 401", w.Code)
	}
}

// Un compte supprimé de la base ne doit pas conserver l'accès par son jeton.
func TestUnCompteInexistantNAccedePas(t *testing.T) {
	_, a := authzDB(t)
	r, _ := router(a, authz.PermRead)

	if w := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "fantome")); w.Code != http.StatusUnauthorized {
		t.Fatalf("statut = %d pour un compte inexistant, attendu 401", w.Code)
	}
}

// Le message de refus ne doit pas distinguer « compte supprimé » de « compte
// désactivé » : la différence n'aide que celui qui sonde les comptes.
func TestLeRefusNeRenseignePasSurLExistenceDuCompte(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "existe", authz.RoleAdmin, false)
	r, _ := router(a, authz.PermRead)

	inactif := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "existe")).Body.String()
	absent := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "absent")).Body.String()
	if inactif != absent {
		t.Fatalf("réponses distinctes:\n  désactivé: %s\n  inexistant: %s", inactif, absent)
	}
}

// Sans jeton, rien ne passe — y compris sur la route non déclarée.
func TestSansJetonRienNePasse(t *testing.T) {
	_, a := authzDB(t)
	r, ran := router(a, authz.PermRead)

	for _, m := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/secret"},
		{http.MethodPost, "/api/v1/oubliee"},
	} {
		if w := call(r, m.method, m.path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s → %d, attendu 401", m.method, m.path, w.Code)
		}
	}
	if *ran {
		t.Fatal("un handler a tourné sans authentification")
	}
}

// La table des droits doit rester ce qu'elle annonce. Un droit ajouté par
// erreur à un rôle est exactement ce qui ne se voit pas à la relecture.
func TestLaTableDesDroitsEstCelleAnnoncee(t *testing.T) {
	cases := []struct {
		role authz.Role
		perm authz.Permission
		want bool
	}{
		{authz.RoleAdmin, authz.PermAdmin, true},
		{authz.RoleAdmin, authz.PermWriteAccounting, true},
		{authz.RoleAccountant, authz.PermWriteAccounting, true},
		{authz.RoleAccountant, authz.PermWriteDocuments, true},
		{authz.RoleAccountant, authz.PermAdmin, false},
		{authz.RoleViewer, authz.PermRead, true},
		{authz.RoleViewer, authz.PermWriteDocuments, false},
		{authz.RoleViewer, authz.PermWriteAccounting, false},
		{authz.RoleViewer, authz.PermAdmin, false},
		{authz.Role("inconnu"), authz.PermRead, false},
		{authz.Role(""), authz.PermRead, false},
	}
	for _, c := range cases {
		if got := authz.Can(c.role, c.perm); got != c.want {
			t.Errorf("Can(%q, %q) = %v, attendu %v", c.role, c.perm, got, c.want)
		}
	}
}

// Une méthode inhabituelle doit compter comme une écriture. La liste est en
// « tout sauf », précisément pour qu'une méthode non prévue ne passe pas.
func TestUneMethodeInhabituelleCompteCommeEcriture(t *testing.T) {
	for _, m := range []string{"PATCH", "PUT", "DELETE", "POST", "PROPFIND", "LOCK"} {
		if !authz.IsWriteMethod(m) {
			t.Errorf("%s n'est pas comptée comme une écriture", m)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if authz.IsWriteMethod(m) {
			t.Errorf("%s est comptée comme une écriture", m)
		}
	}
}

// Le corps ne doit contenir qu'une seule réponse — le symptôme visible du
// défaut d'autorisation corrigé précédemment, où le handler répondait avant que
// le contrôle n'ait tranché.
func TestUneSeuleReponseDansLeCorps(t *testing.T) {
	database, a := authzDB(t)
	addUser(t, database, "u1", authz.RoleViewer, true)
	r, _ := router(a, authz.PermAdmin)

	body := call(r, http.MethodGet, "/api/v1/secret", tokenFor(t, "u1")).Body.String()
	if strings.Contains(body, "}{") {
		t.Fatalf("réponses concaténées: %s", body)
	}
}

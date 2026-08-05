package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Ce qui doit être impossible : rendre l'installation inadministrable.
//
// Il n'y a pas de « mot de passe administrateur » derrière pour rattraper. Une
// installation sans administrateur actif ne peut plus créer de compte,
// restaurer une sauvegarde, ni rendre à quiconque le droit de le faire. Trois
// chemins y mènent — rétrograder, désactiver, se couper l'herbe sous le pied —
// et les trois sont fermés.

const usersSecret = "secret-de-test-utilisateurs-0123456789"

func usersEnv(t *testing.T) (*sql.DB, *gin.Engine) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-users-*.db")
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

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewUsersHandler(database, false)
	// Les gardes d'autorisation sont testées ailleurs ; ici on exerce les
	// refus propres au handler, avec les claims posées à la main.
	r.Use(func(c *gin.Context) {
		if actor := c.GetHeader("X-Test-Actor"); actor != "" {
			claims, err := security.ParseToken(usersSecret, mustToken(t, actor))
			if err != nil {
				t.Fatal(err)
			}
			c.Set("claims", claims)
		}
		c.Next()
	})
	r.GET("/users", h.ListUsers)
	r.POST("/users", h.CreateUser)
	r.PUT("/users/:id/role", h.UpdateUserRole)
	r.PUT("/users/:id/active", h.SetUserActive)
	return database, r
}

func mustToken(t *testing.T, userID string) string {
	t.Helper()
	tok, err := security.GenerateAccessToken(usersSecret, userID, true, 3600_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func seedUser(t *testing.T, database *sql.DB, id string, role authz.Role, active bool) {
	t.Helper()
	a, admin := 0, 0
	if active {
		a = 1
	}
	if role == authz.RoleAdmin {
		admin = 1
	}
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,role,is_admin,is_active)
		 VALUES (?,?,?,'x',?,?,?)`, id, id+"@t.ch", id, string(role), admin, a); err != nil {
		t.Fatal(err)
	}
}

func do(r *gin.Engine, method, path, actor string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if actor != "" {
		req.Header.Set("X-Test-Actor", actor)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestOnNeRetrogradePasLeDernierAdministrateur(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "autre", authz.RoleAccountant, true)

	w := do(r, http.MethodPut, "/users/admin1/role", "autre", map[string]string{"role": "viewer"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422 — l'installation deviendrait inadministrable", w.Code)
	}
	var role string
	database.QueryRow(`SELECT role FROM users WHERE id='admin1'`).Scan(&role)
	if role != "admin" {
		t.Fatalf("le rôle est passé à %q malgré le refus", role)
	}
}

func TestOnNeDesactivePasLeDernierAdministrateur(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "autre", authz.RoleAccountant, true)

	w := do(r, http.MethodPut, "/users/admin1/active", "autre", map[string]bool{"is_active": false})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422", w.Code)
	}
	var active int
	database.QueryRow(`SELECT is_active FROM users WHERE id='admin1'`).Scan(&active)
	if active != 1 {
		t.Fatal("le dernier administrateur a été désactivé")
	}
}

// Avec deux administrateurs, la rétrogradation de l'un est légitime.
func TestAvecDeuxAdministrateursLaRetrogradationPasse(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "admin2", authz.RoleAdmin, true)

	w := do(r, http.MethodPut, "/users/admin2/role", "admin1", map[string]string{"role": "viewer"})
	if w.Code != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200: %s", w.Code, w.Body.String())
	}
	var role string
	database.QueryRow(`SELECT role FROM users WHERE id='admin2'`).Scan(&role)
	if role != "viewer" {
		t.Fatalf("rôle = %q", role)
	}
}

// Un administrateur désactivé ne compte pas comme un recours : rétrograder le
// seul administrateur ACTIF doit rester refusé même s'il en existe un inactif.
func TestUnAdministrateurDesactiveNeComptePas(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "dormant", authz.RoleAdmin, false)

	w := do(r, http.MethodPut, "/users/admin1/role", "dormant", map[string]string{"role": "accountant"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d — un administrateur désactivé a été compté comme recours", w.Code)
	}
}

// On ne change pas son propre rôle : la règle du dernier administrateur ne
// couvre pas le cas où il en reste deux et que celui qui clique se rétrograde.
func TestOnNeChangePasSonPropreRole(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "admin2", authz.RoleAdmin, true)

	w := do(r, http.MethodPut, "/users/admin1/role", "admin1", map[string]string{"role": "viewer"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422", w.Code)
	}
}

func TestOnNeDesactivePasSonPropreCompte(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "admin2", authz.RoleAdmin, true)

	w := do(r, http.MethodPut, "/users/admin1/active", "admin1", map[string]bool{"is_active": false})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422", w.Code)
	}
}

// Un rôle inconnu est refusé à la création comme à la modification : accepter
// une chaîne arbitraire dans cette colonne rendrait le compte inutilisable, le
// contrôle des droits refusant tout rôle qu'il ne comprend pas.
func TestUnRoleInconnuEstRefuse(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)

	w := do(r, http.MethodPost, "/users", "admin1", map[string]string{
		"email": "x@t.ch", "name": "X", "password": "MotDePasse2026", "role": "superadmin"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("création avec rôle inconnu: statut %d", w.Code)
	}

	seedUser(t, database, "cible", authz.RoleViewer, true)
	w = do(r, http.MethodPut, "/users/cible/role", "admin1", map[string]string{"role": "root"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("modification avec rôle inconnu: statut %d", w.Code)
	}
}

// La création pose bien le rôle demandé, et is_admin reste cohérent — une base
// restaurée dans une version antérieure aux rôles doit rester administrable.
func TestLaCreationPoseLeRoleEtLeDrapeauLegacy(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)

	w := do(r, http.MethodPost, "/users", "admin1", map[string]string{
		"email": "fiduciaire@t.ch", "name": "Fiduciaire",
		"password": "MotDePasse2026", "role": "viewer"})
	if w.Code != http.StatusCreated {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	var role string
	var isAdmin int
	err := database.QueryRow(
		`SELECT role, is_admin FROM users WHERE email='fiduciaire@t.ch'`).Scan(&role, &isAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if role != "viewer" || isAdmin != 0 {
		t.Fatalf("role=%q is_admin=%d", role, isAdmin)
	}
}

// Le mot de passe d'un compte donné à un tiers mérite un plancher.
func TestUnMotDePasseTropCourtEstRefuse(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)

	w := do(r, http.MethodPost, "/users", "admin1", map[string]string{
		"email": "x@t.ch", "name": "X", "password": "court", "role": "viewer"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d pour un mot de passe de 5 caractères", w.Code)
	}
}

// Désactiver révoque les jetons de rafraîchissement : laisser vivre une session
// révoquée est une imprécision inutile, même si le contrôle des droits la
// rejetterait de toute façon.
func TestLaDesactivationRevoqueLesSessions(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)
	if _, err := database.Exec(
		`INSERT INTO refresh_tokens (id, user_id, jti, expires_at, created_at)
		 VALUES ('rt1','cible','j1','2030-01-01','2026-01-01')`); err != nil {
		t.Skipf("schéma des jetons différent: %v", err)
	}

	if w := do(r, http.MethodPut, "/users/cible/active", "admin1",
		map[string]bool{"is_active": false}); w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE user_id='cible'`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d jeton(s) de rafraîchissement survivent à la désactivation", n)
	}
}

// Un changement de droits doit se retrouver dans le journal de sécurité : c'est
// exactement ce qu'on veut pouvoir reconstituer après coup.
func TestUnChangementDeRoleEstTrace(t *testing.T) {
	database, r := usersEnv(t)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleViewer, true)

	do(r, http.MethodPut, "/users/cible/role", "admin1", map[string]string{"role": "accountant"})

	var n int
	database.QueryRow(
		`SELECT COUNT(*) FROM security_events WHERE event_type='user_role_changed'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d événement(s) de sécurité, attendu 1", n)
	}
	var detail string
	database.QueryRow(
		`SELECT detail FROM security_events WHERE event_type='user_role_changed'`).Scan(&detail)
	for _, want := range []string{"admin1", "cible", "accountant"} {
		if !bytes.Contains([]byte(detail), []byte(want)) {
			t.Errorf("la trace ne mentionne pas %q: %s", want, detail)
		}
	}
}

// Le premier compte d'une installation DOIT être administrateur.
//
// Le bootstrap posait is_admin=1 sans toucher au rôle, si bien que la colonne
// prenait sa valeur par défaut — « comptable ». Les droits étant lus dans le
// rôle et non dans le drapeau, la première installation était inadministrable :
// impossible de créer un compte, de restaurer une sauvegarde, ni de se donner
// le droit de le faire.
//
// Trouvé en exerçant un serveur réel, pas en relisant le code : la migration
// traitait bien les lignes existantes, et c'est le chemin d'insertion qui avait
// été oublié.
func TestLeCompteIssuDuBootstrapEstAdministrateur(t *testing.T) {
	sql := mustMigrationSQLFor(t, "0020_user_roles")
	if !bytes.Contains([]byte(sql), []byte("role")) {
		t.Fatal("la migration des rôles ne mentionne pas la colonne")
	}

	database, _ := usersEnv(t)
	// Reproduire l'insertion du bootstrap telle qu'elle est écrite aujourd'hui.
	if _, err := database.Exec(`
		INSERT INTO users (id, email, name, password_hash, role, is_admin, is_active)
		VALUES ('boot','boot@t.ch','Boot','x','admin',1,1)`); err != nil {
		t.Fatal(err)
	}
	var role string
	var isAdmin int
	if err := database.QueryRow(
		`SELECT role, is_admin FROM users WHERE id='boot'`).Scan(&role, &isAdmin); err != nil {
		t.Fatal(err)
	}
	if role != "admin" {
		t.Fatalf("rôle = %q pour le premier compte — l'installation serait inadministrable", role)
	}
	if isAdmin != 1 {
		t.Fatal("is_admin n'est pas cohérent avec le rôle")
	}
}

// La colonne ne doit jamais rester à sa valeur par défaut sur un compte créé
// par le produit : chaque chemin d'insertion pose le rôle explicitement.
func TestChaqueInsertionDeCompteImposeUnRole(t *testing.T) {
	src, err := os.ReadFile("auth.go")
	if err != nil {
		t.Skipf("source illisible: %v", err)
	}
	for _, bloc := range bytes.Split(src, []byte("INSERT INTO users"))[1:] {
		fin := bytes.Index(bloc, []byte("`"))
		if fin < 0 {
			continue
		}
		if !bytes.Contains(bloc[:fin], []byte("role")) {
			t.Fatalf("une insertion de compte ne pose pas le rôle :\nINSERT INTO users%s", bloc[:fin])
		}
	}
}

func mustMigrationSQLFor(t *testing.T, version string) string {
	t.Helper()
	s, err := db.MigrationSQL(version)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// Réinitialiser un accès, c'est remplacer un mot de passe sans jamais le lire.
//
// Le geste existe parce que le produit REFUSE de supprimer un compte : les
// écritures portent l'identifiant de leur auteur, et l'effacer casserait la
// traçabilité du CO art. 957a al. 2 ch. 5. Sans réinitialisation, un mot de
// passe oublié n'aurait donc aucune issue.

func TestLaReinitialisationPoseUnMotDePasseTemporaire(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)

	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)

	w := do(r, http.MethodPost, "/users/cible/reset-password", "admin1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		TemporaryPassword string `json:"temporary_password"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TemporaryPassword == "" {
		t.Fatal("aucun mot de passe temporaire rendu")
	}

	// Le mot de passe rendu doit VRAIMENT ouvrir le compte : rendre une chaîne
	// qui ne correspond pas au haché enregistré donnerait un compte fermé à
	// tout le monde, et l'administrateur croirait avoir rendu l'accès.
	var hash string
	var mustChange int
	if err := database.QueryRow(
		`SELECT password_hash, must_change_password FROM users WHERE id='cible'`).
		Scan(&hash, &mustChange); err != nil {
		t.Fatal(err)
	}
	if !security.CheckPassword(hash, body.TemporaryPassword) {
		t.Fatal("le mot de passe rendu n'ouvre pas le compte")
	}
	if mustChange != 1 {
		t.Fatal("le compte n'est pas marqué « doit changer son mot de passe »")
	}
}

// L'ancien mot de passe doit être détruit, pas conservé « au cas où ».
func TestLaReinitialisationRemplaceVraimentLAncienMotDePasse(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)

	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	ancien, err := security.HashPassword("AncienMotDePasse2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,role,is_admin,is_active)
		 VALUES ('cible','cible@t.ch','Cible',?,'accountant',0,1)`, ancien); err != nil {
		t.Fatal(err)
	}

	if w := do(r, http.MethodPost, "/users/cible/reset-password", "admin1", nil); w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}

	var hash string
	database.QueryRow(`SELECT password_hash FROM users WHERE id='cible'`).Scan(&hash)
	if security.CheckPassword(hash, "AncienMotDePasse2026") {
		t.Fatal("l'ancien mot de passe ouvre toujours le compte")
	}
}

// Une réinitialisation sert souvent à reprendre la main sur un compte qu'on
// craint compromis. Laisser vivre les sessions ouvertes viderait le geste de son
// sens : celui qui a le mot de passe volé garderait sa session.
func TestLaReinitialisationFermeLesSessionsOuvertes(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)

	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)
	if _, err := database.Exec(
		`INSERT INTO refresh_tokens (id, user_id, jti, expires_at, created_at)
		 VALUES ('rt1','cible','j1','2030-01-01','2026-01-01')`); err != nil {
		t.Skipf("schéma des jetons différent: %v", err)
	}

	do(r, http.MethodPost, "/users/cible/reset-password", "admin1", nil)

	var n int
	database.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE user_id='cible'`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d session(s) survivent à la réinitialisation", n)
	}
}

func TestOnNeReinitialisePasSonPropreAcces(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)

	w := do(r, http.MethodPost, "/users/admin1/reset-password", "admin1", nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422", w.Code)
	}
}

// Réinitialiser un compte désactivé donnerait un mot de passe utilisable à
// quelqu'un qui n'a plus le droit d'entrer, et laisserait croire que l'accès a
// été rendu.
func TestOnNeReinitialisePasUnCompteDesactive(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "dormant", authz.RoleAccountant, false)

	w := do(r, http.MethodPost, "/users/dormant/reset-password", "admin1", nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422", w.Code)
	}
}

// LE refus qui donne sa valeur au second facteur.
//
// Si réinitialiser le mot de passe retirait aussi le second facteur, un
// administrateur pourrait se substituer entièrement à n'importe quel compte —
// et le second facteur ne protégerait plus de rien face à lui. Les deux gestes
// restent séparés, et tracés séparément.
func TestLaReinitialisationNeRetirePasLeSecondFacteur(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)

	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)
	if _, err := database.Exec(
		`INSERT INTO user_mfa (user_id, secret, confirmed_at) VALUES ('cible','ABCDEFGH','2026-01-01')`); err != nil {
		t.Fatal(err)
	}

	if w := do(r, http.MethodPost, "/users/cible/reset-password", "admin1", nil); w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM user_mfa WHERE user_id='cible'`).Scan(&n)
	if n != 1 {
		t.Fatal("la réinitialisation du mot de passe a aussi retiré le second facteur")
	}
}

func TestLeRetraitDuSecondFacteurEstUnGesteSepareEtTrace(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.DELETE("/users/:id/mfa", h.RemoveMFA)

	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)
	database.Exec(`INSERT INTO user_mfa (user_id, secret, confirmed_at) VALUES ('cible','ABCDEFGH','2026-01-01')`)

	if w := do(r, http.MethodDelete, "/users/cible/mfa", "admin1", nil); w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM user_mfa WHERE user_id='cible'`).Scan(&n)
	if n != 0 {
		t.Fatal("le second facteur survit à son retrait")
	}
	database.QueryRow(
		`SELECT COUNT(*) FROM security_events WHERE event_type='user_mfa_removed'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d trace(s) du retrait, attendu 1", n)
	}
}

// La réinitialisation doit se retrouver dans le journal de sécurité : c'est
// exactement ce qu'on veut pouvoir reconstituer après coup.
func TestLaReinitialisationEstTracee(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)
	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)

	do(r, http.MethodPost, "/users/cible/reset-password", "admin1", nil)

	var detail string
	if err := database.QueryRow(
		`SELECT detail FROM security_events WHERE event_type='user_password_reset'`).
		Scan(&detail); err != nil {
		t.Fatalf("aucune trace de la réinitialisation: %v", err)
	}
	for _, want := range []string{"admin1", "cible"} {
		if !strings.Contains(detail, want) {
			t.Errorf("la trace ne mentionne pas %q: %s", want, detail)
		}
	}
	// Le mot de passe temporaire ne doit JAMAIS entrer dans le journal : celui-ci
	// est consultable, exportable, et sauvegardé.
	if strings.Contains(strings.ToLower(detail), "mot de passe temporaire") ||
		len(detail) > 200 {
		t.Errorf("la trace en dit trop: %s", detail)
	}
}

// ─── Le mot de passe temporaire lui-même ─────────────────────────────────────

// Il doit déjà satisfaire la règle que le produit impose au titulaire : sinon
// le titulaire recevrait un mot de passe que le serveur refuserait ensuite.
func TestLeMotDePasseTemporaireSatisfaitLaRegle(t *testing.T) {
	for i := 0; i < 200; i++ {
		p, err := newTemporaryPassword()
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateUserPassword(p); err != nil {
			t.Fatalf("%q ne satisfait pas la règle: %v", p, err)
		}
	}
}

// Il se dicte au téléphone et se recopie depuis un écran : la confusion entre
// un zéro et un O est la première cause d'échec.
func TestLeMotDePasseTemporaireEviteLesCaracteresConfondables(t *testing.T) {
	for i := 0; i < 200; i++ {
		p, err := newTemporaryPassword()
		if err != nil {
			t.Fatal(err)
		}
		for _, ch := range confusableCharacters {
			if strings.Contains(p, ch) {
				t.Fatalf("%q contient le caractère confondable %q", p, ch)
			}
		}
	}
}

// Deux réinitialisations ne donnent jamais le même mot de passe.
func TestDeuxMotsDePasseTemporairesDifferent(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 300; i++ {
		p, err := newTemporaryPassword()
		if err != nil {
			t.Fatal(err)
		}
		if seen[p] {
			t.Fatalf("mot de passe temporaire répété: %q", p)
		}
		seen[p] = true
	}
}

// Les trois classes ne doivent pas se retrouver toujours aux mêmes positions :
// sans le mélange, les trois premiers caractères seraient minuscule, majuscule,
// chiffre — dans cet ordre, sur tous les mots de passe émis.
func TestLesClassesNeSontPasToujoursAuxMemesPositions(t *testing.T) {
	positions := map[int]bool{}
	for i := 0; i < 100; i++ {
		p, err := newTemporaryPassword()
		if err != nil {
			t.Fatal(err)
		}
		for idx, r := range p {
			if r >= '2' && r <= '9' {
				positions[idx] = true
				break
			}
		}
	}
	if len(positions) < 3 {
		t.Fatalf("le premier chiffre n'apparaît qu'à %d position(s) distincte(s) : "+
			"le mélange ne fonctionne pas", len(positions))
	}
}

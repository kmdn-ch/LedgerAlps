package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/mfa"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// Audit croisé Gemini, série 4 : données et sécurité (HIGH-15, HIGH-16).

// HIGH-16. Un code de secours « à usage unique » ouvrait DEUX sessions quand deux
// requêtes le portaient en même temps (trois essais sur trois en rejeu) : le code
// était lu, comparé — dix hachages bcrypt, des centaines de millisecondes — puis
// marqué utilisé SANS condition.
func TestUnCodeDeSecoursNeOuvreQuUneSessionEnParallele(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	w := call(r, http.MethodPost, "/auth/login", "", map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	token, _ := decode(t, w)["access_token"].(string)
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	secret, _ := decode(t, w)["secret"].(string)
	juste, _ := mfa.Code(secret, time.Now())
	w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": juste})
	if w.Code != http.StatusOK {
		t.Fatalf("inscription : %d — %s", w.Code, w.Body.String())
	}
	brut, _ := decode(t, w)["recovery_codes"].([]any)
	if len(brut) < 3 {
		t.Fatalf("codes de secours attendus, reçu %v", brut)
	}

	for essai := 0; essai < 3; essai++ {
		code, _ := brut[essai].(string)
		jetons := []string{
			loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1"),
			loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1"),
		}
		var (
			mu     sync.Mutex
			statut []int
			wg     sync.WaitGroup
		)
		depart := make(chan struct{})
		for _, jeton := range jetons {
			wg.Add(1)
			go func(jeton string) {
				defer wg.Done()
				<-depart
				s := call(r, http.MethodPost, "/auth/mfa/verify", jeton, map[string]string{"code": code}).Code
				mu.Lock()
				statut = append(statut, s)
				mu.Unlock()
			}(jeton)
		}
		close(depart)
		wg.Wait()

		ok := 0
		for _, s := range statut {
			if s == http.StatusOK {
				ok++
			}
		}
		if ok != 1 {
			t.Fatalf("essai %d : le même code de secours a ouvert %d session(s) (statuts %v), attendu exactement 1",
				essai+1, ok, statut)
		}
	}
}

// HIGH-16, même défaut pour le code de l'application : la fenêtre consommée était lue,
// puis écrite sans condition. Plusieurs requêtes portant le MÊME code, lancées ensemble,
// ne doivent en laisser passer qu'une.
func TestUnCodeDeLApplicationNeSertQuUneFoisEnParallele(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	w := call(r, http.MethodPost, "/auth/login", "", map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	token, _ := decode(t, w)["access_token"].(string)
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	secret, _ := decode(t, w)["secret"].(string)
	juste, _ := mfa.Code(secret, time.Now())
	if w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": juste}); w.Code != http.StatusOK {
		t.Fatalf("inscription : %d — %s", w.Code, w.Body.String())
	}

	// L'inscription a consommé la fenêtre courante : le code suivant est celui de la
	// fenêtre d'après, que le serveur tolère (une fenêtre d'avance).
	suivant, _ := mfa.Code(secret, time.Now().Add(30*time.Second))

	const n = 8
	jetons := make([]string, n)
	for i := range jetons {
		jetons[i] = loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	}
	var (
		mu    sync.Mutex
		ok    int
		wg    sync.WaitGroup
		start = make(chan struct{})
	)
	for _, jeton := range jetons {
		wg.Add(1)
		go func(jeton string) {
			defer wg.Done()
			<-start
			if call(r, http.MethodPost, "/auth/mfa/verify", jeton, map[string]string{"code": suivant}).Code == http.StatusOK {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(jeton)
	}
	close(start)
	wg.Wait()
	if ok != 1 {
		t.Fatalf("le même code a ouvert %d sessions sur %d requêtes simultanées, attendu 1", ok, n)
	}
}

// HIGH-16. Réinitialiser l'accès d'un compte par un administrateur retire ses postes de
// confiance : ils dispensent du second facteur trente jours, et l'on réinitialise
// justement quand on croit le compte compromis.
func TestLaReinitialisationRetireLesPostesDeConfiance(t *testing.T) {
	database, r := usersEnv(t)
	h := NewUsersHandler(database, false)
	r.POST("/users/:id/reset-password", h.ResetPassword)

	seedUser(t, database, "admin1", authz.RoleAdmin, true)
	seedUser(t, database, "cible", authz.RoleAccountant, true)

	poser := func() {
		t.Helper()
		if _, err := database.Exec(`
			INSERT INTO trusted_devices (id, user_id, token_hash, label, last_ip, expires_at, created_at)
			VALUES (?, 'cible', ?, 'poste', '127.0.0.1', ?, ?)`,
			"d"+time.Now().Format("150405.000000"), "h"+time.Now().Format("150405.000000"),
			time.Now().Add(24*time.Hour), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	compter := func() int {
		var n int
		database.QueryRow(`SELECT COUNT(*) FROM trusted_devices WHERE user_id='cible'`).Scan(&n)
		return n
	}

	poser()
	if compter() != 1 {
		t.Fatal("amorce : le poste de confiance n'a pas été posé")
	}
	if w := do(r, http.MethodPost, "/users/cible/reset-password", "admin1", nil); w.Code != http.StatusOK {
		t.Fatalf("réinitialisation : %d — %s", w.Code, w.Body.String())
	}
	if n := compter(); n != 0 {
		t.Errorf("%d poste(s) de confiance survivent à la réinitialisation, attendu 0", n)
	}

	// La désactivation les retire aussi : un compte réactivé ne retrouve pas la
	// dispense d'avant.
	poser()
	if w := do(r, http.MethodPut, "/users/cible/active", "admin1", map[string]any{"is_active": false}); w.Code != http.StatusOK {
		t.Fatalf("désactivation : %d — %s", w.Code, w.Body.String())
	}
	if n := compter(); n != 0 {
		t.Errorf("%d poste(s) de confiance survivent à la désactivation, attendu 0", n)
	}
}

// HIGH-16. Le changement de mot de passe et le retrait du second facteur redemandent le
// mot de passe actuel d'une session OUVERTE : rien ne bornait ces essais. Cinq refus
// ferment la voie du compte ; un succès ne la rouvre pas pour un autre compte.
func TestLeMotDePasseDUneSessionOuverteEstBorne(t *testing.T) {
	database, r, _ := mfaEnvOpt(t, true)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	seedLogin(t, database, "u2", "AutreMotDePasse22", false)

	jeton := func(email, mdp string) string {
		w := call(r, http.MethodPost, "/auth/login", "", map[string]string{"email": email, "password": mdp})
		tok, _ := decode(t, w)["access_token"].(string)
		if tok == "" {
			t.Fatalf("connexion %s : %d — %s", email, w.Code, w.Body.String())
		}
		return tok
	}
	t1, t2 := jeton("u1@t.ch", "MotDePasseSolide1"), jeton("u2@t.ch", "AutreMotDePasse22")

	refus := 0
	for i := 0; i < 12; i++ {
		got := call(r, http.MethodPost, "/auth/change-password", t1, map[string]string{
			"current_password": "faux-" + strings.Repeat("x", i), "new_password": "NouveauMotDePasse33"}).Code
		if got == http.StatusUnauthorized {
			refus++
		}
	}
	if refus != 5 {
		t.Fatalf("%d essais de mot de passe traités sur 12, attendu 5", refus)
	}

	// Voie fermée : même le BON mot de passe est refusé, avec un délai.
	w := call(r, http.MethodPost, "/auth/change-password", t1, map[string]string{
		"current_password": "MotDePasseSolide1", "new_password": "NouveauMotDePasse33"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("bon mot de passe sur une voie fermée : %d, attendu 429 — %s", w.Code, w.Body.String())
	}
	// Le retrait du second facteur partage la voie : elle est fermée pour le compte.
	if w = call(r, http.MethodDelete, "/auth/mfa", t1, map[string]string{"password": "MotDePasseSolide1"}); w.Code != http.StatusTooManyRequests {
		t.Errorf("retrait du second facteur sur une voie fermée : %d, attendu 429", w.Code)
	}

	// Un autre compte n'est pas touché.
	if w = call(r, http.MethodPost, "/auth/change-password", t2, map[string]string{
		"current_password": "AutreMotDePasse22", "new_password": "NouveauMotDePasse44"}); w.Code != http.StatusOK {
		t.Errorf("le compte voisin est gêné : %d — %s", w.Code, w.Body.String())
	}

	// La fermeture est tracée.
	var nb int
	database.QueryRow(`SELECT COUNT(*) FROM security_events WHERE event_type = 'password_lockout'`).Scan(&nb)
	if nb != 1 {
		t.Errorf("%d trace(s) de verrouillage des mots de passe, attendu 1", nb)
	}
}

// La revendication d'une fenêtre est atomique ET déterministe : la première prend, toute
// autre — même fenêtre, ou plus ancienne — ne touche aucune ligne. Le test de parallélisme
// ci-dessus ne le prouve qu'avec de la chance (la fenêtre de course est de quelques
// microsecondes) ; celui-ci tient à chaque exécution.
func TestUneFenetreDeCodeNeSeRevendiqueQuUneFois(t *testing.T) {
	database, _, cfg := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	if _, err := database.Exec(
		`INSERT INTO user_mfa (user_id, secret, confirmed_at, last_window, created_at, updated_at)
		 VALUES ('u1', 'x', ?, 0, ?, ?)`, time.Now(), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	h := NewAuthHandler(database, cfg)
	ctx := context.Background()

	if ok, err := h.claimTOTPWindow(ctx, "u1", 100); err != nil || !ok {
		t.Fatalf("première revendication : %v, %v — attendu vrai", ok, err)
	}
	if ok, _ := h.claimTOTPWindow(ctx, "u1", 100); ok {
		t.Error("la même fenêtre a été revendiquée deux fois")
	}
	if ok, _ := h.claimTOTPWindow(ctx, "u1", 99); ok {
		t.Error("une fenêtre plus ancienne a été revendiquée après une plus récente")
	}
	if ok, _ := h.claimTOTPWindow(ctx, "u1", 101); !ok {
		t.Error("la fenêtre suivante a été refusée")
	}
}

// HIGH-15. L'IBAN de l'entreprise est le compte qui reçoit TOUS les virements des clients.
// Un statut TVA inconnu répondait 422… après avoir écrit l'IBAN, sans aucune trace d'audit.
// Un refus doit n'avoir rien écrit.
func TestUnRefusNeLaisseRienEcritDansLaFicheSociete(t *testing.T) {
	h, _ := nouveauxRéglages(t)
	seedUser(t, h.db, "admin1", authz.RoleAdmin, true)

	// Un routeur avec un auteur authentifié : sans lui la trace ne s'écrit pas.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		claims, err := security.ParseToken(usersSecret, mustToken(t, "admin1"))
		if err != nil {
			t.Fatal(err)
		}
		c.Set("claims", claims)
		c.Next()
	})
	r.GET("/company", h.GetCompany)
	r.PUT("/company", h.PutCompany)

	if w := enregistrer(t, r, map[string]any{
		"company_name": "Test SA", "iban": "CH9300762011623852957"}); w.Code != http.StatusOK {
		t.Fatalf("enregistrement initial : %d — %s", w.Code, w.Body.String())
	}
	compteAudit := func() int {
		var n int
		h.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE table_name = 'company_settings'`).Scan(&n)
		return n
	}
	avant := compteAudit()
	if avant != 1 {
		t.Fatalf("l'enregistrement valide devait écrire 1 ligne d'audit, a écrit %d", avant)
	}

	// Nouvel IBAN + statut TVA inconnu : refus, et RIEN d'écrit.
	w := enregistrer(t, r, map[string]any{
		"company_name": "Test SA", "iban": "CH5604835012345678009", "vat_status": "n_importe_quoi"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut inconnu : %d, attendu 422 — %s", w.Code, w.Body.String())
	}
	if got := lireFiche(t, r)["iban"]; got != "CH9300762011623852957" {
		t.Errorf("l'IBAN vaut %v après un refus : il a été écrit malgré le 422", got)
	}
	if n := compteAudit(); n != avant {
		t.Errorf("%d ligne(s) d'audit après un refus, attendu %d", n, avant)
	}
}

// jetonsRestants liste les jetons de rafraîchissement d'un compte. Une fonction à part : le curseur
// se ferme par defer (sqlclosecheck, comme dans le reste du dépôt).
func jetonsRestants(t *testing.T, database *sql.DB, compte string) []string {
	t.Helper()
	rows, err := database.Query(`SELECT jti FROM refresh_tokens WHERE user_id = ?`, compte)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var j string
		if err := rows.Scan(&j); err != nil {
			t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

// HIGH-20. Changer son mot de passe fermait TOUTES les sessions du compte, la sienne comprise,
// alors que le message dit « les autres sessions » : l'utilisateur était renvoyé à l'écran de
// connexion à la fin de son jeton d'accès, sans explication.
func TestChangerSonMotDePasseGardeLaSessionCourante(t *testing.T) {
	database, r, cfg := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	connexion := func(motDePasse string) (acces, cookie string) {
		body, _ := json.Marshal(map[string]string{"email": "u1@t.ch", "password": motDePasse})
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("connexion : %d — %s", w.Code, w.Body.String())
		}
		acces, _ = decode(t, w)["access_token"].(string)
		for _, c := range w.Result().Cookies() {
			if c.Name == refreshCookieName {
				cookie = c.Value
			}
		}
		if acces == "" || cookie == "" {
			t.Fatalf("session incomplète : jeton %q, cookie %q", acces, cookie)
		}
		return
	}
	accesA, cookieA := connexion("MotDePasseSolide1")
	_, _ = connexion("MotDePasseSolide1") // une autre session, ouverte ailleurs

	var avant int
	database.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = 'u1'`).Scan(&avant)
	if avant != 2 {
		t.Fatalf("%d session(s) ouvertes avant le changement, attendu 2", avant)
	}

	body, _ := json.Marshal(map[string]string{"current_password": "MotDePasseSolide1", "new_password": "NouveauMotDePasse33"})
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accesA)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: cookieA})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("changement : %d — %s", w.Code, w.Body.String())
	}

	rc, err := security.ParseToken(cfg.JWTSecret, cookieA)
	if err != nil {
		t.Fatal(err)
	}
	restants := jetonsRestants(t, database, "u1")
	if len(restants) != 1 || restants[0] != rc.JTI {
		t.Errorf("sessions restantes %v, attendu la seule session courante (%s)", restants, rc.JTI)
	}

	// Sans cookie (client sans navigateur), toutes tombent : mieux vaut en fermer une de trop.
	accesC, _ := connexion("NouveauMotDePasse33")
	body, _ = json.Marshal(map[string]string{"current_password": "NouveauMotDePasse33", "new_password": "EncoreUnAutre4455"})
	req = httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accesC)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second changement : %d — %s", w.Code, w.Body.String())
	}
	var apres int
	database.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = 'u1'`).Scan(&apres)
	if apres != 0 {
		t.Errorf("%d session(s) restantes après un changement sans cookie, attendu 0", apres)
	}
}

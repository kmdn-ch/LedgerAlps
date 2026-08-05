package handlers

// Le second facteur exercé de bout en bout, par HTTP, sur une vraie base.
//
// Ce qui est vérifié ici n'est pas l'algorithme — il l'est contre les vecteurs
// officiels de la RFC 6238 dans internal/core/mfa — mais ce qui l'entoure : que
// le mot de passe seul ne délivre plus de session, que le jeton d'attente ne
// vaille rien ailleurs, qu'un code de secours ne serve qu'une fois, et qu'une
// inscription abandonnée n'enferme personne.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/mfa"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

const mfaSecret = "secret-de-test-second-facteur-0123456789"

func mfaEnv(t *testing.T) (*sql.DB, *gin.Engine, *config.Config) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-mfa-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	cfg := &config.Config{
		SQLitePath:       tmp.Name(),
		Host:             "127.0.0.1",
		JWTSecret:        mfaSecret,
		JWTAccessMinutes: 60,
		JWTRefreshDays:   30,
	}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAuthHandler(database, cfg)
	r.POST("/auth/login", h.Login)
	r.POST("/auth/mfa/verify", mw.RequireMFAChallenge(cfg.JWTSecret), h.MFAVerify)
	authed := r.Group("", mw.RequireAuth(cfg.JWTSecret))
	authed.GET("/auth/mfa", h.MFAStatus)
	authed.POST("/auth/mfa/setup", h.MFASetup)
	authed.POST("/auth/mfa/confirm", h.MFAConfirm)
	authed.DELETE("/auth/mfa", h.MFADisable)
	// Une route ordinaire, pour prouver que le jeton d'attente n'y donne accès.
	authed.GET("/protegee", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	return database, r, cfg
}

func seedLogin(t *testing.T, database *sql.DB, id, password string, admin bool) {
	t.Helper()
	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	role, flag := "accountant", 0
	if admin {
		role, flag = "admin", 1
	}
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,role,is_admin,is_active)
		 VALUES (?,?,?,?,?,?,1)`, id, id+"@t.ch", id, hash, role, flag); err != nil {
		t.Fatal(err)
	}
}

func call(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("réponse illisible (%d): %s", w.Code, w.Body.String())
	}
	return m
}

// Le parcours complet : inscription, puis connexion en deux temps.
func TestLeParcoursCompletDuSecondFacteur(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	// Connexion sans second facteur : session immédiate.
	w := call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	if w.Code != http.StatusOK {
		t.Fatalf("connexion: statut %d — %s", w.Code, w.Body.String())
	}
	token, _ := decode(t, w)["access_token"].(string)
	if token == "" {
		t.Fatal("aucun jeton d'accès sans second facteur")
	}

	// Inscription.
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("inscription: statut %d — %s", w.Code, w.Body.String())
	}
	setup := decode(t, w)
	secret, _ := setup["secret"].(string)
	if secret == "" {
		t.Fatal("aucun secret rendu")
	}
	if uri, _ := setup["uri"].(string); uri == "" {
		t.Fatal("aucune URI otpauth rendue")
	}
	if qr, _ := setup["qr_png"].(string); len(qr) < 100 {
		t.Fatalf("le QR est vide ou tronqué: %q", qr)
	}

	// Tant que ce n'est pas confirmé, la connexion NE DOIT PAS réclamer de code :
	// quelqu'un qui ferme l'onglet à cet instant serait sinon enfermé dehors.
	w = call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	if _, demande := decode(t, w)["mfa_required"]; demande {
		t.Fatal("une inscription non confirmée réclame déjà un code")
	}

	// Confirmation.
	code, err := mfa.Code(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": code})
	if w.Code != http.StatusOK {
		t.Fatalf("confirmation: statut %d — %s", w.Code, w.Body.String())
	}
	confirm := decode(t, w)
	codes, _ := confirm["recovery_codes"].([]any)
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("%d code(s) de secours, attendu %d — sans eux, un téléphone perdu "+
			"enferme le dernier administrateur", len(codes), RecoveryCodeCount)
	}

	// Connexion : le mot de passe ne suffit plus.
	w = call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	body := decode(t, w)
	if req, _ := body["mfa_required"].(bool); !req {
		t.Fatalf("le mot de passe seul délivre encore une session: %s", w.Body.String())
	}
	if _, ok := body["access_token"]; ok {
		t.Fatal("un jeton d'accès est délivré avant le second facteur")
	}
	challenge, _ := body["mfa_token"].(string)
	if challenge == "" {
		t.Fatal("aucun jeton d'attente")
	}

	// Le jeton d'attente ne vaut RIEN ailleurs.
	if w := call(r, http.MethodGet, "/protegee", challenge, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("le jeton d'attente ouvre une route protégée: statut %d", w.Code)
	}

	// Deuxième étape.
	// La fenêtre courante a peut-être été consommée par la confirmation ; on
	// prend celle d'après, comme le ferait un téléphone trente secondes plus tard.
	later := time.Now().Add(mfa.Period * time.Second)
	code2, _ := mfa.Code(secret, later)
	w = call(r, http.MethodPost, "/auth/mfa/verify", challenge, map[string]string{"code": code2})
	if w.Code != http.StatusOK {
		t.Fatalf("vérification: statut %d — %s", w.Code, w.Body.String())
	}
	final, _ := decode(t, w)["access_token"].(string)
	if final == "" {
		t.Fatal("aucune session après le second facteur")
	}
	if w := call(r, http.MethodGet, "/protegee", final, nil); w.Code != http.StatusOK {
		t.Fatalf("la session issue du second facteur n'ouvre pas les routes: %d", w.Code)
	}
}

// Le refus qui donne son sens au second facteur : le mot de passe volé ne suffit
// plus, et un mauvais code n'ouvre rien.
func TestUnMauvaisCodeNOuvreRien(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	secret := enrol(t, database, "u1")

	challenge := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	w := call(r, http.MethodPost, "/auth/mfa/verify", challenge,
		map[string]string{"code": "000000"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("statut = %d pour un code faux", w.Code)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("access_token")) {
		t.Fatal("une session est délivrée malgré un code faux")
	}
	_ = secret

	// L'échec doit laisser une trace : c'est le signal d'une tentative sur un
	// compte dont le mot de passe est déjà connu de quelqu'un.
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM security_events WHERE event_type='mfa_failed'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d trace(s) d'échec, attendu 1", n)
	}
}

// Un code ne sert qu'une fois : sans cela, un code lu par-dessus l'épaule reste
// utilisable pendant toute sa minute de validité.
func TestUnCodeDejaUtiliseEstRefuseALaConnexionSuivante(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	secret := enrol(t, database, "u1")

	code, _ := mfa.Code(secret, time.Now())

	challenge := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	if w := call(r, http.MethodPost, "/auth/mfa/verify", challenge,
		map[string]string{"code": code}); w.Code != http.StatusOK {
		t.Fatalf("premier usage refusé: %d — %s", w.Code, w.Body.String())
	}

	challenge2 := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	if w := call(r, http.MethodPost, "/auth/mfa/verify", challenge2,
		map[string]string{"code": code}); w.Code == http.StatusOK {
		t.Fatal("le même code a ouvert une deuxième session")
	}
}

// Le téléphone perdu : un code de secours doit ouvrir, et ne servir qu'une fois.
func TestUnCodeDeSecoursOuvreUneFoisEtUneSeule(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	// Passer par la vraie confirmation pour obtenir de vrais codes de secours.
	w := call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	token, _ := decode(t, w)["access_token"].(string)
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	secret, _ := decode(t, w)["secret"].(string)
	code, _ := mfa.Code(secret, time.Now())
	w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": code})
	codesAny, _ := decode(t, w)["recovery_codes"].([]any)
	if len(codesAny) == 0 {
		t.Fatalf("aucun code de secours: %s", w.Body.String())
	}
	secours, _ := codesAny[0].(string)

	challenge := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	if w := call(r, http.MethodPost, "/auth/mfa/verify", challenge,
		map[string]string{"code": secours}); w.Code != http.StatusOK {
		t.Fatalf("le code de secours n'ouvre pas: %d — %s", w.Code, w.Body.String())
	}

	challenge2 := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	if w := call(r, http.MethodPost, "/auth/mfa/verify", challenge2,
		map[string]string{"code": secours}); w.Code == http.StatusOK {
		t.Fatal("le même code de secours a servi deux fois")
	}

	// L'usage d'un code de secours est tracé : il signale un téléphone perdu ou
	// une reprise d'accès, deux choses qu'on veut voir.
	var n int
	database.QueryRow(
		`SELECT COUNT(*) FROM security_events WHERE event_type='mfa_recovery_code_used'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d trace(s) d'usage d'un code de secours, attendu 1", n)
	}
}

// Les codes de secours sont hachés : quelqu'un qui lit la base ne doit pas y
// trouver de quoi contourner le second facteur.
func TestLesCodesDeSecoursSontHachesEnBase(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	w := call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	token, _ := decode(t, w)["access_token"].(string)
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	secret, _ := decode(t, w)["secret"].(string)
	code, _ := mfa.Code(secret, time.Now())
	w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": code})
	codesAny, _ := decode(t, w)["recovery_codes"].([]any)

	rows, err := database.Query(`SELECT code_hash FROM mfa_recovery_codes`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var stored []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, h)
	}
	if len(stored) != RecoveryCodeCount {
		t.Fatalf("%d empreinte(s) stockée(s)", len(stored))
	}
	for _, c := range codesAny {
		clair, _ := c.(string)
		for _, h := range stored {
			if h == clair {
				t.Fatal("un code de secours est stocké en clair")
			}
		}
	}
}

// Le retrait demande le mot de passe : sans cela, un poste laissé ouvert
// suffirait à désactiver la protection.
func TestLeRetraitDuSecondFacteurExigeLeMotDePasse(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	secret := enrol(t, database, "u1")

	challenge := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	code, _ := mfa.Code(secret, time.Now())
	w := call(r, http.MethodPost, "/auth/mfa/verify", challenge, map[string]string{"code": code})
	token, _ := decode(t, w)["access_token"].(string)
	if token == "" {
		t.Fatalf("pas de session: %s", w.Body.String())
	}

	if w := call(r, http.MethodDelete, "/auth/mfa", token,
		map[string]string{"password": "MauvaisMotDePasse1"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("retrait accepté avec un mauvais mot de passe: %d", w.Code)
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM user_mfa WHERE user_id='u1'`).Scan(&n)
	if n != 1 {
		t.Fatal("le second facteur a été retiré malgré le mauvais mot de passe")
	}

	if w := call(r, http.MethodDelete, "/auth/mfa", token,
		map[string]string{"password": "MotDePasseSolide1"}); w.Code != http.StatusOK {
		t.Fatalf("retrait refusé avec le bon mot de passe: %d — %s", w.Code, w.Body.String())
	}
	database.QueryRow(`SELECT COUNT(*) FROM user_mfa WHERE user_id='u1'`).Scan(&n)
	if n != 0 {
		t.Fatal("le second facteur survit à son retrait")
	}
}

// Un compte désactivé entre les deux étapes ne doit pas obtenir de session :
// cinq minutes peuvent passer, et il serait absurde qu'une session naisse avec
// des droits périmés.
func TestUnCompteDesactiveEntreLesDeuxEtapesNObtientRien(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	secret := enrol(t, database, "u1")

	challenge := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	if _, err := database.Exec(`UPDATE users SET is_active = 0 WHERE id='u1'`); err != nil {
		t.Fatal(err)
	}
	code, _ := mfa.Code(secret, time.Now())
	w := call(r, http.MethodPost, "/auth/mfa/verify", challenge, map[string]string{"code": code})
	if w.Code == http.StatusOK {
		t.Fatalf("une session est née pour un compte désactivé: %s", w.Body.String())
	}
}

// L'étape de vérification n'accepte QUE le jeton d'attente : présenter une
// session complète permettrait de consommer un code de secours depuis une
// session ordinaire.
func TestLaVerificationRefuseUnJetonDeSessionComplet(t *testing.T) {
	database, r, cfg := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	enrol(t, database, "u1")

	complet, err := security.GenerateAccessToken(cfg.JWTSecret, "u1", false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if w := call(r, http.MethodPost, "/auth/mfa/verify", complet,
		map[string]string{"code": "123456"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("statut = %d pour un jeton de session complet", w.Code)
	}
}

// L'état rend l'obligation d'après le rôle EN BASE, pas d'après le jeton : un
// jeton figé à la connexion peut avoir une heure de retard.
func TestLEtatLitLObligationDansLaBase(t *testing.T) {
	database, r, cfg := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	// Jeton émis pour un compte non-administrateur, puis promotion en base.
	token, err := security.GenerateAccessToken(cfg.JWTSecret, "u1", false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE users SET role='admin', is_admin=1 WHERE id='u1'`); err != nil {
		t.Fatal(err)
	}

	w := call(r, http.MethodGet, "/auth/mfa", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("statut = %d: %s", w.Code, w.Body.String())
	}
	if req, _ := decode(t, w)["required_for_this_role"].(bool); !req {
		t.Fatal("l'obligation est lue dans le jeton et non dans la base")
	}
}

// ─── outils ──────────────────────────────────────────────────────────────────

// enrol pose une inscription confirmée directement en base, pour les épreuves
// qui portent sur la connexion et non sur l'assistant.
func enrol(t *testing.T, database *sql.DB, userID string) string {
	t.Helper()
	secret, err := mfa.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO user_mfa (user_id, secret, confirmed_at, last_window)
		 VALUES (?, ?, ?, 0)`, userID, secret, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return secret
}

func loginChallenge(t *testing.T, r *gin.Engine, email, password string) string {
	t.Helper()
	w := call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": email, "password": password})
	if w.Code != http.StatusOK {
		t.Fatalf("connexion: statut %d — %s", w.Code, w.Body.String())
	}
	tok, _ := decode(t, w)["mfa_token"].(string)
	if tok == "" {
		t.Fatalf("aucun jeton d'attente: %s", w.Body.String())
	}
	return tok
}

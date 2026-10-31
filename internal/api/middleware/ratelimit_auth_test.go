package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// Audit croisé Gemini, CRIT-13 : le second facteur se devinait sans limite.
//
// `/auth/login` et `/auth/mfa/verify` partageaient UN limiteur, compté par adresse
// et remis à zéro sur tout succès. Un `login` réussi (200) tous les neuf codes
// faux ramenait le compteur à zéro : 120 codes traités sur 120, aucun 429.

const secretDeTest = "secret-de-test-des-limiteurs-0123456789"

// routesAuth monte les routes comme le serveur : mêmes chaînes (AuthLimiters), les
// gestionnaires étant remplacés par des doublures qui n'ont que la réponse du vrai.
//
//   - /login : 200 si le mot de passe est « bon », 401 sinon. Un compte « admin@x » a
//     un mot de passe que personne ne connaît ; « membre@x » connaît le sien.
//   - /mfa/verify : 401 « code incorrect » sauf le code « 123456 » (200).
func routesAuth(t *testing.T) (*gin.Engine, *AuthLimiters) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	lim := NewAuthLimiters(nil)
	r := gin.New()
	r.POST("/login", append(lim.ForLogin(), func(c *gin.Context) {
		var b struct{ Email, Password string }
		if err := c.ShouldBindJSON(&b); err != nil {
			c.Status(http.StatusUnprocessableEntity) // le corps doit arriver intact
			return
		}
		if b.Email == "membre@x" && b.Password == "bon" {
			c.Status(http.StatusOK)
			return
		}
		c.Status(http.StatusUnauthorized)
	})...)
	r.POST("/mfa/verify", append(lim.ForMFAVerify(secretDeTest), func(c *gin.Context) {
		var b struct{ Code string }
		_ = c.ShouldBindJSON(&b)
		if b.Code == "123456" {
			c.Status(http.StatusOK)
			return
		}
		c.Status(http.StatusUnauthorized)
	})...)
	return r, lim
}

func envoyer(r *gin.Engine, chemin, ip, jeton string, corps any) int {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(corps)
	req := httptest.NewRequest(http.MethodPost, chemin, &buf)
	req.Header.Set("Content-Type", "application/json")
	if jeton != "" {
		req.Header.Set("Authorization", "Bearer "+jeton)
	}
	req.RemoteAddr = ip + ":4000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func jetonDAttente(t *testing.T, compte string) string {
	t.Helper()
	jeton, err := security.GenerateMFAChallengeToken(secretDeTest, compte, false)
	if err != nil {
		t.Fatal(err)
	}
	return jeton
}

// Le scénario de l'audit : un `login` réussi entre les essais de code. Cinq codes
// faux ferment la voie, quoi qu'il se passe à côté.
func TestUnLoginReussiNeRemetPasLeSecondFacteurAZero(t *testing.T) {
	r, _ := routesAuth(t)
	traites := 0
	for i := 0; i < 120; i++ {
		// Le mot de passe est connu : l'attaquant en redemande un jeton d'attente à
		// chaque fois, et chaque `login` réussit.
		if got := envoyer(r, "/login", "192.0.2.7", "", map[string]string{"email": "membre@x", "password": "bon"}); got != http.StatusOK {
			t.Fatalf("login %d : %d, attendu 200", i, got)
		}
		got := envoyer(r, "/mfa/verify", "192.0.2.7", jetonDAttente(t, "membre"), map[string]string{"code": "000000"})
		if got == http.StatusUnauthorized {
			traites++
		}
	}
	if traites != MFAMaxAttempts {
		t.Fatalf("%d codes faux traités sur 120, attendu %d — un login réussi a remis le compteur à zéro", traites, MFAMaxAttempts)
	}
}

// Changer d'adresse ne donne pas de nouveaux essais : le compteur est celui du COMPTE.
func TestLeSecondFacteurCompteParCompteEtNonParAdresse(t *testing.T) {
	r, _ := routesAuth(t)
	jeton := jetonDAttente(t, "membre")
	for i := 0; i < MFAMaxAttempts; i++ {
		envoyer(r, "/mfa/verify", "192.0.2.1", jeton, map[string]string{"code": "000000"})
	}
	if got := envoyer(r, "/mfa/verify", "198.51.100.9", jeton, map[string]string{"code": "000000"}); got != http.StatusTooManyRequests {
		t.Fatalf("autre adresse : %d, attendu 429 — le compteur est resté propre à l'adresse", got)
	}
	// Même le BON code est refusé tant que la voie est fermée : sinon on pourrait
	// continuer à deviner en regardant si la réponse change.
	if got := envoyer(r, "/mfa/verify", "198.51.100.9", jeton, map[string]string{"code": "123456"}); got != http.StatusTooManyRequests {
		t.Fatalf("bon code sur une voie fermée : %d, attendu 429", got)
	}
}

// Le verrouillage d'un compte n'en touche pas un autre.
func TestLeVerrouDUnCompteNeToucheQueLui(t *testing.T) {
	r, _ := routesAuth(t)
	for i := 0; i < MFAMaxAttempts; i++ {
		envoyer(r, "/mfa/verify", "192.0.2.1", jetonDAttente(t, "u1"), map[string]string{"code": "000000"})
	}
	if got := envoyer(r, "/mfa/verify", "192.0.2.1", jetonDAttente(t, "u1"), map[string]string{"code": "000000"}); got != http.StatusTooManyRequests {
		t.Fatalf("u1 : %d, attendu 429", got)
	}
	if got := envoyer(r, "/mfa/verify", "192.0.2.1", jetonDAttente(t, "u2"), map[string]string{"code": "123456"}); got != http.StatusOK {
		t.Fatalf("u2 : %d, attendu 200 — le verrou de u1 a débordé", got)
	}
}

// Un code juste efface les échecs de CE compte : quatre fautes de frappe, le bon
// code, puis quatre autres fautes ne ferment rien.
func TestUnCodeJusteEffaceLesEchecsDuCompte(t *testing.T) {
	r, _ := routesAuth(t)
	for tour := 0; tour < 3; tour++ {
		for i := 0; i < MFAMaxAttempts-1; i++ {
			if got := envoyer(r, "/mfa/verify", "192.0.2.1", jetonDAttente(t, "membre"), map[string]string{"code": "000000"}); got != http.StatusUnauthorized {
				t.Fatalf("tour %d, faute %d : %d, attendu 401", tour, i, got)
			}
		}
		if got := envoyer(r, "/mfa/verify", "192.0.2.1", jetonDAttente(t, "membre"), map[string]string{"code": "123456"}); got != http.StatusOK {
			t.Fatalf("tour %d, bon code : %d, attendu 200", tour, got)
		}
	}
}

// Sans jeton valide, rien n'est compté : la requête est refusée plus tôt et ne peut
// tenter aucun code. Elle ne doit pas non plus fermer la voie de quelqu'un d'autre.
func TestUneRequeteSansJetonNeFermeLaVoiePourPersonne(t *testing.T) {
	r, _ := routesAuth(t)
	for i := 0; i < 50; i++ {
		if got := envoyer(r, "/mfa/verify", "192.0.2.1", "jeton-invalide", map[string]string{"code": "000000"}); got != http.StatusUnauthorized {
			t.Fatalf("jeton invalide : %d, attendu 401", got)
		}
	}
	if got := envoyer(r, "/mfa/verify", "192.0.2.1", jetonDAttente(t, "membre"), map[string]string{"code": "123456"}); got != http.StatusOK {
		t.Fatalf("après 50 jetons invalides : %d, attendu 200", got)
	}
}

// Le même défaut sur le mot de passe : un utilisateur ordinaire remettait à zéro le
// compteur dont il se servait pour deviner le mot de passe de l'administrateur —
// un `login` réussi sur SON compte entre deux essais sur l'autre.
func TestUnSuccesSurUnCompteNEffacePasLesEchecsSurUnAutre(t *testing.T) {
	r, _ := routesAuth(t)
	traites := 0
	for i := 0; i < 120; i++ {
		if got := envoyer(r, "/login", "192.0.2.50", "", map[string]string{"email": "membre@x", "password": "bon"}); got != http.StatusOK {
			t.Fatalf("connexion du membre %d : %d, attendu 200", i, got)
		}
		if envoyer(r, "/login", "192.0.2.50", "", map[string]string{"email": "admin@x", "password": "essai"}) == http.StatusUnauthorized {
			traites++
		}
	}
	if traites != DefaultLoginMaxAttempts {
		t.Fatalf("%d mots de passe faux traités sur 120 contre l'administrateur, attendu %d", traites, DefaultLoginMaxAttempts)
	}
}

// Le verrou du couple ne ferme pas le compte à son propriétaire, qui écrit depuis
// une autre adresse : sans cela, n'importe qui sur le réseau local mettrait
// l'administrateur dehors en tapant son adresse électronique.
func TestLeVerrouDuCoupleNeFermePasLeCompteALAutreAdresse(t *testing.T) {
	r, _ := routesAuth(t)
	for i := 0; i < DefaultLoginMaxAttempts+3; i++ {
		envoyer(r, "/login", "192.0.2.66", "", map[string]string{"email": "membre@x", "password": "faux"})
	}
	if got := envoyer(r, "/login", "192.0.2.66", "", map[string]string{"email": "membre@x", "password": "bon"}); got != http.StatusTooManyRequests {
		t.Fatalf("l'adresse de l'attaquant : %d, attendu 429", got)
	}
	if got := envoyer(r, "/login", "192.0.2.5", "", map[string]string{"email": "membre@x", "password": "bon"}); got != http.StatusOK {
		t.Fatalf("le propriétaire, depuis son poste : %d, attendu 200", got)
	}
}

// Majuscules, espaces et adresse démesurée : aucun moyen d'échapper au plafond en
// variant l'écriture du compte visé.
func TestLaCleDuCompteNEstPasContournableParLEcriture(t *testing.T) {
	r, _ := routesAuth(t)
	variantes := []string{"admin@x", "ADMIN@x", " admin@x ", "Admin@X"}
	traites := 0
	for i := 0; i < 40; i++ {
		if envoyer(r, "/login", "192.0.2.77", "", map[string]string{"email": variantes[i%len(variantes)], "password": "essai"}) == http.StatusUnauthorized {
			traites++
		}
	}
	if traites != DefaultLoginMaxAttempts {
		t.Fatalf("%d essais traités avec des écritures variées, attendu %d", traites, DefaultLoginMaxAttempts)
	}

	long := strings.Repeat("a", emailMax)
	traites = 0
	for i := 0; i < 40; i++ {
		// Même préfixe de 254 caractères, suffixes différents : un seul compte visé.
		if envoyer(r, "/login", "192.0.2.78", "", map[string]string{"email": long + string(rune('a'+i%26)), "password": "essai"}) == http.StatusUnauthorized {
			traites++
		}
	}
	if traites != DefaultLoginMaxAttempts {
		t.Fatalf("%d essais traités avec des adresses démesurées, attendu %d", traites, DefaultLoginMaxAttempts)
	}
}

// Un corps illisible n'est pas compté (le gestionnaire le refuse) et n'empêche pas
// le gestionnaire de répondre.
func TestUnCorpsIllisibleNEstPasCompte(t *testing.T) {
	r, _ := routesAuth(t)
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("{pas du json"))
		req.RemoteAddr = "192.0.2.90:1"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("corps illisible : %d, attendu 422", w.Code)
		}
	}
	if got := envoyer(r, "/login", "192.0.2.90", "", map[string]string{"email": "membre@x", "password": "bon"}); got != http.StatusOK {
		t.Fatalf("après 50 corps illisibles : %d, attendu 200", got)
	}
}

// Ce que faisait le défaut, gardé comme contre-exemple : le limiteur unique, partagé
// et remis à zéro sur tout succès, se laisse contourner. Si ce test cesse de passer,
// c'est que le limiteur de base a changé de nature — et la structure d'AuthLimiters
// est à reconsidérer, pas ce test à supprimer.
func TestLeLimiteurPartageSeContourneraitParUnLoginReussi(t *testing.T) {
	gin.SetMode(gin.TestMode)
	partage := NewLoginRateLimiter(10, time.Minute, time.Hour)
	r := gin.New()
	r.POST("/login", partage.Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/mfa/verify", partage.Middleware(), func(c *gin.Context) { c.Status(http.StatusUnauthorized) })

	traites := 0
	for i := 0; i < 120; i++ {
		if i%9 == 0 {
			envoyer(r, "/login", "192.0.2.7", "", map[string]string{"email": "membre@x"})
		}
		if envoyer(r, "/mfa/verify", "192.0.2.7", "", map[string]string{"code": "000000"}) == http.StatusUnauthorized {
			traites++
		}
	}
	if traites <= 10 {
		t.Fatalf("%d codes traités : le contre-exemple ne contourne plus rien", traites)
	}
}

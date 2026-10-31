package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/mfa"
)

// Audit croisé Gemini, CRIT-13, de bout en bout : vrais gestionnaires, vraie base,
// limiteurs montés par la fonction du serveur.
//
// Le scénario de l'audit : le mot de passe est connu, un `login` réussi est
// intercalé entre les essais de code. Il remettait le compteur à zéro — 120 codes
// faux traités sur 120. Cinq codes faux ferment désormais la voie du COMPTE, et
// aucun `login` ne la rouvre.
func TestLeSecondFacteurSeFermeMalgreDesLoginsReussisIntercales(t *testing.T) {
	database, r, _ := mfaEnvOpt(t, true)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	// Inscription du second facteur.
	w := call(r, http.MethodPost, "/auth/login", "", map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	token, _ := decode(t, w)["access_token"].(string)
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	secret, _ := decode(t, w)["secret"].(string)
	juste, _ := mfa.Code(secret, time.Now())
	if w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": juste}); w.Code != http.StatusOK {
		t.Fatalf("inscription : %d — %s", w.Code, w.Body.String())
	}

	// Un code certainement faux : à l'opposé du code juste sur le cadran.
	n, _ := strconv.Atoi(juste)
	faux := fmt.Sprintf("%06d", (n+500000)%1000000)

	traites := 0
	for i := 0; i < 30; i++ {
		// `loginChallenge` échoue le test si le login n'est pas un 200 : chaque
		// tour est bien précédé d'une connexion réussie.
		jeton := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
		got := call(r, http.MethodPost, "/auth/mfa/verify", jeton, map[string]string{"code": faux}).Code
		if got == http.StatusUnauthorized {
			traites++
		}
	}
	if traites != mw.MFAMaxAttempts {
		t.Fatalf("%d codes faux traités sur 30, attendu %d", traites, mw.MFAMaxAttempts)
	}

	// La voie est fermée pour le compte : le BON code lui-même est refusé, avec un
	// délai d'attente que l'interface sait afficher.
	jeton := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
	suivant, _ := mfa.Code(secret, time.Now().Add(30*time.Second))
	w = call(r, http.MethodPost, "/auth/mfa/verify", jeton, map[string]string{"code": suivant})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("bon code sur une voie fermée : %d, attendu 429 — %s", w.Code, w.Body.String())
	}
	if ra, _ := decode(t, w)["retry_after"].(float64); ra < 1 {
		t.Errorf("la réponse 429 ne dit pas combien attendre : %s", w.Body.String())
	}

	// La fermeture est tracée une fois, au franchissement du seuil — pas à chaque refus.
	var nb int
	var detail string
	database.QueryRow(`SELECT COUNT(*) FROM security_events WHERE event_type = 'mfa_lockout'`).Scan(&nb)
	database.QueryRow(`SELECT COALESCE(detail,'') FROM security_events WHERE event_type = 'mfa_lockout'`).Scan(&detail)
	if nb != 1 {
		t.Fatalf("%d trace(s) de verrouillage du second facteur, attendu 1", nb)
	}
	if !strings.Contains(detail, "compte=u1") || !strings.Contains(detail, "locked until") {
		t.Errorf("la trace ne dit pas quel compte est fermé, ni jusqu'à quand : %q", detail)
	}
}

// Un verrouillage par adresse + compte est tracé avec les deux, sans mélanger l'adresse
// et l'adresse électronique.
func TestLeVerrouDuMotDePasseTraceLAdresseEtLeCompte(t *testing.T) {
	database, r, _ := mfaEnvOpt(t, true)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)
	for i := 0; i < mw.DefaultLoginMaxAttempts; i++ {
		call(r, http.MethodPost, "/auth/login", "", map[string]string{"email": "u1@t.ch", "password": "faux"})
	}
	var nb int
	database.QueryRow(`SELECT COUNT(*) FROM security_events WHERE event_type = 'login_lockout' AND detail LIKE '%compte=u1@t.ch%'`).Scan(&nb)
	if nb != 1 {
		t.Fatalf("%d trace(s) de verrouillage par compte, attendu 1", nb)
	}
	var ip string
	database.QueryRow(`SELECT COALESCE(ip_address,'') FROM security_events WHERE detail LIKE '%compte=u1@t.ch%'`).Scan(&ip)
	if strings.Contains(ip, "|") || strings.Contains(ip, "u1@t.ch") {
		t.Errorf("l'adresse notée contient le compte : %q", ip)
	}
}

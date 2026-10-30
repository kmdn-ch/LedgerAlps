package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/mfa"
)

func TestLePlafondDesCodesDeSecoursSeFermePuisSeRouvre(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	th := newRecoveryThrottle(3, 15*time.Minute)
	th.now = func() time.Time { return now }

	for i := 1; i <= 3; i++ {
		if th.blocked("u") {
			t.Fatalf("fermé avant l'échec n° %d", i)
		}
		if closed := th.fail("u"); closed != (i == 3) {
			t.Fatalf("échec n° %d : fail() = %v, ne doit rendre true qu'au dernier", i, closed)
		}
	}
	if !th.blocked("u") {
		t.Fatal("trois échecs n'ont pas fermé la voie")
	}
	if th.blocked("autre") {
		t.Fatal("le plafond d'un compte ferme celui d'un autre")
	}

	now = now.Add(16 * time.Minute)
	if th.blocked("u") {
		t.Fatal("la voie reste fermée après la fenêtre")
	}

	// Une réussite efface les échecs précédents.
	th.fail("u")
	th.fail("u")
	th.reset("u")
	th.fail("u")
	if th.blocked("u") {
		t.Fatal("les échecs d'avant la réussite sont comptés")
	}
}

// La valeur nulle ne limite rien : un AuthHandler construit sans passer par
// NewAuthHandler garde l'ancien comportement, sans panique.
func TestUnPlafondNulNeLimiteRien(t *testing.T) {
	var th *recoveryThrottle
	if th.blocked("u") || th.fail("u") {
		t.Fatal("un plafond nul a limité")
	}
	th.reset("u")
}

// Le défaut : un code de secours erroné se comparait aux dix codes du compte,
// près d'une seconde de calcul par essai, sans plafond par compte. Après cinq
// échecs la voie se ferme — même pour le BON code, qui n'est plus essayé —
// l'événement est tracé une fois, et le code TOTP continue d'ouvrir.
func TestCinqCodesDeSecoursRatesFermentLaVoie(t *testing.T) {
	database, r, _ := mfaEnv(t)
	seedLogin(t, database, "u1", "MotDePasseSolide1", false)

	w := call(r, http.MethodPost, "/auth/login", "",
		map[string]string{"email": "u1@t.ch", "password": "MotDePasseSolide1"})
	token, _ := decode(t, w)["access_token"].(string)
	w = call(r, http.MethodPost, "/auth/mfa/setup", token, nil)
	secret, _ := decode(t, w)["secret"].(string)
	code, _ := mfa.Code(secret, time.Now())
	w = call(r, http.MethodPost, "/auth/mfa/confirm", token, map[string]string{"code": code})
	codes, _ := decode(t, w)["recovery_codes"].([]any)
	bon, _ := codes[0].(string)

	verifie := func(c string) int {
		ch := loginChallenge(t, r, "u1@t.ch", "MotDePasseSolide1")
		return call(r, http.MethodPost, "/auth/mfa/verify", ch, map[string]string{"code": c}).Code
	}

	for i := 1; i <= recoveryMaxFailures; i++ {
		if got := verifie("ZZZZZ-ZZZZZ"); got != http.StatusUnauthorized {
			t.Fatalf("essai raté n° %d : %d, attendu 401", i, got)
		}
	}

	// Voie fermée : le bon code lui-même n'est plus essayé.
	if got := verifie(bon); got != http.StatusUnauthorized {
		t.Fatalf("code de secours valide accepté sur une voie fermée : %d", got)
	}

	var n int
	database.QueryRow(
		`SELECT COUNT(*) FROM security_events WHERE event_type='mfa_recovery_throttled'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d trace(s) de fermeture, attendu 1", n)
	}

	// Le code TOTP, lui, ouvre toujours : la connexion normale n'est pas bloquée.
	totp, _ := mfa.Code(secret, time.Now().Add(30*time.Second))
	if got := verifie(totp); got != http.StatusOK {
		t.Fatalf("le code TOTP est refusé alors que seule la voie de secours est fermée : %d", got)
	}
}

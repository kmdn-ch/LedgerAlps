package middleware

// L'échelle des verrouillages.
//
// # Ce que ces tests protègent, et ce qu'ils ne protègent pas
//
// Ils ne mesurent pas « la sécurité ». Ils tiennent une RÈGLE énoncée : dix
// échecs verrouillent, et l'attente s'allonge à chaque nouvelle série — 30 s,
// 1 min, 5 min, 15 min, 1 h. Cette règle est ce qui fait la différence entre
// gêner un humain distrait et ruiner un automate : trente secondes ne coûtent
// presque rien à l'un et divisent par mille la cadence de l'autre.
//
// Le temps est injecté. Un test qui attendrait réellement une heure ne serait
// jamais exécuté, donc ne protégerait rien.

import (
	"net/http"
	"testing"
	"time"
)

// horloge pilotée à la main.
type horloge struct{ t time.Time }

func (h *horloge) maintenant() time.Time  { return h.t }
func (h *horloge) avance(d time.Duration) { h.t = h.t.Add(d) }

func limiteurPilote(t *testing.T) (*LoginRateLimiter, *horloge) {
	t.Helper()
	h := &horloge{t: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)}
	l := NewLoginRateLimiter(DefaultLoginMaxAttempts, DefaultLoginWindow, DefaultLoginPaliers...)
	l.now = h.maintenant
	return l, h
}

// echouer joue n échecs et rend le code de la dernière requête.
func echouer(t *testing.T, l *LoginRateLimiter, n int) int {
	t.Helper()
	r := newTestRouter(l, func() int { return http.StatusUnauthorized })
	code := 0
	for i := 0; i < n; i++ {
		code = post(r).Code
	}
	return code
}

// Neuf échecs ne verrouillent pas. Le dixième, si. C'est le seuil énoncé, et
// se tromper d'un cran punirait quelqu'un qui a droit à sa dixième tentative.
func TestDixEchecsVerrouillentPasNeuf(t *testing.T) {
	l, _ := limiteurPilote(t)

	if code := echouer(t, l, 9); code != http.StatusUnauthorized {
		t.Fatalf("après 9 échecs, code = %d — le verrou est tombé trop tôt", code)
	}
	if code := echouer(t, l, 1); code != http.StatusUnauthorized {
		t.Fatalf("le 10e échec doit être traité, code = %d", code)
	}
	// Le 11e appel se heurte au verrou.
	if code := echouer(t, l, 1); code != http.StatusTooManyRequests {
		t.Fatalf("après 10 échecs, code = %d, attendu 429", code)
	}
}

// LE test : l'échelle. Chaque série de dix échecs coûte plus cher que la
// précédente, et le dernier barreau se répète au lieu de croître sans fin.
func TestLAttenteSAllongeAChaqueSerie(t *testing.T) {
	l, h := limiteurPilote(t)

	attendus := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		1 * time.Hour,
		1 * time.Hour, // le dernier barreau se répète
	}

	for i, attendu := range attendus {
		echouer(t, l, DefaultLoginMaxAttempts)

		reste, verrouille := l.locked("192.0.2.10")
		if !verrouille {
			t.Fatalf("série %d : aucun verrou après %d échecs", i+1, DefaultLoginMaxAttempts)
		}
		if reste != attendu {
			t.Errorf("série %d : attente = %s, attendu %s", i+1, reste, attendu)
		}

		// On laisse le verrou expirer, sans laisser passer assez de temps pour
		// que l'échelle redescende.
		h.avance(attendu + time.Second)
		if _, encore := l.locked("192.0.2.10"); encore {
			t.Fatalf("série %d : le verrou n'a pas expiré", i+1)
		}
	}
}

// Une longue accalmie ramène l'échelle à son premier barreau. Sans cet oubli,
// quelqu'un qui s'est trompé dix fois un mardi retrouverait une heure d'attente
// au premier faux pas du mois suivant.
func TestUneAccalmieRamèneLEchelleAuDebut(t *testing.T) {
	l, h := limiteurPilote(t)

	echouer(t, l, DefaultLoginMaxAttempts) // 1er verrou : 30 s
	h.avance(31 * time.Second)

	echouer(t, l, DefaultLoginMaxAttempts) // 2e verrou : 1 min
	if reste, _ := l.locked("192.0.2.10"); reste != time.Minute {
		t.Fatalf("2e série : attente = %s, attendu 1m", reste)
	}

	// Plus d'une heure sans le moindre échec.
	h.avance(escaladeOubli + 2*time.Minute)

	echouer(t, l, DefaultLoginMaxAttempts)
	if reste, _ := l.locked("192.0.2.10"); reste != 30*time.Second {
		t.Errorf("après l'accalmie : attente = %s, attendu 30s", reste)
	}
}

// Une connexion réussie efface tout, échelle comprise. Se souvenir des échecs
// de quelqu'un qui vient de prouver son identité n'a aucun sens.
func TestUneConnexionReussieEffaceLEchelle(t *testing.T) {
	l, h := limiteurPilote(t)

	echouer(t, l, DefaultLoginMaxAttempts) // 30 s
	h.avance(31 * time.Second)

	ok := newTestRouter(l, func() int { return http.StatusOK })
	if code := post(ok).Code; code != http.StatusOK {
		t.Fatalf("connexion réussie refusée : %d", code)
	}

	echouer(t, l, DefaultLoginMaxAttempts)
	if reste, _ := l.locked("192.0.2.10"); reste != 30*time.Second {
		t.Errorf("après une réussite : attente = %s, attendu 30s", reste)
	}
}

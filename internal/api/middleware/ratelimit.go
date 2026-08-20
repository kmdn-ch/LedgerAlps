package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Defaults for authentication endpoints. Chosen to stop credential stuffing
// without locking out a legitimate user who mistypes a password a few times.
const (
	DefaultLoginMaxAttempts = 10
	DefaultLoginWindow      = 15 * time.Minute

	// staleAfter is how long an idle record is kept before being purged.
	staleAfter = time.Hour
	// purgeEvery bounds how often the opportunistic sweep runs.
	purgeEvery = 10 * time.Minute

	// escaladeOubli : après ce temps sans nouvel échec, l'échelle redescend à
	// son premier barreau. Sans cet oubli, quelqu'un qui s'est trompé dix fois
	// un mardi retrouverait une heure d'attente au premier faux pas du mois
	// suivant — une punition que rien ne justifie.
	escaladeOubli = time.Hour
)

// DefaultLoginPaliers est l'échelle des verrouillages successifs.
//
// # Pourquoi une échelle plutôt qu'une durée fixe
//
// Une durée fixe traite de la même façon les deux populations qui se trompent :
// celle qui a mal tapé son mot de passe, et celle qui en essaie des milliers.
// La première se reconnaît à ce qu'elle s'arrête ; la seconde revient. Trente
// secondes ne gênent presque pas un humain et ruinent déjà un automate, qui
// passe de quelques milliers d'essais par minute à vingt par dix minutes. Les
// barreaux suivants achèvent de rendre l'exercice sans intérêt.
//
// Le dernier barreau se répète indéfiniment : au-delà d'une heure, allonger
// encore ne protège plus de rien et enfermerait dehors quelqu'un qui a
// simplement oublié son mot de passe.
var DefaultLoginPaliers = []time.Duration{
	30 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
}

// LoginRateLimiter throttles repeated failed authentication attempts from the
// same client address.
//
// LedgerAlps ships as a single local-first binary, so the state is deliberately
// in-memory: there is no external store to coordinate with, and a restart
// clearing the counters is acceptable for the threat model (an attacker cannot
// trigger a restart remotely).
//
// A client is locked out once it accumulates maxAttempts failures inside
// window. Any successful authentication clears its record immediately.
type LoginRateLimiter struct {
	mu        sync.Mutex
	records   map[string]*attemptRecord
	lastPurge time.Time

	maxAttempts int
	window      time.Duration
	// paliers : la durée du 1er, 2e, … verrouillage d'un même client. Le
	// dernier vaut pour tous les suivants.
	paliers []time.Duration
	now     func() time.Time // injectable for tests

	// onLockout, when set, is invoked once each time a client crosses the
	// threshold. It lets the caller persist the event without giving this
	// package a database dependency. Never called while holding the mutex.
	onLockout func(ip string, until time.Time)
}

// OnLockout registers a callback fired when a client is locked out. Intended for
// security telemetry; it must not block, as it runs on the request path.
func (l *LoginRateLimiter) OnLockout(fn func(ip string, until time.Time)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLockout = fn
}

type attemptRecord struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
	lastSeen    time.Time
	// verrous compte les verrouillages déjà subis : c'est lui qui choisit le
	// barreau de l'échelle. Remis à zéro par une connexion réussie, ou par
	// `escaladeOubli` de calme.
	verrous int
}

// NewLoginRateLimiter builds a limiter. Non-positive or empty arguments fall
// back to the package defaults.
func NewLoginRateLimiter(maxAttempts int, window time.Duration, paliers ...time.Duration) *LoginRateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = DefaultLoginMaxAttempts
	}
	if window <= 0 {
		window = DefaultLoginWindow
	}
	if len(paliers) == 0 {
		paliers = DefaultLoginPaliers
	}
	return &LoginRateLimiter{
		records:     make(map[string]*attemptRecord),
		maxAttempts: maxAttempts,
		window:      window,
		paliers:     paliers,
		now:         time.Now,
	}
}

// Middleware rejects requests from locked-out clients with 503-style backpressure
// (429 + Retry-After), then inspects the response status to score the attempt.
//
// Scoring by status keeps the handler untouched: 401/403 count as failures,
// 2xx clears the record. Validation errors (422) and server faults (5xx) are
// ignored so a malformed body or an outage cannot lock a user out.
func (l *LoginRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()

		if retryAfter, locked := l.locked(key); locked {
			secs := int(retryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			c.Header("Retry-After", strconv.Itoa(secs))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many failed attempts, try again later",
				"retry_after": secs,
			})
			return
		}

		c.Next()

		switch status := c.Writer.Status(); {
		case status == http.StatusUnauthorized, status == http.StatusForbidden:
			l.recordFailure(key)
		case status >= 200 && status < 300:
			l.reset(key)
		}
	}
}

// locked reports whether the key is currently locked out and for how long.
func (l *LoginRateLimiter) locked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.purgeLocked(now)

	rec, ok := l.records[key]
	if !ok {
		return 0, false
	}
	rec.lastSeen = now
	if now.Before(rec.lockedUntil) {
		return rec.lockedUntil.Sub(now), true
	}
	return 0, false
}

// recordFailure increments the failure counter, starting a new window when the
// previous one has elapsed, and locks the key once the threshold is reached.
func (l *LoginRateLimiter) recordFailure(key string) {
	l.mu.Lock()

	now := l.now()
	rec, ok := l.records[key]
	if !ok {
		rec = &attemptRecord{windowStart: now}
		l.records[key] = rec
	}
	// Expired window (or one that ended with a lockout) starts fresh.
	if now.Sub(rec.windowStart) > l.window {
		rec.failures = 0
		rec.windowStart = now
	}
	// L'échelle redescend après une longue accalmie, mesurée depuis la FIN DU
	// VERROU et non depuis le dernier échec.
	//
	// La nuance décide de tout. Mesurée depuis le dernier échec, l'attente
	// passée dans le verrou compterait comme du calme : un automate verrouillé
	// une heure reviendrait à trente secondes, et l'échelle ne monterait
	// jamais au-delà du deuxième barreau. Il faut une heure de SILENCE APRÈS la
	// fin du verrou pour repartir de zéro.
	if rec.verrous > 0 && !rec.lockedUntil.IsZero() &&
		now.After(rec.lockedUntil) && now.Sub(rec.lockedUntil) > escaladeOubli {
		rec.verrous = 0
	}
	rec.failures++
	rec.lastSeen = now

	var (
		lockedNow bool
		until     time.Time
		notify    = l.onLockout
	)
	if rec.failures >= l.maxAttempts {
		rec.lockedUntil = now.Add(l.palier(rec.verrous))
		rec.verrous++
		rec.failures = 0
		rec.windowStart = now
		lockedNow, until = true, rec.lockedUntil
	}
	l.mu.Unlock()

	// Invoked outside the lock: the callback does I/O and must not stall
	// concurrent requests, nor deadlock if it re-enters the limiter.
	if lockedNow && notify != nil {
		notify(key, until)
	}
}

// palier rend la durée du n-ième verrouillage (n commence à 0). Au-delà de
// l'échelle, le dernier barreau se répète.
func (l *LoginRateLimiter) palier(n int) time.Duration {
	if n >= len(l.paliers) {
		n = len(l.paliers) - 1
	}
	return l.paliers[n]
}

// reset clears any record for the key after a successful authentication.
func (l *LoginRateLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, key)
}

// purgeLocked drops idle records so the map cannot grow without bound. The
// caller must hold l.mu.
//
// # Le piège, et il a coûté un test
//
// Un enregistrement porte AUSSI l'échelle des verrouillages déjà subis. Le
// balayage ne regardait que l'inactivité : après une heure de verrou, la
// dernière tentative datait de plus d'une heure, l'enregistrement partait, et
// l'échelle repartait de trente secondes. Autrement dit, le barreau le plus
// long effaçait lui-même la mémoire qui l'avait produit — un automate n'aurait
// jamais dépassé la première minute.
//
// On ne jette donc un enregistrement que lorsqu'il ne peut plus rien décider :
// verrou expiré, plus aucune tentative depuis longtemps, ET échelle déjà
// oubliée.
func (l *LoginRateLimiter) purgeLocked(now time.Time) {
	if now.Sub(l.lastPurge) < purgeEvery {
		return
	}
	l.lastPurge = now
	for k, rec := range l.records {
		if !now.After(rec.lockedUntil) || now.Sub(rec.lastSeen) <= staleAfter {
			continue
		}
		echelleOubliee := rec.verrous == 0 || now.Sub(rec.lockedUntil) > escaladeOubli
		if echelleOubliee {
			delete(l.records, k)
		}
	}
}

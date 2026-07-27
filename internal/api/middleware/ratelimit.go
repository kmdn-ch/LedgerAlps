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
	DefaultLoginMaxAttempts = 5
	DefaultLoginWindow      = 15 * time.Minute
	DefaultLoginLockout     = 15 * time.Minute

	// staleAfter is how long an idle record is kept before being purged.
	staleAfter = time.Hour
	// purgeEvery bounds how often the opportunistic sweep runs.
	purgeEvery = 10 * time.Minute
)

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
	lockout     time.Duration
	now         func() time.Time // injectable for tests

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
}

// NewLoginRateLimiter builds a limiter. Non-positive arguments fall back to the
// package defaults.
func NewLoginRateLimiter(maxAttempts int, window, lockout time.Duration) *LoginRateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = DefaultLoginMaxAttempts
	}
	if window <= 0 {
		window = DefaultLoginWindow
	}
	if lockout <= 0 {
		lockout = DefaultLoginLockout
	}
	return &LoginRateLimiter{
		records:     make(map[string]*attemptRecord),
		maxAttempts: maxAttempts,
		window:      window,
		lockout:     lockout,
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
	rec.failures++
	rec.lastSeen = now

	var (
		lockedNow bool
		until     time.Time
		notify    = l.onLockout
	)
	if rec.failures >= l.maxAttempts {
		rec.lockedUntil = now.Add(l.lockout)
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

// reset clears any record for the key after a successful authentication.
func (l *LoginRateLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, key)
}

// purgeLocked drops idle records so the map cannot grow without bound. The
// caller must hold l.mu.
func (l *LoginRateLimiter) purgeLocked(now time.Time) {
	if now.Sub(l.lastPurge) < purgeEvery {
		return
	}
	l.lastPurge = now
	for k, rec := range l.records {
		if now.After(rec.lockedUntil) && now.Sub(rec.lastSeen) > staleAfter {
			delete(l.records, k)
		}
	}
}

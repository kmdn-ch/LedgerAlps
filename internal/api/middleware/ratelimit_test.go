package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newTestRouter wires the limiter in front of a handler that returns the status
// codes supplied by next(), so tests can script a sequence of outcomes.
func newTestRouter(l *LoginRateLimiter, next func() int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/login", l.Middleware(), func(c *gin.Context) {
		c.JSON(next(), gin.H{})
	})
	return r
}

func post(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	r.ServeHTTP(w, req)
	return w
}

func TestLockoutAfterMaxFailures(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute, time.Minute)
	r := newTestRouter(l, func() int { return http.StatusUnauthorized })

	for i := 1; i <= 3; i++ {
		if got := post(r).Code; got != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i, got)
		}
	}
	// The 4th attempt must be refused by the limiter, not the handler.
	w := post(r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("after lockout: got %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 response must carry a Retry-After header")
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute, time.Minute)
	status := http.StatusUnauthorized
	r := newTestRouter(l, func() int { return status })

	post(r) // 1 failure
	post(r) // 2 failures

	status = http.StatusOK
	if got := post(r).Code; got != http.StatusOK {
		t.Fatalf("successful login: got %d, want 200", got)
	}

	// Counter cleared: three more failures must be needed to lock out again.
	status = http.StatusUnauthorized
	for i := 1; i <= 3; i++ {
		if got := post(r).Code; got != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d: got %d, want 401", i, got)
		}
	}
	if got := post(r).Code; got != http.StatusTooManyRequests {
		t.Errorf("expected lockout after reset cycle: got %d, want 429", got)
	}
}

func TestLockoutExpires(t *testing.T) {
	l := NewLoginRateLimiter(2, time.Minute, 5*time.Minute)
	current := time.Now()
	l.now = func() time.Time { return current }
	r := newTestRouter(l, func() int { return http.StatusUnauthorized })

	post(r)
	post(r)
	if got := post(r).Code; got != http.StatusTooManyRequests {
		t.Fatalf("expected lockout: got %d, want 429", got)
	}

	// Advance past the lockout window.
	current = current.Add(6 * time.Minute)
	if got := post(r).Code; got != http.StatusUnauthorized {
		t.Errorf("after lockout expiry the handler should run again: got %d, want 401", got)
	}
}

func TestValidationAndServerErrorsDoNotCount(t *testing.T) {
	l := NewLoginRateLimiter(2, time.Minute, time.Minute)
	// A malformed body (422) or a database outage (500) must never lock a user out.
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		r := newTestRouter(l, func() int { return status })
		for i := 0; i < 5; i++ {
			if got := post(r).Code; got != status {
				t.Fatalf("status %d attempt %d: got %d", status, i, got)
			}
		}
	}
}

func TestPerClientIsolation(t *testing.T) {
	l := NewLoginRateLimiter(2, time.Minute, time.Minute)
	r := newTestRouter(l, func() int { return http.StatusUnauthorized })

	send := func(ip string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)
		return w.Code
	}

	send("192.0.2.1")
	send("192.0.2.1")
	if got := send("192.0.2.1"); got != http.StatusTooManyRequests {
		t.Fatalf("first client should be locked out: got %d", got)
	}
	// A different address must be unaffected by the first client's lockout.
	if got := send("192.0.2.2"); got != http.StatusUnauthorized {
		t.Errorf("second client should not be locked out: got %d, want 401", got)
	}
}

func TestOnLockoutFiresOncePerLockout(t *testing.T) {
	l := NewLoginRateLimiter(2, time.Minute, time.Minute)

	var mu sync.Mutex
	var events []string
	l.OnLockout(func(ip string, until time.Time) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ip)
	})

	r := newTestRouter(l, func() int { return http.StatusUnauthorized })
	post(r)
	post(r) // threshold crossed here
	post(r) // already locked — refused by the limiter, must not re-fire

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 lockout event, got %d (%v)", len(events), events)
	}
	if events[0] != "192.0.2.10" {
		t.Errorf("lockout reported ip %q, want the client address", events[0])
	}
}

func TestOnLockoutNotCalledBelowThreshold(t *testing.T) {
	l := NewLoginRateLimiter(5, time.Minute, time.Minute)
	called := false
	l.OnLockout(func(string, time.Time) { called = true })

	r := newTestRouter(l, func() int { return http.StatusUnauthorized })
	post(r)
	post(r)
	if called {
		t.Error("lockout callback fired before the threshold was reached")
	}
}

func TestConcurrentAccessIsRaceFree(t *testing.T) {
	l := NewLoginRateLimiter(100, time.Minute, time.Minute)
	r := newTestRouter(l, func() int { return http.StatusUnauthorized })

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			post(r)
		}()
	}
	wg.Wait()
}

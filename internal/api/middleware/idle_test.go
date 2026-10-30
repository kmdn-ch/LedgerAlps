package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// L'horloge est maîtrisée : un test d'inactivité qui dormirait vraiment des
// minutes ne serait jamais lancé.
type horlogeIdle struct{ t time.Time }

func (h *horlogeIdle) now() time.Time         { return h.t }
func (h *horlogeIdle) avance(d time.Duration) { h.t = h.t.Add(d) }

func moteurIdle(minutes *int, tr *ActivityTracker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// À la place de RequireAuth : on ne teste ici que ce qui suit les droits.
	r.Use(func(c *gin.Context) {
		c.Set(claimsKey, &security.Claims{UserID: "u1"})
	})
	r.Use(IdleLimit(tr, func() int { return *minutes }))
	r.GET("/x", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	return r
}

func appelle(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w
}

// Le défaut : le délai n'était appliqué que par l'interface. Un jeton copié
// restait utilisable après l'inactivité.
func TestUneSessionInactiveAuDelaDuReglageEstRefusee(t *testing.T) {
	h := &horlogeIdle{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	tr := NewActivityTrackerWithClock(h.now)
	minutes := 10
	r := moteurIdle(&minutes, tr)

	if w := appelle(r); w.Code != 200 {
		t.Fatalf("première requête : %d", w.Code)
	}

	// 10 min + 1 min de marge d'avertissement : encore permis à la limite.
	h.avance(10*time.Minute + 30*time.Second)
	if w := appelle(r); w.Code != 200 {
		t.Fatalf("dans le délai et la marge d'avertissement : %d, attendu 200", w.Code)
	}

	// Plus de 11 minutes sans le moindre signe : refusé, motif stable.
	h.avance(11*time.Minute + time.Second)
	w := appelle(r)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "session_idle") {
		t.Fatalf("après l'inactivité : %d %s, attendu 401 session_idle", w.Code, w.Body.String())
	}
	// Et ça le reste : un refus ne vaut pas activité.
	if w := appelle(r); w.Code != http.StatusUnauthorized {
		t.Fatalf("la session refusée a été rouverte par la requête refusée : %d", w.Code)
	}
}

// Une requête acceptée repousse l'échéance ; se reconnecter la repart de zéro.
func TestLActiviteRepousseLEcheanceEtLaConnexionLaRepart(t *testing.T) {
	h := &horlogeIdle{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	tr := NewActivityTrackerWithClock(h.now)
	minutes := 10
	r := moteurIdle(&minutes, tr)

	for i := 0; i < 6; i++ { // 6 × 8 min = 48 min : bien plus que le délai, mais jamais 8 min d'inactivité
		appelle(r)
		h.avance(8 * time.Minute)
	}
	if w := appelle(r); w.Code != 200 {
		t.Fatalf("une session continuellement active a été coupée : %d", w.Code)
	}

	h.avance(30 * time.Minute)
	if w := appelle(r); w.Code != http.StatusUnauthorized {
		t.Fatalf("session inactive acceptée : %d", w.Code)
	}
	tr.Touch("u1") // une nouvelle connexion
	if w := appelle(r); w.Code != 200 {
		t.Fatalf("après une nouvelle connexion : %d, attendu 200", w.Code)
	}
}

// Délai à zéro : aucune coupure — et l'activité reste notée, pour qu'allumer le
// réglage ensuite ne déconnecte pas tout le monde d'un coup.
func TestSansDelaiReglePersonneNEstCoupe(t *testing.T) {
	h := &horlogeIdle{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	tr := NewActivityTrackerWithClock(h.now)
	minutes := 0
	r := moteurIdle(&minutes, tr)

	appelle(r)
	h.avance(48 * time.Hour)
	if w := appelle(r); w.Code != 200 {
		t.Fatalf("coupure alors que le réglage est à zéro : %d", w.Code)
	}

	// Le réglage s'allume : la requête de tout à l'heure compte comme activité.
	minutes = 10
	if w := appelle(r); w.Code != 200 {
		t.Fatalf("activer le réglage a déconnecté un compte actif à l'instant : %d", w.Code)
	}
}

// Un compte jamais vu (serveur redémarré) n'est pas expiré, et un suivi nul ne
// panique pas.
func TestUnCompteInconnuNEstPasExpire(t *testing.T) {
	tr := NewActivityTracker()
	if tr.Expired("jamais-vu", time.Minute) {
		t.Fatal("un compte jamais vu est expiré")
	}
	var nul *ActivityTracker
	nul.Touch("u")
	if nul.Expired("u", time.Minute) {
		t.Fatal("un suivi nul a expiré un compte")
	}
}

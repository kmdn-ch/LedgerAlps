package middleware

// Audit croisé Gemini, finitions (P3) : CORS et limiteur (MED-12).

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func reponseCORS(t *testing.T, origine, methode string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS("http://localhost:5173"))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	req := httptest.NewRequest(methode, "/x", nil)
	if origine != "" {
		req.Header.Set("Origin", origine)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// MED-12. Les en-têtes CORS dépendent de l'origine : `Vary: Origin` doit donc figurer sur TOUTE
// réponse. Il n'était posé que pour une origine admise — un cache intermédiaire qui retenait la
// réponse faite à une origine refusée (sans Access-Control-Allow-Origin) la servait ensuite à une
// origine admise.
func TestVaryOriginEstPoseSurToutesLesReponses(t *testing.T) {
	for nom, origine := range map[string]string{
		"origine admise":  "http://localhost:5173",
		"origine refusée": "https://evil.example",
		"sans origine":    "",
	} {
		w := reponseCORS(t, origine, http.MethodGet)
		if got := strings.Join(w.Header().Values("Vary"), ","); !strings.Contains(got, "Origin") {
			t.Errorf("%s : Vary = %q, attendu Origin", nom, got)
		}
	}
	// Et l'origine refusée ne reçoit toujours aucune autorisation.
	w := reponseCORS(t, "https://evil.example", http.MethodGet)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("une origine refusée reçoit Access-Control-Allow-Origin")
	}
	w = reponseCORS(t, "http://localhost:5173", http.MethodGet)
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("l'origine admise perd son autorisation : %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	// La réponse à une requête de pré-vérification suit la même règle.
	w = reponseCORS(t, "https://evil.example", http.MethodOptions)
	if w.Code != http.StatusNoContent || !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "Origin") {
		t.Errorf("pré-vérification refusée : %d, Vary = %v", w.Code, w.Header().Values("Vary"))
	}
}

// MED-12. La table du limiteur n'avait pas de plafond : un flot de clés distinctes (adresse|compte
// avec des courriels inventés) s'y accumulait une heure durant. Elle est bornée, et une clé
// VERROUILLÉE n'est jamais évincée : l'évincer rendrait l'accès à qui vient de l'épuiser.
func TestLaTableDuLimiteurEstBornee(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute, time.Hour)
	maintenant := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return maintenant }

	// Une clé qui épuise ses essais : verrouillée une heure.
	for i := 0; i < 3; i++ {
		l.recordFailure("verrouillee")
	}
	if _, verrouille := l.locked("verrouillee"); !verrouille {
		t.Fatal("la clé de départ devrait être verrouillée")
	}

	// Un flot de clés distinctes, une seule erreur chacune, chacune un peu plus récente.
	for i := 0; i < maxRecords+2500; i++ {
		maintenant = maintenant.Add(time.Millisecond)
		l.recordFailure(fmt.Sprintf("192.0.2.1|inconnu-%d@exemple.ch", i))
	}

	l.mu.Lock()
	taille := len(l.records)
	l.mu.Unlock()
	if taille > maxRecords {
		t.Errorf("la table compte %d enregistrements, plafond %d", taille, maxRecords)
	}
	if _, verrouille := l.locked("verrouillee"); !verrouille {
		t.Error("la clé verrouillée a été évincée : son verrou est perdu")
	}
	// Ce sont les plus anciennes qu'on retire, pas les dernières.
	l.mu.Lock()
	_, derniere := l.records[fmt.Sprintf("192.0.2.1|inconnu-%d@exemple.ch", maxRecords+2499)]
	_, premiere := l.records["192.0.2.1|inconnu-0@exemple.ch"]
	l.mu.Unlock()
	if !derniere {
		t.Error("la clé la plus récente n'est pas suivie")
	}
	if premiere {
		t.Error("la clé la plus ancienne est encore là : l'éviction ne vise pas les moins récentes")
	}
}

// Quand la table est pleine de clés toutes verrouillées, une clé de plus n'est pas suivie plutôt
// que de faire croître la mémoire — et rien ne plante.
func TestUneTablePleineDeVerrousNeGrossitPas(t *testing.T) {
	l := NewLoginRateLimiter(1, time.Minute, time.Hour)
	maintenant := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return maintenant }

	for i := 0; i < maxRecords; i++ {
		l.recordFailure(fmt.Sprintf("a%d", i)) // un seul essai suffit : chacune est verrouillée
	}
	l.recordFailure("une-de-plus")

	l.mu.Lock()
	taille := len(l.records)
	l.mu.Unlock()
	if taille > maxRecords {
		t.Errorf("la table compte %d enregistrements, plafond %d", taille, maxRecords)
	}
}

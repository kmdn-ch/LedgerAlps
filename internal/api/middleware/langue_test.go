package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Un refus écrit par un gestionnaire ressort dans la langue demandée.
func TestLeRefusEstTraduit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Langue())
	r.POST("/x", func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "identifiants incorrects"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", nil)
	req.Header.Set("Accept-Language", "de")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("statut %d au lieu de 401 — l'intercepteur a perdu le code", rec.Code)
	}
	var corps map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &corps); err != nil {
		t.Fatalf("corps illisible (%v) : %q", err, rec.Body.String())
	}
	msg, _ := corps["error"].(string)
	if msg == "identifiants incorrects" {
		t.Errorf("le message n'a pas été traduit : %q", msg)
	}
	if msg == "" {
		t.Errorf("le message a disparu")
	}
}

// En français, rien n'est touché : ni le corps, ni le statut.
func TestLeFrançaisPasseIntact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Langue())
	r.GET("/x", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "aucune facture sélectionnée"})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("statut %d au lieu de 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "aucune facture sélectionnée") {
		t.Errorf("le français a été modifié : %q", rec.Body.String())
	}
}

// Ce qui n'est pas du JSON traverse sans être mis en mémoire tampon : un PDF
// de dix mégaoctets n'a aucune raison de passer par là.
func TestLeNonJSONNestPasTouché(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Langue())
	charge := strings.Repeat("A", 4096)
	r.GET("/x", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/pdf", []byte(charge))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Accept-Language", "de")
	r.ServeHTTP(rec, req)

	if rec.Body.String() != charge {
		t.Errorf("le corps binaire a été altéré (%d octets au lieu de %d)",
			rec.Body.Len(), len(charge))
	}
}

// Un champ « error » qui n'est pas une chaîne ne doit pas faire tomber la
// réponse : le corps ressort tel quel.
func TestUnChampNonTextuelNeCassePas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Langue())
	r.GET("/x", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"error": []string{"a", "b"}})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Accept-Language", "de")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("statut %d au lieu de 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"a"`) {
		t.Errorf("le corps a été perdu : %q", rec.Body.String())
	}
}

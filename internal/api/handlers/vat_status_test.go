package handlers

// Le statut TVA, là où il a des conséquences.
//
// Deux choses seulement sont contrôlées ici, mais ce sont celles qui coûtent :
//
//  1. « Non assujetti » EFFACE le numéro de TVA. Ce numéro s'imprime sur la
//     facture ; le laisser produirait un document qui affirme le contraire de
//     la fiche, ce que la LTVA art. 27 al. 1 interdit et dont l'al. 2 rend
//     redevable. La contradiction se refuse là où elle naît.
//
//  2. Un statut inconnu est refusé. Sans ce contrôle, une valeur arrivée d'un
//     script ou d'une version future s'écrirait en base et se comporterait
//     comme « non déclaré » — un troisième état silencieux.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

func nouveauxRéglages(t *testing.T) (*SettingsHandler, *gin.Engine) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-tva-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	cfg := &config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	h := NewSettingsHandler(database, false)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/company", h.GetCompany)
	r.PUT("/company", h.PutCompany)
	return h, r
}

func enregistrer(t *testing.T, r *gin.Engine, corps map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(corps)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/company", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func lireFiche(t *testing.T, r *gin.Engine) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/company", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET statut = %d : %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON invalide : %v", err)
	}
	return out
}

func TestNonAssujettiEffaceLeNumeroDeTVA(t *testing.T) {
	_, r := nouveauxRéglages(t)

	// D'abord assujetti, avec son numéro.
	if w := enregistrer(t, r, map[string]any{
		"company_name": "Test SA",
		"vat_number":   "CHE-123.456.789 TVA",
		"vat_status":   models.VatLiable,
	}); w.Code != http.StatusOK {
		t.Fatalf("statut = %d : %s", w.Code, w.Body.String())
	}
	if got := lireFiche(t, r)["vat_number"]; got != "CHE-123.456.789 TVA" {
		t.Fatalf("vat_number = %v, le numéro aurait dû être conservé", got)
	}

	// Puis non assujetti — le numéro part avec la déclaration, même si le
	// formulaire le renvoie encore : c'est exactement ce que ferait un écran
	// qui n'aurait pas vidé son champ.
	if w := enregistrer(t, r, map[string]any{
		"company_name": "Test SA",
		"vat_number":   "CHE-123.456.789 TVA",
		"vat_status":   models.VatExempt,
	}); w.Code != http.StatusOK {
		t.Fatalf("statut = %d : %s", w.Code, w.Body.String())
	}

	fiche := lireFiche(t, r)
	if got := fiche["vat_number"]; got != "" {
		t.Errorf("vat_number = %q — il devait être effacé par « non assujetti »", got)
	}
	if got := fiche["vat_status"]; got != models.VatExempt {
		t.Errorf("vat_status = %v, attendu %q", got, models.VatExempt)
	}
}

// Le statut ABSENT ne remet pas la réponse à zéro. Le formulaire de la fiche
// société ne porte pas toujours ce champ : l'inclure d'office dans l'UPDATE
// effacerait une déclaration à chaque enregistrement, comme cela avait failli
// arriver à la comptabilisation automatique.
func TestUnStatutAbsentNeToucheARien(t *testing.T) {
	_, r := nouveauxRéglages(t)

	enregistrer(t, r, map[string]any{
		"company_name": "Test SA",
		"vat_status":   models.VatExempt,
	})
	enregistrer(t, r, map[string]any{"company_name": "Test SA renommée"})

	if got := lireFiche(t, r)["vat_status"]; got != models.VatExempt {
		t.Errorf("vat_status = %v — un enregistrement sans le champ l'a écrasé", got)
	}
}

func TestUnStatutInconnuEstRefuse(t *testing.T) {
	_, r := nouveauxRéglages(t)

	w := enregistrer(t, r, map[string]any{
		"company_name": "Test SA",
		"vat_status":   "peut-être",
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("statut = %d, attendu 422 : %s", w.Code, w.Body.String())
	}
}

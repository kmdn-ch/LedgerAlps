package handlers

// Audit croisé Gemini, HIGH-17 : le redémarrage que le chiffrement programmé attend.
//
// `POST /database/encryption` programme la conversion « au prochain démarrage », et le serveur
// refusait ensuite de redémarrer (« rien à appliquer ») : la conversion n'arrivait que si le
// propriétaire fermait et rouvrait l'application à la main.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// isolerDonnees redirige le répertoire de données de l'application : `config.AppDataDir()` ne lit
// APPDATA que sous Windows et retombe sur le dossier personnel ailleurs. Un test ne doit ni dépendre de
// la machine, ni toucher aux données de l'utilisateur.
func isolerDonnees(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

func routeurSysteme(t *testing.T) (*gin.Engine, chan struct{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	restart := make(chan struct{}, 1)
	h := NewSystemHandler(restart, &config.Config{SQLitePath: "sans-importance.db", Host: "127.0.0.1"}, nil)
	r := gin.New()
	r.GET("/settings/server", h.GetServerSettings)
	r.POST("/system/restart", h.Restart)
	return r, restart
}

func lireRestart(t *testing.T, r *gin.Engine) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/settings/server", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /settings/server : %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func demanderRestart(r *gin.Engine) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/system/restart", nil))
	return w.Code
}

// Rien n'attend : pas de redémarrage proposé, et le serveur le refuse.
func TestSansRienEnAttenteLeRedemarrageEstRefuse(t *testing.T) {
	isolerDonnees(t)
	r, restart := routeurSysteme(t)

	etat := lireRestart(t, r)
	if etat["restart_required"] != false || etat["network_restart_required"] != false {
		t.Errorf("rien n'attend, reçu %v", etat)
	}
	if code := demanderRestart(r); code != http.StatusUnprocessableEntity {
		t.Errorf("redémarrage sans rien à appliquer : %d, attendu 422", code)
	}
	if len(restart) != 0 {
		t.Error("un redémarrage a été déclenché sans rien à appliquer")
	}
}

// HIGH-17. Un chiffrement (ou un retour en clair) programmé est exactement ce qu'un redémarrage
// applique : il est proposé, et accepté.
func TestUnChiffrementProgrammeAppelleLeRedemarrage(t *testing.T) {
	for _, action := range []string{db.ActionEncrypt, db.ActionDecrypt} {
		t.Run(action, func(t *testing.T) {
			isolerDonnees(t)
			r, restart := routeurSysteme(t)
			if _, err := db.StageEncryption(config.AppDataDir(), action, "u1"); err != nil {
				t.Fatal(err)
			}

			etat := lireRestart(t, r)
			if etat["restart_required"] != true {
				t.Errorf("restart_required = %v avec une conversion programmée, attendu true", etat["restart_required"])
			}
			// Les réglages RÉSEAU, eux, n'ont pas changé : leur écran ne doit pas annoncer
			// « enregistrés mais pas appliqués ».
			if etat["network_restart_required"] != false {
				t.Errorf("network_restart_required = %v, attendu false (aucun réglage réseau écrit)", etat["network_restart_required"])
			}
			if code := demanderRestart(r); code != http.StatusAccepted {
				t.Errorf("redémarrage avec une conversion programmée : %d, attendu 202", code)
			}
			if len(restart) != 1 {
				t.Error("le redémarrage accepté n'a pas été demandé à la boucle principale")
			}
		})
	}

	// Annulée, la conversion ne justifie plus rien.
	isolerDonnees(t)
	r, _ := routeurSysteme(t)
	if _, err := db.StageEncryption(config.AppDataDir(), db.ActionEncrypt, "u1"); err != nil {
		t.Fatal(err)
	}
	db.ClearPendingEncryption(config.AppDataDir())
	if etat := lireRestart(t, r); etat["restart_required"] != false {
		t.Errorf("conversion annulée : restart_required = %v, attendu false", etat["restart_required"])
	}
}

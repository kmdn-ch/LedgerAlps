package handlers

// Audit croisé Gemini, HIGH-17 : le redémarrage que le chiffrement programmé attend.
//
// `POST /database/encryption` programme la conversion « au prochain démarrage », et le serveur
// refusait ensuite de redémarrer (« rien à appliquer ») : la conversion n'arrivait que si le
// propriétaire fermait et rouvrait l'application à la main.

import (
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
	h := NewSystemHandler(restart, &config.Config{SQLitePath: "sans-importance.db"}, nil)
	r := gin.New()
	r.POST("/system/restart", h.Restart)
	return r, restart
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
	r, restart := routeurSysteme(t)
	if _, err := db.StageEncryption(config.AppDataDir(), db.ActionEncrypt, "u1"); err != nil {
		t.Fatal(err)
	}
	db.ClearPendingEncryption(config.AppDataDir())
	if code := demanderRestart(r); code != http.StatusUnprocessableEntity {
		t.Errorf("conversion annulée : redémarrage %d, attendu 422", code)
	}
	if len(restart) != 0 {
		t.Error("un redémarrage a été déclenché alors que la conversion était annulée")
	}
}

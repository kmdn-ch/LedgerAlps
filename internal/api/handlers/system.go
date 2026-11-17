package handlers

// Restarting from the interface.
//
// A staged restore is applied at the next start, and until then the user has to
// close and reopen the application themselves. That is a real instruction to
// follow, and one people get wrong — they close the browser tab, which leaves
// the server running and nothing restored.
//
// The launcher starts the server and does not supervise it, so nothing outside
// would bring it back. The server therefore restarts itself: it stops serving,
// releases the database, launches a fresh copy of its own binary, and exits.
// The order matters — the new process applies the restore before opening the
// database, and it cannot do that while the old one still holds the file.

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// SystemHandler exposes the restart. The channel is closed by the handler and
// consumed by main, which owns the shutdown sequence — a handler must not tear
// down the server it is answering from.
type SystemHandler struct {
	restart chan<- struct{}
	cfg     *config.Config
	// La base sert à révoquer les jetons de rafraîchissement lors d'une
	// rotation du secret : ils survivraient sinon dans une table décrivant des
	// sessions qui n'existent plus.
	db          *sql.DB
	usePostgres bool

	// settingsChanged records that config.json was edited since this process
	// started (la rotation de la clé de signature), so the restart button has
	// something to offer beyond a pending restore. Guarded because it is written
	// by one request and read by others.
	mu              sync.Mutex
	settingsChanged bool
}

func NewSystemHandler(restart chan<- struct{}, cfg *config.Config, database *sql.DB) *SystemHandler {
	return &SystemHandler{restart: restart, cfg: cfg, db: database, usePostgres: cfg.UsePostgres()}
}

// configChanged reports whether config.json was edited since this process started.
func (h *SystemHandler) configChanged() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.settingsChanged
}

// restartPending reports whether a restart would actually achieve something.
//
// Three things wait for one: a signing-key rotation written since this process started, a staged
// restore, and a staged encryption or decryption of the database. The last was missing (audit
// croisé Gemini, HIGH-17): after `POST /database/encryption` the screen said "restart" and the
// server answered "nothing to apply" when asked to — the conversion could only happen if the
// owner closed and reopened the application by hand.
func (h *SystemHandler) restartPending() bool {
	return h.configChanged() ||
		db.ReadPendingRestore(db.BackupDir()) != nil ||
		db.ReadPendingEncryption(config.AppDataDir()) != nil
}

// Restart POST /api/v1/system/restart
//
// Only when something is actually waiting for it — a staged restore, a staged
// encryption conversion, or a signing key rotated since this process started. Outside those cases a restart
// button is a power tool with no purpose, and one more way to interrupt someone
// mid-invoice.
func (h *SystemHandler) Restart(c *gin.Context) {
	if !h.restartPending() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "rien à appliquer : le redémarrage n'est proposé que pour une restauration " +
				"préparée, une conversion du chiffrement programmée ou une clé de signature régénérée",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "LedgerAlps redémarre pour appliquer les changements. " +
			"Cette page va se recharger automatiquement.",
	})

	// Signal after responding: main waits for the response to flush before it
	// stops listening, otherwise the caller sees a dropped connection and
	// cannot tell a restart from a crash.
	select {
	case h.restart <- struct{}{}:
	default: // a restart is already under way
	}
}

// ─── POST /api/v1/settings/server/rotate-secret ──────────────────────────────

// RotateJWTSecret régénère le secret de signature des jetons.
//
// À utiliser si `config.json` a pu fuiter : joint à un ticket de support, copié
// sur une clé, poussé par erreur dans un dépôt. Qui détient ce secret forge un
// jeton valide pour n'importe quel compte, administrateur compris, **sans
// connaître aucun mot de passe** — et jusqu'ici rien ne permettait d'y répondre
// autrement qu'en éditant le fichier à la main.
//
// Portée exacte : le secret ne sert qu'à signer et relire les jetons. Le
// régénérer déconnecte toutes les sessions, et rien d'autre. Mots de passe
// intacts, aucune donnée touchée, sauvegardes toujours utilisables.
//
// Les jetons de rafraîchissement en base sont révoqués dans la foulée. Ils
// deviendraient de toute façon invérifiables avec le nouveau secret, mais les
// laisser actifs entretiendrait une table de sessions qui n'existent plus, et
// l'écran de sécurité mentirait sur l'état réel du système.
//
// Accès : administrateur uniquement.
func (h *SystemHandler) RotateJWTSecret(c *gin.Context) {
	// Quand la clé vient de la variable d'environnement JWT_SECRET, la régénérer dans
	// config.json ne ferait rien : la variable reprend la main à chaque démarrage. Répondre
	// « secret régénéré » alors que la clé en service reste celle qu'on voulait remplacer —
	// peut-être compromise — serait pire que de refuser (audit croisé Gemini, CRIT-16).
	if h.cfg != nil && h.cfg.JWTSecretDepuisEnv {
		c.JSON(http.StatusConflict, gin.H{
			"reason": "secret_from_environment",
			"error": "La clé de signature vient de la variable d'environnement JWT_SECRET : " +
				"la régénérer ici ne changerait rien, la variable reprendrait la main au prochain démarrage. " +
				"Changez la valeur de la variable, puis redémarrez LedgerAlps.",
		})
		return
	}
	length, err := config.RotateJWTSecret()
	if err != nil {
		erreurInterne(c, msgErreurInterne, err)
		return
	}

	revoked := int64(0)
	if h.db != nil {
		q := db.Rebind(
			`UPDATE refresh_tokens SET revoked_at = ? WHERE revoked_at IS NULL`, h.usePostgres)
		if res, err := h.db.ExecContext(c.Request.Context(), q, time.Now().UTC()); err == nil {
			revoked, _ = res.RowsAffected()
		}
		// Une erreur ici ne doit pas masquer la rotation, qui a déjà eu lieu :
		// le nouveau secret rend ces jetons invérifiables de toute façon.
	}

	h.mu.Lock()
	h.settingsChanged = true
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"rotated":          true,
		"secret_length":    length,
		"sessions_revoked": revoked,
		"restart_required": true,
		"message": "Secret de signature régénéré. Il prend effet au redémarrage de LedgerAlps, " +
			"après quoi toutes les sessions devront se reconnecter. " +
			"Vos mots de passe, vos données et vos sauvegardes ne sont pas affectés.",
	})
}

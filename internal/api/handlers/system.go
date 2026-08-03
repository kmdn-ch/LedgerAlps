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
	"os"
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
	// started, so the restart button has something to offer beyond a pending
	// restore. Guarded because it is written by one request and read by others.
	mu              sync.Mutex
	settingsChanged bool
}

func NewSystemHandler(restart chan<- struct{}, cfg *config.Config, database *sql.DB) *SystemHandler {
	return &SystemHandler{restart: restart, cfg: cfg, db: database, usePostgres: cfg.UsePostgres()}
}

// restartPending reports whether a restart would actually achieve something.
func (h *SystemHandler) restartPending() bool {
	h.mu.Lock()
	changed := h.settingsChanged
	h.mu.Unlock()
	return changed || db.ReadPendingRestore(db.BackupDir()) != nil
}

// Restart POST /api/v1/system/restart
//
// Only when something is actually waiting for it — a staged restore, or network
// settings written since this process started. Outside those cases a restart
// button is a power tool with no purpose, and one more way to interrupt someone
// mid-invoice.
func (h *SystemHandler) Restart(c *gin.Context) {
	if !h.restartPending() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "rien à appliquer : le redémarrage n'est proposé que pour une restauration " +
				"préparée ou un changement de réglages réseau",
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

// ─── Réglages réseau ──────────────────────────────────────────────────────────

// GetServerSettings GET /api/v1/settings/server
func (h *SystemHandler) GetServerSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"settings":         config.CurrentServerSettings(h.cfg),
		"restart_required": h.restartPending(),
		"config_file":      config.ConfigFilePath(),
	})
}

// PutServerSettings PUT /api/v1/settings/server
//
// Writes to config.json and asks for a restart. It cannot apply live: the
// listening socket and its TLS configuration are chosen once, at start.
func (h *SystemHandler) PutServerSettings(c *gin.Context) {
	var s config.ServerSettings
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if s.Host == "" {
		s.Host = "127.0.0.1"
	}
	// Half a certificate is worse than none: the server would refuse to start,
	// and the user would have shut themselves out of their own accounts.
	if (s.TLSCert == "") != (s.TLSKey == "") {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "le certificat et la clé doivent être fournis ensemble"})
		return
	}
	for _, p := range []string{s.TLSCert, s.TLSKey} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "fichier introuvable: " + p + " — vérifiez le chemin avant d'enregistrer"})
			return
		}
	}

	if err := config.SaveServerSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.mu.Lock()
	h.settingsChanged = true
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"settings":         s,
		"restart_required": true,
		"message": "Réglages enregistrés. Ils prennent effet au redémarrage de LedgerAlps : " +
			"l'adresse d'écoute et le chiffrement sont choisis une seule fois, au démarrage.",
	})
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
	length, err := config.RotateJWTSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

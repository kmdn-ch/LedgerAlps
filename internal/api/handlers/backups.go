package handlers

// Backup management from the interface.
//
// Taking a snapshot is safe while the server runs — SQLite's VACUUM INTO writes
// a consistent copy of a live database. Restoring is not: it replaces the file
// every open connection is using. So the two verbs behave differently here.
// Creating a backup happens immediately; restoring is staged and applied at the
// next start, and the interface has to say so plainly rather than pretend the
// click did the work.

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

type BackupsHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewBackupsHandler(database *sql.DB, cfg *config.Config) *BackupsHandler {
	return &BackupsHandler{db: database, cfg: cfg}
}

type backupItem struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	Encrypted bool      `json:"encrypted"`
}

// ListBackups GET /api/v1/backups
func (h *BackupsHandler) ListBackups(c *gin.Context) {
	dir := db.BackupDir()
	entries, err := db.ListBackups(dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "impossible de lire le dossier de sauvegarde"})
		return
	}

	items := make([]backupItem, 0, len(entries))
	for _, e := range entries {
		encrypted, _ := db.IsEncrypted(e.Path)
		items = append(items, backupItem{
			Name: e.Name, SizeBytes: e.SizeBytes, CreatedAt: e.CreatedAt, Encrypted: encrypted,
		})
	}

	resp := gin.H{"items": items, "directory": dir}
	// Surfacing a staged restore matters: the user needs to know a restart is
	// still owed, especially if they staged it and walked away.
	if p := db.ReadPendingRestore(dir); p != nil {
		resp["pending_restore"] = gin.H{
			"source_name":  p.SourceName,
			"requested_at": p.RequestedAt,
		}
	}
	c.JSON(http.StatusOK, resp)
}

// CreateBackup POST /api/v1/backups
//
// An empty passphrase produces a plaintext snapshot, which is the current
// behaviour and stays available — a passphrase the user cannot remember turns
// a lost machine into a lost ledger, so it must be their choice.
func (h *BackupsHandler) CreateBackup(c *gin.Context) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	_ = c.ShouldBindJSON(&body)

	// An empty passphrase means "do not encrypt", which stays a legitimate
	// choice. A short one is not a choice, it is a mistake: this file can be
	// carried off and attacked offline, with no rate limit and nobody watching.
	if body.Passphrase != "" {
		if err := db.ValidatePassphrase(body.Passphrase); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
	}

	// Argon2id is deliberately slow and a large database takes a while to
	// copy; the default request timeout would cut this short.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	dir := db.BackupDir()
	path, err := db.Backup(ctx, h.db, h.cfg, dir, body.Passphrase)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	info, _ := os.Stat(path)
	var size int64
	if info != nil {
		size = info.Size()
	}
	encrypted, _ := db.IsEncrypted(path)

	if _, err := db.Prune(dir, db.DefaultKeep); err != nil {
		// The snapshot exists; failing to prune is not worth losing it over.
		c.JSON(http.StatusCreated, gin.H{
			"name": filepath.Base(path), "size_bytes": size, "encrypted": encrypted,
			"warning": "sauvegarde créée, mais le nettoyage des anciennes copies a échoué",
		})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"name": filepath.Base(path), "size_bytes": size, "encrypted": encrypted,
	})
}

// StageRestore POST /api/v1/backups/restore
//
// Does not restore. It decrypts and verifies the chosen snapshot now — while
// the user is present to fix a wrong passphrase — and leaves it ready for the
// next start. The response says a restart is required; the UI must not
// pretend otherwise.
func (h *BackupsHandler) StageRestore(c *gin.Context) {
	var body struct {
		Name       string `json:"name" binding:"required"`
		Passphrase string `json:"passphrase"`
		Confirm    bool   `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if !body.Confirm {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "confirmation requise: la restauration remplacera la comptabilité actuelle"})
		return
	}

	dir := db.BackupDir()
	// Resolve through the listing rather than joining the name onto the path:
	// a name is user input, and "../../" must not reach the filesystem.
	entries, err := db.ListBackups(dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "impossible de lire le dossier de sauvegarde"})
		return
	}
	src := ""
	for _, e := range entries {
		if e.Name == body.Name {
			src = e.Path
			break
		}
	}
	if src == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "sauvegarde introuvable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	// The user id, not the email: the JWT deliberately omits the email
	// (data minimisation, nLPD art. 6), and an id identifies the account
	// well enough for a record of who asked for the restore.
	requestedBy := ""
	if claims := mw.GetClaims(c); claims != nil {
		requestedBy = claims.UserID
	}

	p, err := db.StageRestore(ctx, src, dir, body.Passphrase, requestedBy)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"pending_restore": gin.H{"source_name": p.SourceName, "requested_at": p.RequestedAt},
		"message": "Restauration préparée et vérifiée. Elle sera appliquée au prochain " +
			"démarrage de LedgerAlps : fermez puis rouvrez l'application.",
	})
}

// CancelRestore DELETE /api/v1/backups/restore — undo a staged restore.
func (h *BackupsHandler) CancelRestore(c *gin.Context) {
	dir := db.BackupDir()
	p := db.ReadPendingRestore(dir)
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "aucune restauration en attente"})
		return
	}
	db.ClearPendingRestore(dir, p)
	c.JSON(http.StatusOK, gin.H{"message": "restauration annulée"})
}

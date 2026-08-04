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
// Sans phrase de passe explicite, celle enregistrée dans la politique s'applique.
//
// Le défaut inverse a été mesuré sur un serveur réel : la phrase de passe était
// bien enregistrée, les sauvegardes automatiques bien chiffrées, et le bouton
// « Créer une sauvegarde » produisait quand même un fichier en clair, parce
// qu'il ne lisait que le corps de la requête. Le trou que la politique ferme
// d'un côté restait ouvert de l'autre — et sur le chemin que l'utilisateur
// emprunte justement avant de copier le fichier sur une clé USB.
//
// Une copie en clair reste possible, mais elle se demande maintenant :
// « plaintext: true ». Le silence ne veut plus dire « en clair ».
func (h *BackupsHandler) CreateBackup(c *gin.Context) {
	var body struct {
		Passphrase string `json:"passphrase"`
		// Plaintext demande explicitement une copie non chiffrée, malgré la
		// politique. Cas légitime : remettre le fichier à sa fiduciaire.
		Plaintext bool `json:"plaintext"`
	}
	_ = c.ShouldBindJSON(&body)

	if body.Passphrase == "" && !body.Plaintext {
		if stored, source := db.NewBackupPolicy(config.AppDataDir()).Passphrase(); stored != "" {
			body.Passphrase = stored
			_ = source
		}
	}

	// A short passphrase is not a choice, it is a mistake: this file can be
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

	// The undo copy is taken here, not when the restore is applied: this is the
	// only moment the user is present with their passphrase. Taken at startup
	// instead, it could only ever be written in clear — silently undoing the
	// very choice someone made by encrypting their backups.
	//
	// It goes through Backup, so it lands in the list like any other snapshot
	// and rotates with them, instead of accumulating as a special file nobody
	// prunes.
	//
	// Restaurer une sauvegarde EN CLAIR laissait la copie d'annulation en clair
	// elle aussi — sur une installation dont le propriétaire a pourtant
	// enregistré une phrase de passe. La politique s'applique donc ici comme
	// ailleurs : c'est la copie de la comptabilité en cours, pas un fichier de
	// second rang.
	undoPass := body.Passphrase
	if undoPass == "" {
		if stored, _ := db.NewBackupPolicy(config.AppDataDir()).Passphrase(); stored != "" {
			undoPass = stored
		}
	}
	undo, err := db.Backup(ctx, h.db, h.cfg, dir, undoPass)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "impossible de sauvegarder la comptabilité actuelle avant la restauration: " + err.Error()})
		return
	}

	p, err := db.StageRestore(ctx, src, dir, body.Passphrase, requestedBy)
	if err != nil {
		// The undo copy is a perfectly good backup on its own; keeping it
		// costs nothing and losing it could cost everything.
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"pending_restore":  gin.H{"source_name": p.SourceName, "requested_at": p.RequestedAt},
		"undo_backup_name": filepath.Base(undo),
		"message": "Restauration préparée et vérifiée. Votre comptabilité actuelle a été " +
			"sauvegardée. La restauration sera appliquée au redémarrage de LedgerAlps.",
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

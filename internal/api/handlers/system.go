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
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// SystemHandler exposes the restart. The channel is closed by the handler and
// consumed by main, which owns the shutdown sequence — a handler must not tear
// down the server it is answering from.
type SystemHandler struct {
	restart chan<- struct{}
}

func NewSystemHandler(restart chan<- struct{}) *SystemHandler {
	return &SystemHandler{restart: restart}
}

// Restart POST /api/v1/system/restart
//
// Only when a restore is staged. Outside that case a restart button is a power
// tool with no purpose, and one more way to interrupt someone mid-invoice.
func (h *SystemHandler) Restart(c *gin.Context) {
	if p := db.ReadPendingRestore(db.BackupDir()); p == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "aucune restauration en attente : le redémarrage n'est proposé que pour en appliquer une",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "LedgerAlps redémarre pour appliquer la restauration. " +
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

package handlers

// UIDLookup exposes the ZEFIX company lookup to the application.
//
// The lookup logic lives in internal/core/zefix and is shared with the
// first-run wizard in the launcher. It used to be written twice, and when only
// one copy was fixed the wizard — the place users actually meet it — kept
// failing. One implementation, two thin HTTP wrappers.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/zefix"
)

// UIDLookup handles GET /api/v1/uid-lookup?che=CHE-123.456.789
func UIDLookup(c *gin.Context) {
	raw := c.Query("che")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paramètre 'che' requis"})
		return
	}

	// Two sequential calls, plus an occasional legal-form fetch.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	company, err := zefix.Lookup(ctx, raw)
	switch {
	case errors.Is(err, zefix.ErrInvalidFormat):
		c.JSON(http.StatusBadRequest, gin.H{"error": "format IDE invalide — attendu CHE-XXX.XXX.XXX"})
	case errors.Is(err, zefix.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "numéro IDE introuvable au registre du commerce"})
	case err != nil:
		// The registry being unreachable is not the user's mistake; say so, and
		// make clear the form can still be filled in by hand.
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "registre IDE momentanément indisponible — saisissez les informations manuellement",
		})
	default:
		c.JSON(http.StatusOK, company)
	}
}

package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kmdn-ch/ledgeralps/internal/i18n"
)

// Téléchargement groupé de documents.
//
// Depuis la fiche d'un client, on veut souvent repartir avec « toutes ses
// factures 2025 » — pour les transmettre à une fiduciaire, répondre à une
// demande d'accès, ou simplement archiver. Les télécharger une par une est
// possible mais absurde dès qu'il y en a dix.
//
// Un seul document sort en PDF, plusieurs sortent en ZIP. Emballer un unique
// PDF obligerait à le dézipper pour le lire, ce qui n'apporte rien ; ne pas
// emballer plusieurs fichiers n'est pas possible en une réponse HTTP.
//
// Sur le plan légal : ces documents sont vos pièces comptables, dont le CO
// art. 958f vous impose la conservation. Les exporter ne pose aucune question
// de principe — c'est même ce qui vous permet de répondre à une demande d'accès
// (nLPD art. 25) ou de remise des données (art. 28). Ce qui compte est ce qui
// arrive au fichier ensuite : il porte des données personnelles et quitte la
// machine.

// maxBulkPDF borne le lot. Chaque PDF est généré à la volée, y compris son
// QR-facture ; quelques centaines suffisent à immobiliser le serveur, et une
// interface qui propose « tout sélectionner » rend un lot énorme trivial à
// demander par accident.
const maxBulkPDF = 200

type bulkPDFRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

// BulkInvoicePDF POST /api/v1/invoices/bulk-pdf
//
// Un identifiant → un PDF. Plusieurs → un ZIP.
func (h *InvoicesHandler) BulkInvoicePDF(c *gin.Context) {
	var req bulkPDFRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// Dédoublonnage : une sélection construite par clics peut contenir deux
	// fois la même ligne, et le ZIP porterait alors deux entrées de même nom —
	// que certains outils refusent d'extraire.
	seen := map[string]bool{}
	ids := make([]string, 0, len(req.IDs))
	for _, id := range req.IDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}

	switch {
	case len(ids) == 0:
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "aucun document sélectionné"})
		return
	case len(ids) > maxBulkPDF:
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": fmt.Sprintf(
				"%d documents demandés, %d au maximum par téléchargement. Affinez la période ou le statut.",
				len(ids), maxBulkPDF)})
		return
	}

	// Le budget suit la taille du lot : un délai fixe suffirait pour un
	// document et couperait au milieu d'un lot de cent, laissant l'utilisateur
	// avec une archive tronquée sans savoir pourquoi.
	ctx, cancel := context.WithTimeout(c.Request.Context(),
		time.Duration(10+len(ids))*time.Second)
	defer cancel()

	// ── Un seul document : le PDF nu ────────────────────────────────────────
	if len(ids) == 1 {
		pdfBytes, filename, err := h.buildInvoicePDF(ctx, ids[0], i18n.Langue(c))
		switch {
		case err == errInvoiceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "document introuvable"})
			return
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
		return
	}

	// ── Plusieurs : une archive ─────────────────────────────────────────────
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := map[string]int{}
	var missing []string

	for _, id := range ids {
		pdfBytes, filename, err := h.buildInvoicePDF(ctx, id, i18n.Langue(c))
		if err == errInvoiceNotFound {
			// Un document disparu entre l'affichage de la liste et le clic ne
			// doit pas faire échouer les autres. Il est nommé dans l'en-tête de
			// réponse plutôt que passé sous silence : une archive plus courte
			// que la sélection, sans explication, se remarque trop tard.
			missing = append(missing, id)
			continue
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Deux documents peuvent porter le même numéro s'ils sont de types
		// différents ; le suffixe évite qu'une entrée en écrase une autre.
		names[filename]++
		if n := names[filename]; n > 1 {
			base := strings.TrimSuffix(filename, ".pdf")
			filename = fmt.Sprintf("%s-%d.pdf", base, n)
		}

		w, err := zw.Create(filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "écriture dans l'archive impossible"})
			return
		}
		if _, err := w.Write(pdfBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "écriture dans l'archive impossible"})
			return
		}
	}

	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "l'archive ZIP n'a pas pu être finalisée"})
		return
	}
	if len(names) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "aucun des documents demandés n'existe"})
		return
	}

	if len(missing) > 0 {
		c.Header("X-LedgerAlps-Missing", fmt.Sprintf("%d", len(missing)))
	}
	filename := fmt.Sprintf("documents-%s.zip", time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

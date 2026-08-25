package handlers

// Rapprochement bancaire — points d'entrée.
//
// L'import camt.053 existait et ne gardait rien. Il conserve désormais, et ces
// routes exposent ce qui suit : la liste des écritures non rapprochées avec
// leur suggestion, la confirmation, l'annulation, la mise à l'écart.
//
// Aucune de ces routes ne crée de paiement. Confirmer un rapprochement dit
// « j'ai identifié ce versement » ; encaisser reste un geste distinct, fait
// depuis la facture, par le chemin déjà éprouvé. Solder une facture parce qu'un
// montant correspond ferait passer pour réglées des créances que personne n'a
// vérifiées.

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/banking"
)

type ReconciliationHandler struct {
	svc *banking.Service

	// db et usePostgres servent a la piste d'audit : rapprocher une ecriture
	// bancaire d'une facture est une decision comptable, et elle ne laissait
	// aucune trace alors qu'ActionBankEntryMatched etait declaree.
	db          *sql.DB
	usePostgres bool
}

func NewReconciliationHandler(database *sql.DB, usePostgres bool) *ReconciliationHandler {
	return &ReconciliationHandler{
		svc:         banking.New(database, usePostgres),
		db:          database,
		usePostgres: usePostgres,
	}
}

// ListBankEntries GET /api/v1/bank-entries?all=true
func (h *ReconciliationHandler) ListBankEntries(c *gin.Context) {
	entries, err := h.svc.List(c.Request.Context(), c.Query("all") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []banking.Entry{}
	}
	c.JSON(http.StatusOK, gin.H{"items": entries, "total": len(entries)})
}

// MatchBankEntry PUT /api/v1/bank-entries/:id/match
func (h *ReconciliationHandler) MatchBankEntry(c *gin.Context) {
	var body struct {
		InvoiceID string `json:"invoice_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	userID := ""
	if claims := mw.GetClaims(c); claims != nil {
		userID = claims.UserID
	}
	if err := h.svc.Match(c.Request.Context(), c.Param("id"), body.InvoiceID, userID); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	trace(c, h.db, h.usePostgres, TableBankEntries,
		accounting.ActionBankEntryMatched, c.Param("id"),
		accounting.Creation(map[string]any{"invoice_id": body.InvoiceID}))

	c.JSON(http.StatusOK, gin.H{
		"message": "Écriture rapprochée. L'encaissement reste à enregistrer depuis la facture : " +
			"rapprocher identifie le versement, il ne solde pas la créance.",
	})
}

// UnmatchBankEntry DELETE /api/v1/bank-entries/:id/match
func (h *ReconciliationHandler) UnmatchBankEntry(c *gin.Context) {
	if err := h.svc.Unmatch(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "rapprochement annulé"})
}

// IgnoreBankEntry PUT /api/v1/bank-entries/:id/ignore
func (h *ReconciliationHandler) IgnoreBankEntry(c *gin.Context) {
	var body struct {
		Ignored bool `json:"ignored"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.svc.Ignore(c.Request.Context(), c.Param("id"), body.Ignored); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "écriture mise à jour"})
}

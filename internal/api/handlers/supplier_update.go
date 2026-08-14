package handlers

// Modifier une facture fournisseur.
//
// # Seulement au brouillon
//
// Une facture comptabilisée porte une écriture scellée dans la chaîne du
// CO art. 957a. Changer son montant ferait mentir le journal : les livres
// diraient une chose, la pièce une autre, et l'empreinte ne protégerait plus
// que la version d'origine. La correction passe donc par le retour au
// brouillon, qui n'existe pas — donc par une contrepassation, comme pour toute
// écriture déjà passée.
//
// # Pourquoi cette route manquait
//
// L'écran Achats permettait de saisir et de comptabiliser, mais pas de
// corriger. Une faute de frappe sur un montant obligeait donc à supprimer le
// brouillon et à tout ressaisir — ce qui est faisable, mais absurde tant que la
// pièce n'est engagée nulle part.

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// UpdateSupplierInvoice PUT /api/v1/supplier-invoices/:id
func (h *SupplierInvoicesHandler) UpdateSupplierInvoice(c *gin.Context) {
	id := c.Param("id")

	var req createSupplierInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if _, err := time.Parse("2006-01-02", req.IssueDate); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "la date de la facture doit être au format AAAA-MM-JJ"})
		return
	}
	if req.DueDate != "" {
		if _, err := time.Parse("2006-01-02", req.DueDate); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "l'échéance doit être au format AAAA-MM-JJ"})
			return
		}
	}
	if req.Currency == "" {
		req.Currency = "CHF"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var status string
	if err := h.db.QueryRowContext(ctx,
		db.Rebind(`SELECT status FROM supplier_invoices WHERE id = ?`, h.usePostgres), id).
		Scan(&status); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "facture fournisseur introuvable"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	if status != "draft" {
		// Le message dit quoi faire, pas seulement que c'est refusé.
		c.JSON(http.StatusConflict, gin.H{
			"error": "cette facture est déjà comptabilisée : son écriture est scellée " +
				"(CO art. 957a) et la modifier ferait mentir le journal. Passez par une " +
				"écriture de correction."})
		return
	}

	subtotal, vat, total, dominantRate := computeTotals(req.Lines)

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	upd := db.Rebind(`
		UPDATE supplier_invoices
		SET supplier_id = ?, supplier_reference = ?, issue_date = ?, due_date = ?,
		    currency = ?, subtotal_amount = ?, vat_amount = ?, total_amount = ?,
		    vat_rate = ?, expense_account_code = ?, payment_reference = ?, notes = ?,
		    updated_at = ?
		WHERE id = ? AND status = 'draft'`, h.usePostgres)
	res, err := tx.ExecContext(ctx, upd,
		req.SupplierID, req.SupplierReference, req.IssueDate, nullIfEmpty(req.DueDate),
		req.Currency, subtotal, vat, total, dominantRate,
		nullIfEmpty(req.ExpenseAccountCode), strings.TrimSpace(req.PaymentReference),
		nullIfEmpty(req.Notes), time.Now().UTC(), id)
	if err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "ce fournisseur a déjà une facture portant cette référence"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	// La condition `status = 'draft'` est REPÉTÉE dans le UPDATE : entre la
	// lecture du statut et l'écriture, quelqu'un a pu comptabiliser la facture.
	// Sans elle, cette course écraserait une pièce déjà scellée.
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "la facture a été comptabilisée entre-temps : rechargez la page"})
		return
	}

	// Les lignes sont remplacées en bloc. Les rapprocher une à une supposerait
	// un identifiant stable côté client, que le formulaire ne porte pas — et la
	// pièce est un brouillon, donc rien ne dépend de ces identifiants.
	if _, err := tx.ExecContext(ctx,
		db.Rebind(`DELETE FROM supplier_invoice_lines WHERE supplier_invoice_id = ?`, h.usePostgres),
		id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	insLine := db.Rebind(`
		INSERT INTO supplier_invoice_lines
		    (id, supplier_invoice_id, description, quantity, unit_price, vat_rate,
		     line_total, expense_account_code, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
	for i, l := range req.Lines {
		if _, err := tx.ExecContext(ctx, insLine, db.NewID(), id, l.Description,
			l.Quantity, l.UnitPrice, l.VATRate, l.LineTotal,
			nullIfEmpty(l.ExpenseAccountCode), i); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id": id, "status": "draft",
		"subtotal_amount": subtotal, "vat_amount": vat, "total_amount": total,
	})
}

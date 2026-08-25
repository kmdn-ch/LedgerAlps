package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/invoicing"
)

type InvoicesHandler struct {
	db          *sql.DB
	usePostgres bool
	svc         *invoicing.Service
}

func NewInvoicesHandler(database *sql.DB, usePostgres bool, acctSvc *accounting.Service) *InvoicesHandler {
	return &InvoicesHandler{
		db:          database,
		usePostgres: usePostgres,
		svc:         invoicing.NewWithAccounting(database, usePostgres, acctSvc),
	}
}

// ListInvoices GET /api/v1/invoices
func (h *InvoicesHandler) ListInvoices(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	status := c.Query("status")
	where := " WHERE 1=1"
	args := []any{}

	// Filtrer par client ou fournisseur : sans cela on ne peut consulter que la
	// liste globale, ce qui devient inutilisable dès quelques dizaines de pièces.
	if contactID := c.Query("contact_id"); contactID != "" {
		where += " AND i.contact_id = ?"
		args = append(args, contactID)
	}
	// Facultatif : 'invoice', 'quote' ou 'credit_note'.
	if docType := c.Query("document_type"); docType != "" {
		where += " AND i.document_type = ?"
		args = append(args, docType)
	}

	// Bornes de date sur la date d'émission. Elles se filtrent ici et non dans
	// le navigateur : la pagination est côté serveur, donc un filtre client ne
	// verrait que la page déjà chargée — et donnerait un résultat qui change
	// selon la page où l'on se trouve.
	for _, f := range []struct {
		param, op string
	}{{"from", ">="}, {"to", "<="}} {
		v := c.Query(f.param)
		if v == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", v); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": f.param + " doit être au format AAAA-MM-JJ"})
			return
		}
		where += " AND i.issue_date " + f.op + " ?"
		args = append(args, v)
	}

	// Les factures NE SE FILTRENT PAS par auteur.
	//
	// Ce filtre ne montrait a un non-administrateur que les factures qu'il avait
	// lui-meme creees, sous couvert de minimisation des donnees. C'est un
	// contresens : les factures sont les pieces de l'entreprise, pas la boite de
	// reception de celui qui les a saisies. Un comptable ne voyait aucune des
	// factures emises par l'administrateur, une fiduciaire en lecture seule ne
	// voyait rien du tout, et le total du tableau de bord contredisait la liste.
	//
	// La minimisation nLPD porte sur les donnees personnelles, pas sur des
	// pieces que la loi oblige a conserver dix ans (CO art. 958f). Ce qui borne
	// l'acces ici est le ROLE : lire demande la permission de lecture, ecrire
	// celle d'ecriture, et un compte en lecture seule ne peut rien modifier.
	//
	// Meme correction que pour le journal en v1.4.8 — c'est la troisieme fois
	// que ce motif produit un defaut.

	// "overdue" is a filter, never a stored state: an invoice becomes late
	// because the due date passed, not because someone marked it. Translating
	// it here keeps server-side pagination correct — filtering client-side
	// would only ever search the page currently loaded.
	if status == "overdue" {
		where += " AND i.status = 'sent' AND i.due_date < ?"
		args = append(args, time.Now().Format("2006-01-02"))
	} else if status != "" {
		where += " AND i.status = ?"
		args = append(args, status)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	countQ := db.Rebind("SELECT COUNT(*) FROM invoices i"+where, h.usePostgres)
	var total int
	if err := h.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	// The contact join names the client; the correlated subquery reports how
	// much of each invoice has already been credited, which is what lets the
	// UI stop offering a credit note on an invoice that is fully credited.
	listQ := db.Rebind(`
		SELECT i.id, i.invoice_number, i.document_type, i.contact_id, i.status, i.issue_date, i.due_date,
		       i.currency, i.subtotal_amount, i.vat_amount, i.total_amount, i.vat_rate, i.amount_paid,
		       i.notes, i.terms, i.converted_from_id, i.quote_outcome, i.corrects_invoice_id,
		       i.created_at, i.updated_at,
		       COALESCE(co.name, ''),
		       COALESCE((SELECT SUM(cn.total_amount) FROM invoices cn
		                 WHERE cn.corrects_invoice_id = i.id AND cn.status <> 'cancelled'), 0)
		FROM invoices i
		LEFT JOIN contacts co ON co.id = i.contact_id`+where+`
		ORDER BY i.issue_date DESC, i.created_at DESC LIMIT ? OFFSET ?`, h.usePostgres)
	offset := (page - 1) * pageSize
	rows, err := h.db.QueryContext(ctx, listQ, append(args, pageSize, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	invoices := []models.Invoice{}
	for rows.Next() {
		var inv models.Invoice
		if err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.DocumentType, &inv.ContactID, &inv.Status,
			&inv.IssueDate, &inv.DueDate, &inv.Currency,
			&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate, &inv.AmountPaid,
			&inv.Notes, &inv.Terms, &inv.ConvertedFromID, &inv.QuoteOutcome, &inv.CorrectsInvoiceID,
			&inv.CreatedAt, &inv.UpdatedAt, &inv.ContactName, &inv.CreditedAmount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		invoices = append(invoices, inv)
	}

	pages := (total + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"items": invoices, "total": total, "page": page, "page_size": pageSize, "pages": pages,
	})
}

// GetInvoice GET /api/v1/invoices/:id
func (h *InvoicesHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT i.id, i.invoice_number, i.document_type, i.contact_id, i.status, i.issue_date, i.due_date,
		       i.currency, i.subtotal_amount, i.vat_amount, i.total_amount, i.vat_rate, i.amount_paid,
		       i.notes, i.terms, i.converted_from_id, i.quote_outcome, i.corrects_invoice_id,
		       i.created_at, i.updated_at,
		       COALESCE(co.name, ''),
		       COALESCE((SELECT SUM(cn.total_amount) FROM invoices cn
		                 WHERE cn.corrects_invoice_id = i.id AND cn.status <> 'cancelled'), 0)
		FROM invoices i
		LEFT JOIN contacts co ON co.id = i.contact_id
		WHERE i.id = ?`, h.usePostgres)

	var inv models.Invoice
	err := h.db.QueryRowContext(ctx, q, id).Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.DocumentType, &inv.ContactID, &inv.Status,
		&inv.IssueDate, &inv.DueDate, &inv.Currency,
		&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate, &inv.AmountPaid,
		&inv.Notes, &inv.Terms, &inv.ConvertedFromID, &inv.QuoteOutcome, &inv.CorrectsInvoiceID,
		&inv.CreatedAt, &inv.UpdatedAt, &inv.ContactName, &inv.CreditedAmount)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "facture introuvable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	// Load invoice lines.
	linesQ := db.Rebind(`
		SELECT id, invoice_id, description, quantity, unit, unit_price, discount_pct, vat_rate, line_total, sequence
		FROM invoice_lines WHERE invoice_id = ? ORDER BY sequence`, h.usePostgres)
	lrows, err := h.db.QueryContext(ctx, linesQ, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer lrows.Close()
	inv.Lines = []models.InvoiceLine{}
	for lrows.Next() {
		var l models.InvoiceLine
		if err := lrows.Scan(&l.ID, &l.InvoiceID, &l.Description, &l.Quantity, &l.Unit,
			&l.UnitPrice, &l.DiscountPct, &l.VATRate, &l.LineTotal, &l.Sequence); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		inv.Lines = append(inv.Lines, l)
	}

	c.JSON(http.StatusOK, inv)
}

type createInvoiceRequest struct {
	DocumentType string    `json:"document_type"`
	ContactID    string    `json:"contact_id" binding:"required"`
	IssueDate    string    `json:"issue_date" binding:"required"`
	DueDate      string    `json:"due_date" binding:"required"`
	Currency     string    `json:"currency"`
	Notes        *string   `json:"notes"`
	Terms        *string   `json:"terms"`
	Lines        []lineReq `json:"lines" binding:"required,min=1"`
}

type lineReq struct {
	Description string  `json:"description" binding:"required"`
	Quantity    float64 `json:"quantity" binding:"required,gt=0"`
	Unit        *string `json:"unit"`
	UnitPrice   float64 `json:"unit_price" binding:"required"`
	DiscountPct float64 `json:"discount_pct"`
	VATRate     float64 `json:"vat_rate"`
	Sequence    int     `json:"sequence"`
}

// CreateInvoice POST /api/v1/invoices
func (h *InvoicesHandler) CreateInvoice(c *gin.Context) {
	var req createInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'émission doit être au format AAAA-MM-JJ"})
		return
	}
	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'échéance doit être au format AAAA-MM-JJ"})
		return
	}

	lines := make([]invoicing.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = invoicing.LineInput{
			Description: l.Description,
			Quantity:    l.Quantity,
			Unit:        l.Unit,
			UnitPrice:   l.UnitPrice,
			DiscountPct: l.DiscountPct,
			VATRate:     l.VATRate,
			Sequence:    l.Sequence,
		}
	}

	claims := mw.GetClaims(c)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}
	inv, err := h.svc.CreateInvoice(c.Request.Context(), userID, invoicing.CreateInvoiceRequest{
		DocumentType: req.DocumentType,
		ContactID:    req.ContactID,
		IssueDate:    issueDate,
		DueDate:      dueDate,
		Currency:     req.Currency,
		Notes:        req.Notes,
		Terms:        req.Terms,
		Lines:        lines,
	})
	if err != nil {
		// 409 : la demande est bien formée, elle se heurte à une règle légale.
		// Le distinguer d'un 422 permet à l'interface de présenter un refus
		// explicable plutôt qu'une erreur de saisie.
		if errors.Is(err, invoicing.ErrVATWithoutNumber) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "reason": "vat_without_number"})
			return
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, inv)
}

// UpdateInvoice PATCH /api/v1/invoices/:id
func (h *InvoicesHandler) UpdateInvoice(c *gin.Context) {
	id := c.Param("id")
	var req createInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'émission doit être au format AAAA-MM-JJ"})
		return
	}
	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'échéance doit être au format AAAA-MM-JJ"})
		return
	}

	lines := make([]invoicing.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = invoicing.LineInput{
			Description: l.Description,
			Quantity:    l.Quantity,
			Unit:        l.Unit,
			UnitPrice:   l.UnitPrice,
			DiscountPct: l.DiscountPct,
			VATRate:     l.VATRate,
			Sequence:    l.Sequence,
		}
	}

	// UpdateInvoiceBy : l'auteur et son adresse entrent dans la chaine.
	actor := invoicing.Actor{IP: c.ClientIP()}
	if claims := mw.GetClaims(c); claims != nil {
		actor.UserID = claims.UserID
	}
	_, err = h.svc.UpdateInvoiceBy(c.Request.Context(), id, invoicing.CreateInvoiceRequest{
		DocumentType: req.DocumentType,
		ContactID:    req.ContactID,
		IssueDate:    issueDate,
		DueDate:      dueDate,
		Currency:     req.Currency,
		Notes:        req.Notes,
		Terms:        req.Terms,
		Lines:        lines,
	}, actor)
	if err != nil {
		switch err {
		case invoicing.ErrInvoiceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case invoicing.ErrInvoicePaid:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			// Les refus métier arrivaient ici et sortaient en 500, ce qui les
			// faisait passer pour une panne alors qu'ils sont une décision.
			switch {
			case errors.Is(err, invoicing.ErrCreditExceedsInvoice):
				c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "reason": "credit_exceeds_invoice"})
			case errors.Is(err, invoicing.ErrVATWithoutNumber):
				c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "reason": "vat_without_number"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
		}
		return
	}
	h.GetInvoice(c)
}

// TransitionInvoice POST /api/v1/invoices/:id/transition
func (h *InvoicesHandler) TransitionInvoice(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// TransitionBy et non Transition : sans auteur, « qui a annulé cette
	// facture » reste sans réponse. Le chemin HTTP est le seul qui connaisse
	// l'utilisateur, c'est donc lui qui doit le transmettre.
	actor := invoicing.Actor{IP: c.ClientIP()}
	if claims := mw.GetClaims(c); claims != nil {
		actor.UserID = claims.UserID
	}
	if err := h.svc.TransitionBy(c.Request.Context(), id, models.InvoiceStatus(body.Status), actor); err != nil {
		switch err {
		case invoicing.ErrInvoiceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case invoicing.ErrInvalidTransition:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		}
		return
	}
	h.GetInvoice(c)
}

// ConvertQuote POST /api/v1/invoices/:id/convert
//
// Produces an invoice from a price offer. The offer is kept: the client holds
// a copy of it, so replacing the record would leave them citing a reference
// that no longer exists here. Both documents keep their own number and point
// at each other (CO art. 957a al. 2 ch. 5, art. 958f al. 3).
func (h *InvoicesHandler) ConvertQuote(c *gin.Context) {
	id := c.Param("id")

	var body struct {
		IssueDate string `json:"issue_date"`
		DueDate   string `json:"due_date"`
	}
	// An empty body is valid: the dates then default to today and today + 30d.
	_ = c.ShouldBindJSON(&body)

	var req invoicing.ConvertQuoteRequest
	if body.IssueDate != "" {
		d, err := time.Parse("2006-01-02", body.IssueDate)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'émission doit être au format AAAA-MM-JJ"})
			return
		}
		req.IssueDate = d
	}
	if body.DueDate != "" {
		d, err := time.Parse("2006-01-02", body.DueDate)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'échéance doit être au format AAAA-MM-JJ"})
			return
		}
		req.DueDate = d
	}

	claims := mw.GetClaims(c)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	inv, err := h.svc.ConvertQuote(c.Request.Context(), id, userID, req)
	if err != nil {
		switch err {
		case invoicing.ErrInvoiceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case invoicing.ErrQuoteAlreadyConv:
			// The offer already produced an invoice; billing it twice is the
			// mistake this status guards against.
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case invoicing.ErrNotAQuote, invoicing.ErrQuoteNotConvertible:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, inv)
}

// SetQuoteOutcome POST /api/v1/invoices/:id/outcome
//
// Records that an offer was refused or expired. "accepted" is not settable
// here — an offer is accepted by producing the invoice, via /convert.
func (h *InvoicesHandler) SetQuoteOutcome(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Outcome string `json:"outcome" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.SetQuoteOutcome(c.Request.Context(), id, body.Outcome); err != nil {
		switch err {
		case invoicing.ErrInvoiceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case invoicing.ErrInvalidQuoteOutcome:
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "issue attendue: 'refused' ou 'expired' — une offre est acceptée en la convertissant en facture"})
		default:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		}
		return
	}
	h.GetInvoice(c)
}

// CreateCreditNote POST /api/v1/invoices/:id/credit-note
//
// Issues a credit note against an invoice. An empty body credits the invoice in
// full; supplying lines credits part of it.
func (h *InvoicesHandler) CreateCreditNote(c *gin.Context) {
	id := c.Param("id")

	var body struct {
		IssueDate string    `json:"issue_date"`
		Reason    *string   `json:"reason"`
		Lines     []lineReq `json:"lines"`
	}
	_ = c.ShouldBindJSON(&body)

	req := invoicing.CreateCreditNoteRequest{Reason: body.Reason}
	if body.IssueDate != "" {
		d, err := time.Parse("2006-01-02", body.IssueDate)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date d'émission doit être au format AAAA-MM-JJ"})
			return
		}
		req.IssueDate = d
	}
	for _, l := range body.Lines {
		req.Lines = append(req.Lines, invoicing.LineInput{
			Description: l.Description,
			Quantity:    l.Quantity,
			Unit:        l.Unit,
			UnitPrice:   l.UnitPrice,
			DiscountPct: l.DiscountPct,
			VATRate:     l.VATRate,
			Sequence:    l.Sequence,
		})
	}

	claims := mw.GetClaims(c)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	note, err := h.svc.CreateCreditNote(c.Request.Context(), id, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, invoicing.ErrInvoiceNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, invoicing.ErrCreditExceedsInvoice):
			// 409: the request is well-formed, it conflicts with what has
			// already been credited against this invoice.
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, note)
}

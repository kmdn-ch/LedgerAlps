package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/vat"
)

// ─── FiscalYearHandler ────────────────────────────────────────────────────────

// FiscalYearHandler serves fiscal year management and VAT declaration endpoints.
type FiscalYearHandler struct {
	db          *sql.DB
	usePostgres bool
	fySvc       *accounting.FiscalYearService
	vatSvc      *vat.Service
}

// NewFiscalYearHandler creates a FiscalYearHandler wiring up the required services.
func NewFiscalYearHandler(database *sql.DB, usePostgres bool) *FiscalYearHandler {
	return &FiscalYearHandler{
		db:          database,
		usePostgres: usePostgres,
		fySvc:       accounting.NewFiscalYearService(database, usePostgres),
		vatSvc:      vat.New(database, usePostgres),
	}
}

// ─── GET /api/v1/fiscal-years ─────────────────────────────────────────────────

// ListFiscalYears returns all fiscal years ordered by start_date descending.
// Access: any authenticated user.
func (h *FiscalYearHandler) ListFiscalYears(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, name, start_date, end_date, is_closed, created_at, updated_at
		FROM fiscal_years
		ORDER BY start_date DESC
	`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	years := []models.FiscalYear{}
	for rows.Next() {
		var fy models.FiscalYear
		var isClosed int
		if err := rows.Scan(
			&fy.ID, &fy.Name, &fy.StartDate, &fy.EndDate,
			&isClosed, &fy.CreatedAt, &fy.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		fy.IsClosed = isClosed == 1
		years = append(years, fy)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": years, "total": len(years)})
}

// ─── POST /api/v1/fiscal-years ───────────────────────────────────────────────

type createFiscalYearRequest struct {
	Name      string `json:"name" binding:"required"`
	StartDate string `json:"start_date" binding:"required"` // YYYY-MM-DD
	EndDate   string `json:"end_date" binding:"required"`   // YYYY-MM-DD
}

// CreateFiscalYear déclare un exercice comptable.
//
// Il n'existait aucune route pour en créer un : l'installation n'en sème aucun
// et seule la clôture en créait un — le suivant. Un exercice décalé (juillet à
// juin, fréquent en Suisse) était donc impossible à déclarer, et LedgerAlps
// crée alors l'année civile au premier enregistrement. Cette route permet de
// poser le bon exercice **avant** d'y comptabiliser quoi que ce soit.
//
// Accès : administrateur uniquement.
func (h *FiscalYearHandler) CreateFiscalYear(c *gin.Context) {
	// La garde qui lisait le drapeau administrateur DU JETON a ete retiree.
	//
	// Deux defauts en un. Elle lisait un drapeau fige a la connexion : rétrograder
	// quelqu'un le laissait agir jusqu'a l'expiration de son jeton. Et elle
	// reservait a l'administrateur les exercices comptables, qui est
	// le metier du COMPTABLE — il devait demander a quelqu'un dont le role est de
	// gerer des mots de passe.
	//
	// La permission est desormais declaree sur la route (authz.PermManage) et lue
	// dans la base a chaque requete.

	var req createFiscalYearRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		refus(c, http.StatusUnprocessableEntity, err)
		return
	}

	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date de début doit être au format AAAA-MM-JJ"})
		return
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date de fin doit être au format AAAA-MM-JJ"})
		return
	}
	if !end.After(start) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la date de fin doit suivre la date de début"})
		return
	}
	// Un exercice « 0026 » ou « 9999 » rendrait les écrans d'exercices et l'archive
	// légale illisibles (audit 9, A-03).
	for _, d := range []time.Time{start, end} {
		if err := compliance.CheckBookingDate(d); err != nil {
			refus(c, http.StatusUnprocessableEntity, err)
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Le contrôle de chevauchement et l'insertion vivent dans UNE transaction (audit
	// croisé Gemini, MED-11) : lus l'un après l'autre hors transaction, deux créations
	// simultanées passaient toutes deux le contrôle.
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Deux exercices qui se chevauchent rendraient le rattachement d'une
	// écriture arbitraire, donc la clôture arbitraire. Refuser ici est la seule
	// occasion de l'empêcher.
	var overlapping string
	overlapQ := db.Rebind(`
		SELECT name FROM fiscal_years
		WHERE start_date <= ? AND end_date >= ?
		LIMIT 1`, h.usePostgres)
	err = tx.QueryRowContext(ctx, overlapQ, req.EndDate, req.StartDate).Scan(&overlapping)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "la période chevauche l'exercice « " + overlapping + " »",
		})
		return
	}
	if err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	id := db.NewID()
	insertQ := db.Rebind(`
		INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed)
		VALUES (?, ?, ?, ?, 0)`, h.usePostgres)
	if _, err := tx.ExecContext(ctx, insertQ, id, req.Name, req.StartDate, req.EndDate); err != nil {
		// 409 pour le SEUL cas qu'il décrit : le nom est pris. Toute erreur d'insertion
		// répondait « un exercice porte déjà ce nom » — un disque plein s'y lisait comme
		// une faute de saisie (MED-11).
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "un exercice porte déjà ce nom"})
			return
		}
		erreurInterne(c, msgErreurInterne, err)
		return
	}
	if err := tx.Commit(); err != nil {
		erreurInterne(c, msgErreurInterne, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id": id, "name": req.Name,
		"start_date": req.StartDate, "end_date": req.EndDate,
		"is_closed": false,
	})
}

// ─── POST /api/v1/fiscal-years/:id/close ─────────────────────────────────────

// CloseFiscalYear triggers the year-end closing procedure (CO art. 958).
// Access: admin only (RequireAdmin middleware must be applied at the router level).
func (h *FiscalYearHandler) CloseFiscalYear(c *gin.Context) {
	fiscalYearID := c.Param("id")
	if fiscalYearID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "l'identifiant de l'exercice est requis"})
		return
	}

	userID := currentUserID(c)

	ctx := accounting.WithClientIP(c.Request.Context(), c.ClientIP())
	if err := h.fySvc.CloseYear(ctx, fiscalYearID, userID); err != nil {
		refus(c, http.StatusUnprocessableEntity, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "closed", "fiscal_year_id": fiscalYearID})
}

// ─── /api/v1/fiscal-years/:id/closing-repair ─────────────────────────────────

// PlanClosingRepair GET /api/v1/fiscal-years/:id/closing-repair
//
// Ce que le complément de la clôture passerait, sans rien écrire. À lire — et à faire lire à la
// fiduciaire — avant d'appliquer (voir accounting.FiscalYearService.RepairClosing).
func (h *FiscalYearHandler) PlanClosingRepair(c *gin.Context) {
	plan, err := h.fySvc.PlanClosingRepair(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.refuserReparation(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// RepairClosing POST /api/v1/fiscal-years/:id/closing-repair
//
// Passe le complément. Le corps doit porter `{"confirm": true}` : une réparation du journal ne
// part pas d'un clic sans aperçu, ni d'un appel qui oublie un paramètre.
func (h *FiscalYearHandler) RepairClosing(c *gin.Context) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := bindOptionalJSON(c, &body); err != nil {
		refus(c, http.StatusUnprocessableEntity, err)
		return
	}
	if !body.Confirm {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":  "la réparation doit être confirmée : consultez d'abord ce qu'elle passera, puis envoyez confirm: true",
			"reason": "confirmation_required",
		})
		return
	}
	plan, err := h.fySvc.RepairClosing(accounting.WithClientIP(c.Request.Context(), c.ClientIP()), c.Param("id"), currentUserID(c))
	if err != nil {
		h.refuserReparation(c, err)
		return
	}
	c.JSON(http.StatusCreated, plan)
}

// refuserReparation traduit un refus de la réparation en 409 (l'état de l'exercice ne s'y prête
// pas) ou 404 ; tout le reste est une erreur de lecture ordinaire.
func (h *FiscalYearHandler) refuserReparation(c *gin.Context, err error) {
	switch {
	case errors.Is(err, accounting.ErrReparationExerciceOuvert),
		errors.Is(err, accounting.ErrReparationSansCloture),
		errors.Is(err, accounting.ErrReparationClotureEquilibree),
		errors.Is(err, accounting.ErrReparationDejaFaite),
		errors.Is(err, accounting.ErrReparationInexpliquee):
		refus(c, http.StatusConflict, err)
	case strings.Contains(err.Error(), "not found"):
		c.JSON(http.StatusNotFound, gin.H{"error": "exercice introuvable"})
	default:
		refus(c, http.StatusUnprocessableEntity, err)
	}
}

// ─── POST /api/v1/vat/declaration ────────────────────────────────────────────

type vatDeclarationRequest struct {
	PeriodStart string `json:"period_start" binding:"required"` // YYYY-MM-DD
	PeriodEnd   string `json:"period_end" binding:"required"`   // YYYY-MM-DD
	Method      string `json:"method" binding:"required"`       // "effective" or "tdfn"
}

// GenerateVATDeclaration computes the VAT declaration for a given period.
// Access: admin only.
func (h *FiscalYearHandler) GenerateVATDeclaration(c *gin.Context) {

	var req vatDeclarationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		refus(c, http.StatusUnprocessableEntity, err)
		return
	}

	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "le début de période doit être au format AAAA-MM-JJ"})
		return
	}
	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la fin de période doit être au format AAAA-MM-JJ"})
		return
	}
	if periodEnd.Before(periodStart) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "la fin de période doit être égale ou postérieure à son début"})
		return
	}

	decl, err := h.vatSvc.GenerateDeclaration(c.Request.Context(), periodStart, periodEnd, req.Method)
	if err != nil {
		refus(c, http.StatusUnprocessableEntity, err)
		return
	}

	c.JSON(http.StatusOK, decl)
}

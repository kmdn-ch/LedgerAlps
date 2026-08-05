package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

type JournalWriteHandler struct {
	svc         *accounting.Service
	db          *sql.DB
	usePostgres bool
}

func NewJournalWriteHandler(svc *accounting.Service, database *sql.DB, usePostgres bool) *JournalWriteHandler {
	return &JournalWriteHandler{svc: svc, db: database, usePostgres: usePostgres}
}

// resolveAccount traduit un numéro de compte en identifiant.
//
// Le refus nomme le numéro fautif et dit où trouver les bons. « compte
// introuvable » oblige à deviner lequel des quatre de l'écriture est en cause,
// et un compte désactivé refusé sans explication ressemble à une faute de
// frappe alors que c'est une décision.
func (h *JournalWriteHandler) resolveAccount(c *gin.Context, code string) (string, error) {
	var id string
	var isActive int
	q := db.Rebind(`SELECT id, is_active FROM accounts WHERE code = ?`, h.usePostgres)
	err := h.db.QueryRowContext(c.Request.Context(), q, code).Scan(&id, &isActive)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("le compte %s n'existe pas dans le plan comptable "+
			"(Plan comptable → la liste des numéros disponibles)", code)
	}
	if err != nil {
		return "", fmt.Errorf("le compte %s n'a pas pu être lu", code)
	}
	if isActive != 1 {
		return "", fmt.Errorf("le compte %s est désactivé", code)
	}
	return id, nil
}

// journalLineReq accepte le compte par IDENTIFIANT ou par NUMÉRO.
//
// Le numéro est ce qu'un comptable a sous les yeux : « 1000 », « 3200 ». Exiger
// l'identifiant interne obligeait toute interface à charger le plan comptable,
// à faire la traduction, et à échouer silencieusement quand elle se trompait.
// C'est exactement ce qui s'est produit : le formulaire du journal envoyait des
// numéros dans un champ qui attendait des identifiants, et chaque saisie
// répondait 422 sans jamais dire pourquoi.
type journalLineReq struct {
	AccountID    string   `json:"account_id"`
	AccountCode  string   `json:"account_code"`
	DebitAmount  *float64 `json:"debit_amount"`
	CreditAmount *float64 `json:"credit_amount"`
	Description  string   `json:"description"`
	Sequence     int      `json:"sequence"`
}

type createEntryRequest struct {
	Date        string           `json:"date" binding:"required"`
	Description string           `json:"description"`
	Lines       []journalLineReq `json:"lines" binding:"required,min=2"`
}

// CreateEntry POST /api/v1/journal
func (h *JournalWriteHandler) CreateEntry(c *gin.Context) {
	var req createEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Le message du validateur est illisible pour qui reçoit la réponse :
		// « Field validation for 'Lines' failed on the 'min' tag » n'apprend
		// rien à personne. Une écriture en partie double a par construction au
		// moins deux lignes — un débit et un crédit — et c'est cela qu'il faut
		// dire.
		if len(req.Lines) < 2 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "une écriture comporte au moins deux lignes : ce qui est débité " +
					"et ce qui est crédité (CO art. 957)"})
			return
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "la date doit être au format AAAA-MM-JJ"})
		return
	}

	lines := make([]accounting.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		accountID := l.AccountID
		if accountID == "" {
			code := strings.TrimSpace(l.AccountCode)
			if code == "" {
				c.JSON(http.StatusUnprocessableEntity, gin.H{
					"error": fmt.Sprintf("ligne %d : aucun compte indiqué", i+1)})
				return
			}
			resolved, err := h.resolveAccount(c, code)
			if err != nil {
				c.JSON(http.StatusUnprocessableEntity, gin.H{
					"error": fmt.Sprintf("ligne %d : %s", i+1, err.Error())})
				return
			}
			accountID = resolved
		}
		if (l.DebitAmount == nil || *l.DebitAmount == 0) &&
			(l.CreditAmount == nil || *l.CreditAmount == 0) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": fmt.Sprintf("ligne %d : ni débit ni crédit", i+1)})
			return
		}
		if l.DebitAmount != nil && *l.DebitAmount != 0 &&
			l.CreditAmount != nil && *l.CreditAmount != 0 {
			// Une même ligne débitée ET créditée est presque toujours une
			// erreur de saisie. Elle passerait le contrôle d'équilibre en
			// s'annulant elle-même, et le compte porterait deux mouvements
			// fantômes que rien n'expliquerait à la relecture.
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": fmt.Sprintf("ligne %d : un compte est débité ou crédité, pas les deux", i+1)})
			return
		}
		if (l.DebitAmount != nil && *l.DebitAmount < 0) ||
			(l.CreditAmount != nil && *l.CreditAmount < 0) {
			// Un montant négatif inverserait le sens de l'écriture sans le dire.
			// La contrepartie s'écrit de l'autre côté, pas avec un signe.
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": fmt.Sprintf("ligne %d : un montant ne peut pas être négatif — "+
					"inscrivez-le de l'autre côté", i+1)})
			return
		}
		lines[i] = accounting.LineInput{
			AccountID:    accountID,
			DebitAmount:  l.DebitAmount,
			CreditAmount: l.CreditAmount,
			Description:  l.Description,
			Sequence:     l.Sequence,
		}
	}

	claims := mw.GetClaims(c)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	entry, err := h.svc.CreateEntry(c.Request.Context(), userID, accounting.CreateEntryRequest{
		Date:        date,
		Description: req.Description,
		Lines:       lines,
	})
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, entry)
}

// PostEntry POST /api/v1/journal/:id/post
func (h *JournalWriteHandler) PostEntry(c *gin.Context) {
	entryID := c.Param("id")

	claims := mw.GetClaims(c)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	if err := h.svc.PostEntry(c.Request.Context(), userID, entryID, c.ClientIP()); err != nil {
		switch err {
		case accounting.ErrEntryNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case accounting.ErrAlreadyPosted:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "posted"})
}

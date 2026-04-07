package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

type JournalWriteHandler struct {
	svc *accounting.Service
}

func NewJournalWriteHandler(svc *accounting.Service) *JournalWriteHandler {
	return &JournalWriteHandler{svc: svc}
}

type journalLineReq struct {
	AccountID    string   `json:"account_id" binding:"required"`
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
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "date must be YYYY-MM-DD"})
		return
	}

	lines := make([]accounting.LineInput, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = accounting.LineInput{
			AccountID:    l.AccountID,
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

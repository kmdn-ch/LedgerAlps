package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/services/iso20022"
)

// ISO20022Handler handles ISO 20022 payment generation and bank statement import.
type ISO20022Handler struct{}

func NewISO20022Handler() *ISO20022Handler { return &ISO20022Handler{} }

// ─── pain.001 — Credit Transfer Export ───────────────────────────────────────

type pain001Transaction struct {
	EndToEndID   string  `json:"end_to_end_id"  binding:"required"`
	CreditorName string  `json:"creditor_name"  binding:"required"`
	CreditorIBAN string  `json:"creditor_iban"  binding:"required"`
	Amount       float64 `json:"amount"         binding:"required,gt=0"`
	Currency     string  `json:"currency"`
	Reference    string  `json:"reference"`    // QRR ref (structured)
	Unstructured string  `json:"unstructured"` // free text
}

type pain001Request struct {
	ExecutionDate string               `json:"execution_date" binding:"required"` // YYYY-MM-DD
	DebtorName    string               `json:"debtor_name"    binding:"required"`
	DebtorIBAN    string               `json:"debtor_iban"    binding:"required"`
	DebtorBIC     string               `json:"debtor_bic"`
	Transactions  []pain001Transaction `json:"transactions"   binding:"required,min=1"`
}

// ExportPain001 godoc
// POST /api/v1/payments/export
// Generates a pain.001.001.09 XML file for the given payment batch.
// Returns application/xml as a download.
func (h *ISO20022Handler) ExportPain001(c *gin.Context) {
	var req pain001Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	execDate, err := time.Parse("2006-01-02", req.ExecutionDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "execution_date must be YYYY-MM-DD"})
		return
	}

	// Fall back to company env var if debtor name not provided per-request
	if req.DebtorName == "" {
		req.DebtorName = os.Getenv("COMPANY_NAME")
	}
	if req.DebtorIBAN == "" {
		req.DebtorIBAN = os.Getenv("COMPANY_IBAN")
	}

	txs := make([]iso20022.CreditTransfer, len(req.Transactions))
	for i, t := range req.Transactions {
		cur := t.Currency
		if cur == "" {
			cur = "CHF"
		}
		txs[i] = iso20022.CreditTransfer{
			EndToEndID:   t.EndToEndID,
			CreditorName: t.CreditorName,
			CreditorIBAN: t.CreditorIBAN,
			Amount:       t.Amount,
			Currency:     cur,
			Reference:    t.Reference,
			Unstructured: t.Unstructured,
		}
	}

	xmlBytes, err := iso20022.GeneratePain001(iso20022.Pain001Request{
		DebtorName:    req.DebtorName,
		DebtorIBAN:    req.DebtorIBAN,
		DebtorBIC:     req.DebtorBIC,
		ExecutionDate: execDate,
		Transactions:  txs,
	})
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("pain001-%s.xml", req.ExecutionDate)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/xml; charset=UTF-8", xmlBytes)
}

// ─── camt.053 — Bank Statement Import ────────────────────────────────────────

// ImportCamt053 godoc
// POST /api/v1/bank-statements/import
// Accepts a raw camt.053.001.08 XML body (Content-Type: application/xml)
// or a multipart file upload (field name: "file").
// Returns parsed bank entries as JSON.
func (h *ISO20022Handler) ImportCamt053(c *gin.Context) {
	var xmlData []byte

	contentType := c.GetHeader("Content-Type")
	if contentType == "application/xml" || contentType == "text/xml" {
		// Raw XML body
		var err error
		xmlData, err = io.ReadAll(io.LimitReader(c.Request.Body, 10<<20)) // 10 MB max
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read request body"})
			return
		}
	} else {
		// Multipart file upload
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "send XML as raw body (Content-Type: application/xml) or multipart field 'file'",
			})
			return
		}
		defer file.Close()
		xmlData, err = io.ReadAll(io.LimitReader(file, 10<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read uploaded file"})
			return
		}
	}

	if len(xmlData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty file"})
		return
	}

	entries, err := iso20022.ParseCamt053(xmlData)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// Convert to API-friendly response
	type entryResponse struct {
		Amount          float64 `json:"amount"`
		Currency        string  `json:"currency"`
		IsCredit        bool    `json:"is_credit"`
		BookingDate     string  `json:"booking_date"`
		ValueDate       string  `json:"value_date"`
		BankRef         string  `json:"bank_ref"`
		EndToEndRef     string  `json:"end_to_end_ref,omitempty"`
		QRReference     string  `json:"qr_reference,omitempty"`
		CounterpartName string  `json:"counterpart_name,omitempty"`
		CounterpartIBAN string  `json:"counterpart_iban,omitempty"`
		Unstructured    string  `json:"unstructured,omitempty"`
	}

	result := make([]entryResponse, 0, len(entries))
	for _, e := range entries {
		r := entryResponse{
			Amount:          e.Amount,
			Currency:        e.Currency,
			IsCredit:        e.IsCredit,
			BankRef:         e.BankRef,
			EndToEndRef:     e.EndToEndRef,
			QRReference:     e.QRReference,
			CounterpartName: e.CounterpartName,
			CounterpartIBAN: e.CounterpartIBAN,
			Unstructured:    e.Unstructured,
		}
		if !e.BookingDate.IsZero() {
			r.BookingDate = e.BookingDate.Format("2006-01-02")
		}
		if !e.ValueDate.IsZero() {
			r.ValueDate = e.ValueDate.Format("2006-01-02")
		}
		result = append(result, r)
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": result,
		"count":   len(result),
	})
}

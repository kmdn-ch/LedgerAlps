package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ─── SettingsHandler ──────────────────────────────────────────────────────────

type SettingsHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewSettingsHandler(database *sql.DB, usePostgres bool) *SettingsHandler {
	return &SettingsHandler{db: database, usePostgres: usePostgres}
}

// companySettingsRequest is the JSON body accepted by PUT /settings/company.
type companySettingsRequest struct {
	CompanyName          string `json:"company_name"`
	LegalForm            string `json:"legal_form"`
	AddressStreet        string `json:"address_street"`
	AddressPostalCode    string `json:"address_postal_code"`
	AddressCity          string `json:"address_city"`
	AddressCountry       string `json:"address_country"`
	CheNumber            string `json:"che_number"`
	VatNumber            string `json:"vat_number"`
	IBAN                 string `json:"iban"`
	FiscalYearStartMonth int    `json:"fiscal_year_start_month"`
	Currency             string `json:"currency"`
}

// GetCompany godoc
// GET /api/v1/settings/company
// Returns the singleton company settings row. If no row exists yet, returns
// a 200 with default (empty) values so the frontend always gets a valid object.
func (h *SettingsHandler) GetCompany(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, company_name, legal_form,
		       address_street, address_postal_code, address_city, address_country,
		       che_number, vat_number, iban,
		       fiscal_year_start_month, currency, logo_data,
		       created_at, updated_at
		FROM company_settings
		LIMIT 1`, h.usePostgres)

	var s models.CompanySettings
	err := h.db.QueryRowContext(ctx, q).Scan(
		&s.ID, &s.CompanyName, &s.LegalForm,
		&s.AddressStreet, &s.AddressPostalCode, &s.AddressCity, &s.AddressCountry,
		&s.CheNumber, &s.VatNumber, &s.IBAN,
		&s.FiscalYearStartMonth, &s.Currency, &s.LogoData,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// No row yet — return sensible defaults; the client can PUT to persist them.
		c.JSON(http.StatusOK, models.CompanySettings{
			AddressCountry:       "CH",
			Currency:             "CHF",
			FiscalYearStartMonth: 1,
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, s)
}

// PutCompany godoc
// PUT /api/v1/settings/company
// Upserts the singleton company settings row. Admin only (enforced by middleware).
func (h *SettingsHandler) PutCompany(c *gin.Context) {
	var req companySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// Apply defaults for zero values.
	if req.AddressCountry == "" {
		req.AddressCountry = "CH"
	}
	if req.Currency == "" {
		req.Currency = "CHF"
	}
	if req.FiscalYearStartMonth == 0 {
		req.FiscalYearStartMonth = 1
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Check whether a row already exists so we can decide INSERT vs UPDATE.
	var existingID string
	selectQ := db.Rebind(`SELECT id FROM company_settings LIMIT 1`, h.usePostgres)
	err := h.db.QueryRowContext(ctx, selectQ).Scan(&existingID)

	now := time.Now().UTC()

	if err == sql.ErrNoRows {
		// No row yet — INSERT.
		newID := db.NewID()
		insertQ := db.Rebind(`
			INSERT INTO company_settings
			    (id, company_name, legal_form,
			     address_street, address_postal_code, address_city, address_country,
			     che_number, vat_number, iban,
			     fiscal_year_start_month, currency,
			     created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, insertQ,
			newID, req.CompanyName, req.LegalForm,
			req.AddressStreet, req.AddressPostalCode, req.AddressCity, req.AddressCountry,
			req.CheNumber, req.VatNumber, req.IBAN,
			req.FiscalYearStartMonth, req.Currency,
			now, now,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		existingID = newID
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	} else {
		// Row exists — UPDATE (do NOT touch logo_data).
		updateQ := db.Rebind(`
			UPDATE company_settings SET
			    company_name            = ?,
			    legal_form              = ?,
			    address_street          = ?,
			    address_postal_code     = ?,
			    address_city            = ?,
			    address_country         = ?,
			    che_number              = ?,
			    vat_number              = ?,
			    iban                    = ?,
			    fiscal_year_start_month = ?,
			    currency                = ?,
			    updated_at              = ?
			WHERE id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, updateQ,
			req.CompanyName, req.LegalForm,
			req.AddressStreet, req.AddressPostalCode, req.AddressCity, req.AddressCountry,
			req.CheNumber, req.VatNumber, req.IBAN,
			req.FiscalYearStartMonth, req.Currency,
			now,
			existingID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	}

	// Return the updated row.
	q := db.Rebind(`
		SELECT id, company_name, legal_form,
		       address_street, address_postal_code, address_city, address_country,
		       che_number, vat_number, iban,
		       fiscal_year_start_month, currency, logo_data,
		       created_at, updated_at
		FROM company_settings WHERE id = ?`, h.usePostgres)

	var s models.CompanySettings
	if err := h.db.QueryRowContext(ctx, q, existingID).Scan(
		&s.ID, &s.CompanyName, &s.LegalForm,
		&s.AddressStreet, &s.AddressPostalCode, &s.AddressCity, &s.AddressCountry,
		&s.CheNumber, &s.VatNumber, &s.IBAN,
		&s.FiscalYearStartMonth, &s.Currency, &s.LogoData,
		&s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, s)
}

// UploadLogo godoc
// POST /api/v1/settings/logo
// Accepts a JSON body {"logo_data": "data:image/png;base64,..."}.
// The frontend reads the file via FileReader.readAsDataURL() and sends the result directly.
// Max decoded size: 2 MB. Accepted formats: PNG or JPEG.
func (h *SettingsHandler) UploadLogo(c *gin.Context) {
	var req struct {
		LogoData string `json:"logo_data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo_data (base64 data URL) required"})
		return
	}

	// Validate data URL format: "data:<mime>;base64,<data>"
	dataURL := req.LogoData
	if len(dataURL) < 22 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid logo data URL"})
		return
	}
	// Split header from base64 payload
	commaIdx := -1
	for i, ch := range dataURL {
		if ch == ',' {
			commaIdx = i
			break
		}
	}
	if commaIdx < 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid logo data URL"})
		return
	}
	header := dataURL[:commaIdx] // e.g. "data:image/png;base64"
	b64Data := dataURL[commaIdx+1:]

	// Validate MIME type from header
	if !strings.Contains(header, "image/png") && !strings.Contains(header, "image/jpeg") {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "logo must be PNG or JPEG"})
		return
	}

	// Decode and check size (max 2 MB uncompressed)
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		// Try without padding
		decoded, err = base64.RawStdEncoding.DecodeString(b64Data)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid base64 data"})
			return
		}
	}
	const maxSize = 2 << 20 // 2 MB
	if len(decoded) > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "logo too large (max 2 MB)"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var existingID string
	selectQ := db.Rebind(`SELECT id FROM company_settings LIMIT 1`, h.usePostgres)
	err = h.db.QueryRowContext(ctx, selectQ).Scan(&existingID)

	now := time.Now().UTC()

	if err == sql.ErrNoRows {
		// No row yet — insert minimal settings row with just the logo.
		newID := db.NewID()
		insertQ := db.Rebind(`
			INSERT INTO company_settings
			    (id, company_name, legal_form,
			     address_street, address_postal_code, address_city, address_country,
			     che_number, vat_number, iban,
			     fiscal_year_start_month, currency, logo_data,
			     created_at, updated_at)
			VALUES (?, '', '', '', '', '', 'CH', '', '', '', 1, 'CHF', ?, ?, ?)`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, insertQ, newID, dataURL, now, now); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	} else {
		updateQ := db.Rebind(`UPDATE company_settings SET logo_data = ?, updated_at = ? WHERE id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, updateQ, dataURL, now, existingID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"logo_data": dataURL})
}

// DeleteLogo godoc
// DELETE /api/v1/settings/logo
// Removes the company logo. Admin only.
func (h *SettingsHandler) DeleteLogo(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`UPDATE company_settings SET logo_data = NULL, updated_at = ?`, h.usePostgres)
	if _, err := h.db.ExecContext(ctx, q, time.Now().UTC()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

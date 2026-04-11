package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	pdfsvc "github.com/kmdn-ch/ledgeralps/internal/services/pdf"
)

// GetInvoicePDF godoc
// GET /api/v1/invoices/:id/pdf
// Renders the invoice as a PDF with a Swiss QR payment slip.
// Returns application/pdf with Content-Disposition: attachment.
func (h *InvoicesHandler) GetInvoicePDF(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Load invoice
	var inv models.Invoice
	invQ := db.Rebind(`
		SELECT id, invoice_number, contact_id, status, issue_date, due_date, currency,
		       subtotal_amount, vat_amount, total_amount, vat_rate, notes, terms, created_at, updated_at
		FROM invoices WHERE id = ?`, h.usePostgres)
	err := h.db.QueryRowContext(ctx, invQ, id).Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.ContactID, &inv.Status,
		&inv.IssueDate, &inv.DueDate, &inv.Currency,
		&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate,
		&inv.Notes, &inv.Terms, &inv.CreatedAt, &inv.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Load invoice lines
	linesQ := db.Rebind(`
		SELECT description, quantity, unit_price, vat_rate, line_total
		FROM invoice_lines WHERE invoice_id = ? ORDER BY sequence`, h.usePostgres)
	rows, err := h.db.QueryContext(ctx, linesQ, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()
	var pdfLines []pdfsvc.InvoiceLine
	for rows.Next() {
		var l pdfsvc.InvoiceLine
		if err := rows.Scan(&l.Description, &l.Quantity, &l.UnitPrice, &l.VATRate, &l.LineTotal); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		pdfLines = append(pdfLines, l)
	}

	// Load contact
	var ct models.Contact
	var isActive int
	ctQ := db.Rebind(`
		SELECT id, contact_type, name, email, phone, address, city, postal_code, country,
		       iban, qr_iban, vat_number, payment_term_days, is_active, created_at, updated_at
		FROM contacts WHERE id = ?`, h.usePostgres)
	err = h.db.QueryRowContext(ctx, ctQ, inv.ContactID).Scan(
		&ct.ID, &ct.ContactType, &ct.Name, &ct.Email, &ct.Phone,
		&ct.Address, &ct.City, &ct.PostalCode, &ct.Country,
		&ct.IBAN, &ct.QRIBAN, &ct.VATNumber, &ct.PaymentTermDays, &isActive,
		&ct.CreatedAt, &ct.UpdatedAt)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	ct.IsActive = isActive == 1

	// Load company settings from DB; fall back to environment variables.
	company := pdfsvc.CompanyInfo{
		Name:      envOr("COMPANY_NAME", "LedgerAlps"),
		Address:   envOr("COMPANY_ADDRESS", ""),
		City:      envOr("COMPANY_CITY", ""),
		Country:   envOr("COMPANY_COUNTRY", "CH"),
		IBAN:      envOr("COMPANY_IBAN", ""),
		QRIBAN:    envOr("COMPANY_QR_IBAN", ""),
		VATNumber: envOr("COMPANY_VAT_NUMBER", ""),
	}
	settingsQ := db.Rebind(`
		SELECT company_name, address_street, address_postal_code, address_city, address_country,
		       iban, vat_number, logo_data
		FROM company_settings LIMIT 1`, h.usePostgres)
	var dbName, dbStreet, dbPostal, dbCity, dbCountry, dbIBAN, dbVAT sql.NullString
	var dbLogo sql.NullString
	if err := h.db.QueryRowContext(ctx, settingsQ).Scan(
		&dbName, &dbStreet, &dbPostal, &dbCity, &dbCountry,
		&dbIBAN, &dbVAT, &dbLogo,
	); err == nil {
		if dbName.Valid && dbName.String != "" {
			company.Name = dbName.String
		}
		if dbStreet.Valid {
			company.Address = dbStreet.String
		}
		if dbPostal.Valid || dbCity.Valid {
			company.City = fmt.Sprintf("%s %s", dbPostal.String, dbCity.String)
		}
		if dbCountry.Valid && dbCountry.String != "" {
			company.Country = dbCountry.String
		}
		if dbIBAN.Valid && dbIBAN.String != "" {
			company.IBAN = dbIBAN.String
		}
		if dbVAT.Valid {
			company.VATNumber = dbVAT.String
		}
		if dbLogo.Valid {
			company.LogoData = dbLogo.String
		}
	}

	// Customer info from contact
	customer := pdfsvc.CustomerInfo{
		Name:    ct.Name,
		Country: ct.Country,
	}
	if ct.Address != nil {
		customer.Address = *ct.Address
	}
	if ct.City != nil && ct.PostalCode != nil {
		customer.City = fmt.Sprintf("%s %s", *ct.PostalCode, *ct.City)
	} else if ct.City != nil {
		customer.City = *ct.City
	}

	// Render PDF
	data := pdfsvc.InvoiceData{
		InvoiceNumber:  inv.InvoiceNumber,
		IssueDate:      inv.IssueDate,
		DueDate:        inv.DueDate,
		Currency:       inv.Currency,
		Status:         string(inv.Status),
		SubtotalAmount: inv.SubtotalAmount,
		VATAmount:      inv.VATAmount,
		TotalAmount:    inv.TotalAmount,
		VATRate:        inv.VATRate,
		Notes:          inv.Notes,
		Terms:          inv.Terms,
		Lines:          pdfLines,
		Company:        company,
		Customer:       customer,
	}

	pdfBytes, err := pdfsvc.Generate(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pdf generation failed"})
		return
	}

	filename := fmt.Sprintf("facture-%s.pdf", inv.InvoiceNumber)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

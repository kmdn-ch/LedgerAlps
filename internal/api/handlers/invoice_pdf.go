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
	// The join resolves the number of the invoice a credit note cancels: LTVA
	// art. 27 al. 4 wants that mention on the document itself.
	invQ := db.Rebind(`
		SELECT i.id, i.invoice_number, i.document_type, i.contact_id, i.status, i.issue_date, i.due_date,
		       i.currency, i.subtotal_amount, i.vat_amount, i.total_amount, i.vat_rate, i.notes, i.terms,
		       i.created_at, i.updated_at, COALESCE(orig.invoice_number, ''),
		       COALESCE(i.recipient_name,''), COALESCE(i.recipient_address,''),
		       COALESCE(i.recipient_postal_code,''), COALESCE(i.recipient_city,''),
		       COALESCE(i.recipient_country,''), COALESCE(i.recipient_vat_number,'')
		FROM invoices i
		LEFT JOIN invoices orig ON orig.id = i.corrects_invoice_id
		WHERE i.id = ?`, h.usePostgres)
	var correctsNumber string
	var rcp recipientIdentity
	err := h.db.QueryRowContext(ctx, invQ, id).Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.DocumentType, &inv.ContactID, &inv.Status,
		&inv.IssueDate, &inv.DueDate, &inv.Currency,
		&inv.SubtotalAmount, &inv.VATAmount, &inv.TotalAmount, &inv.VATRate,
		&inv.Notes, &inv.Terms, &inv.CreatedAt, &inv.UpdatedAt, &correctsNumber,
		&rcp.Name, &rcp.Address, &rcp.PostalCode, &rcp.City, &rcp.Country, &rcp.VATNumber)
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

	// Destinataire : l'identité FIGÉE sur la facture, pas la fiche contact
	// d'aujourd'hui. Relire le contact vivant faisait qu'un client renommé ou
	// déménagé réécrivait toutes ses factures passées — une pièce comptable qui
	// change n'est plus celle qui a été envoyée (CO art. 958f, LTVA art. 26).
	//
	// Repli sur le contact uniquement si l'instantané est vide : le cas d'une
	// facture antérieure à la v1.4.6 que le rattrapage n'aurait pas couverte,
	// faute de contact encore présent.
	if rcp.Name == "" {
		rcp = recipientIdentity{Name: ct.Name, Country: ct.Country}
		if ct.Address != nil {
			rcp.Address = *ct.Address
		}
		if ct.City != nil {
			rcp.City = *ct.City
		}
		if ct.PostalCode != nil {
			rcp.PostalCode = *ct.PostalCode
		}
	}

	// SPC 0200 requires a 2-char country when debtor is identified; default CH.
	ctCountry := rcp.Country
	if len(ctCountry) != 2 {
		ctCountry = "CH"
	}
	customer := pdfsvc.CustomerInfo{
		Name:    rcp.Name,
		Country: ctCountry,
		Address: rcp.Address,
	}
	switch {
	case rcp.PostalCode != "" && rcp.City != "":
		customer.City = fmt.Sprintf("%s %s", rcp.PostalCode, rcp.City)
	case rcp.City != "":
		customer.City = rcp.City
	}

	// Render PDF
	data := pdfsvc.InvoiceData{
		InvoiceNumber:         inv.InvoiceNumber,
		DocumentType:          inv.DocumentType,
		CorrectsInvoiceNumber: correctsNumber,
		IssueDate:             inv.IssueDate,
		DueDate:               inv.DueDate,
		Currency:              inv.Currency,
		Status:                string(inv.Status),
		SubtotalAmount:        inv.SubtotalAmount,
		VATAmount:             inv.VATAmount,
		TotalAmount:           inv.TotalAmount,
		VATRate:               inv.VATRate,
		Notes:                 inv.Notes,
		Terms:                 inv.Terms,
		Lines:                 pdfLines,
		Company:               company,
		Customer:              customer,
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

// recipientIdentity est l'identité du destinataire telle qu'elle a été figée
// sur la facture à son émission.
type recipientIdentity struct {
	Name       string
	Address    string
	PostalCode string
	City       string
	Country    string
	VATNumber  string
}

package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

type ContactsHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewContactsHandler(database *sql.DB, usePostgres bool) *ContactsHandler {
	return &ContactsHandler{db: database, usePostgres: usePostgres}
}

// ListContacts GET /api/v1/contacts
func (h *ContactsHandler) ListContacts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, contact_type, name, email, phone, address, city, postal_code, country,
		       iban, qr_iban, vat_number, payment_term_days, is_active, created_at, updated_at
		FROM contacts WHERE is_active = 1 ORDER BY name`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	contacts := []models.Contact{}
	for rows.Next() {
		var ct models.Contact
		var isActive int
		if err := rows.Scan(&ct.ID, &ct.ContactType, &ct.Name, &ct.Email, &ct.Phone,
			&ct.Address, &ct.City, &ct.PostalCode, &ct.Country,
			&ct.IBAN, &ct.QRIBAN, &ct.VATNumber, &ct.PaymentTermDays, &isActive,
			&ct.CreatedAt, &ct.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		ct.IsActive = isActive == 1
		contacts = append(contacts, ct)
	}
	c.JSON(http.StatusOK, contacts)
}

// GetContact GET /api/v1/contacts/:id
func (h *ContactsHandler) GetContact(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT id, contact_type, name, email, phone, address, city, postal_code, country,
		       iban, qr_iban, vat_number, payment_term_days, is_active, created_at, updated_at
		FROM contacts WHERE id = ?`, h.usePostgres)

	var ct models.Contact
	var isActive int
	err := h.db.QueryRowContext(ctx, q, id).Scan(
		&ct.ID, &ct.ContactType, &ct.Name, &ct.Email, &ct.Phone,
		&ct.Address, &ct.City, &ct.PostalCode, &ct.Country,
		&ct.IBAN, &ct.QRIBAN, &ct.VATNumber, &ct.PaymentTermDays, &isActive,
		&ct.CreatedAt, &ct.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	ct.IsActive = isActive == 1
	c.JSON(http.StatusOK, ct)
}

type createContactRequest struct {
	ContactType     string  `json:"contact_type" binding:"required,oneof=customer supplier both"`
	Name            string  `json:"name" binding:"required,min=1,max=255"`
	Email           *string `json:"email"`
	Phone           *string `json:"phone"`
	Address         *string `json:"address"`
	City            *string `json:"city"`
	PostalCode      *string `json:"postal_code"`
	Country         string  `json:"country"`
	IBAN            *string `json:"iban"`
	QRIBAN          *string `json:"qr_iban"`
	VATNumber       *string `json:"vat_number"`
	PaymentTermDays int     `json:"payment_term_days"`
}

// CreateContact POST /api/v1/contacts
func (h *ContactsHandler) CreateContact(c *gin.Context) {
	var req createContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if req.Country == "" {
		req.Country = "CH"
	}
	if req.PaymentTermDays == 0 {
		req.PaymentTermDays = 30
	}
	// IBAN validation at schema boundary
	if req.IBAN != nil {
		if err := compliance.ValidateIBAN(*req.IBAN); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "iban: " + err.Error()})
			return
		}
	}
	if req.QRIBAN != nil {
		if err := compliance.ValidateQRIBAN(*req.QRIBAN); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "qr_iban: " + err.Error()})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	id := db.NewID()
	now := time.Now()
	q := db.Rebind(`
		INSERT INTO contacts (id, contact_type, name, email, phone, address, city, postal_code,
		                      country, iban, qr_iban, vat_number, payment_term_days, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
	if _, err := h.db.ExecContext(ctx, q, id, req.ContactType, req.Name, req.Email, req.Phone,
		req.Address, req.City, req.PostalCode, req.Country, req.IBAN, req.QRIBAN,
		req.VATNumber, req.PaymentTermDays, now, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusCreated, models.Contact{
		ID: id, ContactType: models.ContactType(req.ContactType), Name: req.Name,
		Email: req.Email, Phone: req.Phone, Address: req.Address,
		City: req.City, PostalCode: req.PostalCode, Country: req.Country,
		IBAN: req.IBAN, QRIBAN: req.QRIBAN, VATNumber: req.VATNumber,
		PaymentTermDays: req.PaymentTermDays, IsActive: true, CreatedAt: now, UpdatedAt: now,
	})
}

type updateContactRequest struct {
	Name            *string `json:"name"`
	Email           *string `json:"email"`
	Phone           *string `json:"phone"`
	Address         *string `json:"address"`
	City            *string `json:"city"`
	PostalCode      *string `json:"postal_code"`
	Country         *string `json:"country"`
	IBAN            *string `json:"iban"`
	QRIBAN          *string `json:"qr_iban"`
	VATNumber       *string `json:"vat_number"`
	PaymentTermDays *int    `json:"payment_term_days"`
	IsActive        *bool   `json:"is_active"`
}

// UpdateContact PATCH /api/v1/contacts/:id
func (h *ContactsHandler) UpdateContact(c *gin.Context) {
	id := c.Param("id")
	var req updateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if req.IBAN != nil {
		if err := compliance.ValidateIBAN(*req.IBAN); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "iban: " + err.Error()})
			return
		}
	}
	if req.QRIBAN != nil {
		if err := compliance.ValidateQRIBAN(*req.QRIBAN); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "qr_iban: " + err.Error()})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Verify contact exists
	existsQ := db.Rebind("SELECT id FROM contacts WHERE id = ?", h.usePostgres)
	var existing string
	if err := h.db.QueryRowContext(ctx, existsQ, id).Scan(&existing); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}

	// Build partial SET clause
	sets := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []any{}
	addField := func(col string, val any) {
		sets = append(sets, col+" = ?")
		args = append(args, val)
	}
	if req.Name != nil {
		addField("name", *req.Name)
	}
	if req.Email != nil {
		addField("email", *req.Email)
	}
	if req.Phone != nil {
		addField("phone", *req.Phone)
	}
	if req.Address != nil {
		addField("address", *req.Address)
	}
	if req.City != nil {
		addField("city", *req.City)
	}
	if req.PostalCode != nil {
		addField("postal_code", *req.PostalCode)
	}
	if req.Country != nil {
		addField("country", *req.Country)
	}
	if req.IBAN != nil {
		addField("iban", *req.IBAN)
	}
	if req.QRIBAN != nil {
		addField("qr_iban", *req.QRIBAN)
	}
	if req.VATNumber != nil {
		addField("vat_number", *req.VATNumber)
	}
	if req.PaymentTermDays != nil {
		addField("payment_term_days", *req.PaymentTermDays)
	}
	if req.IsActive != nil {
		val := 0
		if *req.IsActive {
			val = 1
		}
		addField("is_active", val)
	}

	args = append(args, id)
	updateSQL := "UPDATE contacts SET "
	for i, s := range sets {
		if i > 0 {
			updateSQL += ", "
		}
		updateSQL += s
	}
	updateSQL += " WHERE id = ?"

	if _, err := h.db.ExecContext(ctx, db.Rebind(updateSQL, h.usePostgres), args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Return updated contact
	h.GetContact(c)
}

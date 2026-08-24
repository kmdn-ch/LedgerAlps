package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
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
	CompanyName       string `json:"company_name"`
	LegalForm         string `json:"legal_form"`
	AddressStreet     string `json:"address_street"`
	AddressPostalCode string `json:"address_postal_code"`
	AddressCity       string `json:"address_city"`
	AddressCountry    string `json:"address_country"`
	CheNumber         string `json:"che_number"`
	VatNumber         string `json:"vat_number"`
	// VatStatus : "", "liable" ou "exempt". Pointeur, pour la même raison
	// qu'AutoPostInvoices — « absent » veut dire « ne touche pas ». Un
	// formulaire qui ne porte pas ce champ remettrait sinon le statut à « non
	// déclaré » à chaque enregistrement.
	VatStatus *string `json:"vat_status,omitempty"`
	// Coordonnées de contact. Pas exigées par la LTVA art. 26, mais une facture
	// qu'on ne peut pas contester facilement se paie tard, ou pas.
	Phone string `json:"phone"`
	Email string `json:"email"`
	// Coordonnées de la banque. L'IBAN suffit à la QR-facture ; un virement
	// depuis l'étranger demande le nom de la banque et le BIC.
	BankName    string `json:"bank_name"`
	BankAddress string `json:"bank_address"`
	BankBIC     string `json:"bank_bic"`
	// AutoPostInvoices comptabilise la facture au journal dès son envoi.
	// Pointeur : « absent » veut dire « ne touche pas », et se distingue de
	// « posé à faux », qui est une extinction volontaire. Sans cette
	// distinction, enregistrer la fiche société éteindrait le réglage.
	AutoPostInvoices     *bool  `json:"auto_post_invoices,omitempty"`
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
		       che_number, vat_number, COALESCE(vat_status,''),
		       COALESCE(phone,''), COALESCE(email,''),
		       COALESCE(bank_name,''), COALESCE(bank_address,''), COALESCE(bank_bic,''), iban,
		       COALESCE(auto_post_invoices,0),
		       fiscal_year_start_month, currency, logo_data,
		       created_at, updated_at
		FROM company_settings
		LIMIT 1`, h.usePostgres)

	var s models.CompanySettings
	var autoPost int
	err := h.db.QueryRowContext(ctx, q).Scan(
		&s.ID, &s.CompanyName, &s.LegalForm,
		&s.AddressStreet, &s.AddressPostalCode, &s.AddressCity, &s.AddressCountry,
		&s.CheNumber, &s.VatNumber, &s.VatStatus, &s.Phone, &s.Email,
		&s.BankName, &s.BankAddress, &s.BankBIC, &s.IBAN,
		&autoPost,
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	s.AutoPostInvoices = autoPost == 1

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

	// Relire la fiche AVANT de l'écraser, dans la requête qui décide déjà
	// INSERT ou UPDATE.
	//
	// L'IBAN de l'entreprise est le compte qui recevra les virements de TOUS
	// les clients : le changer en douce redirige les encaissements sans que
	// rien n'apparaisse nulle part. La trace posait jusqu'ici un drapeau
	// `iban_modifie: true` écrit à la main — vrai à chaque enregistrement,
	// même quand l'IBAN n'avait pas bougé, et muet sur les autres champs.
	//
	// Les valeurs personnelles (raison sociale, adresse, IBAN, courriel) sont
	// masquées à l'écriture ; ce qui subsiste est la LISTE des champs qui ont
	// réellement changé, calculée avant masquage. On sait donc que l'IBAN a
	// changé, et qui l'a changé, sans conserver aucun des deux IBAN.
	var existingID string
	avant := map[string]any{}
	selectQ := db.Rebind(`
		SELECT id, COALESCE(company_name,''), COALESCE(legal_form,''),
		       COALESCE(address_street,''), COALESCE(address_postal_code,''),
		       COALESCE(address_city,''), COALESCE(address_country,''),
		       COALESCE(che_number,''), COALESCE(vat_number,''),
		       COALESCE(phone,''), COALESCE(email,''),
		       COALESCE(bank_name,''), COALESCE(bank_bic,''), COALESCE(iban,''),
		       COALESCE(fiscal_year_start_month,1), COALESCE(currency,'')
		  FROM company_settings LIMIT 1`, h.usePostgres)
	var (
		avCompanyName, avLegalForm, avStreet, avNPA, avCity, avCountry string
		avCHE, avVAT, avPhone, avEmail, avBankName, avBIC, avIBAN      string
		avCurrency                                                     string
		avFiscalMonth                                                  int
	)
	err := h.db.QueryRowContext(ctx, selectQ).Scan(
		&existingID, &avCompanyName, &avLegalForm,
		&avStreet, &avNPA, &avCity, &avCountry,
		&avCHE, &avVAT, &avPhone, &avEmail,
		&avBankName, &avBIC, &avIBAN,
		&avFiscalMonth, &avCurrency)
	if err == nil {
		avant = map[string]any{
			"company_name": avCompanyName, "legal_form": avLegalForm,
			"address_street": avStreet, "address_postal_code": avNPA,
			"address_city": avCity, "address_country": avCountry,
			"che_number": avCHE, "vat_number": avVAT,
			"phone": avPhone, "email": avEmail,
			"bank_name": avBankName, "bank_bic": avBIC, "iban": avIBAN,
			"fiscal_year_start_month": avFiscalMonth, "currency": avCurrency,
		}
	}

	now := time.Now().UTC()

	if err == sql.ErrNoRows {
		// No row yet — INSERT.
		newID := db.NewID()
		insertQ := db.Rebind(`
			INSERT INTO company_settings
			    (id, company_name, legal_form,
			     address_street, address_postal_code, address_city, address_country,
			     che_number, vat_number, phone, email,
			     bank_name, bank_address, bank_bic, iban,
			     fiscal_year_start_month, currency,
			     created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, insertQ,
			newID, req.CompanyName, req.LegalForm,
			req.AddressStreet, req.AddressPostalCode, req.AddressCity, req.AddressCountry,
			req.CheNumber, req.VatNumber, req.Phone, req.Email,
			req.BankName, req.BankAddress, req.BankBIC, req.IBAN,
			req.FiscalYearStartMonth, req.Currency,
			now, now,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
		existingID = newID
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
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
			    phone                   = ?,
			    email                   = ?,
			    bank_name               = ?,
			    bank_address            = ?,
			    bank_bic                = ?,
			    iban                    = ?,
			    fiscal_year_start_month = ?,
			    currency                = ?,
			    updated_at              = ?
			WHERE id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, updateQ,
			req.CompanyName, req.LegalForm,
			req.AddressStreet, req.AddressPostalCode, req.AddressCity, req.AddressCountry,
			req.CheNumber, req.VatNumber, req.Phone, req.Email,
			req.BankName, req.BankAddress, req.BankBIC, req.IBAN,
			req.FiscalYearStartMonth, req.Currency,
			now,
			existingID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	}

	// La comptabilisation automatique se met à jour séparément, et seulement
	// quand elle est fournie. L'inclure dans l'UPDATE ci-dessus l'éteindrait à
	// chaque enregistrement de la fiche société : le formulaire de l'entreprise
	// ne porte pas ce champ, il arriverait donc à faux et couperait la
	// comptabilisation sans que personne ne l'ait demandé.
	if req.AutoPostInvoices != nil {
		autoQ := db.Rebind(`UPDATE company_settings SET auto_post_invoices = ?, updated_at = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, autoQ, boolToSQL(*req.AutoPostInvoices), now); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	}

	// Le statut TVA, même logique : fourni ou pas touché.
	//
	// « Non assujetti » EFFACE le numéro de TVA, et ce n'est pas une commodité.
	// Le numéro s'imprime sur la facture : le garder tout en déclarant ne pas
	// être assujetti produirait un document qui affirme le contraire de ce que
	// dit la fiche — exactement ce que la LTVA art. 27 al. 1 interdit, et l'al. 2
	// rendrait redevable de l'impôt ainsi mentionné. Une contradiction se refuse
	// là où elle naît ; la laisser vivre pour la rattraper plus loin, c'est
	// signer qu'un chemin l'oubliera.
	if req.VatStatus != nil {
		statut := strings.TrimSpace(*req.VatStatus)
		if statut != "" && statut != models.VatLiable && statut != models.VatExempt {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "statut TVA inconnu : attendu « assujetti » ou « non assujetti »",
			})
			return
		}
		vatQ := db.Rebind(`UPDATE company_settings SET vat_status = ?, updated_at = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, vatQ, statut, now); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
		if statut == models.VatExempt {
			clearQ := db.Rebind(`UPDATE company_settings SET vat_number = '', updated_at = ?`, h.usePostgres)
			if _, err := h.db.ExecContext(ctx, clearQ, now); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
				return
			}
		}
	}

	// Return the updated row.
	q := db.Rebind(`
		SELECT id, company_name, legal_form,
		       address_street, address_postal_code, address_city, address_country,
		       che_number, vat_number, COALESCE(vat_status,''),
		       COALESCE(phone,''), COALESCE(email,''),
		       COALESCE(bank_name,''), COALESCE(bank_address,''), COALESCE(bank_bic,''), iban,
		       COALESCE(auto_post_invoices,0),
		       fiscal_year_start_month, currency, logo_data,
		       created_at, updated_at
		FROM company_settings WHERE id = ?`, h.usePostgres)

	var s models.CompanySettings
	var autoPostOut int
	if err := h.db.QueryRowContext(ctx, q, existingID).Scan(
		&s.ID, &s.CompanyName, &s.LegalForm,
		&s.AddressStreet, &s.AddressPostalCode, &s.AddressCity, &s.AddressCountry,
		&s.CheNumber, &s.VatNumber, &s.VatStatus, &s.Phone, &s.Email,
		&s.BankName, &s.BankAddress, &s.BankBIC, &s.IBAN,
		&autoPostOut,
		&s.FiscalYearStartMonth, &s.Currency, &s.LogoData,
		&s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	s.AutoPostInvoices = autoPostOut == 1

	// L'état « après » porte les MÊMES clés que l'état « avant » : c'est ce qui
	// rend la comparaison possible, et ce qui borne les données conservées à ce
	// qui a servi à la calculer.
	apres := map[string]any{
		"company_name": req.CompanyName, "legal_form": req.LegalForm,
		"address_street": req.AddressStreet, "address_postal_code": req.AddressPostalCode,
		"address_city": req.AddressCity, "address_country": req.AddressCountry,
		"che_number": req.CheNumber, "vat_number": req.VatNumber,
		"phone": req.Phone, "email": req.Email,
		"bank_name": req.BankName, "bank_bic": req.BankBIC, "iban": req.IBAN,
		"fiscal_year_start_month": req.FiscalYearStartMonth, "currency": req.Currency,
	}
	// Première écriture de la fiche : rien ne précédait, donc une création.
	transition := accounting.Creation(apres)
	if existingID != "" {
		transition = accounting.Modification(avant, apres)
	}
	trace(c, h.db, h.usePostgres, TableCompanySettings,
		ActionCompanySettingsUpdated, "company", transition)

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo_data est requis (adresse de données base64)"})
		return
	}

	// Validate data URL format: "data:<mime>;base64,<data>"
	dataURL := req.LogoData
	if len(dataURL) < 22 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "l'adresse de données du logo est invalide"})
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
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "l'adresse de données du logo est invalide"})
		return
	}
	header := dataURL[:commaIdx] // e.g. "data:image/png;base64"
	b64Data := dataURL[commaIdx+1:]

	// Validate MIME type from header
	if !strings.Contains(header, "image/png") && !strings.Contains(header, "image/jpeg") {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "le logo doit être au format PNG ou JPEG"})
		return
	}

	// Decode and check size (max 2 MB uncompressed)
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		// Try without padding
		decoded, err = base64.RawStdEncoding.DecodeString(b64Data)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "données base64 invalides"})
			return
		}
	}
	const maxSize = 2 << 20 // 2 MB
	if len(decoded) > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "logo trop volumineux (2 Mo au maximum)"})
		return
	}

	// Ramener l'image à 300 px de côté au plus. L'écran le fait déjà avant
	// l'envoi ; c'est ici que la règle tient, parce que c'est ici qu'on écrit
	// en base. Voir logo_resize.go pour le pourquoi de cette limite.
	logo, err := ajusterLogo(dataURL)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "ce fichier n'est pas une image lisible (PNG ou JPEG attendu)",
		})
		return
	}
	dataURL = logo.DataURL

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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	} else {
		updateQ := db.Rebind(`UPDATE company_settings SET logo_data = ?, updated_at = ? WHERE id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(ctx, updateQ, dataURL, now, existingID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
			return
		}
	}

	// La réponse annonce ce qui a été RETENU, pas ce qui a été envoyé. L'écran
	// réduit déjà l'image de son côté ; afficher sa propre estimation reviendrait
	// à se croire sur parole, alors que c'est la base qui fait foi.
	c.JSON(http.StatusOK, gin.H{
		"logo_data": dataURL,
		"width":     logo.Largeur,
		"height":    logo.Hauteur,
		"resized":   logo.Redimens,
	})
}

// DeleteLogo godoc
// DELETE /api/v1/settings/logo
// Removes the company logo. Admin only.
func (h *SettingsHandler) DeleteLogo(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`UPDATE company_settings SET logo_data = NULL, updated_at = ?`, h.usePostgres)
	if _, err := h.db.ExecContext(ctx, q, time.Now().UTC()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// boolToSQL rend 1 ou 0 : SQLite n'a pas de type booléen, et écrire un `true`
// Go y produirait la chaîne « true », que COALESCE(...,0) relirait comme 0.
func boolToSQL(b bool) int {
	if b {
		return 1
	}
	return 0
}

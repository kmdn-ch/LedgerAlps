package handlers

// UIDLookup proxies the Swiss ZEFIX REST API (zefix.admin.ch) to resolve a
// CHE number into company name, legal form, and address — without exposing the
// browser to cross-origin restrictions.
//
// Endpoint: GET /api/v1/uid-lookup?che=CHE-123.456.789
// Returns:  200 with company data, 404 when not found, 400 on bad input.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// reCHE matches the standard Swiss CHE format with or without dots.
var reCHE = regexp.MustCompile(`(?i)^CHE[-.]?(\d{3})\.?(\d{3})\.?(\d{3})$`)

// zefixFirmResponse is the subset of the ZEFIX firm JSON that we use.
// Full spec: https://www.zefix.admin.ch/ZefixREST/swagger-ui.html
type zefixFirmResponse struct {
	Name      string `json:"name"`
	LegalForm struct {
		AbbrevName string `json:"abbrevName"`
	} `json:"legalForm"`
	Address struct {
		Street      string `json:"street"`
		HouseNumber string `json:"houseNumber"`
		Swisszip    string `json:"swissZip"`
		Town        string `json:"town"`
	} `json:"address"`
	LegalSeat  string `json:"legalSeat"`
	Status     string `json:"status"`
}

// UIDLookupResponse is what we return to the wizard.
type UIDLookupResponse struct {
	Name               string `json:"name"`
	LegalForm          string `json:"legal_form"`
	AddressStreet      string `json:"address_street"`
	AddressPostalCode  string `json:"address_postal_code"`
	AddressCity        string `json:"address_city"`
	AddressCountry     string `json:"address_country"`
}

var zefixHTTPClient = &http.Client{Timeout: 8 * time.Second}

// UIDLookup handles GET /api/v1/uid-lookup?che=CHE-123.456.789
func UIDLookup(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("che"))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paramètre 'che' requis"})
		return
	}

	// Normalize to the format ZEFIX expects: CHE-XXXXXXXXX (no dots).
	m := reCHE.FindStringSubmatch(raw)
	if m == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format CHE invalide — attendu CHE-XXX.XXX.XXX"})
		return
	}
	uid := fmt.Sprintf("CHE-%s%s%s", m[1], m[2], m[3])

	apiURL := fmt.Sprintf("https://www.zefix.admin.ch/ZefixREST/api/v1/firm/uid/%s.json", uid)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur interne"})
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LedgerAlps/1.0 (+https://github.com/kmdn-ch/ledgeralps)")

	resp, err := zefixHTTPClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "registre UID inaccessible — réessayez ou saisissez manuellement"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "numéro IDE non trouvé dans le registre"})
		return
	}
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("registre UID: réponse %d", resp.StatusCode)})
		return
	}

	var firm zefixFirmResponse
	if err := json.NewDecoder(resp.Body).Decode(&firm); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "réponse du registre illisible"})
		return
	}

	street := firm.Address.Street
	if firm.Address.HouseNumber != "" {
		street = street + " " + firm.Address.HouseNumber
	}
	city := firm.Address.Town
	if city == "" {
		city = firm.LegalSeat
	}

	c.JSON(http.StatusOK, UIDLookupResponse{
		Name:              firm.Name,
		LegalForm:         firm.LegalForm.AbbrevName,
		AddressStreet:     strings.TrimSpace(street),
		AddressPostalCode: firm.Address.Swisszip,
		AddressCity:       city,
		AddressCountry:    "CH",
	})
}

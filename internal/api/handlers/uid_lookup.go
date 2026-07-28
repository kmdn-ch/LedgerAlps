package handlers

// UIDLookup resolves a Swiss CHE number into company name, legal form and
// address, by proxying the public ZEFIX registry so the browser is not exposed
// to cross-origin restrictions.
//
// Endpoint: GET /api/v1/uid-lookup?che=CHE-123.456.789
//
// ZEFIX exposes several routes and they do not behave alike:
//
//   - GET  /firm/uid/{uid}.json  →  403 for everyone, always. It requires a
//     registered API account, so the wizard used to greet first-time users
//     with "registre IDE: réponse 403" as if they had mistyped something.
//   - POST /firm/search.json     →  public. It rejects a `uid` field, but its
//     `name` field is a general search term that matches UID numbers, which is
//     how a lookup by number is done without an account.
//   - GET  /firm/{ehraid}.json   →  public, and the only route carrying the
//     full address.
//
// So a lookup is: search by number to obtain the ehraid, then read the detail.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// reCHE matches the Swiss CHE format with or without dots.
var reCHE = regexp.MustCompile(`(?i)^CHE[-.]?(\d{3})\.?(\d{3})\.?(\d{3})$`)

// zefixBaseURL is a variable so tests can point it at a stub server.
var zefixBaseURL = "https://www.zefix.admin.ch/ZefixREST/api/v1"

var zefixHTTPClient = &http.Client{Timeout: 10 * time.Second}

type zefixSearchHit struct {
	Name        string `json:"name"`
	EhraID      int    `json:"ehraid"`
	UID         string `json:"uid"`
	LegalSeat   string `json:"legalSeat"`
	LegalFormID int    `json:"legalFormId"`
	Status      string `json:"status"`
}

type zefixSearchResponse struct {
	List []zefixSearchHit `json:"list"`
}

type zefixAddress struct {
	Street       string `json:"street"`
	HouseNumber  string `json:"houseNumber"`
	SwissZipCode string `json:"swissZipCode"` // NOT "swissZip" — the earlier
	Town         string `json:"town"`         // spelling silently dropped the code
	Country      string `json:"country"`
}

type zefixFirmDetail struct {
	Name        string       `json:"name"`
	UID         string       `json:"uid"`
	LegalSeat   string       `json:"legalSeat"`
	LegalFormID int          `json:"legalFormId"`
	Status      string       `json:"status"`
	Address     zefixAddress `json:"address"`
}

type zefixLegalForm struct {
	ID       int               `json:"id"`
	Name     map[string]string `json:"name"`
	Kurzform map[string]string `json:"kurzform"`
}

// UIDLookupResponse is what the setup wizard consumes.
type UIDLookupResponse struct {
	Name              string `json:"name"`
	LegalForm         string `json:"legal_form"`
	AddressStreet     string `json:"address_street"`
	AddressPostalCode string `json:"address_postal_code"`
	AddressCity       string `json:"address_city"`
	AddressCountry    string `json:"address_country"`
	UID               string `json:"uid"`
	Active            bool   `json:"active"`
}

// legalForms is fetched once: the list changes about never, and a wizard should
// not make the same call on every keystroke.
var (
	legalFormsOnce  sync.Once
	legalFormsCache map[int]zefixLegalForm
)

// resetLegalFormsCache clears the process-wide cache. Test-only: sync.Once
// would otherwise make the first test's stub responses leak into the others.
func resetLegalFormsCache(t interface{ Helper() }) {
	t.Helper()
	legalFormsOnce = sync.Once{}
	legalFormsCache = nil
}

func loadLegalForms(ctx context.Context) map[int]zefixLegalForm {
	legalFormsOnce.Do(func() {
		var forms []zefixLegalForm
		if err := zefixGet(ctx, zefixBaseURL+"/legalForm.json", &forms); err != nil {
			return // leave the cache nil; the legal form is simply omitted
		}
		legalFormsCache = make(map[int]zefixLegalForm, len(forms))
		for _, f := range forms {
			legalFormsCache[f.ID] = f
		}
	})
	return legalFormsCache
}

func zefixGet(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LedgerAlps")

	resp, err := zefixHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("zefix GET %s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// searchByUID finds the registry entry for a UID. ZEFIX has no UID field on the
// public search, but its free-text `name` matches UID numbers.
func searchByUID(ctx context.Context, uidPlain string) (*zefixSearchHit, error) {
	body, err := json.Marshal(map[string]any{
		"name":        uidPlain,
		"languageKey": "fr",
		"maxEntries":  10,
		"offset":      0,
		"activeOnly":  false, // a dissolved company must be reported, not hidden
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		zefixBaseURL+"/firm/search.json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LedgerAlps")

	resp, err := zefixHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zefix search: HTTP %d", resp.StatusCode)
	}

	var out zefixSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	// A free-text search can return neighbours; accept only an exact UID match
	// so the wizard never silently fills in a different company.
	for i := range out.List {
		if strings.EqualFold(normaliseUID(out.List[i].UID), uidPlain) {
			return &out.List[i], nil
		}
	}
	return nil, nil
}

func normaliseUID(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", ".", "", " ", "").Replace(s))
}

// UIDLookup handles GET /api/v1/uid-lookup?che=CHE-123.456.789
func UIDLookup(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("che"))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paramètre 'che' requis"})
		return
	}
	m := reCHE.FindStringSubmatch(raw)
	if m == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format IDE invalide — attendu CHE-XXX.XXX.XXX"})
		return
	}
	uidPlain := fmt.Sprintf("CHE%s%s%s", m[1], m[2], m[3])

	// Two sequential calls plus an occasional legal-form fetch.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	hit, err := searchByUID(ctx, uidPlain)
	if err != nil {
		// The registry being unreachable is not the user's mistake; say so, and
		// make clear the form can still be filled in by hand.
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "registre IDE momentanément indisponible — saisissez les informations manuellement",
		})
		return
	}
	if hit == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "numéro IDE introuvable au registre du commerce"})
		return
	}

	// The detail route is the only public one carrying the address.
	var detail zefixFirmDetail
	if err := zefixGet(ctx, fmt.Sprintf("%s/firm/%d.json", zefixBaseURL, hit.EhraID), &detail); err != nil {
		// Degrade rather than fail: the name and seat from the search are
		// already more than the user had to type.
		detail = zefixFirmDetail{
			Name: hit.Name, UID: hit.UID, LegalSeat: hit.LegalSeat,
			LegalFormID: hit.LegalFormID, Status: hit.Status,
		}
	}

	legalForm := ""
	if forms := loadLegalForms(ctx); forms != nil {
		if f, ok := forms[detail.LegalFormID]; ok {
			if v := f.Name["fr"]; v != "" {
				legalForm = v
			}
		}
	}

	street := strings.TrimSpace(detail.Address.Street + " " + detail.Address.HouseNumber)
	city := detail.Address.Town
	if city == "" {
		city = detail.LegalSeat
	}
	country := detail.Address.Country
	if country == "" {
		country = "CH"
	}

	c.JSON(http.StatusOK, UIDLookupResponse{
		Name:              detail.Name,
		LegalForm:         legalForm,
		AddressStreet:     street,
		AddressPostalCode: detail.Address.SwissZipCode,
		AddressCity:       city,
		AddressCountry:    country,
		UID:               detail.UID,
		// ZEFIX reports status in German regardless of languageKey.
		Active: detail.Status == "EXISTIEREND" || detail.Status == "",
	})
}

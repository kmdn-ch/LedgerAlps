// Package zefix resolves a Swiss CHE number into company details via the
// public ZEFIX commercial-register API.
//
// This logic exists once, on purpose. It used to be written twice — in the API
// handler and again in the first-run wizard inside the launcher — and both
// copies called an endpoint that answers 403 to everyone. Fixing the handler
// left the wizard broken, which is where users actually meet it, because the
// wizard runs before the API server and proxies the lookup itself.
//
// About the endpoints, which do not behave alike:
//
//   - GET  /firm/uid/{uid}.json  →  403 for everyone, always. It requires a
//     registered API account. This is what both old copies called.
//   - POST /firm/search.json     →  public. It rejects a `uid` field, but its
//     `name` field is a free-text term that also matches UID numbers, which is
//     how a lookup by number is done without an account.
//   - GET  /firm/{ehraid}.json   →  public, and the only route carrying the
//     full address.
//
// A lookup is therefore: search by number to get the ehraid, then read detail.
package zefix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// BaseURL is a variable so tests can point it at a stub server.
var BaseURL = "https://www.zefix.admin.ch/ZefixREST/api/v1"

var httpClient = &http.Client{Timeout: 10 * time.Second}

// reCHE matches the Swiss CHE format with or without dots.
var reCHE = regexp.MustCompile(`(?i)^CHE[-.]?(\d{3})\.?(\d{3})\.?(\d{3})$`)

var (
	// ErrInvalidFormat means the input is not a CHE number at all.
	ErrInvalidFormat = errors.New("format IDE invalide")
	// ErrNotFound means the registry has no entry for that number.
	ErrNotFound = errors.New("numéro IDE introuvable au registre du commerce")
	// ErrUnavailable means the registry could not be reached or refused us.
	// It is deliberately distinct from ErrNotFound: telling a user their number
	// does not exist when the registry is merely down sends them hunting for a
	// mistake they did not make.
	ErrUnavailable = errors.New("registre IDE momentanément indisponible")
)

// Company is what a successful lookup yields.
type Company struct {
	Name       string `json:"name"`
	LegalForm  string `json:"legal_form"`
	Street     string `json:"address_street"`
	PostalCode string `json:"address_postal_code"`
	City       string `json:"address_city"`
	Country    string `json:"address_country"`
	UID        string `json:"uid"`
	Active     bool   `json:"active"`
}

type searchHit struct {
	Name        string `json:"name"`
	EhraID      int    `json:"ehraid"`
	UID         string `json:"uid"`
	LegalSeat   string `json:"legalSeat"`
	LegalFormID int    `json:"legalFormId"`
	Status      string `json:"status"`
}

type address struct {
	Street       string `json:"street"`
	HouseNumber  string `json:"houseNumber"`
	SwissZipCode string `json:"swissZipCode"` // NOT "swissZip" — both old copies
	Town         string `json:"town"`         // used that spelling and silently
	Country      string `json:"country"`      // dropped the postal code
}

type firmDetail struct {
	Name        string  `json:"name"`
	UID         string  `json:"uid"`
	LegalSeat   string  `json:"legalSeat"`
	LegalFormID int     `json:"legalFormId"`
	Status      string  `json:"status"`
	Address     address `json:"address"`
}

type legalForm struct {
	ID       int               `json:"id"`
	Name     map[string]string `json:"name"`
	Kurzform map[string]string `json:"kurzform"`
}

// legalForms is fetched once: the list changes about never, and a wizard should
// not repeat the call on every keystroke.
var (
	legalFormsOnce  sync.Once
	legalFormsCache map[int]legalForm
)

// ResetCacheForTest clears the process-wide legal-form cache.
func ResetCacheForTest() {
	legalFormsOnce = sync.Once{}
	legalFormsCache = nil
}

// NormaliseUID strips separators and uppercases, so CHE-123.456.789,
// che123456789 and "CHE 123 456 789" all compare equal.
func NormaliseUID(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", ".", "", " ", "").Replace(s))
}

func get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LedgerAlps")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("zefix GET: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func loadLegalForms(ctx context.Context) map[int]legalForm {
	legalFormsOnce.Do(func() {
		var forms []legalForm
		if err := get(ctx, BaseURL+"/legalForm.json", &forms); err != nil {
			return // leave nil; the legal form is simply omitted
		}
		legalFormsCache = make(map[int]legalForm, len(forms))
		for _, f := range forms {
			legalFormsCache[f.ID] = f
		}
	})
	return legalFormsCache
}

func searchByUID(ctx context.Context, uidPlain string) (*searchHit, error) {
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
		BaseURL+"/firm/search.json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LedgerAlps")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zefix search: HTTP %d", resp.StatusCode)
	}

	var out struct {
		List []searchHit `json:"list"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	// A free-text search returns neighbours; accept only an exact UID match so
	// the wizard never silently fills in a different company.
	for i := range out.List {
		if NormaliseUID(out.List[i].UID) == uidPlain {
			return &out.List[i], nil
		}
	}
	return nil, nil
}

// Lookup resolves a CHE number. Returns ErrInvalidFormat, ErrNotFound or
// ErrUnavailable so callers can map each to the right HTTP status and message.
func Lookup(ctx context.Context, raw string) (*Company, error) {
	m := reCHE.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return nil, ErrInvalidFormat
	}
	uidPlain := fmt.Sprintf("CHE%s%s%s", m[1], m[2], m[3])

	hit, err := searchByUID(ctx, uidPlain)
	if err != nil {
		return nil, ErrUnavailable
	}
	if hit == nil {
		return nil, ErrNotFound
	}

	// The detail route is the only public one carrying the address. If it fails
	// we degrade to the search result rather than losing the lookup: a name and
	// a legal seat still beat an empty form.
	var detail firmDetail
	if err := get(ctx, fmt.Sprintf("%s/firm/%d.json", BaseURL, hit.EhraID), &detail); err != nil {
		detail = firmDetail{
			Name: hit.Name, UID: hit.UID, LegalSeat: hit.LegalSeat,
			LegalFormID: hit.LegalFormID, Status: hit.Status,
		}
	}

	form := ""
	if forms := loadLegalForms(ctx); forms != nil {
		if f, ok := forms[detail.LegalFormID]; ok {
			form = f.Name["fr"]
		}
	}

	city := detail.Address.Town
	if city == "" {
		city = detail.LegalSeat
	}
	country := detail.Address.Country
	if country == "" {
		country = "CH"
	}

	return &Company{
		Name:       detail.Name,
		LegalForm:  form,
		Street:     strings.TrimSpace(detail.Address.Street + " " + detail.Address.HouseNumber),
		PostalCode: detail.Address.SwissZipCode,
		City:       city,
		Country:    country,
		UID:        detail.UID,
		// ZEFIX reports status in German regardless of languageKey.
		Active: detail.Status == "EXISTIEREND" || detail.Status == "",
	}, nil
}

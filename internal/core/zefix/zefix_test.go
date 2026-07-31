package zefix

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubRegistry mirrors the real ZEFIX routes, including the one that answers
// 403 to everyone — the behaviour that produced "registre IDE: réponse 403"
// in the setup wizard.
func stubRegistry(t *testing.T, hits, detail string, detailStatus int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/firm/uid/"):
			w.WriteHeader(http.StatusForbidden) // as in production, for everyone
		case strings.HasSuffix(r.URL.Path, "/firm/search.json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(hits))
		case strings.HasSuffix(r.URL.Path, "/legalForm.json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"name":{"fr":"Entreprise individuelle"},"kurzform":{"fr":"EI"}}]`))
		default: // /firm/{ehraid}.json
			w.WriteHeader(detailStatus)
			if detailStatus == http.StatusOK {
				_, _ = w.Write([]byte(detail))
			}
		}
	}))
	t.Cleanup(srv.Close)

	orig := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = orig })
	ResetCacheForTest()
}

const hitsJSON = `{"list":[
  {"name":"Autre SA","ehraid":999,"uid":"CHE999999996","legalSeat":"Genève","legalFormId":1,"status":"EXISTIEREND"},
  {"name":"Acme Sàrl","ehraid":1565377,"uid":"CHE424492624","legalSeat":"Berneck","legalFormId":1,"status":"EXISTIEREND"}
]}`

const detailJSON = `{"name":"Acme Sàrl","uid":"CHE424492624","legalSeat":"Berneck","legalFormId":1,
  "status":"EXISTIEREND","address":{"street":"Kirchgass","houseNumber":"17",
  "swissZipCode":"9442","town":"Berneck","country":"CH"}}`

func TestLookupFillsEveryField(t *testing.T) {
	stubRegistry(t, hitsJSON, detailJSON, http.StatusOK)

	got, err := Lookup(context.Background(), "CHE-424.492.624")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, pair := range map[string][2]string{
		"name":        {got.Name, "Acme Sàrl"},
		"street":      {got.Street, "Kirchgass 17"},
		"postal code": {got.PostalCode, "9442"}, // silently empty while the field was read as swissZip
		"city":        {got.City, "Berneck"},
		"legal form":  {got.LegalForm, "Entreprise individuelle"},
		"country":     {got.Country, "CH"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if !got.Active {
		t.Error("an EXISTIEREND company must be reported as active")
	}
}

// Filling in the wrong company would be worse than filling in nothing.
func TestOnlyExactUIDMatches(t *testing.T) {
	stubRegistry(t, hitsJSON, detailJSON, http.StatusOK)
	if _, err := Lookup(context.Background(), "CHE-111.111.111"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound for a UID absent from the results", err)
	}
}

func TestUnknownUID(t *testing.T) {
	stubRegistry(t, `{"list":[]}`, detailJSON, http.StatusOK)
	if _, err := Lookup(context.Background(), "CHE-424.492.624"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// A registry outage must never be reported as "your number does not exist" —
// that sends the user hunting for a mistake they did not make.
func TestOutageIsDistinctFromNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	orig := BaseURL
	BaseURL = srv.URL
	defer func() { BaseURL = orig }()
	ResetCacheForTest()

	_, err := Lookup(context.Background(), "CHE-424.492.624")
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("an outage must not be reported as a missing company")
	}
}

// Losing the address is better than losing the whole lookup.
func TestDegradesWhenDetailFails(t *testing.T) {
	stubRegistry(t, hitsJSON, "", http.StatusInternalServerError)
	got, err := Lookup(context.Background(), "CHE-424.492.624")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Acme Sàrl" {
		t.Errorf("name = %q, want the value from the search hit", got.Name)
	}
	if got.City != "Berneck" {
		t.Errorf("city = %q, want the legal seat as fallback", got.City)
	}
}

func TestAcceptedInputFormats(t *testing.T) {
	for _, in := range []string{"CHE-424.492.624", "CHE424492624", "che-424492624", "  CHE-424.492.624  "} {
		stubRegistry(t, hitsJSON, detailJSON, http.StatusOK)
		if _, err := Lookup(context.Background(), in); err != nil {
			t.Errorf("%q rejected: %v", in, err)
		}
	}
}

func TestRejectedInputFormats(t *testing.T) {
	for _, in := range []string{"", "CHE-12", "pasunIDE", "CHE-424.492.62X", "424492624"} {
		if _, err := Lookup(context.Background(), in); !errors.Is(err, ErrInvalidFormat) {
			t.Errorf("%q: err = %v, want ErrInvalidFormat", in, err)
		}
	}
}

func TestNormaliseUID(t *testing.T) {
	for _, in := range []string{"CHE-424.492.624", "che424492624", "CHE 424 492 624"} {
		if got := NormaliseUID(in); got != "CHE424492624" {
			t.Errorf("NormaliseUID(%q) = %q", in, got)
		}
	}
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// stubZefix stands in for the registry. It mirrors the real routes, including
// the one that always answers 403 — the behaviour that made the wizard show
// "registre IDE: réponse 403" to first-time users.
func stubZefix(t *testing.T, hits string, detail string, detailStatus int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/firm/uid") || strings.Contains(r.URL.Path, "/firm/uid/"):
			w.WriteHeader(http.StatusForbidden) // as in production
		case strings.HasSuffix(r.URL.Path, "/firm/search.json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(hits))
		case strings.HasSuffix(r.URL.Path, "/legalForm.json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"name":{"fr":"Entreprise individuelle","de":"Einzelunternehmen"},"kurzform":{"fr":"EI"}}]`))
		default: // /firm/{ehraid}.json
			w.WriteHeader(detailStatus)
			if detailStatus == http.StatusOK {
				_, _ = w.Write([]byte(detail))
			}
		}
	}))
	t.Cleanup(srv.Close)

	orig := zefixBaseURL
	zefixBaseURL = srv.URL
	t.Cleanup(func() { zefixBaseURL = orig })

	// The legal-form cache is process-wide; reset it so each test is independent.
	resetLegalFormsCache(t)
}

const hitJSON = `{"list":[
  {"name":"Autre SA","ehraid":999,"uid":"CHE999999996","legalSeat":"Genève","legalFormId":1,"status":"EXISTIEREND"},
  {"name":"Acme Sàrl","ehraid":1565377,"uid":"CHE424492624","legalSeat":"Berneck","legalFormId":1,"status":"EXISTIEREND"}
]}`

const detailJSON = `{"name":"Acme Sàrl","uid":"CHE424492624","legalSeat":"Berneck","legalFormId":1,
  "status":"EXISTIEREND","address":{"street":"Kirchgass","houseNumber":"17",
  "swissZipCode":"9442","town":"Berneck","country":"CH"}}`

func lookup(t *testing.T, che string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/uid-lookup", UIDLookup)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/uid-lookup?che="+che, nil))

	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestLookupFillsCompanyDetails(t *testing.T) {
	stubZefix(t, hitJSON, detailJSON, http.StatusOK)

	w, body := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	for field, want := range map[string]string{
		"name":                "Acme Sàrl",
		"address_street":      "Kirchgass 17",
		"address_postal_code": "9442", // was silently empty: the struct read "swissZip"
		"address_city":        "Berneck",
		"legal_form":          "Entreprise individuelle",
	} {
		if got, _ := body[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
}

// A free-text search returns neighbours; filling in the wrong company would be
// worse than filling in nothing.
func TestOnlyExactUIDMatchIsAccepted(t *testing.T) {
	stubZefix(t, hitJSON, detailJSON, http.StatusOK)
	_, body := lookup(t, "CHE-111.111.111")
	if body["name"] != nil {
		t.Errorf("a non-matching UID must not return a company, got %v", body["name"])
	}
}

func TestUnknownUIDIs404(t *testing.T) {
	stubZefix(t, `{"list":[]}`, detailJSON, http.StatusOK)
	w, _ := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// If the detail route fails we still hand back what the search gave us, rather
// than losing the lookup entirely.
func TestDegradesWhenDetailUnavailable(t *testing.T) {
	stubZefix(t, hitJSON, "", http.StatusInternalServerError)
	w, body := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["name"] != "Acme Sàrl" {
		t.Errorf("name = %v, want the value from the search hit", body["name"])
	}
	if body["address_city"] != "Berneck" {
		t.Errorf("city should fall back to the legal seat, got %v", body["address_city"])
	}
}

func TestAcceptsFormatsWithAndWithoutDots(t *testing.T) {
	for _, in := range []string{"CHE-424.492.624", "CHE424492624", "che-424492624", " CHE-424.492.624 "} {
		stubZefix(t, hitJSON, detailJSON, http.StatusOK)
		w, _ := lookup(t, strings.TrimSpace(in))
		if w.Code != http.StatusOK {
			t.Errorf("%q: status = %d, want 200", in, w.Code)
		}
	}
}

func TestRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{"", "CHE-12", "notaUID", "CHE-424.492.62X"} {
		w, _ := lookup(t, in)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", in, w.Code)
		}
	}
}

// The registry being down is not a user error, and the message must say the
// form can still be completed by hand.
func TestRegistryOutageGivesAnActionableMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	orig := zefixBaseURL
	zefixBaseURL = srv.URL
	defer func() { zefixBaseURL = orig }()
	resetLegalFormsCache(t)

	w, body := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "manuellement") {
		t.Errorf("message %q should tell the user they can type the details in", msg)
	}
	if strings.Contains(msg, "403") {
		t.Errorf("an HTTP status code must not leak into a wizard message: %q", msg)
	}
}

func TestNormaliseUID(t *testing.T) {
	for _, in := range []string{"CHE-424.492.624", "che424492624", "CHE 424 492 624"} {
		if got := normaliseUID(in); got != "CHE424492624" {
			t.Errorf("normaliseUID(%q) = %q", in, got)
		}
	}
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/zefix"
)

// The lookup itself is covered in internal/core/zefix. What matters here is the
// HTTP mapping: each failure mode must reach the user as the right status and a
// message they can act on.

func stubRegistry(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := zefix.BaseURL
	zefix.BaseURL = srv.URL
	t.Cleanup(func() { zefix.BaseURL = orig })
	zefix.ResetCacheForTest()
}

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

func TestLookupReturnsCompany(t *testing.T) {
	stubRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/firm/search.json"):
			_, _ = w.Write([]byte(`{"list":[{"name":"Acme","ehraid":1,"uid":"CHE424492624","legalSeat":"Berneck","legalFormId":1,"status":"EXISTIEREND"}]}`))
		case strings.HasSuffix(r.URL.Path, "/legalForm.json"):
			_, _ = w.Write([]byte(`[{"id":1,"name":{"fr":"Entreprise individuelle"},"kurzform":{"fr":"EI"}}]`))
		default:
			_, _ = w.Write([]byte(`{"name":"Acme","uid":"CHE424492624","legalFormId":1,"status":"EXISTIEREND","address":{"street":"Kirchgass","houseNumber":"17","swissZipCode":"9442","town":"Berneck","country":"CH"}}`))
		}
	})

	w, body := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if body["address_postal_code"] != "9442" {
		t.Errorf("postal code = %v, want 9442", body["address_postal_code"])
	}
}

func TestMalformedInputIs400(t *testing.T) {
	for _, in := range []string{"", "CHE-12", "pasunIDE"} {
		w, _ := lookup(t, in)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", in, w.Code)
		}
	}
}

func TestUnknownCompanyIs404(t *testing.T) {
	stubRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"list":[]}`))
	})
	w, _ := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// The message users saw was "registre IDE: réponse 403", which reads like they
// mistyped their number. An HTTP status code must never reach a wizard.
func TestOutageMessageIsActionableAndLeaksNoStatusCode(t *testing.T) {
	stubRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	w, body := lookup(t, "CHE-424.492.624")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "manuellement") {
		t.Errorf("message %q must tell the user they can type the details in", msg)
	}
	if strings.Contains(msg, "403") {
		t.Errorf("message %q must not contain an HTTP status code", msg)
	}
}

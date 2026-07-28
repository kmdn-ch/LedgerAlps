package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/services/updatecheck"
)

// These tests pin the JSON contract ComplianceBanner.tsx reads. The banner is
// how a legal change reaches the user, so a silent shape change here would fail
// closed — the banner would render nothing and nobody would notice.

func newComplianceRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/compliance/advisories", NewComplianceHandler(nil).ListAdvisories)
	return r
}

func getAdvisories(t *testing.T, query string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	r := newComplianceRouter()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/compliance/advisories"+query, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	return body
}

func TestListAdvisoriesReturnsExpectedEnvelope(t *testing.T) {
	body := getAdvisories(t, "")
	for _, key := range []string{"items", "total", "feed_date", "app_version", "lang"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response is missing %q — the frontend reads this", key)
		}
	}
	if _, ok := body["items"].([]any); !ok {
		t.Errorf("items must be an array, got %T", body["items"])
	}
}

func TestAdvisoryItemsCarryTheFieldsTheBannerRenders(t *testing.T) {
	items, _ := getAdvisories(t, "")["items"].([]any)
	if len(items) == 0 {
		t.Skip("no advisory relevant to this build — nothing to assert on")
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %T", items[0])
	}
	// severity drives the banner colour; source_url is the verifiable citation.
	for _, key := range []string{"id", "domain", "severity", "title", "body", "source_name", "source_url"} {
		v, present := first[key]
		if !present {
			t.Errorf("advisory item is missing %q", key)
			continue
		}
		if s, isStr := v.(string); isStr && s == "" {
			t.Errorf("advisory field %q is empty", key)
		}
	}
	sev, _ := first["severity"].(string)
	switch sev {
	case "info", "action_required", "critical":
	default:
		t.Errorf("severity %q is not one of the values the banner styles", sev)
	}
}

func TestListAdvisoriesLocalises(t *testing.T) {
	fr := getAdvisories(t, "?lang=fr")
	en := getAdvisories(t, "?lang=en")
	if fr["lang"] != "fr" || en["lang"] != "en" {
		t.Fatalf("lang not echoed back: fr=%v en=%v", fr["lang"], en["lang"])
	}

	frItems, _ := fr["items"].([]any)
	enItems, _ := en["items"].([]any)
	if len(frItems) == 0 || len(enItems) != len(frItems) {
		t.Skip("nothing relevant to compare")
	}
	frTitle := frItems[0].(map[string]any)["title"]
	enTitle := enItems[0].(map[string]any)["title"]
	if frTitle == enTitle {
		t.Errorf("fr and en titles are identical (%v) — localisation is not applied", frTitle)
	}
}

func TestUnknownLanguageStillReturnsText(t *testing.T) {
	// Switzerland has four official languages; a missing translation must
	// degrade to readable text, never to an empty banner.
	items, _ := getAdvisories(t, "?lang=rm")["items"].([]any)
	if len(items) == 0 {
		t.Skip("nothing relevant to assert on")
	}
	first := items[0].(map[string]any)
	if s, _ := first["title"].(string); s == "" {
		t.Error("unknown language produced an empty title instead of falling back")
	}
}

// With checking switched off the endpoint must still answer — and must never
// claim an update exists, because it never looked.
func TestUpdateCheckDisabledIsInert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/update-check", NewComplianceHandler(updatecheck.New(false, "", 0)).CheckForUpdate)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/update-check", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false", body["enabled"])
	}
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false", body["update_available"])
	}
}

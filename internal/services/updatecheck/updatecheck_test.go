package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func stubServer(t *testing.T, body string, status int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const releaseJSON = `{"tag_name":"v1.5.0","html_url":"https://example.ch/r/1.5.0","body":"notes","prerelease":false,"draft":false}`

func TestDetectsNewerRelease(t *testing.T) {
	srv, _ := stubServer(t, releaseJSON, http.StatusOK)
	res := New(true, srv.URL, time.Hour).Check(context.Background(), "1.4.0")

	if !res.UpdateAvailable {
		t.Error("1.5.0 should be offered to a user on 1.4.0")
	}
	if res.LatestVersion != "v1.5.0" || res.ReleaseURL == "" {
		t.Errorf("release details missing: %+v", res)
	}
}

func TestNoUpdateWhenCurrent(t *testing.T) {
	srv, _ := stubServer(t, releaseJSON, http.StatusOK)
	if New(true, srv.URL, time.Hour).Check(context.Background(), "1.5.0").UpdateAvailable {
		t.Error("a user already on the latest version must not be prompted")
	}
}

func TestNoUpdateWhenAhead(t *testing.T) {
	srv, _ := stubServer(t, releaseJSON, http.StatusOK)
	if New(true, srv.URL, time.Hour).Check(context.Background(), "1.6.0").UpdateAvailable {
		t.Error("a user ahead of the published release must not be prompted to downgrade")
	}
}

// Being offline is the normal state for local-first software; it must read as
// "no update known", never as an error the user has to interpret.
func TestNetworkFailureIsSilent(t *testing.T) {
	srv, _ := stubServer(t, "", http.StatusInternalServerError)
	res := New(true, srv.URL, time.Hour).Check(context.Background(), "1.4.0")
	if res.UpdateAvailable {
		t.Error("a failed check must not claim an update exists")
	}
	if !res.Enabled {
		t.Error("Enabled should still report true — checking is on, it merely failed")
	}
}

func TestUnreachableHostIsSilent(t *testing.T) {
	// Reserved TEST-NET-1 address: guaranteed not to answer.
	res := New(true, "http://192.0.2.1:9/latest", time.Hour).Check(context.Background(), "1.4.0")
	if res.UpdateAvailable {
		t.Error("an unreachable host must not produce an update prompt")
	}
}

func TestDisabledMakesNoRequest(t *testing.T) {
	srv, calls := stubServer(t, releaseJSON, http.StatusOK)
	res := New(false, srv.URL, time.Hour).Check(context.Background(), "1.0.0")

	if *calls != 0 {
		t.Errorf("disabled checker must not touch the network, made %d call(s)", *calls)
	}
	if res.Enabled || res.UpdateAvailable {
		t.Errorf("disabled checker reported %+v", res)
	}
}

func TestResultIsCached(t *testing.T) {
	srv, calls := stubServer(t, releaseJSON, http.StatusOK)
	c := New(true, srv.URL, time.Hour)
	for i := 0; i < 5; i++ {
		c.Check(context.Background(), "1.4.0")
	}
	if *calls != 1 {
		t.Errorf("expected 1 network call within the interval, got %d", *calls)
	}
}

func TestFailureIsAlsoCached(t *testing.T) {
	// A blocked network must not be retried on every request.
	srv, calls := stubServer(t, "", http.StatusForbidden)
	c := New(true, srv.URL, time.Hour)
	for i := 0; i < 4; i++ {
		c.Check(context.Background(), "1.4.0")
	}
	if *calls != 1 {
		t.Errorf("expected the failure to be cached, got %d calls", *calls)
	}
}

func TestCacheExpires(t *testing.T) {
	srv, calls := stubServer(t, releaseJSON, http.StatusOK)
	c := New(true, srv.URL, time.Hour)
	current := time.Now()
	c.now = func() time.Time { return current }

	c.Check(context.Background(), "1.4.0")
	current = current.Add(2 * time.Hour)
	c.Check(context.Background(), "1.4.0")

	if *calls != 2 {
		t.Errorf("expected a refresh after the interval, got %d calls", *calls)
	}
}

func TestPrereleaseIsIgnored(t *testing.T) {
	srv, _ := stubServer(t,
		`{"tag_name":"v2.0.0-rc1","html_url":"u","prerelease":true,"draft":false}`, http.StatusOK)
	if New(true, srv.URL, time.Hour).Check(context.Background(), "1.4.0").UpdateAvailable {
		t.Error("a release candidate must never be offered to a stable install")
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v1.5.0", "1.4.0", true},
		{"1.4.1", "1.4.0", true},
		{"2.0.0", "1.9.9", true},
		{"1.4.0", "1.4.0", false},
		{"1.3.0", "1.4.0", false},
		{"v1.4.0", "v1.4.0", false},
		{"1.5.0", "dev", false}, // dev build: never prompted
		{"not-a-version", "1.4.0", false},
		{"1.5.0", "", false},
		{"1.5.0-rc1", "1.4.0", true}, // suffix dropped
		{"1.5", "1.4.0", true},       // short form
		{"1.2.3.4", "1.0.0", false},  // four components: refuse rather than guess
	}
	for _, c := range cases {
		if got := IsNewer(c.candidate, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

// Une adresse de publication vient du RESEAU et finit dans un attribut href.
// React 18 avertit sur « javascript: » mais le rend tout de meme -- le blocage
// n'arrive qu'en React 19. La garde doit donc etre ici.
func TestUneAdresseDePublicationDouteuseNEstPasRendue(t *testing.T) {
	douteuses := map[string]string{
		"javascript:alert(1)":               "schema javascript",
		"JaVaScRiPt:alert(1)":               "schema javascript, casse melangee",
		"data:text/html,<script>x</script>": "schema data",
		"file:///C:/Windows/System32":       "schema file",
		"http://exemple.ch/r/1.5.0":         "http en clair",
		"//exemple.ch/r/1.5.0":              "sans schema",
		"https://":                          "sans hote",
	}
	for brut, pourquoi := range douteuses {
		corps := `{"tag_name":"v9.0.0","html_url":"` + brut +
			`","body":"x","prerelease":false,"draft":false}`
		srv, _ := stubServer(t, corps, http.StatusOK)
		res := New(true, srv.URL, time.Hour).Check(context.Background(), "1.0.0")
		if res.ReleaseURL != "" {
			t.Errorf("adresse rendue %q — %s", res.ReleaseURL, pourquoi)
		}
		// La version, elle, reste annoncee : refuser le lien ne doit pas
		// cacher qu'une mise a jour existe.
		if res.LatestVersion != "v9.0.0" {
			t.Errorf("version perdue avec le lien (%s)", pourquoi)
		}
	}
}

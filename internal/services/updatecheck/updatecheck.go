// Package updatecheck reports whether a newer LedgerAlps release exists.
//
// It closes the loop of the compliance system. The CI watcher tells maintainers
// that a law or standard moved; they ship a conforming update; this tells the
// user to install it. Without this last step a user who never updates keeps
// issuing invoices that banks reject, with no way of knowing.
//
// Deliberately minimal, because LedgerAlps is sovereign software:
//
//   - one plain GET of a public endpoint, no identifiers, no telemetry, nothing
//     about the user or their data leaves the machine;
//   - disabled with a single setting;
//   - cached, so a running instance asks at most once a day;
//   - fails silently — being offline is the normal case for this product and
//     must never surface as an error.
//
// This is why no signing key is involved: the check reveals only that a version
// number exists, and the user downloads the release from the same host anyway.
// A signed advisory feed would add a private key to protect — the highest-value
// target in an accounting product, since whoever holds it can push fabricated
// legal instructions — in exchange for saving a version bump.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/semver"
)

const (
	// DefaultEndpoint is GitHub's public "latest release" API. It excludes
	// pre-releases, so release candidates cut from `test` are never proposed
	// to users running a stable build.
	DefaultEndpoint = "https://api.github.com/repos/kmdn-ch/LedgerAlps/releases/latest"

	// DefaultInterval bounds how often the network is touched.
	DefaultInterval = 24 * time.Hour

	requestTimeout = 10 * time.Second
)

// Result describes the outcome of a check.
type Result struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	CheckedAt       string `json:"checked_at,omitempty"`

	// Enabled reports whether checking is switched on at all, so the UI can
	// distinguish "up to date" from "we never looked".
	Enabled bool `json:"enabled"`
}

// Checker performs and caches update checks.
type Checker struct {
	mu       sync.Mutex
	cached   *Result
	fetched  time.Time
	endpoint string
	interval time.Duration
	enabled  bool
	client   *http.Client
	now      func() time.Time // injectable for tests
}

// New builds a Checker. When enabled is false every call reports
// Enabled: false and no network request is ever made.
func New(enabled bool, endpoint string, interval time.Duration) *Checker {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Checker{
		endpoint: endpoint,
		interval: interval,
		enabled:  enabled,
		client:   &http.Client{Timeout: requestTimeout},
		now:      time.Now,
	}
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// Check returns the cached result, refreshing it when the interval has elapsed.
//
// It never returns an error: an unreachable network is the expected condition
// for local-first software, and the honest answer in that case is "no update
// known", not a failure the user must interpret.
// urlDePublication ne laisse passer qu'une adresse https, avec un hote.
//
// La garde porte sur le SCHEMA et non sur l'hote : le point d'acces est
// configurable (voir New), et epingler github.com condamnerait un miroir
// interne ou un depot d'entreprise -- une installation local-first a de
// bonnes raisons de ne pas interroger GitHub.
//
// C'est aussi la propriete qui compte pour la destination : cette chaine
// finit dans un attribut href, ou « javascript: », « data: » et « file: »
// sont le danger. Meme regle que scripts/compliance_watch.py sur ses
// sources. Une adresse refusee devient vide, et l'interface n'affiche
// alors aucun lien -- mieux vaut pas de lien qu'un mauvais.
func urlDePublication(brut string) string {
	u, err := url.Parse(brut)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return brut
}

func (c *Checker) Check(ctx context.Context, currentVersion string) Result {
	if !c.enabled {
		return Result{CurrentVersion: currentVersion, Enabled: false}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	if c.cached != nil && now.Sub(c.fetched) < c.interval {
		out := *c.cached
		out.CurrentVersion = currentVersion
		return out
	}

	res := Result{CurrentVersion: currentVersion, Enabled: true}

	latest, err := c.fetchLatest(ctx)
	if err != nil {
		// Cache the failure for the same interval so a broken or blocked
		// network is not retried on every single request.
		c.cached, c.fetched = &res, now
		return res
	}

	res.LatestVersion = latest.TagName
	// Cette URL vient du reseau et finit dans un attribut href. React 18
	// avertit sur un href en « javascript: » mais le rend tout de meme --
	// le blocage n'arrive qu'en React 19. On n'accepte donc que https, et
	// seulement vers l'hote de publication.
	//
	// Meme raisonnement que scripts/compliance_watch.py, qui valide deja le
	// schema de ses sources : « la garde coute deux lignes et ferme la
	// classe entiere ». Elle manquait ici, sur la meme espece de donnee.
	res.ReleaseURL = urlDePublication(latest.HTMLURL)
	res.ReleaseNotes = latest.Body
	res.CheckedAt = now.UTC().Format(time.RFC3339)
	res.UpdateAvailable = IsNewer(latest.TagName, currentVersion)

	c.cached, c.fetched = &res, now
	return res
}

func (c *Checker) fetchLatest(ctx context.Context) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, err
	}
	// The User-Agent carries the product name only — no version, no machine
	// identifier, nothing that could profile the installation.
	req.Header.Set("User-Agent", "LedgerAlps")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check: HTTP %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.Draft || rel.Prerelease {
		return nil, fmt.Errorf("update check: endpoint returned a draft or pre-release")
	}
	return &rel, nil
}

// IsNewer reports whether candidate is a strictly higher version than current.
//
// A development build (an unparseable current version) never triggers the
// prompt: a developer running from source is not a user who needs to download
// an installer.
func IsNewer(candidate, current string) bool {
	cand, candOK := semver.Parse(candidate)
	cur, curOK := semver.Parse(current)
	if !candOK || !curOK {
		return false
	}
	for i := 0; i < 3; i++ {
		if cand[i] != cur[i] {
			return cand[i] > cur[i]
		}
	}
	return false
}

package compliance

// Compliance advisories: how a legal or specification change reaches a user
// who already has LedgerAlps installed.
//
// The problem this solves: a binary shipped today cannot know what the law will
// say in two years. Two mechanisms cover that, in order of trust:
//
//  1. Bundled feed — compliance/advisories.json is embedded at build time. It
//     always works, needs no network, and reflects what was known at release.
//     This is the sovereign default: LedgerAlps runs entirely offline.
//
//  2. Signed refresh (opt-in) — the app may fetch an updated feed and merge it,
//     so an advisory written after the installed release still reaches the user.
//     The payload is Ed25519-signed and verified against a key compiled into
//     the binary. Without that check, anyone able to intercept the connection
//     could push fabricated legal instructions into an accounting product; an
//     unsigned feed would be worse than no feed at all.
//
// The fetch sends no identifiers, no telemetry and no user data — it is a plain
// GET of a static document — and any failure leaves the bundled feed in place.

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/semver"
)

// Severity ranks how urgently a user must act.
const (
	SeverityInfo           = "info"            // awareness only
	SeverityActionRequired = "action_required" // the user must do something
	SeverityCritical       = "critical"        // non-compliant right now
)

var severityRank = map[string]int{
	SeverityCritical:       0,
	SeverityActionRequired: 1,
	SeverityInfo:           2,
}

// Advisory is one compliance notice. Text is per-language so the UI can show
// the user's own language; Switzerland has four official ones.
type Advisory struct {
	ID         string            `json:"id"`
	Domain     string            `json:"domain"`
	Severity   string            `json:"severity"`
	Title      map[string]string `json:"title"`
	Body       map[string]string `json:"body"`
	SourceName string            `json:"source_name"`
	SourceURL  string            `json:"source_url"`

	PublishedAt   string `json:"published_at"`
	EffectiveFrom string `json:"effective_from"`

	// ResolvedInVersion is the first release that satisfies the requirement.
	// A user already on that version does not need to be told about it.
	ResolvedInVersion string `json:"resolved_in_version"`

	// AssumesAbsent lists the capabilities this advisory claims LedgerAlps
	// lacks. It is what keeps a notice honest: ship the capability, and the
	// build fails until the advisory is rewritten or retired.
	//
	// A stale compliance warning costs more than it looks. Users act on it,
	// spend effort on a problem that no longer exists, and stop believing the
	// next one — and these notices only work while they are believed. See
	// capabilities.go.
	AssumesAbsent []Capability `json:"assumes_absent,omitempty"`

	// Condition, when set, names a runtime check that decides whether this
	// advisory applies to THIS machine. `assumes_absent` guards against the
	// product changing; this guards against telling a particular user something
	// untrue of their installation — such as asking for disk encryption on a
	// machine that already has it.
	//
	// An unrecognised condition shows the advisory. Hiding a compliance notice
	// because of a typo is the failure that must not happen silently.
	Condition Condition `json:"condition,omitempty"`
}

// Condition names a runtime check evaluated when advisories are served.
type Condition string

// DiskNotEncrypted — the machine's disk is not confirmed encrypted.
const DiskNotEncrypted Condition = "disk_not_encrypted"

// KnownCondition reports whether a condition name is one we evaluate.
func KnownCondition(c Condition) bool {
	return c == DiskNotEncrypted
}

// Feed is the advisory document, bundled and optionally refreshed.
type Feed struct {
	SchemaVersion int        `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Notice        string     `json:"notice,omitempty"`
	Advisories    []Advisory `json:"advisories"`
}

// SupportedSchemaVersion is the feed layout this build understands. A newer
// feed is ignored rather than half-parsed — showing a mangled legal notice is
// worse than showing the older bundled one.
const SupportedSchemaVersion = 1

// ParseFeed decodes and validates a feed document.
func ParseFeed(data []byte) (*Feed, error) {
	var f Feed
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing advisory feed: %w", err)
	}
	if f.SchemaVersion != SupportedSchemaVersion {
		return nil, fmt.Errorf(
			"advisory feed schema version %d is not supported by this build (expects %d)",
			f.SchemaVersion, SupportedSchemaVersion)
	}
	for i, a := range f.Advisories {
		if a.ID == "" {
			return nil, fmt.Errorf("advisory %d has no id", i)
		}
		// A notice a user cannot verify is not actionable, and unverifiable
		// legal claims are exactly what this system must not produce.
		if a.SourceURL == "" {
			return nil, fmt.Errorf("advisory %q has no source_url", a.ID)
		}
		if _, ok := severityRank[a.Severity]; !ok {
			return nil, fmt.Errorf("advisory %q has unknown severity %q", a.ID, a.Severity)
		}
	}
	return &f, nil
}

// VerifySignedFeed checks an Ed25519 signature over data before parsing it.
// Used for the refresh path; the bundled feed needs no signature because it is
// already inside the signed binary.
func VerifySignedFeed(data, signature []byte, publicKey ed25519.PublicKey) (*Feed, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid advisory public key length %d", len(publicKey))
	}
	if !ed25519.Verify(publicKey, data, signature) {
		return nil, fmt.Errorf("advisory feed signature verification failed — refusing to load")
	}
	return ParseFeed(data)
}

// Relevant selects the advisories worth showing.
//
// An advisory is hidden when the running version already resolves it, and when
// it takes effect further ahead than horizon — warning about an obligation two
// years out on every launch trains users to dismiss the banner, which is how
// the one that matters gets missed.
func (f *Feed) Relevant(currentVersion string, now time.Time, horizon time.Duration) []Advisory {
	var out []Advisory
	for _, a := range f.Advisories {
		if a.ResolvedInVersion != "" && semver.AuMoins(currentVersion, a.ResolvedInVersion) {
			continue
		}
		if a.EffectiveFrom != "" {
			eff, err := time.Parse("2006-01-02", a.EffectiveFrom)
			if err == nil && eff.After(now.Add(horizon)) {
				continue
			}
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank[out[i].Severity], severityRank[out[j].Severity]
		if ri != rj {
			return ri < rj
		}
		return out[i].EffectiveFrom < out[j].EffectiveFrom
	})
	return out
}

// Localised returns the text for lang, falling back to French (the product's
// default) then English then any available translation, so a missing
// translation degrades to a readable notice rather than an empty banner.
func Localised(m map[string]string, lang string) string {
	if v, ok := m[lang]; ok && v != "" {
		return v
	}
	for _, fb := range []string{"fr", "en"} {
		if v, ok := m[fb]; ok && v != "" {
			return v
		}
	}
	for _, v := range m {
		if v != "" {
			return v
		}
	}
	return ""
}

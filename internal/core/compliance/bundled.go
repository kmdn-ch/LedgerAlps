package compliance

import (
	_ "embed"
	"sync"
)

// advisories.json is compiled into the binary so LedgerAlps can warn about a
// compliance change with no network access at all — the sovereign default.
// The optional signed refresh (see advisory.go) only ever adds to this baseline.
//
//go:embed advisories.json
var bundledFeedJSON []byte

var (
	bundledOnce sync.Once
	bundledFeed *Feed
	bundledErr  error
)

// BundledFeed returns the advisory feed shipped with this build.
//
// It is parsed once and cached. A parse failure is returned rather than
// panicking, but it should be impossible in a released binary: TestBundledFeedIsValid
// parses this exact file in CI, so a malformed feed fails the build instead of
// reaching a user.
func BundledFeed() (*Feed, error) {
	bundledOnce.Do(func() {
		bundledFeed, bundledErr = ParseFeed(bundledFeedJSON)
	})
	return bundledFeed, bundledErr
}

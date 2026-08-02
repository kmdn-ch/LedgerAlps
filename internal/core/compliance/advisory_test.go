package compliance

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"
)

func feedJSON(t *testing.T, advisories ...Advisory) []byte {
	t.Helper()
	b, err := json.Marshal(Feed{SchemaVersion: 1, Advisories: advisories})
	if err != nil {
		t.Fatalf("marshal feed: %v", err)
	}
	return b
}

func adv(id, severity, resolvedIn, effectiveFrom string) Advisory {
	return Advisory{
		ID:                id,
		Domain:            "qr_bill",
		Severity:          severity,
		Title:             map[string]string{"fr": "Titre " + id, "en": "Title " + id},
		Body:              map[string]string{"fr": "Corps", "en": "Body"},
		SourceName:        "Source",
		SourceURL:         "https://example.ch/" + id,
		ResolvedInVersion: resolvedIn,
		EffectiveFrom:     effectiveFrom,
	}
}

// ─── Parsing and validation ───────────────────────────────────────────────────

func TestParseFeedRejectsUnsupportedSchema(t *testing.T) {
	data := []byte(`{"schema_version": 99, "advisories": []}`)
	if _, err := ParseFeed(data); err == nil {
		t.Error("a newer schema must be refused, not half-parsed")
	}
}

func TestParseFeedRequiresSourceURL(t *testing.T) {
	a := adv("no-source", SeverityInfo, "", "")
	a.SourceURL = ""
	if _, err := ParseFeed(feedJSON(t, a)); err == nil {
		t.Error("an advisory with no citable source must be rejected")
	}
}

func TestParseFeedRejectsUnknownSeverity(t *testing.T) {
	a := adv("bad-sev", "catastrophic", "", "")
	if _, err := ParseFeed(feedJSON(t, a)); err == nil {
		t.Error("unknown severity must be rejected")
	}
}

func TestParseFeedAcceptsValidFeed(t *testing.T) {
	f, err := ParseFeed(feedJSON(t, adv("ok", SeverityInfo, "", "")))
	if err != nil {
		t.Fatalf("valid feed rejected: %v", err)
	}
	if len(f.Advisories) != 1 {
		t.Errorf("got %d advisories, want 1", len(f.Advisories))
	}
}

// ─── Signature verification ───────────────────────────────────────────────────

func TestVerifySignedFeedAcceptsGoodSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := feedJSON(t, adv("signed", SeverityInfo, "", ""))
	if _, err := VerifySignedFeed(data, ed25519.Sign(priv, data), pub); err != nil {
		t.Errorf("correctly signed feed rejected: %v", err)
	}
}

func TestVerifySignedFeedRejectsTamperedPayload(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	data := feedJSON(t, adv("signed", SeverityInfo, "", ""))
	sig := ed25519.Sign(priv, data)

	// An attacker rewrites the legal text but keeps the original signature.
	tampered := feedJSON(t, adv("signed", SeverityCritical, "", ""))
	if _, err := VerifySignedFeed(tampered, sig, pub); err == nil {
		t.Error("tampered feed must be refused — fabricated legal advice is the threat here")
	}
}

func TestVerifySignedFeedRejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)
	data := feedJSON(t, adv("signed", SeverityInfo, "", ""))
	if _, err := VerifySignedFeed(data, ed25519.Sign(priv, data), otherPub); err == nil {
		t.Error("a feed signed by an unknown key must be refused")
	}
}

func TestVerifySignedFeedRejectsMalformedKey(t *testing.T) {
	data := feedJSON(t, adv("signed", SeverityInfo, "", ""))
	if _, err := VerifySignedFeed(data, []byte("sig"), ed25519.PublicKey("too-short")); err == nil {
		t.Error("a malformed public key must be refused")
	}
}

// ─── Relevance filtering ──────────────────────────────────────────────────────

func TestResolvedAdvisoriesAreHidden(t *testing.T) {
	f, err := ParseFeed(feedJSON(t,
		adv("fixed", SeverityActionRequired, "1.3.14", "2026-01-01"),
		adv("open", SeverityActionRequired, "", "2026-01-01"),
	))
	if err != nil {
		t.Fatal(err)
	}
	got := f.Relevant("1.3.15", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 180*24*time.Hour)
	if len(got) != 1 || got[0].ID != "open" {
		t.Errorf("expected only the unresolved advisory, got %+v", ids(got))
	}
}

func TestAdvisoryStillShownOnOlderVersion(t *testing.T) {
	f, _ := ParseFeed(feedJSON(t, adv("fixed", SeverityActionRequired, "1.3.14", "2026-01-01")))
	got := f.Relevant("1.3.13", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 180*24*time.Hour)
	if len(got) != 1 {
		t.Error("a user on an older build must still be told")
	}
}

func TestFarFutureAdvisoriesAreDeferred(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	f, _ := ParseFeed(feedJSON(t,
		adv("soon", SeverityInfo, "", "2026-09-01"),
		adv("distant", SeverityInfo, "", "2029-01-01"),
	))
	got := f.Relevant("1.3.15", now, 180*24*time.Hour)
	if len(got) != 1 || got[0].ID != "soon" {
		t.Errorf("only the near-term advisory should show, got %v", ids(got))
	}
}

func TestAdvisoryWithoutEffectiveDateAlwaysShows(t *testing.T) {
	f, _ := ParseFeed(feedJSON(t, adv("undated", SeverityInfo, "", "")))
	got := f.Relevant("1.3.15", time.Now(), 180*24*time.Hour)
	if len(got) != 1 {
		t.Error("an advisory with no effective date must not be filtered out")
	}
}

func TestSortedBySeverityThenDate(t *testing.T) {
	f, _ := ParseFeed(feedJSON(t,
		adv("c", SeverityInfo, "", "2026-01-01"),
		adv("a", SeverityCritical, "", "2026-05-01"),
		adv("b", SeverityActionRequired, "", "2026-02-01"),
	))
	got := f.Relevant("1.3.15", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 365*24*time.Hour)
	want := []string{"a", "b", "c"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("order = %v, want %v", ids(got), want)
		}
	}
}

// ─── Version comparison ───────────────────────────────────────────────────────

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		current, target string
		want            bool
	}{
		{"1.3.15", "1.3.14", true},
		{"1.3.14", "1.3.14", true},
		{"1.3.13", "1.3.14", false},
		{"v1.3.15", "1.3.14", true},    // leading v tolerated
		{"1.4.0", "1.3.99", true},      // minor beats patch
		{"2.0.0", "1.9.9", true},       // major beats minor
		{"1.3.15-rc1", "1.3.14", true}, // pre-release suffix ignored
		{"dev", "1.3.14", true},        // dev build assumed current
		{"1.3", "1.3.0", true},         // short form
	}
	for _, c := range cases {
		if got := versionAtLeast(c.current, c.target); got != c.want {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", c.current, c.target, got, c.want)
		}
	}
}

// ─── Localisation ─────────────────────────────────────────────────────────────

func TestLocalisedFallsBack(t *testing.T) {
	m := map[string]string{"fr": "Bonjour", "en": "Hello"}
	if got := Localised(m, "fr"); got != "Bonjour" {
		t.Errorf("fr = %q", got)
	}
	if got := Localised(m, "de"); got != "Bonjour" {
		t.Errorf("missing de should fall back to fr, got %q", got)
	}
	if got := Localised(map[string]string{"it": "Ciao"}, "de"); got != "Ciao" {
		t.Errorf("should fall back to any available translation, got %q", got)
	}
	if got := Localised(map[string]string{}, "fr"); got != "" {
		t.Errorf("empty map should yield empty string, got %q", got)
	}
}

func ids(as []Advisory) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}

// ─── Bundled feed ─────────────────────────────────────────────────────────────

// The shipped feed must always parse: a malformed advisories.json should fail
// the build, never reach a user as a blank or mangled legal notice.
func TestBundledFeedIsValid(t *testing.T) {
	f, err := BundledFeed()
	if err != nil {
		t.Fatalf("bundled advisories.json does not parse: %v", err)
	}
	if len(f.Advisories) == 0 {
		t.Error("bundled feed contains no advisories")
	}
	for _, a := range f.Advisories {
		if a.SourceURL == "" || a.SourceName == "" {
			t.Errorf("advisory %q ships without a citable source", a.ID)
		}
		if Localised(a.Title, "fr") == "" || Localised(a.Body, "fr") == "" {
			t.Errorf("advisory %q has no French text (product default language)", a.ID)
		}
		if Localised(a.Title, "en") == "" {
			t.Errorf("advisory %q has no English title", a.ID)
		}
	}
}

func TestBundledFeedRelevanceAtCurrentRelease(t *testing.T) {
	f, err := BundledFeed()
	if err != nil {
		t.Fatal(err)
	}
	// A user on 1.3.15 should no longer see the QR-bill items already fixed.
	got := f.Relevant("1.3.15", time.Now(), 180*24*time.Hour)
	for _, a := range got {
		if a.ID == "qr-bill-qrr-chf-only" || a.ID == "qr-bill-address-type-s" {
			t.Errorf("advisory %q is resolved in this version and must not be shown", a.ID)
		}
	}
}

// ─── Cohérence entre les avis et le produit ───────────────────────────────────

// The guard this whole mechanism exists for.
//
// Encrypted backups shipped in v1.4.4 and the advisory went on telling users
// their backups were in clear. The roadmap even said this entry would be
// retired — and it was not, because nothing forced the question. Now shipping a
// capability and forgetting the notice fails the build instead of reaching a
// user as false compliance advice.
func TestNoAdvisoryContradictsWhatTheProductDoes(t *testing.T) {
	f, err := BundledFeed()
	if err != nil {
		t.Fatalf("BundledFeed: %v", err)
	}
	for _, a := range f.Advisories {
		for _, cap := range a.AssumesAbsent {
			if Has(cap) {
				t.Errorf("l'avis %q suppose que LedgerAlps n'a pas %q — or il l'a désormais.\n"+
					"  Réécrivez cet avis, ou renseignez resolved_in_version, avant de livrer.",
					a.ID, cap)
			}
		}
	}
}

// A typo in assumes_absent would read as "capability absent" and retire the
// guard in silence — the failure mode this design has to avoid above all.
func TestAssumedCapabilitiesAreAllKnown(t *testing.T) {
	f, err := BundledFeed()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range f.Advisories {
		for _, cap := range a.AssumesAbsent {
			if !KnownCapability(cap) {
				t.Errorf("l'avis %q déclare la capacité inconnue %q — faute de frappe ? "+
					"Une capacité inconnue serait lue comme absente et désactiverait le contrôle.",
					a.ID, cap)
			}
		}
	}
}

// An advisory that is neither resolved nor tied to a capability drifts with
// nothing to catch it. Requiring one or the other is what makes the check
// complete rather than decorative.
func TestOpenAdvisoriesDeclareWhatTheyAssume(t *testing.T) {
	f, err := BundledFeed()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range f.Advisories {
		if a.ResolvedInVersion != "" {
			continue // already tied to a release
		}
		if len(a.AssumesAbsent) == 0 {
			t.Errorf("l'avis ouvert %q ne déclare ni resolved_in_version ni assumes_absent : "+
				"rien ne détectera qu'il est devenu faux", a.ID)
		}
	}
}

// The capability map is the single place a developer touches when shipping;
// an empty one would make every check above pass vacuously.
func TestCapabilityMapIsPopulated(t *testing.T) {
	if len(Capabilities) == 0 {
		t.Fatal("aucune capacité déclarée : les contrôles de cohérence ne vérifieraient rien")
	}
	if !Has(CapEncryptedBackups) {
		t.Error("CapEncryptedBackups devrait être vrai depuis la v1.4.4")
	}
	if Has(CapEncryptedDatabase) {
		t.Error("CapEncryptedDatabase devrait être faux : SQLCipher est incompatible avec CGO_ENABLED=0")
	}
}

// A typo in `condition` must not silently retire an advisory — the same failure
// the capability check guards against, from the other direction.
func TestAdvisoryConditionsAreAllKnown(t *testing.T) {
	f, err := BundledFeed()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range f.Advisories {
		if a.Condition == "" {
			continue
		}
		if !KnownCondition(a.Condition) {
			t.Errorf("l'avis %q déclare la condition inconnue %q — faute de frappe ?", a.ID, a.Condition)
		}
	}
}

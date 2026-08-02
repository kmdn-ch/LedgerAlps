package compliance

// What LedgerAlps actually does, so an advisory cannot claim otherwise.
//
// An advisory that describes a gap the product has since closed is worse than
// no advisory at all: users act on it, waste effort on a problem that no longer
// exists, and — the part that matters — stop believing the next one. Compliance
// warnings only work while they are trusted, and trust is spent by every notice
// that turns out to be stale.
//
// Remembering to revisit the advisories when shipping a feature does not work.
// It failed here: encrypted backups shipped in v1.4.4 and the advisory kept
// telling people their backups were in clear, even though the roadmap said in
// so many words that this entry would be retired.
//
// So the check is mechanical. Every advisory declares the capabilities it
// assumes LedgerAlps lacks. This file records which capabilities exist. A test
// fails the build when an advisory assumes the absence of something now
// present — naming the advisory and the capability. Shipping the feature and
// forgetting the notice is no longer possible; the build stops first.
//
// This is the inward-facing half of compliance monitoring. scripts/
// compliance_watch.py watches the law for changes from outside; this watches
// the product for changes from inside. Drift can come from either direction.

// Capability names something LedgerAlps can or cannot do that a compliance
// advisory might depend on.
type Capability string

const (
	// CapEncryptedBackups — snapshots can be encrypted with a passphrase.
	CapEncryptedBackups Capability = "encrypted_backups"
	// CapEncryptedDatabase — the live database is encrypted at rest.
	CapEncryptedDatabase Capability = "encrypted_database"
	// CapNativeTLS — the server can serve HTTPS itself.
	CapNativeTLS Capability = "native_tls"
	// CapContactAnonymisation — a contact can be anonymised while the
	// accounting records the CO requires are kept.
	CapContactAnonymisation Capability = "contact_anonymisation"
	// CapPeriodLocking — a closed period refuses new entries.
	CapPeriodLocking Capability = "period_locking"
	// CapQRBillSPS2026 — the QR-bill follows the SPS 2026 Implementation
	// Guidelines. Forward-looking advisories about a coming standard need a
	// capability too, or nothing tells us to retire them once we implement it.
	CapQRBillSPS2026 Capability = "qr_bill_sps_2026"
)

// Capabilities records what is true of the product today.
//
// Changing a value here is part of shipping the feature, not an afterthought:
// flip it, and the test tells you which advisories now contradict reality.
var Capabilities = map[Capability]bool{
	// v1.4.4 — Argon2id + XChaCha20-Poly1305, opt-in per backup.
	CapEncryptedBackups: true,

	// Not planned. SQLCipher is a C library and the product builds with
	// CGO_ENABLED=0 on a pure-Go SQLite driver, which is what gives it
	// cross-compilation and a single dependency-free binary. Disk encryption
	// (BitLocker, LUKS) is the answer, and the advisory says so.
	CapEncryptedDatabase: false,

	// v1.4.5 — TLS on any non-loopback interface, with a generated
	// certificate when none is supplied.
	CapNativeTLS: true,

	// Roadmap: "Suppression & droit à l'effacement (nLPD)".
	CapContactAnonymisation: false,

	// Roadmap: "Paramètres → Maintenance & Système", section Conformité.
	// Closing a fiscal year exists; refusing writes to a closed period does not.
	CapPeriodLocking: false,

	// SIX has not published SPS 2026 yet; the QR payload still follows IG v2.4.
	// The compliance watcher will notice the publication; this flips when the
	// implementation follows, and the advisory has to be rewritten then.
	CapQRBillSPS2026: false,
}

// Has reports whether the product has a capability. An unknown name reads as
// absent, so a typo in advisories.json cannot quietly retire an advisory —
// and ValidateCapabilityNames rejects the typo outright.
func Has(c Capability) bool { return Capabilities[c] }

// KnownCapability reports whether the name is one we define.
func KnownCapability(c Capability) bool {
	_, ok := Capabilities[c]
	return ok
}

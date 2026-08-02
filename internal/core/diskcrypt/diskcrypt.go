// Package diskcrypt reports whether this machine's disk is encrypted.
//
// It exists to stop nagging people who have already done the work. A compliance
// notice telling someone to enable BitLocker when BitLocker is on is exactly
// the kind of stale warning that teaches users to ignore the next one — and the
// next one might matter.
//
// The honest difficulty: detecting BitLocker properly needs administrator
// rights. Get-BitLockerVolume, the Win32_EncryptableVolume WMI class and
// manage-bde all answer "access denied" to a normal user, and LedgerAlps runs
// as a normal user. Measured, not assumed: all three were tried.
//
// So this reads HKLM\SYSTEM\CurrentControlSet\Control\BitLockerStatus\BootStatus,
// which a normal user can read. It reports whether the boot volume was
// BitLocker-protected at boot. That is a heuristic, and it is treated as one:
// a positive result hides the notice, anything else leaves it visible but
// worded as "check this", never as "your disk is not encrypted". Accusing
// someone wrongly is the failure this package is meant to avoid, so it is the
// one it refuses to risk.
package diskcrypt

// Status is what we could establish about disk encryption.
type Status string

const (
	// Encrypted — the boot volume is protected. Confident enough to stay quiet.
	Encrypted Status = "encrypted"
	// NotEncrypted — the system reports no protection on the boot volume.
	NotEncrypted Status = "not_encrypted"
	// Unknown — no supported way to look, or the lookup failed. Never presented
	// to the user as "not encrypted": we did not look, we do not know.
	Unknown Status = "unknown"
)

// Report describes what was found, and how it was found, so the interface can
// phrase itself accordingly rather than guessing.
type Report struct {
	Status Status `json:"status"`
	// Mechanism names what answered, for the diagnostics page. Empty when
	// nothing could be consulted.
	Mechanism string `json:"mechanism,omitempty"`
	// Advisory is true when the user should still be prompted. Deliberately not
	// simply `Status != Encrypted`: the caller should not have to re-derive the
	// rule, and getting it backwards would either nag the protected or reassure
	// the exposed.
	Advisory bool `json:"advisory"`
}

// Check reports the disk encryption status of this machine.
func Check() Report {
	st, mech := detect()
	return Report{
		Status:    st,
		Mechanism: mech,
		Advisory:  st != Encrypted,
	}
}

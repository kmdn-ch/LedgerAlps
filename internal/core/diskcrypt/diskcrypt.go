// Package diskcrypt reports whether this machine's disk is encrypted, and tells
// the user how to turn it on if it is not.
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
//
// # Why the Windows edition matters
//
// The feature has two names and two places, and telling a Windows Famille user
// to open BitLocker sends them looking for something their edition does not
// have. Famille has « Chiffrement de l'appareil », a restricted BitLocker that
// only appears when the firmware supports it; Professionnel and above have the
// full BitLocker control panel. Advice that names the wrong one is advice the
// user cannot follow, which is barely better than no advice.
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

// Report describes what was found, how it was found, and what to do about it,
// so the interface can phrase itself accordingly rather than guessing.
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

	// Feature is what this machine's operating system calls the thing to turn
	// on — "BitLocker" on Windows Professionnel, "Chiffrement de l'appareil" on
	// Famille, "LUKS" on Linux. Naming the wrong one sends the user hunting for
	// a menu entry that does not exist on their edition.
	Feature string `json:"feature,omitempty"`
	// Edition identifies the system, so a support question can be answered
	// without asking the user to go and find it.
	Edition string `json:"edition,omitempty"`
	// Steps is the route to the setting, in the words the system uses.
	Steps []string `json:"steps,omitempty"`
	// SettingsURI opens the right page directly where the system offers one.
	SettingsURI string `json:"settings_uri,omitempty"`
	// Caveat states the limit of what was checked. Shown alongside a positive
	// result too: "encrypted" here means the boot volume, not every disk.
	Caveat string `json:"caveat,omitempty"`
}

// Check reports the disk encryption status of this machine.
func Check() Report {
	r := detect()
	r.Advisory = r.Status != Encrypted
	return r
}

//go:build !windows

package diskcrypt

// detect has no non-elevated equivalent outside Windows.
//
// LUKS state lives in /etc/crypttab and the device mapper, both of which a
// service user typically cannot read reliably, and a wrong answer here costs
// more than no answer. So the advisory stays visible on these systems, worded
// as something to check rather than something we observed.
func detect() (Status, string) {
	return Unknown, ""
}

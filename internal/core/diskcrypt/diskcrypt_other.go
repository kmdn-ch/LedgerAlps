//go:build !windows

package diskcrypt

// detect has no non-elevated equivalent outside Windows.
//
// LUKS state lives in /etc/crypttab and the device mapper, both of which a
// service user typically cannot read reliably, and a wrong answer here costs
// more than no answer. So the advisory stays visible on these systems, worded
// as something to check rather than something we observed — and it carries the
// steps, because "we could not look" is only useful to someone who is then told
// where to look themselves.
func detect() Report {
	return Report{
		Status:  Unknown,
		Feature: "LUKS",
		Steps: []string{
			"Vérifiez l'état actuel : lsblk -o NAME,FSTYPE,MOUNTPOINT — une partition chiffrée apparaît en crypto_LUKS.",
			"Sur une installation existante, LUKS ne s'active pas après coup sans réinstaller ou migrer : prévoyez-le à l'installation du système.",
			"À défaut, chiffrez la partition qui porte les données de LedgerAlps et montez-la au démarrage.",
		},
		Caveat: "LedgerAlps ne peut pas constater l'état du chiffrement sur ce " +
			"système sans droits élevés : il ne l'affirme donc dans aucun sens.",
	}
}

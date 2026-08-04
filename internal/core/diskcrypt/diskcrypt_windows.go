//go:build windows

package diskcrypt

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// detect reads the BitLocker boot status a normal user is allowed to see, and
// the Windows edition, so the advice names the feature this machine actually
// has.
//
// HKLM\SYSTEM\CurrentControlSet\Control\BitLockerStatus\BootStatus is a DWORD
// set by Windows: 1 when the boot volume was BitLocker-protected at start.
// Unlike the WMI class and manage-bde, it does not require elevation — which
// matters, because LedgerAlps has none.
//
// Only the boot volume is covered. In practice %APPDATA% lives there, so that
// is where the database and its backups sit; a second, unencrypted data drive
// would not be seen. That limit is why a negative answer is phrased as "check"
// rather than "your disk is not encrypted" — and why it is also stated next to
// a positive one.
func detect() Report {
	edition, home := windowsEdition()
	r := Report{
		Edition: edition,
		Caveat: "Ce constat porte sur le disque de démarrage, là où LedgerAlps " +
			"range ses données. Un second disque, lui, n'est pas vérifié.",
	}

	if home {
		r.Feature = "Chiffrement de l'appareil"
		r.SettingsURI = "ms-settings:deviceencryption"
		r.Steps = []string{
			"Ouvrez Paramètres → Confidentialité et sécurité → Chiffrement de l'appareil.",
			"Basculez l'interrupteur sur Activé.",
			"Conservez la clé de récupération que Windows vous propose d'enregistrer : sans elle, un incident de démarrage rend le disque illisible.",
			"Si l'entrée n'apparaît pas, votre édition Famille ne peut pas l'activer — le matériel ne remplit pas les conditions (TPM 2.0, démarrage sécurisé). Une édition Professionnel, ou VeraCrypt, sont alors les voies possibles.",
		}
	} else {
		r.Feature = "BitLocker"
		r.SettingsURI = "ms-settings:deviceencryption"
		r.Steps = []string{
			"Ouvrez le Panneau de configuration → Système et sécurité → Chiffrement de lecteur BitLocker.",
			"Sur le lecteur du système d'exploitation (C:), cliquez sur Activer BitLocker.",
			"Enregistrez la clé de récupération ailleurs que sur ce disque : sans elle, un incident de démarrage rend le disque illisible.",
			"Le chiffrement se poursuit en arrière-plan ; vous pouvez continuer à travailler.",
		}
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\BitLockerStatus`, registry.QUERY_VALUE)
	if err != nil {
		r.Status = Unknown
		return r
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue("BootStatus")
	if err != nil {
		r.Status = Unknown
		return r
	}
	r.Mechanism = r.Feature
	if v == 1 {
		r.Status = Encrypted
		return r
	}
	r.Status = NotEncrypted
	return r
}

// windowsEdition returns a readable edition name and whether it is a Famille
// (Home) edition, which is the edition that has « Chiffrement de l'appareil »
// instead of the BitLocker control panel.
//
// EditionID is readable without elevation — verified on Windows 11 25H2, where
// it returned "Professional".
func windowsEdition() (name string, home bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		// Unknown edition: the BitLocker wording is the safer default. A
		// Professionnel user sent to the control panel finds it; a Famille user
		// sent there finds nothing and asks, which is recoverable. The reverse
		// error tells a Professionnel user their edition cannot do it, which is
		// false and stops them.
		return "", false
	}
	defer k.Close()

	editionID, _, _ := k.GetStringValue("EditionID")
	product, _, _ := k.GetStringValue("ProductName")
	display, _, _ := k.GetStringValue("DisplayVersion")

	name = product
	if display != "" {
		name = strings.TrimSpace(product + " " + display)
	}

	// Home comes in several flavours: Core, CoreN, CoreSingleLanguage,
	// CoreCountrySpecific. Matching the prefix covers them without listing
	// editions Microsoft has not shipped yet.
	home = strings.HasPrefix(editionID, "Core")
	return name, home
}

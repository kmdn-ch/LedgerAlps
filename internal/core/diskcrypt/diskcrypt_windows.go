//go:build windows

package diskcrypt

import "golang.org/x/sys/windows/registry"

// detect reads the BitLocker boot status a normal user is allowed to see.
//
// HKLM\SYSTEM\CurrentControlSet\Control\BitLockerStatus\BootStatus is a DWORD
// set by Windows: 1 when the boot volume was BitLocker-protected at start.
// Unlike the WMI class and manage-bde, it does not require elevation — which
// matters, because LedgerAlps has none.
//
// Only the boot volume is covered. In practice %APPDATA% lives there, so that
// is where the database and its backups sit; a second, unencrypted data drive
// would not be seen. That limit is why a negative answer is phrased as "check"
// rather than "your disk is not encrypted".
func detect() (Status, string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\BitLockerStatus`, registry.QUERY_VALUE)
	if err != nil {
		return Unknown, ""
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue("BootStatus")
	if err != nil {
		return Unknown, ""
	}
	if v == 1 {
		return Encrypted, "BitLocker"
	}
	return NotEncrypted, "BitLocker"
}

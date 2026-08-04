//go:build !windows

package secretstore

import "errors"

// Linux and macOS have no equivalent that works without a desktop session.
//
// The candidates all need something LedgerAlps cannot assume: the Secret
// Service API needs a running D-Bus and an unlocked keyring, which a headless
// server does not have; the kernel keyring does not survive a reboot. Rather
// than depend on a component that is absent exactly when the server starts
// unattended, the secret is stored in a 0600 file and the interface says that
// the file's permissions are the whole protection.
//
// The measure that does work there is disk encryption (LUKS), which is what
// docs/PRODUCTION.md recommends.

var errNoSeal = errors.New("cette plateforme ne peut pas sceller un secret au compte")

func sealAvailable() bool { return false }

func seal([]byte) ([]byte, error) { return nil, errNoSeal }

func unseal([]byte) ([]byte, error) { return nil, errNoSeal }

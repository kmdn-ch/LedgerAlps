//go:build windows

package secretstore

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// entropy is mixed into every seal. It does not add secrecy — it is a constant
// in a public binary — but it scopes the sealed blobs to LedgerAlps: a blob
// sealed here cannot be unsealed by another program that merely calls
// CryptUnprotectData on the same account.
var entropy = []byte("LedgerAlps/secretstore/v1")

func sealAvailable() bool { return true }

// seal wraps secret with DPAPI, bound to the current Windows user on this
// machine.
//
// CRYPTPROTECT_UI_FORBIDDEN because LedgerAlps runs as a background server: a
// blocking credential dialog behind the browser window would look like a hang.
// It fails rather than prompting, which is the behaviour we want.
func seal(secret []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(secret)), Data: &secret[0]}
	ent := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, &ent, 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	// Retour ignore volontairement : LocalFree n'echoue qu'en cas de handle
	// invalide, et la copie du secret a deja eu lieu (ligne suivante). Au pire
	// une fuite de quelques octets locaux au processus, jamais du secret.
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	// The blob belongs to Windows until LocalFree; copy before releasing it.
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func unseal(sealed []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(sealed)), Data: &sealed[0]}
	ent := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	// Retour ignore volontairement : LocalFree n'echoue qu'en cas de handle
	// invalide, et la copie du secret a deja eu lieu (ligne suivante). Au pire
	// une fuite de quelques octets locaux au processus, jamais du secret.
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

package db

// Backup passphrase strength.
//
// This passphrase is the only thing standing between a mislaid USB stick and
// the entire ledger. Unlike a login password, it protects a file an attacker
// can take away and attack at leisure, with no rate limiting and nobody
// watching. Argon2id makes each guess expensive, but nothing saves a short
// passphrase from a determined offline attack — so the rule is length first.

import (
	"fmt"
	"unicode"
)

// MinPassphraseLength is the floor for a backup passphrase.
//
// Sixteen characters, not eight: at eight, a modern offline attack is a matter
// of hours whatever the KDF. It is deliberately longer than what a login
// password would demand, because a login is behind rate limiting and lockout
// and this is not.
const MinPassphraseLength = 16

// ValidatePassphrase reports why a passphrase is too weak, or nil.
//
// The message names what is missing rather than restating the whole rule: the
// interface shows the checklist, and an error the user cannot act on is noise.
func ValidatePassphrase(p string) error {
	runes := []rune(p)
	if len(runes) < MinPassphraseLength {
		return fmt.Errorf("la phrase de passe doit compter au moins %d caractères (%d saisis)",
			MinPassphraseLength, len(runes))
	}

	var hasLower, hasUpper, hasDigit bool
	for _, r := range runes {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}

	var missing []string
	if !hasLower {
		missing = append(missing, "une minuscule")
	}
	if !hasUpper {
		missing = append(missing, "une majuscule")
	}
	if !hasDigit {
		missing = append(missing, "un chiffre")
	}
	if len(missing) > 0 {
		return fmt.Errorf("la phrase de passe doit contenir %s", joinFr(missing))
	}
	return nil
}

func joinFr(parts []string) string {
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " et " + parts[1]
	default:
		out := ""
		for i, p := range parts[:len(parts)-1] {
			if i > 0 {
				out += ", "
			}
			out += p
		}
		return out + " et " + parts[len(parts)-1]
	}
}

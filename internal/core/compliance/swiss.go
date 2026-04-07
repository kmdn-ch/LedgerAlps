package compliance

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ─── Swiss rounding (0.05 CHF — 5 Rappen rule) ───────────────────────────────

// RoundTo5Rappen rounds amount to the nearest 0.05 CHF as required by Swiss TVA law.
// Example: 10.123 → 10.10, 10.125 → 10.15, 10.127 → 10.15
func RoundTo5Rappen(amount float64) float64 {
	return math.Round(amount*20) / 20
}

// ─── TVA rates (Swiss 2024) ───────────────────────────────────────────────────

const (
	VATRateStandard  = 0.081 // 8.1%
	VATRateReduced   = 0.026 // 2.6% (food, books, etc.)
	VATRateSpecial   = 0.038 // 3.8% (hotel accommodation)
)

// ─── IBAN validation ──────────────────────────────────────────────────────────

var ibanClean = regexp.MustCompile(`\s+`)

// ValidateIBAN checks the IBAN checksum using the MOD-97 algorithm (ISO 13616).
// Returns nil if valid, an error with a descriptive message otherwise.
func ValidateIBAN(iban string) error {
	iban = strings.ToUpper(ibanClean.ReplaceAllString(iban, ""))
	if len(iban) < 5 || len(iban) > 34 {
		return fmt.Errorf("IBAN length %d is invalid (must be 5–34 characters)", len(iban))
	}

	// Move the first 4 characters to the end
	rearranged := iban[4:] + iban[:4]

	// Replace letters with digits (A=10, B=11, …, Z=35)
	var numeric strings.Builder
	for _, ch := range rearranged {
		if ch >= 'A' && ch <= 'Z' {
			numeric.WriteString(strconv.Itoa(int(ch-'A') + 10))
		} else if ch >= '0' && ch <= '9' {
			numeric.WriteRune(ch)
		} else {
			return fmt.Errorf("IBAN contains invalid character: %c", ch)
		}
	}

	// MOD-97 check
	remainder := mod97(numeric.String())
	if remainder != 1 {
		return fmt.Errorf("IBAN checksum invalid (expected MOD-97 = 1, got %d)", remainder)
	}
	return nil
}

// ValidateQRIBAN validates a Swiss QR-IBAN (must start with CH, IID 30000–31999).
func ValidateQRIBAN(qrIBAN string) error {
	if err := ValidateIBAN(qrIBAN); err != nil {
		return fmt.Errorf("QR-IBAN: %w", err)
	}
	clean := strings.ToUpper(ibanClean.ReplaceAllString(qrIBAN, ""))
	if !strings.HasPrefix(clean, "CH") {
		return fmt.Errorf("QR-IBAN must be a Swiss IBAN (CH prefix)")
	}
	// IID (bank identifier) is digits 5–9 in the IBAN (positions 4–8, 0-indexed)
	if len(clean) < 9 {
		return fmt.Errorf("QR-IBAN too short")
	}
	iid, err := strconv.Atoi(clean[4:9])
	if err != nil {
		return fmt.Errorf("QR-IBAN IID is not numeric")
	}
	if iid < 30000 || iid > 31999 {
		return fmt.Errorf("QR-IBAN IID %d is not in the QR-IID range (30000–31999)", iid)
	}
	return nil
}

// mod97 computes the MOD-97 remainder for a numeric string of arbitrary length.
func mod97(numeric string) int {
	remainder := 0
	for _, ch := range numeric {
		remainder = (remainder*10 + int(ch-'0')) % 97
	}
	return remainder
}

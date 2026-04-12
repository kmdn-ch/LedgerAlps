package compliance

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

// QR-bill (Swiss Payment Standard SPC 0200, Six-Group)
// Reference: https://www.six-group.com/dam/download/banking-services/standardization/qr-bill/ig-qr-bill-v2.3-en.pdf

// QRBillData contains all data needed to generate a Swiss QR-bill payload.
type QRBillData struct {
	// Creditor
	CreditorIBAN    string // QR-IBAN (for QRR) or IBAN (for SCOR/NON)
	CreditorName    string // max 70 chars
	CreditorAddress string // street + nr, max 70 chars
	CreditorCity    string // postal code + city, max 70 chars
	CreditorCountry string // ISO 3166-1 alpha-2, e.g. "CH"

	// Amount (use 0 to leave blank — for open-amount invoices)
	Amount   float64
	Currency string // CHF or EUR

	// Debtor (optional — leave Name empty to omit)
	DebtorName    string
	DebtorAddress string
	DebtorCity    string
	DebtorCountry string

	// Payment reference
	ReferenceType string // "QRR", "SCOR", or "NON"
	Reference     string // 27-digit QRR ref, ISO 11649 SCOR ref, or empty for NON

	// Message (optional, max 140 chars)
	Message string

	// Invoice metadata for bill information (optional)
	InvoiceDate   time.Time
	InvoiceNumber string
}

// GenerateQRBillPayload returns the newline-delimited text payload that is
// encoded into the Swiss QR code (SPC 0200 spec, section 4).
//
// Field structure follows QRCodeText.java from manuelbl/SwissQRBill exactly:
//   - fields are separated by LF (0x0A)
//   - NO trailing LF after the last field (SPC 0200 §4.1)
func GenerateQRBillPayload(d QRBillData) (string, error) {
	if err := validateQRBillData(d); err != nil {
		return "", fmt.Errorf("qr-bill: %w", err)
	}

	// field appends LF *before* each field except the first — this guarantees
	// that the final field never has a trailing LF, matching the reference
	// implementation and the SPC 0200 §4.1 requirement.
	var b strings.Builder
	first := true
	field := func(s string) {
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(s)
		first = false
	}

	// ── Header ────────────────────────────────────────────────────────────────
	field("SPC")  // Swiss Payments Code
	field("0200") // version
	field("1")    // coding: 1 = Latin (ISO 8859-1)

	// ── Creditor ──────────────────────────────────────────────────────────────
	clean := strings.ToUpper(ibanClean.ReplaceAllString(d.CreditorIBAN, ""))
	field(clean) // IBAN / QR-IBAN (no spaces, uppercase)
	field("K")   // address type: K = combined elements
	field(d.CreditorName)
	field(d.CreditorAddress) // street + building number
	field(d.CreditorCity)    // postal code + city (combined for K)
	field("")                // StrtNm / BldgNb — unused for K
	field("")                // PstCd / TwnNm   — unused for K
	field(d.CreditorCountry)

	// ── Ultimate creditor (§4.3.3 — reserved, all 7 fields empty) ────────────
	field("") // UC address type
	field("") // UC name
	field("") // UC address line 1
	field("") // UC address line 2
	field("") // UC zip / town
	field("") // UC town / unused
	field("") // UC country

	// ── Amount ────────────────────────────────────────────────────────────────
	if d.Amount > 0 {
		field(fmt.Sprintf("%.2f", math.Round(d.Amount*100)/100))
	} else {
		field("") // open amount
	}
	field(d.Currency)

	// ── Debtor ────────────────────────────────────────────────────────────────
	if d.DebtorName != "" {
		field("K")
		field(d.DebtorName)
		field(d.DebtorAddress)
		field(d.DebtorCity)
		field("") // unused for K
		field("") // unused for K
		field(d.DebtorCountry)
	} else {
		// Unknown debtor — 7 blank fields
		for i := 0; i < 7; i++ {
			field("")
		}
	}

	// ── Payment reference ─────────────────────────────────────────────────────
	field(d.ReferenceType) // QRR / SCOR / NON
	field(d.Reference)     // reference number (empty for NON)

	// ── Additional information ────────────────────────────────────────────────
	field(truncate(d.Message, 140))
	field("EPD") // End Payment Data — mandatory trailer; last field, no trailing LF

	// Bill information (§4.4.11, optional) is intentionally omitted:
	// the invoice number is already encoded in the Message field above.
	// Including a Swico S1 string here causes strict validators in some
	// Swiss banking apps to reject the QR bill when the invoice number
	// contains non-numeric characters.

	return b.String(), nil
}

// ─── QRR Reference (MOD-10 recursive) ────────────────────────────────────────

// modTable is the MOD-10 recursive carry table (Six-Group QRR spec).
var modTable = [10]int{0, 9, 4, 6, 8, 2, 7, 1, 3, 5}

// ComputeQRRCheckDigit computes the single MOD-10-recursive check digit for a
// numeric string (e.g. 26-digit partial reference → 27-digit QRR reference).
// The check digit is appended at the end.
func ComputeQRRCheckDigit(digits string) (int, error) {
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("QRR digits must be numeric, got %q", string(ch))
		}
	}
	carry := 0
	for _, ch := range digits {
		carry = modTable[(carry+int(ch-'0'))%10]
	}
	return (10 - carry) % 10, nil
}

// GenerateQRRReference generates a 27-digit QRR reference from a numeric
// invoice ID (up to 26 digits). Pads with leading zeros then appends the
// MOD-10 recursive check digit.
func GenerateQRRReference(invoiceID string) (string, error) {
	// Strip non-digits
	var digits strings.Builder
	for _, ch := range invoiceID {
		if unicode.IsDigit(ch) {
			digits.WriteRune(ch)
		}
	}
	d := digits.String()
	if len(d) > 26 {
		return "", fmt.Errorf("invoice ID too long: max 26 digits, got %d", len(d))
	}
	// Pad to 26 digits
	padded := fmt.Sprintf("%026s", d)
	padded = strings.ReplaceAll(padded, " ", "0")

	check, err := ComputeQRRCheckDigit(padded)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", padded, check), nil
}

// FormatQRRReference formats a 27-digit QRR reference in the Swiss display
// format: groups of 5+5+5+5+5+2 separated by spaces (right-aligned grouping).
// Example: "00 00000 00000 00000 00000 12345 6" → "000000000000000000001234 56"
// Standard display: "00000 00000 00000 00000 12345 6"
func FormatQRRReference(ref27 string) string {
	if len(ref27) != 27 {
		return ref27
	}
	// Display groups: 2+5+5+5+5+5 from the left (Six-Group display format)
	return fmt.Sprintf("%s %s %s %s %s %s",
		ref27[0:2],
		ref27[2:7],
		ref27[7:12],
		ref27[12:17],
		ref27[17:22],
		ref27[22:27])
}

// ─── Validation ───────────────────────────────────────────────────────────────

func validateQRBillData(d QRBillData) error {
	if d.CreditorIBAN == "" {
		return fmt.Errorf("creditor IBAN is required")
	}
	// Validate IBAN checksum (MOD-97, ISO 13616) — an invalid IBAN produces a
	// technically scannable but standards-non-compliant QR bill that all
	// Swiss banking apps and validators will reject.
	if err := ValidateIBAN(d.CreditorIBAN); err != nil {
		return fmt.Errorf("creditor IBAN: %w", err)
	}
	if d.CreditorName == "" {
		return fmt.Errorf("creditor name is required")
	}
	if len([]rune(d.CreditorName)) > 70 {
		return fmt.Errorf("creditor name exceeds 70 chars")
	}
	if d.Currency != "CHF" && d.Currency != "EUR" {
		return fmt.Errorf("currency must be CHF or EUR, got %q", d.Currency)
	}
	// SPC 0200 §4.3.5: if debtor is identified, country must be a valid 2-char ISO code.
	if d.DebtorName != "" && len(d.DebtorCountry) != 2 {
		return fmt.Errorf("debtor country (ISO 3166-1 alpha-2) is required when debtor is identified")
	}
	switch d.ReferenceType {
	case "QRR":
		if len(d.Reference) != 27 {
			return fmt.Errorf("QRR reference must be 27 digits, got %d", len(d.Reference))
		}
		for _, ch := range d.Reference {
			if ch < '0' || ch > '9' {
				return fmt.Errorf("QRR reference must be numeric")
			}
		}
		// Validate check digit
		check, _ := ComputeQRRCheckDigit(d.Reference[:26])
		expected := int(d.Reference[26] - '0')
		if check != expected {
			return fmt.Errorf("QRR reference check digit invalid (expected %d, got %d)", check, expected)
		}
	case "SCOR":
		if d.Reference == "" {
			return fmt.Errorf("SCOR reference type requires a reference value")
		}
	case "NON":
		// no reference required
	default:
		return fmt.Errorf("reference type must be QRR, SCOR, or NON; got %q", d.ReferenceType)
	}
	return nil
}

// truncate cuts s to maxRunes runes (SPC 0200 character limits).
func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}


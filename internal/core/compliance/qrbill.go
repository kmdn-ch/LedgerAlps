package compliance

// Swiss QR-bill payload generator — SPC 0200 v2.3
// Reference: ig-qr-bill-v2.3 (SIX-Group)
//
// Breaking change vs v2.2: combined address type "K" is removed.
// Only structured address type "S" is accepted by banking apps in v2.3.

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

// QRBillData contains all data needed to generate a Swiss QR-bill payload.
//
// Address fields use the SPC 0200 v2.3 Structured ("S") address type.
// Combined address type "K" was removed in v2.3 and is rejected by current
// Swiss banking applications.
//
// Street format: building number may be included in CreditorStreet
// (e.g. "Bahnhofstrasse 1") per the transitional provision in §4.2.2;
// CreditorBuildingNr may be left empty in that case.
type QRBillData struct {
	// ── Creditor ────────────────────────────────────────────────────────────
	CreditorIBAN       string // QR-IBAN (QRR) or regular IBAN (SCOR/NON), no spaces
	CreditorName       string // max 70 chars
	CreditorStreet     string // street + nr (§4.2.2: building nr may be included here), max 70
	CreditorBuildingNr string // building number only (optional), max 16
	CreditorPostalCode string // postal code, no country prefix, max 16 — mandatory
	CreditorTown       string // city / town, max 35 — mandatory
	CreditorCountry    string // ISO 3166-1 alpha-2, e.g. "CH" — mandatory

	// ── Amount ──────────────────────────────────────────────────────────────
	Amount   float64 // use 0 for open-amount invoice (field left blank)
	Currency string  // "CHF" or "EUR"

	// ── Debtor (optional — leave DebtorName empty for unknown debtor) ────────
	DebtorName       string
	DebtorStreet     string
	DebtorBuildingNr string
	DebtorPostalCode string
	DebtorTown       string
	DebtorCountry    string

	// ── Payment reference ────────────────────────────────────────────────────
	ReferenceType string // "QRR" (with QR-IBAN), "SCOR" (ISO 11649), or "NON"
	Reference     string // 27-digit QRR ref; ISO 11649 SCOR ref; empty for NON

	// ── Additional information ───────────────────────────────────────────────
	Message string // unstructured message, max 140 chars (field 30)

	// ── Invoice metadata — used for display only, NOT included in payload ────
	InvoiceDate   time.Time
	InvoiceNumber string
}

// GenerateQRBillPayload returns the LF-separated text payload encoded in the
// Swiss QR code (SPC 0200 v2.3, §4.2.2).
//
// Payload structure — 31 mandatory fields, LF-separated, NO trailing LF:
//
//	 1  SPC              — header, fixed
//	 2  0200             — version
//	 3  1                — coding type (UTF-8, Latin character set)
//	 4  IBAN             — creditor account
//	 5  S                — creditor address type (structured, v2.3)
//	 6  Name             — creditor name
//	 7  StrtNmOrAdrLine1 — street (building nr may be included)
//	 8  BldgNbOrAdrLine2 — building number (optional)
//	 9  PstCd            — postal code
//	10  TwnNm            — town
//	11  Ctry             — country (ISO alpha-2)
//	12–18               — ultimate creditor (7 × empty, reserved)
//	19  Amt              — amount or empty for open amount
//	20  Ccy              — currency (CHF/EUR)
//	21–27               — debtor (S + 6 fields, or 7 × empty if unknown)
//	28  Tp               — reference type (QRR/SCOR/NON)
//	29  Ref              — reference value or empty
//	30  Ustrd            — unstructured message
//	31  EPD              — end payment data, mandatory trailer
func GenerateQRBillPayload(d QRBillData) (string, error) {
	if err := validateQRBillData(d); err != nil {
		return "", fmt.Errorf("qr-bill: %w", err)
	}

	// Prefix-LF emitter: prepend '\n' before every field except the first.
	// Guarantees no trailing newline after EPD — required by §4.1.4.
	var b strings.Builder
	first := true
	field := func(s string) {
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(s)
		first = false
	}

	// ── 1–3: Header ───────────────────────────────────────────────────────────
	field("SPC")  // Swiss Payments Code
	field("0200") // version
	field("1")    // coding: 1 = UTF-8, Latin character set (§4.1.1)

	// ── 4–11: Creditor — address type S (structured) ─────────────────────────
	// Strip all whitespace and uppercase for canonical IBAN form.
	cleanIBAN := strings.ToUpper(ibanClean.ReplaceAllString(d.CreditorIBAN, ""))
	field(cleanIBAN)
	field("S") // structured address — only valid type in SPC 0200 v2.3
	field(d.CreditorName)
	field(d.CreditorStreet)     // StrtNmOrAdrLine1 (building nr may be included)
	field(d.CreditorBuildingNr) // BldgNbOrAdrLine2 (may be empty)
	field(d.CreditorPostalCode) // PstCd — mandatory
	field(d.CreditorTown)       // TwnNm — mandatory
	field(d.CreditorCountry)    // Ctry — mandatory

	// ── 12–18: Ultimate creditor — reserved, ALL fields must be empty ─────────
	for i := 0; i < 7; i++ {
		field("")
	}

	// ── 19–20: Amount and currency ────────────────────────────────────────────
	if d.Amount > 0 {
		// Exactly 2 decimal places, no leading zeros, period as decimal separator.
		field(fmt.Sprintf("%.2f", math.Round(d.Amount*100)/100))
	} else {
		field("") // open-amount invoice
	}
	field(d.Currency)

	// ── 21–27: Debtor — address type S or 7 × empty if unknown ───────────────
	if d.DebtorName != "" {
		field("S")
		field(d.DebtorName)
		field(d.DebtorStreet)
		field(d.DebtorBuildingNr)
		field(d.DebtorPostalCode)
		field(d.DebtorTown)
		field(d.DebtorCountry)
	} else {
		// Unknown debtor — 7 blank fields (§4.2.2: separators still required)
		for i := 0; i < 7; i++ {
			field("")
		}
	}

	// ── 28–29: Payment reference ──────────────────────────────────────────────
	field(d.ReferenceType)
	field(d.Reference) // empty string for NON

	// ── 30–31: Additional information ────────────────────────────────────────
	field(truncate(d.Message, 140))
	field("EPD") // End Payment Data — mandatory trailer, last field, no trailing LF

	return b.String(), nil
}

// ─── QRR Reference (Modulo 10 recursive) ─────────────────────────────────────

// modTable is the Modulo 10 recursive carry table (SIX-Group QRR specification,
// Annexe B).
var modTable = [10]int{0, 9, 4, 6, 8, 2, 7, 1, 3, 5}

// ComputeQRRCheckDigit computes the single Modulo-10-recursive check digit for
// a numeric string (e.g. 26-digit partial reference → 27-digit QRR reference).
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
// invoice ID (up to 26 digits). Strips non-digits, pads to 26, appends the
// Modulo-10-recursive check digit.
func GenerateQRRReference(invoiceID string) (string, error) {
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
	padded := fmt.Sprintf("%026s", d)
	padded = strings.ReplaceAll(padded, " ", "0")

	check, err := ComputeQRRCheckDigit(padded)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", padded, check), nil
}

// FormatQRRReference formats a 27-digit QRR reference for display in the
// printed payment slip: groups of 2+5+5+5+5+5 separated by spaces.
// Example: "210000000031394714300090172" → "21 00000 00031 39471 43000 90172"
func FormatQRRReference(ref27 string) string {
	if len(ref27) != 27 {
		return ref27
	}
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
	// Creditor IBAN
	if d.CreditorIBAN == "" {
		return fmt.Errorf("creditor IBAN is required")
	}
	if err := ValidateIBAN(d.CreditorIBAN); err != nil {
		return fmt.Errorf("creditor IBAN: %w", err)
	}

	// Creditor name
	if d.CreditorName == "" {
		return fmt.Errorf("creditor name is required")
	}
	if len([]rune(d.CreditorName)) > 70 {
		return fmt.Errorf("creditor name exceeds 70 chars")
	}

	// Creditor structured address — postal code, town and country are mandatory (§4.2.2)
	if d.CreditorPostalCode == "" {
		return fmt.Errorf("creditor postal code is required (address type S)")
	}
	if d.CreditorTown == "" {
		return fmt.Errorf("creditor town is required (address type S)")
	}
	if len(d.CreditorCountry) != 2 {
		return fmt.Errorf("creditor country (ISO 3166-1 alpha-2) is required")
	}

	// Currency
	if d.Currency != "CHF" && d.Currency != "EUR" {
		return fmt.Errorf("currency must be CHF or EUR, got %q", d.Currency)
	}

	// Debtor — when identified, postal code, town and country are all mandatory
	if d.DebtorName != "" {
		if d.DebtorPostalCode == "" {
			return fmt.Errorf("debtor postal code is required when debtor is identified")
		}
		if d.DebtorTown == "" {
			return fmt.Errorf("debtor town is required when debtor is identified")
		}
		if len(d.DebtorCountry) != 2 {
			return fmt.Errorf("debtor country (ISO 3166-1 alpha-2) is required when debtor is identified")
		}
	}

	// Reference type
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

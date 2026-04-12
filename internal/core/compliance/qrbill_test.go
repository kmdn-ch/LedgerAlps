package compliance

import (
	"strings"
	"testing"
	"time"
)

// ─── QRR reference tests ──────────────────────────────────────────────────────

func TestComputeQRRCheckDigit(t *testing.T) {
	tests := []struct {
		name   string
		digits string
		want   int
	}{
		{"all zeros", "00000000000000000000000000", 0},
		{"invoice seq 1", "00000000000000000000000001", 1},
		{"invoice seq 2", "00000000000000000000000002", 6},
		{"invoice seq 100", "00000000000000000000000100", 8},
		{"example from spec", "21000000003139471430009017", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeQRRCheckDigit(tc.digits)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ComputeQRRCheckDigit(%q) = %d, want %d", tc.digits, got, tc.want)
			}
		})
	}
}

func TestComputeQRRCheckDigit_InvalidInput(t *testing.T) {
	_, err := ComputeQRRCheckDigit("1234abc")
	if err == nil {
		t.Error("expected error for non-numeric input")
	}
}

func TestGenerateQRRReference(t *testing.T) {
	tests := []struct {
		name      string
		invoiceID string
		wantLen   int
		wantErr   bool
	}{
		{"short number", "1", 27, false},
		{"numeric string", "12345", 27, false},
		{"with dashes stripped", "INV-2024-001", 27, false},
		{"26 digits", "12345678901234567890123456", 27, false},
		{"too long", "123456789012345678901234567", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := GenerateQRRReference(tc.invoiceID)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ref) != tc.wantLen {
				t.Errorf("length = %d, want %d; ref = %q", len(ref), tc.wantLen, ref)
			}
			check, _ := ComputeQRRCheckDigit(ref[:26])
			got := int(ref[26] - '0')
			if check != got {
				t.Errorf("check digit invalid: expected %d, got %d", check, got)
			}
		})
	}
}

func TestFormatQRRReference(t *testing.T) {
	ref := "210000000031394714300090172"
	formatted := FormatQRRReference(ref)
	parts := strings.Fields(formatted)
	if len(parts) != 6 {
		t.Errorf("expected 6 groups, got %d: %q", len(parts), formatted)
	}
	joined := strings.Join(parts, "")
	if joined != ref {
		t.Errorf("reassembled %q ≠ original %q", joined, ref)
	}
}

// ─── QR-bill payload tests ────────────────────────────────────────────────────

// validQRBillData returns a fully-populated QRBillData using structured address
// type S — the only valid type in SPC 0200 v2.3.
func validQRBillData() QRBillData {
	ref, _ := GenerateQRRReference("1")
	return QRBillData{
		CreditorIBAN:       "CH4431999123000889012",
		CreditorName:       "LedgerAlps AG",
		CreditorStreet:     "Bahnhofstrasse 1",
		CreditorPostalCode: "8001",
		CreditorTown:       "Zürich",
		CreditorCountry:    "CH",
		Amount:             100.00,
		Currency:           "CHF",
		ReferenceType:      "QRR",
		Reference:          ref,
	}
}

func TestGenerateQRBillPayload_Valid(t *testing.T) {
	d := validQRBillData()
	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(payload, "\n")

	// SPC 0200 §4.2.2 — check mandatory fixed header fields
	if lines[0] != "SPC" {
		t.Errorf("line 0: want SPC, got %q", lines[0])
	}
	if lines[1] != "0200" {
		t.Errorf("line 1: want 0200, got %q", lines[1])
	}
	if lines[2] != "1" {
		t.Errorf("line 2: want 1 (UTF-8), got %q", lines[2])
	}
	// IBAN at line 3
	if lines[3] != "CH4431999123000889012" {
		t.Errorf("line 3 (IBAN): want CH4431999123000889012, got %q", lines[3])
	}
	// Address type must be S (structured) — K was removed in v2.3
	if lines[4] != "S" {
		t.Errorf("line 4 (address type): want S (structured), got %q — combined type K is invalid in SPC 0200 v2.3", lines[4])
	}
	// Creditor name at line 5
	if lines[5] != "LedgerAlps AG" {
		t.Errorf("line 5 (creditor name): want LedgerAlps AG, got %q", lines[5])
	}
	// Postal code at line 8 (field 9: 0-indexed = line 8)
	if lines[8] != "8001" {
		t.Errorf("line 8 (postal code): want 8001, got %q", lines[8])
	}
	// Town at line 9
	if lines[9] != "Zürich" {
		t.Errorf("line 9 (town): want Zürich, got %q", lines[9])
	}
	// EPD must be the last line
	if lines[len(lines)-1] != "EPD" {
		t.Errorf("last field must be EPD, got %q", lines[len(lines)-1])
	}
	// SPC 0200 §4.1.4: no trailing LF after last field
	if strings.HasSuffix(payload, "\n") {
		t.Error("payload must not end with a trailing LF (§4.1.4)")
	}
}

func TestGenerateQRBillPayload_FieldCount(t *testing.T) {
	d := validQRBillData()
	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(payload, "\n")
	// 31 mandatory fields
	if len(lines) != 31 {
		t.Errorf("expected 31 fields, got %d\npayload:\n%s", len(lines), payload)
	}
}

func TestGenerateQRBillPayload_NON(t *testing.T) {
	d := validQRBillData()
	d.ReferenceType = "NON"
	d.Reference = ""
	d.CreditorIBAN = "CH9300762011623852957" // regular IBAN (not QR-IBAN)

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(payload, "\nNON\n") {
		t.Error("payload should contain NON reference type")
	}
}

func TestGenerateQRBillPayload_OpenAmount(t *testing.T) {
	d := validQRBillData()
	d.Amount = 0 // open amount

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(payload, "\n")
	// Amount is field 19 (0-indexed line 18):
	// 3 header + 8 creditor (S:4–11) + 7 UC = 18 lines before amount
	amountLine := lines[18]
	if amountLine != "" {
		t.Errorf("open amount should produce empty line, got %q", amountLine)
	}
}

func TestGenerateQRBillPayload_WithDebtor(t *testing.T) {
	d := validQRBillData()
	d.DebtorName = "Hans Muster"
	d.DebtorStreet = "Musterstrasse 1"
	d.DebtorPostalCode = "3000"
	d.DebtorTown = "Bern"
	d.DebtorCountry = "CH"

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(payload, "Hans Muster") {
		t.Error("debtor name should appear in payload")
	}
	// Debtor address type must also be S
	lines := strings.Split(payload, "\n")
	// Debtor starts at field 21 = line index 20
	if lines[20] != "S" {
		t.Errorf("debtor address type (line 20): want S, got %q", lines[20])
	}
}

func TestGenerateQRBillPayload_WithMessage(t *testing.T) {
	d := validQRBillData()
	d.InvoiceNumber = "INV-2024-001"
	d.InvoiceDate = time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	d.Message = "Facture mars 2024"

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Message must appear in payload
	if !strings.Contains(payload, "Facture mars 2024") {
		t.Error("message should appear in payload")
	}
	// Swico S1 must NOT be present (breaks strict validators with non-numeric invoice numbers)
	if strings.Contains(payload, "//S1/") {
		t.Error("Swico S1 bill info must not be present")
	}
	// No trailing LF (§4.1.4)
	if strings.HasSuffix(payload, "\n") {
		t.Error("payload must not end with a trailing LF (§4.1.4)")
	}
	// EPD is last field
	lines := strings.Split(payload, "\n")
	if lines[len(lines)-1] != "EPD" {
		t.Errorf("last field must be EPD, got %q", lines[len(lines)-1])
	}
}

func TestGenerateQRBillPayload_AddressTypeS(t *testing.T) {
	// Regression test: ensure combined address type K is never emitted.
	// K was removed in SPC 0200 v2.3 and causes scan failures.
	d := validQRBillData()
	d.DebtorName = "Test Debtor"
	d.DebtorStreet = "Teststrasse 42"
	d.DebtorPostalCode = "1200"
	d.DebtorTown = "Genève"
	d.DebtorCountry = "CH"

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(payload, "\nK\n") {
		t.Error("combined address type K must not appear in payload (removed in SPC 0200 v2.3)")
	}
}

func TestGenerateQRBillPayload_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*QRBillData)
		wantErr string
	}{
		{
			"missing IBAN",
			func(d *QRBillData) { d.CreditorIBAN = "" },
			"IBAN is required",
		},
		{
			"missing creditor name",
			func(d *QRBillData) { d.CreditorName = "" },
			"creditor name is required",
		},
		{
			"missing postal code",
			func(d *QRBillData) { d.CreditorPostalCode = "" },
			"creditor postal code is required",
		},
		{
			"missing town",
			func(d *QRBillData) { d.CreditorTown = "" },
			"creditor town is required",
		},
		{
			"missing country",
			func(d *QRBillData) { d.CreditorCountry = "" },
			"creditor country",
		},
		{
			"invalid currency",
			func(d *QRBillData) { d.Currency = "USD" },
			"currency must be CHF or EUR",
		},
		{
			"QRR ref wrong length",
			func(d *QRBillData) { d.Reference = "123" },
			"QRR reference must be 27 digits",
		},
		{
			"unknown reference type",
			func(d *QRBillData) { d.ReferenceType = "UNKNOWN" },
			"reference type must be QRR, SCOR, or NON",
		},
		{
			"debtor without postal code",
			func(d *QRBillData) {
				d.DebtorName = "Jean Dupont"
				d.DebtorTown = "Lausanne"
				d.DebtorCountry = "CH"
				// DebtorPostalCode intentionally left empty
			},
			"debtor postal code is required",
		},
		{
			"debtor without country",
			func(d *QRBillData) {
				d.DebtorName = "Jean Dupont"
				d.DebtorPostalCode = "1000"
				d.DebtorTown = "Lausanne"
				// DebtorCountry intentionally left empty
			},
			"debtor country",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validQRBillData()
			tc.mutate(&d)
			_, err := GenerateQRBillPayload(d)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

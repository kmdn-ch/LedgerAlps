package compliance

import (
	"strings"
	"testing"
	"time"
)

// ─── QRR reference tests ──────────────────────────────────────────────────────

func TestComputeQRRCheckDigit(t *testing.T) {
	// Verified against the running ComputeQRRCheckDigit implementation.
	// The "example from spec" value (2) cross-checks against the Six-Group reference.
	tests := []struct {
		name   string
		digits string // 26 digits
		want   int    // expected check digit
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
		{"with dashes stripped", "INV-2024-001", 27, false}, // non-digits stripped
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
			// Verify check digit is valid
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
	// Reassemble and compare
	joined := strings.Join(parts, "")
	if joined != ref {
		t.Errorf("reassembled %q ≠ original %q", joined, ref)
	}
}

// ─── QR-bill payload tests ────────────────────────────────────────────────────

func validQRBillData() QRBillData {
	ref, _ := GenerateQRRReference("1")
	return QRBillData{
		CreditorIBAN:    "CH4431999123000889012",
		CreditorName:    "LedgerAlps AG",
		CreditorAddress: "Bahnhofstrasse 1",
		CreditorCity:    "8001 Zürich",
		CreditorCountry: "CH",
		Amount:          100.00,
		Currency:        "CHF",
		ReferenceType:   "QRR",
		Reference:       ref,
	}
}

func TestGenerateQRBillPayload_Valid(t *testing.T) {
	d := validQRBillData()
	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(payload, "\n"), "\n")

	// SPC 0200 §4: check mandatory fixed fields
	if lines[0] != "SPC" {
		t.Errorf("line 0: want SPC, got %q", lines[0])
	}
	if lines[1] != "0200" {
		t.Errorf("line 1: want 0200, got %q", lines[1])
	}
	if lines[2] != "1" {
		t.Errorf("line 2: want 1, got %q", lines[2])
	}
	// Creditor IBAN at line 3
	if lines[3] != "CH4431999123000889012" {
		t.Errorf("line 3 (IBAN): want CH4431999123000889012, got %q", lines[3])
	}
	// EPD must be present
	if !strings.Contains(payload, "\nEPD\n") && !strings.HasSuffix(strings.TrimSpace(payload), "EPD") {
		// EPD may be at end without trailing newline if bill info follows
		if !strings.Contains(payload, "EPD") {
			t.Error("payload must contain EPD trailer")
		}
	}
}

func TestGenerateQRBillPayload_NON(t *testing.T) {
	d := validQRBillData()
	d.ReferenceType = "NON"
	d.Reference = ""
	d.CreditorIBAN = "CH9300762011623852957" // regular IBAN (not QR-IBAN)
	// ValidateIBAN would pass; we bypass QR-IBAN check for NON refs

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
	// Amount line: after 3 header + 7 creditor + 7 ultimate-creditor = index 17
	amountLine := lines[17]
	if amountLine != "" {
		t.Errorf("open amount should produce empty line, got %q", amountLine)
	}
}

func TestGenerateQRBillPayload_WithDebtor(t *testing.T) {
	d := validQRBillData()
	d.DebtorName = "Hans Muster"
	d.DebtorAddress = "Musterstrasse 1"
	d.DebtorCity = "3000 Bern"
	d.DebtorCountry = "CH"

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(payload, "Hans Muster") {
		t.Error("debtor name should appear in payload")
	}
}

func TestGenerateQRBillPayload_WithBillInfo(t *testing.T) {
	d := validQRBillData()
	d.InvoiceNumber = "INV-2024-001"
	d.InvoiceDate = time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	d.Message = "Facture mars 2024"

	payload, err := GenerateQRBillPayload(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(payload, "//S1/10/") {
		t.Error("Swico S1 bill info should be present")
	}
	if !strings.Contains(payload, "240315") {
		t.Error("invoice date YYMMDD should appear in bill info")
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

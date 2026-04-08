package compliance_test

import (
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
)

// ─── RoundTo5Rappen ──────────────────────────────────────────────────────────

func TestRoundTo5Rappen(t *testing.T) {
	tests := []struct {
		name   string
		input  float64
		want   float64
	}{
		{"10.123 → 10.10", 10.123, 10.10},
		{"10.125 → 10.15 (round-half-up, not banker's)", 10.125, 10.15},
		{"10.127 → 10.15", 10.127, 10.15},
		{"99.99 → 100.00", 99.99, 100.00},
		{"0.005 → 0.00 (below half-step)", 0.005, 0.00},
		{"0.025 → 0.05", 0.025, 0.05},
		// math.Floor(-0.025*20+0.5)/20 = math.Floor(-0.5+0.5)/20 = math.Floor(0)/20 = 0.00
		// The formula uses math.Floor (not round-half-away-from-zero), so negatives
		// at the half-step round toward zero rather than away from zero.
		{"-0.025 → 0.00 (floor semantics for negatives)", -0.025, 0.00},
		{"100.000 → 100.00", 100.000, 100.00},
		{"0.00 → 0.00 (zero)", 0.00, 0.00},
		{"0.05 → 0.05 (exact)", 0.05, 0.05},
		{"0.10 → 0.10 (exact)", 0.10, 0.10},
		{"1.234 → 1.25", 1.234, 1.25},
		{"1.224 → 1.20", 1.224, 1.20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := compliance.RoundTo5Rappen(tc.input)
			// Compare with a tolerance tight enough to distinguish 0.05 steps.
			if diff := got - tc.want; diff > 0.0001 || diff < -0.0001 {
				t.Errorf("RoundTo5Rappen(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ─── ValidateIBAN ────────────────────────────────────────────────────────────

func TestValidateIBAN(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid IBANs
		{"CH valid", "CH9300762011623852957", false},
		{"DE valid", "DE89370400440532013000", false},

		// Edge cases that should still be valid (normalisation)
		{"CH with spaces", "CH93 0076 2011 6238 5295 7", false},
		{"CH lowercase", "ch9300762011623852957", false},
		{"DE with spaces", "DE89 3704 0044 0532 0130 00", false},

		// Invalid: too short
		{"too short (4 chars)", "CH93", true},
		{"empty string", "", true},

		// Invalid: bad checksum
		{"CH bad checksum", "CH0000762011623852957", true},
		{"DE bad checksum", "DE00370400440532013000", true},

		// Invalid: illegal characters
		{"invalid character @", "CH93@0762011623852957", true},
		{"invalid character space in middle after clean", "CH93 007620!1623852957", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := compliance.ValidateIBAN(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateIBAN(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateIBAN(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

// ─── ValidateQRIBAN ──────────────────────────────────────────────────────────

func TestValidateQRIBAN(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid QR-IBAN: IID = 31999 (within 30000–31999)
		{"valid QR-IBAN CH4431999123000889012", "CH4431999123000889012", false},

		// Invalid: IID below range (e.g. 00762 < 30000)
		{"IID below range (CH9300762...)", "CH9300762011623852957", true},

		// Invalid: regular Swiss IBAN, IID not in QR range
		{"regular CH IBAN rejected as QR-IBAN", "CH5604835012345678009", true},

		// Invalid: non-CH IBAN
		{"DE IBAN rejected (no CH prefix)", "DE89370400440532013000", true},

		// Invalid: completely invalid IBAN (bad checksum)
		{"invalid checksum", "CH0031999123000889012", true},

		// Invalid: IID at boundary just above range — 32000
		// Construct a plausible CH IBAN with IID=32000 (checksum will fail, error is still returned)
		{"IID just above range 32000", "CH0032000000000000000", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := compliance.ValidateQRIBAN(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateQRIBAN(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateQRIBAN(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

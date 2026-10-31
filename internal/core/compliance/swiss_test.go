package compliance_test

import (
	"math"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
)

// ─── RoundTo5Rappen ──────────────────────────────────────────────────────────

func TestRoundTo5Rappen(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{"10.123 → 10.10", 10.123, 10.10},
		{"10.125 → 10.15 (demi arrondi à l'écart de zéro, pas au pair)", 10.125, 10.15},
		{"-10.125 → -10.15 (symétrique de 10.125)", -10.125, -10.15},
		{"-10.123 → -10.10", -10.123, -10.10},
		{"-99.99 → -100.00", -99.99, -100.00},
		{"10.127 → 10.15", 10.127, 10.15},
		{"99.99 → 100.00", 99.99, 100.00},
		{"0.005 → 0.00 (below half-step)", 0.005, 0.00},
		{"0.025 → 0.05", 0.025, 0.05},
		// Symétrique de 0.025 → 0.05 : un demi négatif s'arrondit à l'écart de zéro.
		{"-0.025 → -0.05 (symétrique de 0.025)", -0.025, -0.05},
		{"-0.01 → 0.00 (sous le demi-pas)", -0.01, 0.00},
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

// Une facture et sa note de crédit doivent se compenser au centime : arrondir
// x et -x donne deux montants opposés.
func TestRoundTo5RappenEstSymetrique(t *testing.T) {
	for _, x := range []float64{0.025, 0.074, 1.125, 10.125, 10.127, 99.975, 1234.5678} {
		if a, b := compliance.RoundTo5Rappen(x), compliance.RoundTo5Rappen(-x); a != -b {
			t.Errorf("RoundTo5Rappen(%v) = %v mais RoundTo5Rappen(%v) = %v : pas opposés", x, a, -x, b)
		}
	}
}

// ─── RoundCents ──────────────────────────────────────────────────────────────

func TestRoundCents(t *testing.T) {
	cases := []struct {
		name        string
		input, want float64
	}{
		{"déjà au centime", 12.34, 12.34},
		{"demi-centime monte", 1.005, 1.01}, // 1.005*100 = 100.49999999999999 en flottant
		{"piège classique 2.675", 2.675, 2.68},
		{"demi négatif à l'écart de zéro", -1.005, -1.01},
		{"en dessous du demi", 1.004, 1.00},
		{"petit négatif ne donne pas -0", -0.004, 0},
		{"zéro", 0, 0},
		{"grand montant", 123456789.126, 123456789.13},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compliance.RoundCents(tc.input)
			if math.Abs(got-tc.want) > 1e-9 || (got == 0 && math.Signbit(got)) {
				t.Errorf("RoundCents(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// Un montant déjà au centime ne bouge pas : l'opération est idempotente, sinon un
// total relu puis rearrondi dériverait.
func TestRoundCentsEstIdempotent(t *testing.T) {
	for k := -200000; k <= 200000; k++ {
		v := float64(k) / 100
		if got := compliance.RoundCents(v); got != v {
			t.Fatalf("RoundCents(%v) = %v", v, got)
		}
	}
}

func TestRoundCentsLaisseLesValeursNonFiniesEtGigantesques(t *testing.T) {
	if !math.IsNaN(compliance.RoundCents(math.NaN())) {
		t.Error("NaN doit rester NaN")
	}
	if !math.IsInf(compliance.RoundCents(math.Inf(1)), 1) {
		t.Error("+Inf doit rester +Inf")
	}
	// Au-delà de ±9·10¹² les millionièmes entiers débordent : ne pas planter.
	_ = compliance.RoundCents(1e300)
}

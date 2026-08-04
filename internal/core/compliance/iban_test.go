package compliance

import "testing"

// Le contrôle MOD-97 seul laissait passer deux familles d'erreurs qu'une banque
// rejette : la mauvaise longueur pour le pays, et une structure invalide.
// Découvrir le problème à la remise du fichier de virements, c'est le découvrir
// après avoir cru les paiements partis.

func TestValidateIBANAcceptsRealAccounts(t *testing.T) {
	valid := []string{
		"CH9300762011623852957",       // exemple officiel SIX
		"CH 93 0076 2011 6238 5295 7", // le même, saisi avec des espaces
		"ch9300762011623852957",       // et en minuscules
		"LI21088100002324013AA",       // Liechtenstein, avec des lettres dans le BBAN
		"DE89370400440532013000",
		"FR1420041010050500013M02606",
		"GB29NWBK60161331926819",
		"NO9386011117947",                 // le plus court du registre : 15
		"MT84MALT011000012345MTLCAST001S", // 31
	}
	for _, iban := range valid {
		if err := ValidateIBAN(iban); err != nil {
			t.Errorf("ValidateIBAN(%q) refuse un IBAN valide : %v", iban, err)
		}
	}
}

// La longueur imposée par le pays : c'est le contrôle qui manquait.
func TestValidateIBANRejectsWrongLengthForCountry(t *testing.T) {
	cases := []struct{ iban, why string }{
		{"CH930076201162385295", "un CH tronqué d'un caractère"},
		{"CH93007620116238529571", "un CH avec un caractère de trop"},
		{"DE8937040044053201300", "un DE trop court"},
	}
	for _, tc := range cases {
		if err := ValidateIBAN(tc.iban); err == nil {
			t.Errorf("%s est accepté : la banque le rejetterait à la remise du fichier", tc.why)
		}
	}
}

func TestValidateIBANRejectsBadStructure(t *testing.T) {
	cases := []struct{ iban, why string }{
		{"1293007620116238529", "code pays numérique"},
		{"CHAB0076201162385295", "clé de contrôle non numérique"},
		{"CH93-0076-2011-6238-5", "caractères de ponctuation"},
		{"CH93007620116238529*7", "caractère spécial"},
		{"", "chaîne vide"},
		{"CH93", "beaucoup trop court"},
	}
	for _, tc := range cases {
		if err := ValidateIBAN(tc.iban); err == nil {
			t.Errorf("%s (%q) est accepté", tc.why, tc.iban)
		}
	}
}

func TestValidateIBANRejectsBadChecksum(t *testing.T) {
	// Deux chiffres intervertis dans un IBAN par ailleurs bien formé : la faute
	// de frappe la plus fréquente, et celle que MOD-97 existe pour attraper.
	if err := ValidateIBAN("CH9300762011623852975"); err == nil {
		t.Error("un IBAN dont deux chiffres sont intervertis est accepté")
	}
}

// Un code pays absent du registre embarqué ne doit pas être rejeté : le
// registre IBAN évolue après la compilation du binaire, et empêcher quelqu'un
// de facturer coûte plus que d'accepter un IBAN qu'on ne peut pas entièrement
// vérifier.
func TestValidateIBANToleratesUnknownCountryCodes(t *testing.T) {
	// « ZZ » n'existe pas au registre. Construit pour satisfaire MOD-97.
	iban := makeValidIBANFor(t, "ZZ", "0076201162385295")
	if err := ValidateIBAN(iban); err != nil {
		t.Errorf("ValidateIBAN(%q) rejette un pays inconnu du registre : %v", iban, err)
	}
}

// makeValidIBANFor calcule la clé de contrôle correcte pour un pays et un BBAN
// donnés, afin de tester la tolérance sans coder un IBAN en dur.
func makeValidIBANFor(t *testing.T, country, bban string) string {
	t.Helper()
	for k := 2; k <= 98; k++ {
		candidate := country + itoa2(k) + bban
		if err := ValidateIBAN(candidate); err == nil {
			return candidate
		}
	}
	t.Fatalf("aucune clé de contrôle valide trouvée pour %s%s", country, bban)
	return ""
}

func itoa2(n int) string {
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// ─── Présentation ────────────────────────────────────────────────────────────

func TestFormatIBANGroupsByFour(t *testing.T) {
	got := FormatIBAN("CH9300762011623852957")
	want := "CH93 0076 2011 6238 5295 7"
	if got != want {
		t.Errorf("FormatIBAN = %q, attendu %q", got, want)
	}
}

func TestNormaliseIBANStripsSpacesAndUppercases(t *testing.T) {
	if got := NormaliseIBAN(" ch93 0076 2011 6238 5295 7 "); got != "CH9300762011623852957" {
		t.Errorf("NormaliseIBAN = %q", got)
	}
}

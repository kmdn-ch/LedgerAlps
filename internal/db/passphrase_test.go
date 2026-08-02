package db

import "testing"

func TestValidatePassphrase(t *testing.T) {
	cases := []struct {
		name, pass string
		ok         bool
	}{
		// The example the interface shows must itself pass.
		{"exemple affiché", "34CryPt3DB4ckup5@26", true},
		{"16 exactement", "Abcdefghijklmno1", true},
		{"accents comptés comme lettres", "Comptabilité2026Suisse", true},

		{"trop courte", "Abcdefghijklmn1", false}, // 15
		{"longue mais sans chiffre", "MotDePasseTresLongMaisSansChiffre", false},
		{"longue mais sans majuscule", "motdepasse2026longue", false},
		{"longue mais sans minuscule", "MOTDEPASSE2026LONGUE", false},
		{"vide", "", false},
	}
	for _, c := range cases {
		err := ValidatePassphrase(c.pass)
		if c.ok && err != nil {
			t.Errorf("%s: refusée alors qu'elle devrait passer — %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: acceptée alors qu'elle est trop faible", c.name)
		}
	}
}

// The message has to name what is missing; "trop faible" tells the user
// nothing they can act on.
func TestValidatePassphraseSaysWhatIsMissing(t *testing.T) {
	err := ValidatePassphrase("motdepassesansrien!!")
	if err == nil {
		t.Fatal("attendu un refus")
	}
	for _, want := range []string{"majuscule", "chiffre"} {
		if !contains(err.Error(), want) {
			t.Errorf("message %q ne mentionne pas %q", err.Error(), want)
		}
	}
}

// Counting bytes instead of runes would let "éééééééé" (8 runes, 16 bytes)
// through as if it were long enough.
func TestLengthCountsCharactersNotBytes(t *testing.T) {
	if err := ValidatePassphrase("Éé1ÉéÉéÉé"); err == nil {
		t.Error("une phrase de 9 caractères a passé le seuil de 16")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

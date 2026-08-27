package semver

import "testing"

// Les deux copies d'origine avaient divergé sur ces deux cas précis. Les
// figer ici est tout l'intérêt d'avoir fusionné.

func TestParseRefuseAuDelaDeTroisSegments(t *testing.T) {
	// L'une des deux copies tronquait silencieusement « 1.2.3.4 » en « 1.2.3 » :
	// une version inventée qui a l'air valide, sur un chemin qui décide si un
	// avis de sécurité s'applique.
	if _, ok := Parse("1.2.3.4"); ok {
		t.Error("« 1.2.3.4 » accepté : la troncature silencieuse est de retour")
	}
}

func TestParseRefuseUnSegmentNegatif(t *testing.T) {
	if _, ok := Parse("1.-2.3"); ok {
		t.Error("un segment négatif a été accepté")
	}
}

func TestParseLitLesFormesNormales(t *testing.T) {
	cas := map[string][3]int{
		"1.5.9":     {1, 5, 9},
		"v1.5.9":    {1, 5, 9},
		" v1.5.9 ":  {1, 5, 9},
		"1.4.0-rc1": {1, 4, 0},
		"1.4.0+abc": {1, 4, 0},
		"2.0":       {2, 0, 0},
		"3":         {3, 0, 0},
	}
	for in, want := range cas {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %v,%v — attendu %v,true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "  ", "v", "dev", "1.x.3", "abc"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) accepté à tort", in)
		}
	}
}

// La dissymétrie des deux cas d'illisibilité est le cœur du comportement :
// elle décide si un utilisateur est averti ou non.
func TestAuMoinsAvertitQuandLaCibleEstIllisible(t *testing.T) {
	if AuMoins("1.5.9", "pas-une-version") {
		t.Error("une cible illisible a été traitée comme « déjà corrigé » : " +
			"l'avis de conformité serait tu au lieu d'être affiché")
	}
}

func TestAuMoinsNeHarcelePasUnBinaireDeDeveloppement(t *testing.T) {
	if !AuMoins("dev", "1.5.9") {
		t.Error("un binaire « dev » est averti pour des avis déjà corrigés dans son arbre")
	}
}

func TestAuMoinsCompareCorrectement(t *testing.T) {
	cas := []struct {
		cur, tgt string
		want     bool
	}{
		{"1.5.9", "1.5.9", true},
		{"1.5.10", "1.5.9", true},
		{"1.6.0", "1.5.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.5.8", "1.5.9", false},
		{"1.4.9", "1.5.0", false},
		{"0.9.9", "1.0.0", false},
	}
	for _, c := range cas {
		if got := AuMoins(c.cur, c.tgt); got != c.want {
			t.Errorf("AuMoins(%q, %q) = %v, attendu %v", c.cur, c.tgt, got, c.want)
		}
	}
}

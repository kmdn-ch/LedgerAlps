package handlers

import (
	"testing"
)

// Le mot de passe créé par un administrateur pour quelqu'un d'autre a voyagé
// par un canal qui n'est pas fait pour ça, et il est connu de deux personnes.
// Tant qu'il vaut, une action tracée sous ce compte ne prouve pas qui l'a
// faite : le journal d'audit devient trompeur, et non simplement incomplet.

func TestUnMotDePasseTropFaibleEstRefuseEtDitCeQuiManque(t *testing.T) {
	cases := []struct {
		password string
		veut     string
	}{
		{"court", "12 caractères au minimum"},
		{"minusculesseulement", "une majuscule"},
		{"MAJUSCULESSEULEMENT1", "une minuscule"},
		{"SansAucunChiffreIci", "un chiffre"},
		{"", "12 caractères au minimum"},
	}
	for _, c := range cases {
		err := ValidateUserPassword(c.password)
		if err == nil {
			t.Errorf("%q accepté", c.password)
			continue
		}
		// Le message doit nommer ce qui manque : « trop faible » oblige à
		// deviner, et la plupart des gens devinent en ajoutant un chiffre.
		if !contains(err.Error(), c.veut) {
			t.Errorf("pour %q, le message ne dit pas %q : %v", c.password, c.veut, err)
		}
	}
}

func TestUnMotDePasseSolideEstAccepte(t *testing.T) {
	for _, p := range []string{
		"Boulangerie2026",
		"MotDePasseSolide1",
		"Cervin4478metres",
	} {
		if err := ValidateUserPassword(p); err != nil {
			t.Errorf("%q refusé: %v", p, err)
		}
	}
}

// Le seuil est délibérément plus haut que les huit caractères acceptés à
// l'installation : là, l'utilisateur choisit pour lui-même ; ici, le logiciel
// impose parce que le mot de passe remplacé a circulé.
func TestLeSeuilEstPlusHautQueCeluiDeLInstallation(t *testing.T) {
	if MinUserPasswordLength <= 8 {
		t.Fatalf("plancher = %d : pas plus exigeant que l'installation", MinUserPasswordLength)
	}
	if err := ValidateUserPassword("Abcdefg1"); err == nil {
		t.Fatal("huit caractères acceptés")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}

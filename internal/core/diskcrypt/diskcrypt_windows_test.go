//go:build windows

package diskcrypt

import "testing"

// windowsEdition n'existe que dans le fichier a balise windows. Un
// t.Skip() sur runtime.GOOS ne suffit pas : le compilateur a besoin du
// symbole meme sur une plateforme ou le test ne tournera jamais.

// Sur Windows, la fonctionnalité nommée doit correspondre à l'édition : envoyer
// un utilisateur de Famille vers le panneau BitLocker le fait chercher une
// entrée de menu que son édition n'a pas.
func TestWindowsNommeLaBonneFonctionnalite(t *testing.T) {
	r := Check()
	_, home := windowsEdition()
	want := "BitLocker"
	if home {
		want = "Chiffrement de l'appareil"
	}
	if r.Feature != want {
		t.Fatalf("fonctionnalité = %q, attendu %q pour cette édition (%s)", r.Feature, want, r.Edition)
	}
	t.Logf("édition détectée : %q → %s", r.Edition, r.Feature)
}

package pdf

// L'accent qui disparaît.
//
// Les factures portaient « B?n?ficiaire » au lieu de « Bénéficiaire ». La cause
// n'était pas une police manquante mais un encodage appliqué DEUX FOIS : le
// premier passage transforme « é » (U+00E9) en octet 0xE9, ce qui n'est plus de
// l'UTF-8 valide ; le second lit cet octet comme un caractère de remplacement,
// hors de la plage Latin-1, et écrit « ? ».
//
// Le défaut ne se voyait que sur le papier, et seulement sur les mots accentués.

import (
	"os"
	"strings"
	"testing"
)

func TestLEncodageNEstPasIdempotent(t *testing.T) {
	une := latin1("Bénéficiaire")
	deux := latin1(une)
	if !strings.Contains(deux, "?") {
		t.Skip("latin1 est devenue idempotente : ce test n'a plus d'objet")
	}
	// C'est bien le double appel qui casse, pas la fonction elle-même.
	if strings.Contains(une, "?") {
		t.Fatal("un seul passage abîme déjà le texte")
	}
}

// Le vrai contrôle : le code source ne doit plus encoder deux fois. Les
// fonctions d'affichage encodent déjà leurs arguments.
func TestAucunAppelNEncodeDeuxFois(t *testing.T) {
	src, err := os.ReadFile("service.go")
	if err != nil {
		t.Skipf("source illisible: %v", err)
	}
	for _, fautif := range []string{
		"metaRow(latin1(", "totalRow(latin1(", "line(latin1(", "cell(latin1(",
	} {
		if strings.Contains(string(src), fautif) {
			t.Errorf("%s… : le texte est encodé une seconde fois par la fonction appelée, "+
				"ce qui remplace chaque accent par « ? »", fautif)
		}
	}
}

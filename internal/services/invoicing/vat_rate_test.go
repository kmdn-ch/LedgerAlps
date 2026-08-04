package invoicing

import (
	"testing"
)

// Le taux porté par l'en-tête de facture valait 8.1 par défaut et n'était
// remplacé que si la première ligne portait un taux STRICTEMENT positif, ce qui
// confondait « 0 % » et « non renseigné ».
//
// Ce n'est pas un défaut d'affichage. La déclaration TVA agrège les factures en
// groupant par ce taux : le chiffre d'affaires d'une entreprise non assujettie
// remontait comme taxable à 8.1 % avec un impôt nul — une ligne qui ne se
// réconcilie pas. Et la LTVA art. 27 al. 1 interdit à un non-assujetti de faire
// figurer l'impôt sur ses factures, l'al. 2 le rendant redevable de ce qu'il a
// mentionné.

func TestHeaderVATRateFollowsTheLines(t *testing.T) {
	cases := []struct {
		name  string
		lines []LineInput
		want  float64
	}{
		{"taux standard", []LineInput{{VATRate: 8.1}}, 8.1},
		{"taux réduit", []LineInput{{VATRate: 2.6}}, 2.6},
		{"taux spécial hébergement", []LineInput{{VATRate: 3.8}}, 3.8},
		{"non assujetti — 0 %", []LineInput{{VATRate: 0}}, 0},
		{"aucune ligne", nil, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := headerVATRate(tc.lines)
			if got != tc.want {
				t.Fatalf("taux d'en-tête = %v, attendu %v — la déclaration TVA groupe sur cette valeur", got, tc.want)
			}
		})
	}
}

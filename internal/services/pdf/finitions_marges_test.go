package pdf

// La marge droite d'une facture ou d'un devis.
//
// Retour sur la 1.6.6-rc1 : le tableau des lignes allait jusqu'au BORD de la feuille. Les colonnes
// valaient 195 mm à elles toutes, de la marge gauche (15 mm) à 210 mm, alors que les totaux, le bloc
// client, le titre et les notes s'arrêtent à 195 mm. Seules les factures AVEC rabais tombaient juste.

import (
	"math"
	"regexp"
	"strconv"
	"testing"
	"time"
)

const (
	margeDroiteMM  = 15.0
	largeurPageMM  = 210.0
	bordDroitMaxMM = largeurPageMM - margeDroiteMM // 195
)

// rectanglesTracés rend (x, largeur) en mm de chaque rectangle du flux de la page — les cases du
// tableau, avec leurs bordures et leurs fonds.
func rectanglesTracés(t *testing.T, contenu []byte) (xs, largeurs []float64) {
	t.Helper()
	re := regexp.MustCompile(`(-?[\d.]+) (-?[\d.]+) (-?[\d.]+) (-?[\d.]+) re`)
	for _, m := range re.FindAllSubmatch(contenu, -1) {
		x, _ := strconv.ParseFloat(string(m[1]), 64)
		l, _ := strconv.ParseFloat(string(m[3]), 64)
		xs = append(xs, x*mmParPoint)
		largeurs = append(largeurs, l*mmParPoint)
	}
	return xs, largeurs
}

func documentAvecColonnes(typeDoc string, tva, rabais bool, lignes int) InvoiceData {
	d := InvoiceData{
		InvoiceNumber: "FA-2026-0001", DocumentType: typeDoc,
		IssueDate: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF", TotalAmount: 100,
		Company:  CompanyInfo{Name: "Alpes SA", Address: "Rue 1", City: "1700 Fribourg"},
		Customer: CustomerInfo{Name: "Client SA"},
	}
	for i := 0; i < lignes; i++ {
		l := InvoiceLine{Description: "Prestation numéro " + strconv.Itoa(i+1), Quantity: 1, UnitPrice: 100, LineTotal: 100}
		if tva {
			l.VATRate = 8.1
		}
		if rabais {
			l.DiscountPct = 10
		}
		d.Lines = append(d.Lines, l)
	}
	if tva {
		d.VATAmount, d.VATRate = 8.1, 8.1
	}
	return d
}

// Aucune case du tableau ne dépasse 195 mm, et le tableau va bien JUSQU'À 195 mm — pas en deçà : une
// colonne perdue ailleurs se verrait ici aussi.
func TestLeTableauDesLignesRespecteLaMargeDroite(t *testing.T) {
	for _, typeDoc := range []string{"invoice", "quote", "credit_note"} {
		for _, tva := range []bool{true, false} {
			for _, rabais := range []bool{true, false} {
				nom := typeDoc
				if tva {
					nom += "/tva"
				}
				if rabais {
					nom += "/rabais"
				}
				t.Run(nom, func(t *testing.T) {
					// 60 lignes : la table passe sur plusieurs pages, l'en-tête est redessiné.
					sortie, err := Generate(documentAvecColonnes(typeDoc, tva, rabais, 60))
					if err != nil {
						t.Fatalf("génération: %v", err)
					}
					xs, largeurs := rectanglesTracés(t, fluxDécompressés(sortie))
					if len(xs) < 40 {
						t.Fatalf("%d cases lues seulement : le test ne voit pas le tableau", len(xs))
					}
					maxi := 0.0
					for i := range xs {
						if bord := xs[i] + largeurs[i]; bord > maxi {
							maxi = bord
						}
					}
					if maxi > bordDroitMaxMM+0.05 {
						t.Errorf("le tableau va jusqu'à %.2f mm : il dépasse la marge droite de %.0f mm (limite %.0f mm)", maxi, margeDroiteMM, bordDroitMaxMM)
					}
					if math.Abs(maxi-bordDroitMaxMM) > 0.05 {
						t.Errorf("le tableau s'arrête à %.2f mm, attendu %.0f mm : il doit aller jusqu'à la marge, comme les totaux", maxi, bordDroitMaxMM)
					}
				})
			}
		}
	}
}

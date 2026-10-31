package compliance

// Bornes des dates et des montants comptables.
//
// # Pourquoi ces bornes existent (audit 9, A-03 et A-05)
//
// Rien ne bornait une date ni un montant : l'API acceptait `0026-10-30` (un champ
// date HTML laisse taper « 26 » dans le segment de l'année), `9999-12-31`, un
// prix de 1e300 ou une quantité négative. Le journal étant IMMUABLE (CO
// art. 957a), une valeur absurde une fois comptabilisée ne s'efface plus :
//
//   - l'année 26 créait un exercice « 26 » dont les bornes `26-01-01` ne se
//     relisaient plus, et l'écran des exercices comme l'archive légale (CO
//     art. 958f) répondaient 500 — sans que l'application puisse le réparer ;
//   - un montant de 1e300 comptabilisé ruinait la précision de tous les totaux.
//
// Le contrôle vit dans les SERVICES et non seulement dans les routes : un second
// point d'entrée (import, reprise, appel interne) hérite ainsi des mêmes règles.

import (
	"fmt"
	"math"
	"time"
	"unicode/utf8"
)

const (
	// MinBookingYear : aucune pièce de ce produit ne porte une date antérieure.
	MinBookingYear = 2000
	// MaxBookingYearsAhead : une échéance ou une écriture de régularisation peut
	// viser l'avenir, pas le siècle suivant.
	MaxBookingYearsAhead = 5

	// MaxAmount : un milliard de francs. Au-delà, les flottants perdent le centime
	// (2^53 centimes ≈ 9·10¹³ francs) bien avant, mais aucune PME suisse ne
	// facture cela : un tel montant est une faute de frappe.
	MaxAmount = 999_999_999.99

	// MaxDocumentLines borne le nombre de lignes d'un document. Un PDF de plusieurs
	// centaines de pages n'est lisible par personne.
	MaxDocumentLines = 500
	// MaxDescriptionRunes borne le libellé d'une ligne.
	MaxDescriptionRunes = 2000
)

// ErrDateHorsBornes : la date n'est pas une date comptable plausible.
type ErrDateHorsBornes struct{ Date time.Time }

func (e ErrDateHorsBornes) Error() string {
	return fmt.Sprintf("la date %s est hors des bornes admises : les années %d à %d",
		e.Date.Format("2006-01-02"), MinBookingYear, maxBookingYear())
}

func maxBookingYear() int { return time.Now().Year() + MaxBookingYearsAhead }

// CheckBookingDate refuse une date hors de [MinBookingYear, année courante + 5].
func CheckBookingDate(d time.Time) error {
	if y := d.Year(); y < MinBookingYear || y > maxBookingYear() {
		return ErrDateHorsBornes{Date: d}
	}
	return nil
}

// ErrMontantHorsBornes : le montant n'est pas un montant comptable admissible.
type ErrMontantHorsBornes struct{ Value float64 }

func (e ErrMontantHorsBornes) Error() string {
	return fmt.Sprintf("le montant %v est hors des bornes admises : de 0 à %.2f, au centime près", e.Value, MaxAmount)
}

// CheckAmount refuse un montant non fini, négatif ou supérieur à MaxAmount.
func CheckAmount(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > MaxAmount {
		return ErrMontantHorsBornes{Value: v}
	}
	return nil
}

// CheckCents refuse en plus un montant qui ne tombe pas sur un centime : 100.123
// au journal est un montant que personne n'a pu payer.
func CheckCents(v float64) error {
	if err := CheckAmount(v); err != nil {
		return err
	}
	if math.Abs(RoundCents(v)-v) > 1e-9 {
		return ErrMontantHorsBornes{Value: v}
	}
	return nil
}

// Cents convertit un montant au centime en nombre entier de centimes : la seule
// façon d'additionner sans dérive flottante.
func Cents(v float64) int64 { return int64(math.Round(RoundCents(v) * 100)) }

// MaxQuantity borne la quantité d'une ligne, comme un montant.
const MaxQuantity = 999_999_999

// CheckDocumentLine contrôle une ligne de facture ou de facture fournisseur et
// nomme son rang dans le message. `n` compte à partir de 1.
//
// Une quantité nulle ou négative, un prix négatif, un rabais hors de 0–100, un
// libellé de 100 ko ou un montant de 1e300 étaient acceptés puis comptabilisés
// (audit 9, A-05). La correction d'une facture est la note de crédit (LTVA
// art. 27 al. 4), pas une ligne négative. `vatPercent` est en pourcentage (8.1).
func CheckDocumentLine(n int, description string, qty, unitPrice, discountPct, vatPercent float64) error {
	if utf8.RuneCountInString(description) > MaxDescriptionRunes {
		return fmt.Errorf("ligne %d : le libellé dépasse %d caractères", n, MaxDescriptionRunes)
	}
	if !isFinite(qty) || qty <= 0 || qty > MaxQuantity {
		return fmt.Errorf("ligne %d : la quantité doit être strictement positive et ne pas dépasser %d", n, MaxQuantity)
	}
	if !isFinite(unitPrice) || unitPrice < 0 || unitPrice > MaxAmount {
		return fmt.Errorf("ligne %d : le prix unitaire doit être compris entre 0 et %.2f", n, MaxAmount)
	}
	if !isFinite(discountPct) || discountPct < 0 || discountPct > 100 {
		return fmt.Errorf("ligne %d : le rabais doit être compris entre 0 et 100", n)
	}
	if !isFinite(vatPercent) || vatPercent < 0 || vatPercent > 100 {
		return fmt.Errorf("ligne %d : le taux de TVA doit être compris entre 0 et 100", n)
	}
	if base := qty * unitPrice * (1 - discountPct/100); !isFinite(base) || base > MaxAmount {
		return fmt.Errorf("ligne %d : le montant de la ligne dépasse la borne admise de %.2f", n, MaxAmount)
	}
	return nil
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

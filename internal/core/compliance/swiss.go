package compliance

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ─── Swiss rounding (0.05 CHF — 5 Rappen rule) ───────────────────────────────

// RoundTo5Rappen rounds amount to the nearest 0.05 CHF as required by Swiss TVA law.
//
// Les demis s'arrondissent À L'ÉCART de zéro (math.Round), pour les deux signes :
// 10.125 → 10.15 et -10.125 → -10.15. Ce n'est pas l'arrondi « au pair » —
// math.RoundToEven est une autre fonction — et ce n'est pas non plus
// floor(x*20+0.5), qui arrondissait les demis négatifs vers zéro : une note de
// crédit de 10.125 sortait à -10.10 quand sa facture sortait à 10.15, et les
// deux ne se compensaient plus d'un centime.
//
// Examples: 10.123 → 10.10, 10.125 → 10.15, -10.125 → -10.15, 99.99 → 100.00
func RoundTo5Rappen(amount float64) float64 {
	r := math.Round(amount*20) / 20
	if r == 0 {
		return 0 // pas de « -0.00 » pour un petit négatif
	}
	return r
}

// RoundCents arrondit un montant au centime, les demis à l'écart de zéro.
//
// C'est l'arrondi des MONTANTS FACTURÉS : le total d'une facture est HT + TVA au
// centime, sans les 5 centimes de RoundTo5Rappen. Cette règle-ci est celle des
// paiements en espèces, que le logiciel n'encaisse pas. Arrondir une facture
// à 5 centimes cassait l'équilibre de son écriture (débit = total arrondi,
// crédits = HT + TVA) pour 80 % des montants au centime près.
//
// Le centime se décide sur des millionièmes ENTIERS : 1.005 * 100 vaut
// 100.49999999999999 en virgule flottante, et math.Round(v*100) rendrait 1.00 pour
// un demi-centime qui doit monter. Passer d'abord par le millionième efface ce
// bruit-là sans déplacer un vrai montant.
func RoundCents(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	// Hors de ce que les millionièmes entiers savent porter (±9·10¹²), un montant
	// est déjà au-delà de toute valeur comptable : repli sur l'arrondi simple.
	if math.Abs(v) >= 9e12 {
		return math.Round(v*100) / 100
	}
	micro := int64(math.Round(v * 1e6))
	var c int64
	if micro >= 0 {
		c = (micro + 5000) / 10000
	} else {
		c = -((-micro + 5000) / 10000)
	}
	if c == 0 {
		return 0 // pas de « -0.00 » pour un petit négatif
	}
	return float64(c) / 100
}

// ─── TVA rates (Swiss 2024) ───────────────────────────────────────────────────

const (
	VATRateStandard = 0.081 // 8.1%
	VATRateReduced  = 0.026 // 2.6% (food, books, etc.)
	VATRateSpecial  = 0.038 // 3.8% (hotel accommodation)
)

// ─── IBAN validation ──────────────────────────────────────────────────────────

var ibanClean = regexp.MustCompile(`\s+`)

// ValidateIBAN vérifie un IBAN selon l'ISO 13616 : structure, longueur imposée
// par le pays, puis clé de contrôle MOD-97. Voir iban.go pour le détail et pour
// le choix fait sur les pays absents du registre embarqué.
//
// Le contrôle de longueur par pays est la partie qui manquait : un IBAN suisse
// à 20 caractères a environ une chance sur 97 de passer le seul MOD-97, et
// serait alors rejeté par la banque à la remise du fichier de virements —
// c'est-à-dire une fois les paiements supposés partis.
func ValidateIBAN(iban string) error {
	iban = NormaliseIBAN(iban)
	if err := checkIBANStructure(iban); err != nil {
		return err
	}

	// Move the first 4 characters to the end
	rearranged := iban[4:] + iban[:4]

	// Replace letters with digits (A=10, B=11, …, Z=35)
	var numeric strings.Builder
	for _, ch := range rearranged {
		if ch >= 'A' && ch <= 'Z' {
			numeric.WriteString(strconv.Itoa(int(ch-'A') + 10))
		} else if ch >= '0' && ch <= '9' {
			numeric.WriteRune(ch)
		} else {
			return fmt.Errorf("caractère non autorisé dans un IBAN : %q", string(ch))
		}
	}

	// MOD-97 check
	remainder := mod97(numeric.String())
	if remainder != 1 {
		return fmt.Errorf("la clé de contrôle de l'IBAN est fausse — vérifiez la saisie, un chiffre a probablement été inversé")
	}
	return nil
}

// IsQRIBAN reports whether the IBAN carries a QR-IID (30000–31999), which marks
// it as a QR-IBAN. Unlike ValidateQRIBAN it performs no checksum validation and
// never errors — use it to decide which reference type an account requires.
func IsQRIBAN(iban string) bool {
	clean := strings.ToUpper(ibanClean.ReplaceAllString(iban, ""))
	if len(clean) < 9 || (!strings.HasPrefix(clean, "CH") && !strings.HasPrefix(clean, "LI")) {
		return false
	}
	iid, err := strconv.Atoi(clean[4:9])
	if err != nil {
		return false
	}
	return iid >= 30000 && iid <= 31999
}

// ValidateQRIBAN validates a Swiss QR-IBAN (must start with CH, IID 30000–31999).
func ValidateQRIBAN(qrIBAN string) error {
	if err := ValidateIBAN(qrIBAN); err != nil {
		return fmt.Errorf("QR-IBAN: %w", err)
	}
	clean := strings.ToUpper(ibanClean.ReplaceAllString(qrIBAN, ""))
	if !strings.HasPrefix(clean, "CH") {
		return fmt.Errorf("QR-IBAN must be a Swiss IBAN (CH prefix)")
	}
	// IID (bank identifier) is digits 5–9 in the IBAN (positions 4–8, 0-indexed)
	if len(clean) < 9 {
		return fmt.Errorf("QR-IBAN too short")
	}
	iid, err := strconv.Atoi(clean[4:9])
	if err != nil {
		return fmt.Errorf("QR-IBAN IID is not numeric")
	}
	if iid < 30000 || iid > 31999 {
		return fmt.Errorf("QR-IBAN IID %d is not in the QR-IID range (30000–31999)", iid)
	}
	return nil
}

// mod97 computes the MOD-97 remainder for a numeric string of arbitrary length.
func mod97(numeric string) int {
	remainder := 0
	for _, ch := range numeric {
		remainder = (remainder*10 + int(ch-'0')) % 97
	}
	return remainder
}

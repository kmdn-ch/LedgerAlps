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

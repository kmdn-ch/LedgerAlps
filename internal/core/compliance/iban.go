package compliance

import (
	"fmt"
	"strings"
)

// Validation IBAN — ISO 13616, telle que la décrit le guide SIX à l'usage des
// banques et des éditeurs de logiciels.
//
// Le contrôle MOD-97 seul ne suffit pas. Il attrape la faute de frappe la plus
// courante, mais laisse passer :
//
//   - un IBAN de **mauvaise longueur pour son pays** — un CH à 20 caractères
//     au lieu de 21 a environ une chance sur 97 de passer le MOD-97, et sera
//     rejeté par la banque à la remise du fichier de virements, c'est-à-dire
//     trop tard ;
//   - une **structure invalide** : les deux premiers caractères doivent être
//     des lettres (code pays ISO 3166-1) et les deux suivants des chiffres
//     (clé de contrôle). « 12CH… » n'est pas un IBAN, quoi qu'en dise MOD-97.
//
// Choix assumé sur les pays inconnus. Le registre IBAN évolue : des pays y sont
// ajoutés après la compilation de ce binaire. Un code pays absent de la table
// ci-dessous n'est donc **pas** rejeté — l'IBAN passe le contrôle structurel et
// le MOD-97, et c'est tout. Refuser un IBAN que l'utilisateur sait valide, en
// l'empêchant de facturer, coûterait plus qu'accepter un IBAN qu'on ne peut pas
// vérifier entièrement. La table sert à attraper les erreurs, pas à tenir une
// liste blanche.

// ibanLengths est le registre officiel des longueurs par pays (SWIFT IBAN
// Registry). Une longueur qui ne correspond pas est une erreur certaine, pas
// une probabilité.
var ibanLengths = map[string]int{
	"AD": 24, "AE": 23, "AL": 28, "AT": 20, "AZ": 28,
	"BA": 20, "BE": 16, "BG": 22, "BH": 22, "BI": 27, "BR": 29, "BY": 28,
	"CH": 21, "CR": 22, "CY": 28, "CZ": 24,
	"DE": 22, "DJ": 27, "DK": 18, "DO": 28,
	"EE": 20, "EG": 29, "ES": 24,
	"FI": 18, "FK": 18, "FO": 18, "FR": 27,
	"GB": 22, "GE": 22, "GI": 23, "GL": 18, "GR": 27, "GT": 28,
	"HN": 28, "HR": 21, "HU": 28,
	"IE": 22, "IL": 23, "IQ": 23, "IS": 26, "IT": 27,
	"JO": 30,
	"KW": 30, "KZ": 20,
	"LB": 28, "LC": 32, "LI": 21, "LT": 20, "LU": 20, "LV": 21, "LY": 25,
	"MC": 27, "MD": 24, "ME": 22, "MK": 19, "MN": 20, "MR": 27, "MT": 31,
	"MU": 30, "MV": 30,
	"NI": 28, "NL": 18, "NO": 15,
	"OM": 23,
	"PK": 24, "PL": 28, "PS": 29, "PT": 25,
	"QA": 29,
	"RO": 24, "RS": 22, "RU": 33,
	"SA": 24, "SC": 31, "SD": 18, "SE": 24, "SI": 19, "SK": 24, "SM": 27,
	"SO": 23, "ST": 25, "SV": 28,
	"TL": 23, "TN": 24, "TR": 26,
	"UA": 29,
	"VA": 22, "VG": 24,
	"XK": 20,
	"YE": 30,
	// Territoires français d'outre-mer : même structure que la France.
	"BL": 27, "GF": 27, "GP": 27, "MF": 27, "MQ": 27, "NC": 27,
	"PF": 27, "PM": 27, "RE": 27, "WF": 27, "YT": 27,
}

// NormaliseIBAN retire espaces et met en majuscules. Un IBAN se saisit et
// s'imprime par groupes de quatre ; il se stocke et se compare sans espaces.
func NormaliseIBAN(iban string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, iban))
}

// FormatIBAN rend un IBAN lisible par groupes de quatre, comme sur un relevé.
func FormatIBAN(iban string) string {
	clean := NormaliseIBAN(iban)
	var b strings.Builder
	for i, r := range clean {
		if i > 0 && i%4 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// checkIBANStructure vérifie la forme avant tout calcul : pays, clé, charset.
func checkIBANStructure(clean string) error {
	if clean == "" {
		return fmt.Errorf("l'IBAN est vide")
	}
	if len(clean) < 15 || len(clean) > 34 {
		return fmt.Errorf("un IBAN compte entre 15 et 34 caractères, celui-ci en a %d", len(clean))
	}
	for i := 0; i < 2; i++ {
		if clean[i] < 'A' || clean[i] > 'Z' {
			return fmt.Errorf("les deux premiers caractères doivent être le code du pays (deux lettres), pas %q", clean[:2])
		}
	}
	for i := 2; i < 4; i++ {
		if clean[i] < '0' || clean[i] > '9' {
			return fmt.Errorf("les caractères 3 et 4 doivent être la clé de contrôle (deux chiffres), pas %q", clean[2:4])
		}
	}
	for _, r := range clean {
		isDigit := r >= '0' && r <= '9'
		isUpper := r >= 'A' && r <= 'Z'
		if !isDigit && !isUpper {
			return fmt.Errorf("caractère non autorisé dans un IBAN : %q", string(r))
		}
	}

	country := clean[:2]
	if want, known := ibanLengths[country]; known && len(clean) != want {
		return fmt.Errorf("un IBAN %s compte %d caractères, celui-ci en a %d", country, want, len(clean))
	}
	return nil
}

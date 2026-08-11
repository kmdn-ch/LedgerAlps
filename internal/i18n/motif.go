package i18n

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Traduction d'une phrase déjà formatée.
//
// # Le problème
//
// Le catalogue est indexé par la phrase française, mais ce qui sort d'un
// gestionnaire est une phrase DÉJÀ formatée :
//
//	fmt.Sprintf("le montant doit valoir au moins 0.01, reçu %.2f", v)
//
// donne « le montant doit valoir au moins 0.01, reçu 0.00 », qui ne figure
// dans aucune table. Chercher l'entrée telle quelle échouerait sur tous les
// messages portant une valeur — c'est-à-dire les plus utiles, ceux qui disent
// QUEL compte, QUELLE facture, QUEL montant.
//
// # La réponse
//
// Chaque phrase à verbes de format est compilée en expression régulière une
// fois pour toutes : `%d` devient `(-?\d+)`, `%s` devient `(.*)`. Les valeurs
// capturées sont réinjectées dans la traduction, aux mêmes verbes.
//
// C'est pour cela que le test exige que la traduction porte EXACTEMENT les
// mêmes verbes, dans le même ordre, que le français : sinon la réinjection
// glisserait d'un cran et afficherait le montant à la place du numéro de
// compte, ce qui est pire qu'une phrase non traduite.

// reVerbe reconnaît un verbe de format : %s, %d, %q, %.2f, %v, %w.
var reVerbe = regexp.MustCompile(`%[+\-# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z]`)

// équivalents dit quelle portion de texte un verbe peut avoir produite.
//
// `%d` ne capture que des chiffres, ce qui évite qu'un message court avale la
// moitié d'un autre. `%s` capture tout, sans gourmandise, pour qu'une phrase à
// deux `%s` sépare au bon endroit.
func équivalent(verbe string) string {
	switch verbe[len(verbe)-1] {
	case 'd':
		return `(-?\d+)`
	case 'f':
		return `(-?[\d.,]+)`
	default:
		return `(.*?)`
	}
}

type motif struct {
	re     *regexp.Regexp
	phrase string
}

var (
	motifsUneFois sync.Once
	motifs        []motif
)

// compileMotifs prépare les expressions des phrases à verbes de format.
func compileMotifs() {
	for fr := range catalogue {
		if !strings.Contains(fr, "%") {
			continue
		}
		var b strings.Builder
		b.WriteString(`\A`)
		reste := fr
		for {
			loc := reVerbe.FindStringIndex(reste)
			if loc == nil {
				break
			}
			b.WriteString(regexp.QuoteMeta(reste[:loc[0]]))
			b.WriteString(équivalent(reste[loc[0]:loc[1]]))
			reste = reste[loc[1]:]
		}
		b.WriteString(regexp.QuoteMeta(reste))
		b.WriteString(`\z`)
		re, err := regexp.Compile(b.String())
		if err != nil {
			continue // une phrase impossible à compiler ressort en français
		}
		motifs = append(motifs, motif{re: re, phrase: fr})
	}
}

// Traduire rend la version d'un message déjà formaté dans la langue demandée.
//
// L'entrée est ce que le serveur allait écrire ; la sortie est ce que
// l'utilisateur lira. Un message inconnu ressort inchangé — c'est-à-dire en
// français, exactement ce qui s'affichait avant l'existence de ce paquet.
func Traduire(l Lang, message string) string {
	if l == FR || message == "" {
		return message
	}

	// Le cas courant : la phrase est au catalogue telle quelle.
	if trads, ok := catalogue[message]; ok {
		if t := trads[l]; t != "" {
			return t
		}
		return message
	}

	// Le cas à valeurs : retrouver le moule, et y couler la traduction.
	motifsUneFois.Do(compileMotifs)
	for _, m := range motifs {
		captures := m.re.FindStringSubmatch(message)
		if captures == nil {
			continue
		}
		trads := catalogue[m.phrase]
		t := trads[l]
		if t == "" {
			return message
		}
		return réinjecte(t, captures[1:])
	}

	// Le cas « préfixe + cause » : plusieurs messages annoncent l'échec en
	// français puis collent la cause technique telle qu'elle vient de la
	// couche basse — « le fichier CSV n'a pas pu être produit : open /tmp/… ».
	//
	// La cause ne se traduit pas, et ne doit pas l'être : c'est elle qu'on
	// cherchera dans les sources ou qu'on collera dans un ticket. Mais la
	// phrase qui la précède, si. On traduit donc l'annonce et on laisse la
	// cause intacte.
	préfixesUneFois.Do(compilePréfixes)
	for _, p := range préfixes {
		if !strings.HasPrefix(message, p) {
			continue
		}
		t := catalogue[p][l]
		if t == "" {
			return message
		}
		return t + message[len(p):]
	}
	return message
}

var (
	préfixesUneFois sync.Once
	préfixes        []string
)

// compilePréfixes retient les phrases qui annoncent une cause, du plus long au
// plus court.
//
// Du plus long au plus court parce que deux annonces peuvent commencer
// pareil : la plus précise doit gagner, sinon la plus vague la mangerait.
func compilePréfixes() {
	for fr := range catalogue {
		if strings.Contains(fr, "%") {
			continue
		}
		// Une annonce se reconnaît à ce qu'elle s'arrête sur un séparateur :
		// elle attend une suite.
		if strings.HasSuffix(fr, ": ") || strings.HasSuffix(fr, " : ") ||
			strings.HasSuffix(fr, "« ") || strings.HasSuffix(fr, "(") ||
			strings.HasSuffix(fr, "pour ") || strings.HasSuffix(fr, "le ") {
			préfixes = append(préfixes, fr)
		}
	}
	sort.Slice(préfixes, func(i, j int) bool {
		return len(préfixes[i]) > len(préfixes[j])
	})
}

// réinjecte remplace les verbes de la traduction par les valeurs capturées.
func réinjecte(traduction string, valeurs []string) string {
	i := 0
	return reVerbe.ReplaceAllStringFunc(traduction, func(string) string {
		if i >= len(valeurs) {
			return ""
		}
		v := valeurs[i]
		i++
		return v
	})
}

package qrbill

// Lire le TEXTE d'une facture, avec ses positions.
//
// # Pourquoi les positions et pas seulement les mots
//
// Sur une facture, un chiffre ne veut rien dire sans son étiquette. « 538690 »
// est un numéro de facture parce qu'il est écrit sous la colonne « Numéro de
// facture » ; « 31.12.2025 » est une échéance parce qu'il est sous « Échu ». Un
// extracteur qui rend une suite de mots perd exactement ce qui les qualifie, et
// il ne reste plus qu'à deviner — ce que ce paquet refuse de faire.
//
// On lit donc chaque fragment avec ses coordonnées, on reconstitue des lignes,
// et on rattache une valeur à son étiquette par la géométrie : à droite sur la
// même ligne, ou dessous dans la même colonne. C'est ainsi qu'un humain lit une
// facture.
//
// # Ce que ce paquet ne fait pas
//
// Il ne décide rien. Il rend des CANDIDATS avec l'étiquette qui les a produits,
// pour que l'interface montre d'où vient chaque valeur. Une facture mal lue
// entre dans les livres et dans la déclaration de TVA : un champ pré-rempli
// dont on voit la provenance se corrige, un champ pré-rempli anonyme se croit.

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// textItem est un fragment de texte et l'endroit où il est posé.
type textItem struct {
	x, y float64
	s    string
}

// extractText rend les fragments de texte du PDF, avec leurs coordonnées.
func extractText(data []byte) ([]textItem, error) {
	dir, err := os.MkdirTemp("", "ledgeralps-txt-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		return nil, err
	}
	out := filepath.Join(dir, "content")
	if err := os.MkdirAll(out, 0o700); err != nil {
		return nil, err
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.ExtractContentFile(src, out, nil, conf); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(out)
	if err != nil {
		return nil, err
	}
	// Les fichiers sortent nommés par page : les trier garde l'ordre de
	// lecture, et une facture de deux pages met souvent le détail sur la
	// seconde.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var items []textItem
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(out, n))
		if err != nil {
			continue
		}
		items = append(items, parseContentStream(string(b))...)
	}
	return items, nil
}

var (
	// « 10 0 0 10 122.358 549.276 Tm » — la matrice de texte donne la position.
	reTm = regexp.MustCompile(
		`([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+Tm`)
	// « 0 -12 Td » — un déplacement relatif depuis la position courante.
	reTd = regexp.MustCompile(`([-\d.]+)\s+([-\d.]+)\s+T[dD]`)
	// « (texte)Tj » et « [(a) -20 (b)]TJ »
	reShow = regexp.MustCompile(`(\((?:\\.|[^\\()])*\)|\[(?:[^\]\\]|\\.)*\])\s*(Tj|TJ|'|")`)
)

// parseContentStream lit les opérateurs de texte d'un flux de contenu.
//
// L'analyse est délibérément simple : on suit la dernière position posée par Tm
// ou Td, et on l'attribue au texte qui suit. C'est faux pour un PDF qui joue
// avec les états graphiques imbriqués, mais juste pour ce que produisent les
// logiciels de facturation — et l'imprécision ne coûte qu'un rattachement
// d'étiquette, jamais une valeur inventée.
func parseContentStream(s string) []textItem {
	var items []textItem
	var x, y float64

	// On parcourt le flux en repérant, dans l'ordre, les positions et les
	// textes. Un simple balayage par index évite de charger tout un analyseur
	// PostScript pour six opérateurs.
	type ev struct {
		pos  int
		kind int // 0 = position, 1 = texte
		a, b float64
		s    string
	}
	var evs []ev

	for _, m := range reTm.FindAllStringSubmatchIndex(s, -1) {
		e := ev{pos: m[0], kind: 0}
		e.a = atof(s[m[10]:m[11]])
		e.b = atof(s[m[12]:m[13]])
		evs = append(evs, e)
	}
	for _, m := range reTd.FindAllStringSubmatchIndex(s, -1) {
		evs = append(evs, ev{pos: m[0], kind: 2,
			a: atof(s[m[2]:m[3]]), b: atof(s[m[4]:m[5]])})
	}
	for _, m := range reShow.FindAllStringSubmatchIndex(s, -1) {
		evs = append(evs, ev{pos: m[0], kind: 1, s: s[m[2]:m[3]]})
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].pos < evs[j].pos })

	for _, e := range evs {
		switch e.kind {
		case 0:
			x, y = e.a, e.b
		case 2:
			x += e.a
			y += e.b
		case 1:
			txt := decodePDFText(e.s)
			if strings.TrimSpace(txt) != "" {
				items = append(items, textItem{x: x, y: y, s: txt})
			}
		}
	}
	return items
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// decodePDFText rend le texte d'une chaîne PDF, échappements compris.
//
// Les accents arrivent en octal — « Num\351ro » — dans l'encodage WinAnsi, très
// proche du Latin-1. Sans cette conversion, « Échéance » ne se reconnaîtrait
// jamais, et c'est justement l'étiquette qui compte.
func decodePDFText(raw string) string {
	if strings.HasPrefix(raw, "[") {
		// Tableau : on concatène les chaînes en ignorant les crénages.
		var sb strings.Builder
		for _, m := range regexp.MustCompile(`\((?:\\.|[^\\()])*\)`).FindAllString(raw, -1) {
			sb.WriteString(decodePDFText(m))
		}
		return sb.String()
	}
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")

	var out []byte
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' || i+1 >= len(raw) {
			out = append(out, c)
			continue
		}
		i++
		switch n := raw[i]; n {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b', 'f':
			out = append(out, ' ')
		case '(', ')', '\\':
			out = append(out, n)
		default:
			if n >= '0' && n <= '7' {
				oct := string(n)
				for j := 0; j < 2 && i+1 < len(raw) && raw[i+1] >= '0' && raw[i+1] <= '7'; j++ {
					i++
					oct += string(raw[i])
				}
				v, _ := strconv.ParseUint(oct, 8, 16)
				out = append(out, byte(v))
			} else {
				out = append(out, n)
			}
		}
	}
	return winAnsiToUTF8(out)
}

// winAnsiToUTF8 convertit l'encodage des PDF vers de l'UTF-8.
//
// WinAnsi coïncide avec Latin-1 sauf entre 0x80 et 0x9F, où il place les
// guillemets typographiques et l'apostrophe courbe — celle de « jusqu'au », qui
// arrive en 0x92. Traiter cette plage comme du Latin-1 produirait des
// caractères de contrôle et casserait la reconnaissance des étiquettes.
func winAnsiToUTF8(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if r, ok := winAnsiHigh[c]; ok {
			sb.WriteRune(r)
			continue
		}
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

var winAnsiHigh = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡',
	0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž',
	0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•',
	0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›',
	0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

// ─── Mise en lignes ──────────────────────────────────────────────────────────

// line est une suite de fragments à la même hauteur, ordonnés de gauche à
// droite.
type line struct {
	y     float64
	items []textItem
}

// toLines regroupe les fragments par hauteur.
//
// La tolérance de deux points absorbe les décalages d'une même ligne — un
// exposant, une police plus petite pour une étiquette — sans fusionner deux
// lignes voisines, qui sont séparées d'au moins dix points sur une facture.
func toLines(items []textItem) []line {
	sorted := make([]textItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		if math.Abs(sorted[i].y-sorted[j].y) > 2 {
			return sorted[i].y > sorted[j].y // de haut en bas
		}
		return sorted[i].x < sorted[j].x
	})

	var lines []line
	for _, it := range sorted {
		if n := len(lines); n > 0 && math.Abs(lines[n-1].y-it.y) <= 2 {
			lines[n-1].items = append(lines[n-1].items, it)
			continue
		}
		lines = append(lines, line{y: it.y, items: []textItem{it}})
	}
	return lines
}

func (l line) text() string {
	parts := make([]string, 0, len(l.items))
	for _, it := range l.items {
		parts = append(parts, strings.TrimSpace(it.s))
	}
	return strings.Join(parts, " ")
}


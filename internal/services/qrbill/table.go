package qrbill

// Lire une LIGNE de tableau, pas des valeurs isolées.
//
// # Pourquoi c'est nécessaire
//
// Sur un rappel, le document porte deux dates étiquetées « Date » : celle du
// rappel en tête de page, et celle de la facture rappelée, dans le tableau. Une
// recherche qui prend la première trouvée inscrit la mauvaise — et comme le
// numéro de facture, lui, vient du tableau, on obtient une pièce dont le numéro
// et la date ne se rapportent pas au même document.
//
// La règle est donc : le numéro de facture, sa date et son échéance
// appartiennent à la MÊME ligne. On repère l'en-tête qui contient « Numéro de
// facture », on lit la ligne de valeurs en dessous, et on rattache chaque
// cellule à sa colonne par l'abscisse.
//
// # Ce que cela protège
//
// Une facture dont le numéro dit décembre et la date février se rapproche mal,
// se paie en retard, et se retrouve dans le mauvais exercice. Le défaut ne se
// voit qu'au moment du contrôle, longtemps après la saisie.

import (
	"math"
	"regexp"
	"strings"
)

// tableRow est une ligne de tableau, chaque valeur rangée sous son en-tête.
type tableRow map[string]string

// findInvoiceRow cherche la ligne de tableau qui décrit la facture.
//
// Rend les valeurs indexées par l'en-tête en minuscules, ou nil si le document
// n'a pas de tableau reconnaissable — beaucoup de factures n'en ont pas, et
// c'est alors la recherche par étiquette qui prend le relais.
func findInvoiceRow(lines []line) tableRow {
	for i, l := range lines {
		// Un en-tête de tableau porte l'étiquette du numéro de facture ET au
		// moins deux autres colonnes : une étiquette isolée n'est pas un
		// tableau, c'est un champ.
		if len(l.items) < 3 {
			continue
		}
		if !lineHasLabel(l, numberLabels) {
			continue
		}
		// La ligne de valeurs est la suivante qui contient quelque chose
		// d'exploitable. Un en-tête sur deux lignes — « Niveau de » / « relance »
		// — impose de regarder un peu plus loin qu'une seule ligne.
		for j := i + 1; j < len(lines) && j <= i+3; j++ {
			row := alignRow(l, lines[j])
			if row == nil {
				continue
			}
			// La ligne retenue doit porter une valeur sous la colonne du numéro.
			for _, lb := range numberLabels {
				if v, ok := row[lb]; ok && isInvoiceNumber(v) {
					return row
				}
			}
		}
	}
	return nil
}

func lineHasLabel(l line, labels []string) bool {
	for _, it := range l.items {
		low := strings.ToLower(strings.TrimSpace(it.s))
		for _, lb := range labels {
			if strings.Contains(low, lb) {
				return true
			}
		}
	}
	return false
}

// alignRow range les cellules d'une ligne sous les en-têtes d'une autre.
//
// L'appariement se fait par l'abscisse la plus proche. Une tolérance large —
// quarante points — parce qu'un montant est aligné à droite sous un en-tête
// aligné à gauche, et que les deux ne partagent alors aucune abscisse exacte.
func alignRow(header, values line) tableRow {
	if len(values.items) < 2 {
		return nil
	}

	// Un en-tête qui tient sur deux mots — « Niveau de » / « relance » — compte
	// pour DEUX cellules alors qu'il n'y a qu'une colonne. Il se reconnaît à ce
	// que les deux fragments partagent le même bord gauche : une colonne
	// voisine est toujours plus loin. Sans cette fusion, les comptes ne
	// concordent plus et l'appariement par rang est abandonné à tort.
	head := mergeWrappedCells(header)

	// Quand les deux lignes ont AUTANT de cellules, on apparie par RANG.
	//
	// C'est la seule lecture fiable d'un tableau dont les colonnes de nombres
	// sont alignées à droite : leur abscisse tombe alors sous l'en-tête
	// PRÉCÉDENT, et l'appariement par la position décale toute la ligne d'un
	// cran. Mesuré sur une facture réelle : le montant se rangeait sous
	// « Devise », l'échéance sous « Montant », et le niveau de relance sous
	// « Échu » — chaque valeur juste, chaque étiquette fausse.
	if len(head) == len(values.items) {
		row := tableRow{}
		for i, v := range values.items {
			val := strings.TrimSpace(v.s)
			colonne := strings.ToLower(strings.TrimSpace(head[i].s))
			if val == "" || colonne == "" {
				continue
			}
			if _, déjà := row[colonne]; !déjà {
				row[colonne] = val
			}
		}
		if len(row) >= 2 && plausible(row) {
			return row
		}
	}

	// Sinon — une cellule vide, un en-tête sur deux lignes — on retombe sur
	// l'abscisse la plus proche. Moins sûr, mais mieux que rien.
	row := tableRow{}
	for _, v := range values.items {
		val := strings.TrimSpace(v.s)
		if val == "" {
			continue
		}
		best, bestDx := "", math.MaxFloat64
		for _, h := range header.items {
			if dx := absf(h.x - v.x); dx < bestDx {
				best, bestDx = strings.ToLower(strings.TrimSpace(h.s)), dx
			}
		}
		if best == "" || bestDx > 40 {
			continue
		}
		// Deux valeurs sous le même en-tête : la première gagne. Sur une facture
		// à plusieurs postes, c'est celle de la ligne qu'on lit.
		if _, déjà := row[best]; !déjà {
			row[best] = val
		}
	}
	if len(row) < 2 {
		return nil
	}
	return row
}

// plausible dit si l'appariement tient debout.
//
// L'égalité des comptes ne prouve rien : un en-tête fusionné à tort décale
// toute la ligne d'un cran, et chaque valeur reste juste tout en se retrouvant
// sous la mauvaise étiquette. C'est le pire des cas — rien ne cloche à l'œil,
// et l'échéance lue est en réalité un montant.
//
// On vérifie donc ce qu'on sait : sous « Montant » il y a un nombre et pas une
// date, sous « Devise » un code de trois lettres. Deux contrôles suffisent à
// détecter le décalage d'un cran, qui est la seule erreur que cette méthode
// produit.
func plausible(row tableRow) bool {
	for head, v := range row {
		switch {
		case containsAny(head, []string{"montant", "betrag", "amount", "importo"}):
			if reDate.MatchString(v) || !reMoney.MatchString(v) {
				return false
			}
		case containsAny(head, []string{"devise", "währung", "waehrung", "currency", "valuta"}):
			if !reCurrency.MatchString(strings.TrimSpace(v)) {
				return false
			}
		}
	}
	return true
}

var (
	reMoney    = regexp.MustCompile(`^-?[\d'’\s]*[.,]?\d+$`)
	reCurrency = regexp.MustCompile(`^[A-Za-z]{3}$`)
)

// mergeWrappedCells fusionne les fragments d'en-tête qui partagent un bord.
//
// Cinq points : au-delà, c'est une autre colonne. En deçà, c'est le même titre
// passé à la ligne — et sur une facture, les colonnes sont espacées d'au moins
// dix points.
func mergeWrappedCells(l line) []textItem {
	var out []textItem
	for _, it := range l.items {
		if n := len(out); n > 0 && absf(out[n-1].x-it.x) < 5 {
			out[n-1].s = strings.TrimSpace(out[n-1].s) + " " + strings.TrimSpace(it.s)
			continue
		}
		out = append(out, it)
	}
	return out
}

// pick rend la première valeur dont l'en-tête correspond à l'une des étiquettes.
func (r tableRow) pick(labels []string, accept func(string) bool) (value, label string) {
	// Les étiquettes les plus longues d'abord : « date de facture » doit être
	// essayée avant « date », sans quoi la colonne la plus vague l'emporterait.
	ordered := make([]string, len(labels))
	copy(ordered, labels)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if len(ordered[j]) > len(ordered[i]) {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	for _, lb := range ordered {
		for head, v := range r {
			if strings.Contains(head, lb) && accept(v) {
				return v, head
			}
		}
	}
	return "", ""
}

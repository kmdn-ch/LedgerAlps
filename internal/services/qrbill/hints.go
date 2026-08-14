package qrbill

// Ce que la couche texte d'une facture peut apprendre en plus du QR.
//
// Le QR porte le créancier, l'IBAN, le montant et la référence de paiement. Il
// ne porte NI le numéro de la facture, NI sa date, NI l'échéance, NI le taux de
// TVA — ce ne sont pas des données du bulletin de versement. Elles sont sur la
// facture, en toutes lettres, à côté de leur étiquette.
//
// # Chaque valeur voyage avec sa provenance
//
// Un champ pré-rempli dont on voit l'étiquette d'origine se corrige ; un champ
// pré-rempli anonyme se croit. Comme une facture fournisseur entre dans les
// livres ET dans la déclaration de TVA, chaque valeur rendue ici dit d'où elle
// vient, et l'écran l'affiche.
//
// # La TVA : absence de mention vaut zéro
//
// Un fournisseur non assujetti facture sans TVA. Le montant du QR est alors le
// montant total, il n'y a rien à déduire (LTVA art. 28 al. 1 exige une facture
// mentionnant l'impôt pour le récupérer), et le taux est 0 %.
//
// Le piège : le numéro d'identification d'un assujetti contient les lettres
// « MWST » ou « TVA » — « IDE CHE-103.727.240 MWST ». Chercher ces mots
// suffirait donc à croire qu'une facture porte de la TVA alors qu'elle n'en
// porte aucune. On cherche un TAUX ou un MONTANT de TVA, jamais un mot seul.

import (
	"regexp"
	"strconv"
	"strings"
)

// Hints regroupe ce que le texte de la facture a livré.
type Hints struct {
	InvoiceNumber string `json:"invoice_number"`
	// La provenance de chaque valeur : l'étiquette lue sur le document.
	InvoiceNumberLabel string `json:"invoice_number_label"`

	IssueDate      string `json:"issue_date"` // AAAA-MM-JJ
	IssueDateLabel string `json:"issue_date_label"`

	DueDate      string `json:"due_date"`
	DueDateLabel string `json:"due_date_label"`

	// VATRate en pourcent. VATMentioned dit si le document parle réellement de
	// TVA : sans mention, le taux est 0 et le montant du QR est le total.
	VATRate      float64 `json:"vat_rate"`
	VATMentioned bool    `json:"vat_mentioned"`
	VATLabel     string  `json:"vat_label"`

	// SupplierUID : le numéro IDE du fournisseur, utile à sa fiche.
	SupplierUID string `json:"supplier_uid"`
}

var (
	reDate = regexp.MustCompile(`\b(\d{2})[./-](\d{2})[./-](\d{4})\b`)
	reISO  = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	reUID  = regexp.MustCompile(`CHE[- ]?(\d{3})[. ]?(\d{3})[. ]?(\d{3})`)
	// Un taux de TVA suisse, écrit « 8.1 % », « 8,1% » ou « 7.7 % ».
	reRate = regexp.MustCompile(`\b(\d{1,2})[.,](\d)\s*%`)
	// Un numéro de facture : chiffres, éventuellement avec tirets ou barres.
	reNumber = regexp.MustCompile(`^[A-Z]{0,4}[-/ ]?\d[\d\-/.]{2,}$`)
)

// Étiquettes reconnues, de la plus explicite à la plus vague.
//
// L'ordre compte : sur un rappel, « Échu » désigne la vraie échéance de la
// facture d'origine, tandis que « Paiements pris en compte jusqu'au » désigne
// la date d'arrêté des encaissements. Les deux sont des dates plausibles ;
// préférer la première évite d'inscrire une échéance postérieure à la réalité,
// ce qui ferait mentir l'indicateur de retard.
var dueLabels = []string{
	"échéance", "echeance", "payable jusqu'au", "payable jusqu’au",
	"délai de paiement", "delai de paiement", "à payer jusqu'au", "a payer jusqu'au",
	"zahlbar bis", "fälligkeit", "faelligkeit", "fällig", "faellig",
	"scadenza", "due date", "payment due",
	"échu", "echu",
	"paiements pris en compte jusqu'au", "paiements pris en compte jusqu’au",
	"paiement pris en compte jusqu'au", "paiement pris en compte jusqu’au",
}

var issueLabels = []string{
	"date de facture", "date de la facture", "rechnungsdatum", "invoice date",
	"data fattura", "date",
}

var numberLabels = []string{
	"numéro de facture", "numero de facture", "n° de facture", "no de facture",
	"n° facture", "no facture", "rechnungsnummer", "rechnungs-nr", "invoice number",
	"invoice no", "numero fattura", "facture n°", "facture no",
}

var vatLabels = []string{
	"tva", "mwst", "iva", "vat", "mehrwertsteuer",
}

// ExtractHints lit la couche texte et rend ce qu'elle apprend.
//
// Un PDF sans couche texte — un scan — rend des indications vides sans erreur :
// le QR reste exploitable, et la saisie manuelle prend le relais. C'est une
// absence, pas une panne.
func ExtractHints(data []byte) Hints {
	var h Hints
	items, err := extractText(data)
	if err != nil || len(items) == 0 {
		return h
	}
	lines := toLines(items)

	// ── Le numéro IDE du fournisseur ────────────────────────────────────────
	for _, l := range lines {
		if m := reUID.FindStringSubmatch(l.text()); m != nil {
			h.SupplierUID = "CHE-" + m[1] + "." + m[2] + "." + m[3]
			break
		}
	}

	// ── La TVA : un TAUX, pas un mot ────────────────────────────────────────
	//
	// Le mot seul figure dans le numéro d'assujetti du fournisseur ; s'y fier
	// ferait croire à de la TVA sur une facture qui n'en porte pas.
	for _, l := range lines {
		t := strings.ToLower(l.text())
		if !containsAny(t, vatLabels) {
			continue
		}
		// Le numéro d'identification n'est pas une mention de TVA facturée.
		if reUID.MatchString(l.text()) {
			continue
		}
		if m := reRate.FindStringSubmatch(l.text()); m != nil {
			if r, err := strconv.ParseFloat(m[1]+"."+m[2], 64); err == nil && r > 0 && r < 30 {
				h.VATRate, h.VATMentioned = r, true
				h.VATLabel = strings.TrimSpace(l.text())
				break
			}
		}
	}

	// ── La LIGNE de tableau d'abord ─────────────────────────────────────────
	//
	// Le numéro de facture, sa date et son échéance appartiennent à la même
	// ligne. Les chercher séparément fait prendre, sur un rappel, la date du
	// rappel pour celle de la facture — et l'on obtient une pièce dont le
	// numéro et la date ne se rapportent pas au même document.
	estDate := func(s string) bool { return reDate.MatchString(s) || reISO.MatchString(s) }
	if row := findInvoiceRow(lines); row != nil {
		if v, lb := row.pick(numberLabels, isInvoiceNumber); v != "" {
			h.InvoiceNumber, h.InvoiceNumberLabel = v, lb
		}
		if v, lb := row.pick(issueLabels, estDate); v != "" {
			h.IssueDate, h.IssueDateLabel = normaliseDate(v), lb
		}
		if v, lb := row.pick(dueLabels, estDate); v != "" {
			h.DueDate, h.DueDateLabel = normaliseDate(v), lb
		}
	}

	// ── À défaut, les valeurs étiquetées une à une ──────────────────────────
	if h.InvoiceNumber == "" {
		h.InvoiceNumber, h.InvoiceNumberLabel = findLabelled(lines, numberLabels, isInvoiceNumber)
	}
	if h.DueDate == "" {
		h.DueDate, h.DueDateLabel = findLabelledDate(lines, dueLabels)
	}
	if h.IssueDate == "" {
		h.IssueDate, h.IssueDateLabel = findLabelledDate(lines, issueLabels)
	}

	// Une échéance antérieure à la date de facture est une lecture ratée : on
	// préfère ne rien proposer plutôt qu'une incohérence que personne ne
	// remarquerait avant le rapprochement.
	if h.IssueDate != "" && h.DueDate != "" && h.DueDate < h.IssueDate {
		h.DueDate, h.DueDateLabel = "", ""
	}
	return h
}

func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

func isInvoiceNumber(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 || len(s) > 24 {
		return false
	}
	// Une date n'est pas un numéro de facture, et elle en a la forme.
	if reDate.MatchString(s) {
		return false
	}
	return reNumber.MatchString(strings.ToUpper(s))
}

// findLabelledDate cherche une date rattachée à l'une des étiquettes.
func findLabelledDate(lines []line, labels []string) (string, string) {
	return findLabelled(lines, labels, func(s string) bool {
		return reDate.MatchString(s) || reISO.MatchString(s)
	}, normaliseDate)
}

// findLabelled rattache une valeur à son étiquette par la géométrie.
//
// Trois dispositions couvrent ce que produisent les logiciels de facturation :
// la valeur sur la même ligne que l'étiquette (à droite comme à gauche — un
// bulletin suisse met souvent l'étiquette en petit à gauche du champ), la
// valeur collée à l'étiquette dans le même fragment, et la valeur sous
// l'étiquette dans une colonne de tableau.
func findLabelled(
	lines []line, labels []string, accept func(string) bool, transform ...func(string) string,
) (string, string) {
	norm := func(s string) string {
		for _, t := range transform {
			s = t(s)
		}
		return s
	}

	for i, l := range lines {
		for j, it := range l.items {
			low := strings.ToLower(strings.TrimSpace(it.s))
			matched := ""
			for _, lb := range labels {
				if strings.Contains(low, lb) {
					if len(lb) > len(matched) {
						matched = lb
					}
				}
			}
			if matched == "" {
				continue
			}

			// a) La valeur est dans le même fragment, après l'étiquette.
			reste := strings.TrimSpace(it.s[minInt(len(it.s), indexFold(it.s, matched)+len(matched)):])
			reste = strings.TrimLeft(reste, " :\t")
			if accept(reste) {
				return norm(firstMatch(reste)), strings.TrimSpace(it.s)
			}

			// b) La valeur est ailleurs sur la même ligne.
			for k := range l.items {
				if k == j {
					continue
				}
				v := strings.TrimSpace(l.items[k].s)
				if accept(v) {
					return norm(firstMatch(v)), strings.TrimSpace(it.s)
				}
			}

			// c) La valeur est dans la ligne suivante, sous la même colonne.
			//    C'est la disposition d'un tableau : en-tête puis valeurs.
			if i+1 < len(lines) {
				best, bestDx := "", 1e9
				for _, v := range lines[i+1].items {
					val := strings.TrimSpace(v.s)
					if !accept(val) {
						continue
					}
					if dx := absf(v.x - it.x); dx < bestDx {
						best, bestDx = val, dx
					}
				}
				// Trente points : la largeur d'une colonne étroite. Au-delà, la
				// valeur appartient à une autre colonne et l'étiquette ne la
				// qualifie plus.
				if best != "" && bestDx < 30 {
					return norm(firstMatch(best)), strings.TrimSpace(it.s)
				}
			}
		}
	}
	return "", ""
}

// firstMatch isole la valeur utile d'un fragment qui en contient plus.
func firstMatch(s string) string {
	if m := reISO.FindString(s); m != "" {
		return m
	}
	if m := reDate.FindString(s); m != "" {
		return m
	}
	return strings.TrimSpace(s)
}

// normaliseDate rend une date au format que l'interface attend.
func normaliseDate(s string) string {
	if m := reISO.FindStringSubmatch(s); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	if m := reDate.FindStringSubmatch(s); m != nil {
		return m[3] + "-" + m[2] + "-" + m[1]
	}
	return ""
}

func indexFold(s, sub string) int {
	i := strings.Index(strings.ToLower(s), strings.ToLower(sub))
	if i < 0 {
		return 0
	}
	return i
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

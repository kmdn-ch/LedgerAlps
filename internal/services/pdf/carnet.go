package pdf

// Le carnet du lait en PDF — le document que l'on tend à l'administration.
//
// # Pourquoi un rendu à part
//
// `Generate` produit des FACTURES : en-tête client, lignes, TVA, bulletin de
// versement. Rien de tout cela ne sert ici, et l'y greffer aurait rendu une
// fonction déjà longue illisible pour les deux usages. Le carnet a sa propre
// mise en page, et partage seulement les conventions du fichier voisin —
// gofpdf, marges, encodage Latin-1, pagination « n/N ».
//
// # Ce que le document doit porter, et pourquoi
//
// L'art. 957 al. 2 exige trois choses : les recettes, les dépenses, et l'état
// du patrimoine. Le PDF les présente dans cet ordre, avec la base légale citée
// en tête — un document fiscal qui ne dit pas sous quel régime il est établi
// oblige le lecteur à le deviner.
//
// La mention de la BASE CAISSE est écrite noir sur blanc. C'est la seule chose
// qui permet à un contrôleur de comprendre pourquoi le total des recettes ne
// correspond pas au chiffre d'affaires : la différence, ce sont les factures
// émises et non encaissées. La taire ferait passer un document juste pour un
// document faux.

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
)

// CarnetData est ce que le rendu attend.
//
// Des types simples plutôt qu'une dépendance vers le paquet `simplified` : le
// rendu ne doit pas savoir COMMENT le carnet est calculé, seulement ce qu'il
// affiche. L'inverse ferait remonter la comptabilité dans la couche
// d'impression.
type CarnetData struct {
	Entreprise string
	Adresse    string
	IDE        string
	Du         string
	Au         string

	Recettes      []CarnetLigne
	Depenses      []CarnetLigne
	TotalRecettes float64
	TotalDepenses float64
	Resultat      float64

	Avoirs           []CarnetLigne
	Engagements      []CarnetLigne
	TotalAvoirs      float64
	TotalEngagements float64
	Fortune          float64

	ChiffreAffaires float64
	Eligible        bool
	AssujettiTVA    bool
	Devise          string
}

// CarnetLigne est un poste.
type CarnetLigne struct {
	Code    string
	Libelle string
	Montant float64
}

const (
	margeCarnet  = 18.0
	largeurUtile = 210 - 2*margeCarnet
)

// GenerateCarnet rend la comptabilité simplifiée en PDF.
func GenerateCarnet(d CarnetData) ([]byte, error) {
	if d.Devise == "" {
		d.Devise = "CHF"
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(margeCarnet, margeCarnet, margeCarnet)
	// Saut de page automatique, contrairement à la facture : le carnet n'a
	// aucun élément à position fixe en bas de page, et un plan comptable
	// fourni peut dépasser une feuille.
	pdf.SetAutoPageBreak(true, 20)

	// « Page n/N » : sur une pièce que le CO art. 958f impose de conserver dix
	// ans, c'est ce qui permet de constater qu'il n'en manque pas.
	pdf.AliasNbPages("{nb}")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(largeurUtile/2, 5,
			latin1("Établi par LedgerAlps — comptabilité en partie double"),
			"", 0, "L", false, 0, "")
		pdf.CellFormat(largeurUtile/2, 5,
			latin1(fmt.Sprintf("Page %d/{nb}", pdf.PageNo())),
			"", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()
	carnetEntete(pdf, d)
	carnetSection(pdf, "RECETTES", d.Recettes, "Total des recettes", d.TotalRecettes, d.Devise)
	carnetSection(pdf, "DÉPENSES", d.Depenses, "Total des dépenses", d.TotalDepenses, d.Devise)
	carnetResultat(pdf, d)
	carnetPatrimoine(pdf, d)
	carnetMentions(pdf, d)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("rendu du PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func carnetEntete(pdf *fpdf.Fpdf, d CarnetData) {
	pdf.SetFont("Helvetica", "B", 15)
	pdf.CellFormat(largeurUtile, 8, latin1("Comptabilité simplifiée"), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 90, 90)
	pdf.CellFormat(largeurUtile, 5,
		latin1("Recettes, dépenses et état du patrimoine — CO art. 957 al. 2 ch. 1"),
		"", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(largeurUtile, 6, latin1(d.Entreprise), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	if d.Adresse != "" {
		pdf.CellFormat(largeurUtile, 5, latin1(d.Adresse), "", 1, "L", false, 0, "")
	}
	if d.IDE != "" {
		pdf.CellFormat(largeurUtile, 5, latin1("N° IDE : "+d.IDE), "", 1, "L", false, 0, "")
	}
	pdf.Ln(2)

	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(largeurUtile, 6,
		latin1(fmt.Sprintf("Exercice du %s au %s", d.Du, d.Au)), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// La mention de la base caisse, en tête et non en note de bas de page.
	//
	// C'est elle qui explique l'écart entre le total des recettes et le chiffre
	// d'affaires — les factures émises non encaissées. Un lecteur qui ne l'a
	// pas lue croit à une erreur.
	pdf.SetFillColor(243, 245, 248)
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.MultiCell(largeurUtile, 4.5, latin1(
		"Établi selon le principe des recettes et des dépenses (base de caisse) : "+
			"les montants sont comptés au moment de l'encaissement et du décaissement. "+
			"Les factures émises et non encore encaissées n'y figurent donc pas ; elles "+
			"apparaissent au chiffre d'affaires et à l'état du patrimoine."),
		"", "L", true)
	pdf.Ln(4)
}

func carnetSection(pdf *fpdf.Fpdf, titre string, lignes []CarnetLigne,
	libelleTotal string, total float64, devise string) {

	pdf.SetFont("Helvetica", "B", 10.5)
	pdf.SetFillColor(232, 236, 241)
	pdf.CellFormat(largeurUtile, 7, latin1("  "+titre), "", 1, "L", true, 0, "")

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(110, 110, 110)
	pdf.CellFormat(22, 5, latin1("Compte"), "", 0, "L", false, 0, "")
	pdf.CellFormat(largeurUtile-22-32, 5, latin1("Libellé"), "", 0, "L", false, 0, "")
	pdf.CellFormat(32, 5, latin1(devise), "", 1, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	pdf.SetFont("Helvetica", "", 9.5)
	if len(lignes) == 0 {
		pdf.SetTextColor(140, 140, 140)
		pdf.CellFormat(largeurUtile, 6, latin1("  Aucun mouvement sur la période"),
			"", 1, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}
	for _, l := range lignes {
		pdf.CellFormat(22, 5.5, latin1(l.Code), "", 0, "L", false, 0, "")
		pdf.CellFormat(largeurUtile-22-32, 5.5, latin1(l.Libelle), "", 0, "L", false, 0, "")
		pdf.CellFormat(32, 5.5, latin1(montantCarnet(l.Montant)), "", 1, "R", false, 0, "")
	}

	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetDrawColor(180, 180, 180)
	pdf.CellFormat(largeurUtile-32, 7, latin1(libelleTotal), "T", 0, "R", false, 0, "")
	pdf.CellFormat(32, 7, latin1(montantCarnet(total)), "T", 1, "R", false, 0, "")
	pdf.Ln(3)
}

func carnetResultat(pdf *fpdf.Fpdf, d CarnetData) {
	pdf.SetFont("Helvetica", "B", 11.5)
	pdf.SetFillColor(28, 54, 86) // bleu nuit de la marque
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(largeurUtile-32, 9, latin1("  RÉSULTAT DE L'EXERCICE"), "", 0, "L", true, 0, "")
	pdf.CellFormat(32, 9, latin1(montantCarnet(d.Resultat)+" "), "", 1, "R", true, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(6)
}

func carnetPatrimoine(pdf *fpdf.Fpdf, d CarnetData) {
	pdf.SetFont("Helvetica", "B", 10.5)
	pdf.SetFillColor(232, 236, 241)
	pdf.CellFormat(largeurUtile, 7,
		latin1("  ÉTAT DU PATRIMOINE AU "+d.Au), "", 1, "L", true, 0, "")
	pdf.Ln(1)

	carnetBloc(pdf, "Avoirs", d.Avoirs, "Total des avoirs", d.TotalAvoirs, d.Devise)
	carnetBloc(pdf, "Engagements", d.Engagements, "Total des engagements", d.TotalEngagements, d.Devise)

	pdf.SetFont("Helvetica", "B", 10.5)
	pdf.SetDrawColor(120, 120, 120)
	pdf.CellFormat(largeurUtile-32, 8, latin1("FORTUNE NETTE"), "T", 0, "R", false, 0, "")
	pdf.CellFormat(32, 8, latin1(montantCarnet(d.Fortune)), "T", 1, "R", false, 0, "")
	pdf.Ln(5)
}

func carnetBloc(pdf *fpdf.Fpdf, titre string, lignes []CarnetLigne,
	libelleTotal string, total float64, devise string) {

	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.CellFormat(largeurUtile, 6, latin1(titre), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 9.5)
	if len(lignes) == 0 {
		pdf.SetTextColor(140, 140, 140)
		pdf.CellFormat(largeurUtile, 5.5, latin1("  Néant"), "", 1, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}
	for _, l := range lignes {
		pdf.CellFormat(22, 5.5, latin1(l.Code), "", 0, "L", false, 0, "")
		pdf.CellFormat(largeurUtile-22-32, 5.5, latin1(l.Libelle), "", 0, "L", false, 0, "")
		pdf.CellFormat(32, 5.5, latin1(montantCarnet(l.Montant)), "", 1, "R", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetDrawColor(210, 210, 210)
	pdf.CellFormat(largeurUtile-32, 6, latin1(libelleTotal), "T", 0, "R", false, 0, "")
	pdf.CellFormat(32, 6, latin1(montantCarnet(total)), "T", 1, "R", false, 0, "")
	pdf.Ln(3)
}

// carnetMentions porte ce que le document doit dire de lui-même.
//
// Un carnet du lait remis par une entreprise qui a dépassé le seuil est un
// document que la loi ne reconnaît pas dans son cas. Le PDF le dit, plutôt que
// de laisser croire qu'il suffit.
func carnetMentions(pdf *fpdf.Fpdf, d CarnetData) {
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.CellFormat(largeurUtile, 6, latin1("Régime applicable"), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(largeurUtile-32, 5.5,
		latin1("Chiffre d'affaires de l'exercice"), "", 0, "L", false, 0, "")
	pdf.CellFormat(32, 5.5, latin1(montantCarnet(d.ChiffreAffaires)), "", 1, "R", false, 0, "")

	pdf.SetFont("Helvetica", "", 8.5)
	if d.Eligible {
		pdf.SetTextColor(30, 100, 50)
		pdf.MultiCell(largeurUtile, 4.5, latin1(
			"Chiffre d'affaires inférieur à 500 000 francs : la comptabilité simplifiée "+
				"est admise (CO art. 957 al. 2 ch. 1)."), "", "L", false)
	} else {
		pdf.SetTextColor(150, 40, 30)
		pdf.MultiCell(largeurUtile, 4.5, latin1(
			"ATTENTION — le chiffre d'affaires atteint ou dépasse 500 000 francs. "+
				"La comptabilité en partie double et les comptes annuels sont obligatoires "+
				"(CO art. 957 al. 1) : ce document ne peut pas être présenté seul."), "", "L", false)
	}
	pdf.SetTextColor(0, 0, 0)

	pdf.SetFont("Helvetica", "", 8.5)
	if d.AssujettiTVA {
		pdf.MultiCell(largeurUtile, 4.5, latin1(
			"Chiffre d'affaires égal ou supérieur à 100 000 francs : assujettissement à la TVA "+
				"(LTVA art. 10). Le décompte TVA accompagne ce document."), "", "L", false)
	} else {
		pdf.MultiCell(largeurUtile, 4.5, latin1(
			"Chiffre d'affaires inférieur à 100 000 francs : libération de l'assujettissement "+
				"à la TVA (LTVA art. 10 al. 2 let. a)."), "", "L", false)
	}
}

// montantCarnet formate un montant à la suisse : apostrophe pour les milliers.
//
// « 12'000.00 » et non « 12000.00 » : c'est la forme qu'un document remis à
// l'administration suisse doit porter, et celle que le lecteur vérifie d'un
// coup d'œil.
func montantCarnet(v float64) string {
	negatif := v < 0
	if negatif {
		v = -v
	}
	s := fmt.Sprintf("%.2f", v)
	entier, decimales := s[:len(s)-3], s[len(s)-2:]

	var out []byte
	for i, c := range []byte(entier) {
		if i > 0 && (len(entier)-i)%3 == 0 {
			out = append(out, '\'')
		}
		out = append(out, c)
	}
	res := string(out) + "." + decimales
	if negatif {
		return "-" + res
	}
	return res
}

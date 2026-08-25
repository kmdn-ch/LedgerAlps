package pdf

// Le PDF du carnet du lait.
//
// Un test de rendu ne peut pas juger de la mise en page — il faut un œil pour
// cela. Il peut en revanche tenir ce qui rendrait le document FAUX : une
// mention légale absente, un avertissement qui ne se déclenche pas au-delà du
// seuil, un montant illisible. C'est ce qu'il vérifie.

import (
	"bytes"
	"compress/zlib"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/i18n"
)

func carnetExemple() CarnetData {
	return CarnetData{
		Entreprise: "Dupont Menuiserie",
		Adresse:    "Rue du Lac 3, 1000 Lausanne",
		IDE:        "CHE-123.456.789",
		Du:         "2026-01-01",
		Au:         "2026-12-31",
		Recettes: []CarnetLigne{
			{Code: "3000", Libelle: "Produits des ventes", Montant: 12000},
		},
		Depenses: []CarnetLigne{
			{Code: "4000", Libelle: "Achats de marchandises", Montant: 2200},
			{Code: "6000", Libelle: "Loyer des locaux", Montant: 3600},
		},
		TotalRecettes: 12000,
		TotalDepenses: 5800,
		Resultat:      6200,
		Avoirs: []CarnetLigne{
			{Code: "1020", Libelle: "Banque", Montant: 5700},
		},
		TotalAvoirs:     5700,
		Fortune:         5700,
		ChiffreAffaires: 20000,
		Eligible:        true,
		Devise:          "CHF",
	}
}

// Le document se produit et ressemble à un PDF.
func TestLeCarnetProduitUnPDF(t *testing.T) {
	b, err := GenerateCarnet(carnetExemple())
	if err != nil {
		t.Fatalf("GenerateCarnet: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Errorf("la sortie ne commence pas par l'en-tête PDF : %q", b[:min(8, len(b))])
	}
	if len(b) < 1000 {
		t.Errorf("PDF de %d octets — trop court pour porter trois sections", len(b))
	}
}

// Les mentions légales sont dans le document.
//
// Un document fiscal qui ne dit pas sous quel régime il est établi oblige le
// lecteur à le deviner ; et la base de caisse doit être annoncée, sans quoi
// l'écart entre les recettes et le chiffre d'affaires passe pour une erreur.
func TestLeCarnetPorteSesMentionsLegales(t *testing.T) {
	b, err := GenerateCarnet(carnetExemple())
	if err != nil {
		t.Fatalf("GenerateCarnet: %v", err)
	}
	texte := texteDuPDF(b)

	for _, attendu := range []string{
		"957",      // la base légale
		"caisse",   // le principe de comptabilisation
		"RECETTES", //
		"PENSES",   // « DÉPENSES », accent encodé
		"PATRIMOINE",
		"500 000", // le seuil du CO
	} {
		if !strings.Contains(texte, attendu) {
			t.Errorf("le document ne porte pas %q", attendu)
		}
	}
}

// LE test qui protège l'utilisateur : au-delà du seuil, le document dit qu'il
// ne suffit pas.
//
// Remettre un carnet du lait quand la partie double est obligatoire, c'est
// remettre un document que la loi ne reconnaît pas dans son cas. Le taire
// serait le pire service à rendre.
func TestAuDelaDuSeuilLeDocumentAvertit(t *testing.T) {
	d := carnetExemple()
	d.ChiffreAffaires = 620000
	d.Eligible = false

	b, err := GenerateCarnet(d)
	if err != nil {
		t.Fatalf("GenerateCarnet: %v", err)
	}
	texte := texteDuPDF(b)

	if !strings.Contains(texte, "ATTENTION") {
		t.Error("aucun avertissement alors que le seuil de 500 000 est dépassé")
	}
	if !strings.Contains(texte, "partie double") {
		t.Error("le document ne dit pas ce qui devient obligatoire")
	}
}

// Le décompte TVA est annoncé quand il devient obligatoire.
func TestLAssujettissementTVAEstAnnonce(t *testing.T) {
	d := carnetExemple()
	d.ChiffreAffaires = 250000
	d.AssujettiTVA = true

	b, err := GenerateCarnet(d)
	if err != nil {
		t.Fatalf("GenerateCarnet: %v", err)
	}
	if texte := texteDuPDF(b); !strings.Contains(texte, "100 000") {
		t.Error("le seuil d'assujettissement TVA n'est pas mentionné")
	}
}

// Les montants sont écrits à la suisse.
func TestLesMontantsSontALaSuisse(t *testing.T) {
	cas := map[float64]string{
		0:          "0.00",
		12.5:       "12.50",
		1000:       "1'000.00",
		12000:      "12'000.00",
		1234567.89: "1'234'567.89",
		-1500:      "-1'500.00",
	}
	for v, attendu := range cas {
		if got := montantCarnet(v); got != attendu {
			t.Errorf("montantCarnet(%.2f) = %q, attendu %q", v, got, attendu)
		}
	}
}

// Un carnet vide se produit quand même : une entreprise qui n'a rien encaissé
// doit pouvoir remettre un document, pas se heurter à une erreur.
func TestUnCarnetVideSeProduitQuandMeme(t *testing.T) {
	b, err := GenerateCarnet(CarnetData{
		Entreprise: "Nouvelle entreprise", Du: "2026-01-01", Au: "2026-12-31", Eligible: true,
	})
	if err != nil {
		t.Fatalf("un carnet vide échoue: %v", err)
	}
	if texte := texteDuPDF(b); !strings.Contains(texte, "Aucun mouvement") {
		t.Error("le document ne dit pas qu'il n'y a eu aucun mouvement")
	}
}

// texteDuPDF extrait le texte lisible du PDF, flux décompressés compris.
//
// gofpdf compresse les flux de contenu : lire les octets bruts ne rend que du
// binaire. On décompresse donc chaque flux avant d'y chercher les chaînes
// affichées, qui apparaissent entre parenthèses dans les opérateurs de texte.
//
// C'est suffisant pour vérifier qu'une mention EST présente — pas pour juger de
// sa position ni de sa lisibilité, qui demandent un œil.
func texteDuPDF(b []byte) string {
	var out strings.Builder
	out.WriteString(chainesEntreParentheses(b))

	reste := b
	for {
		i := bytes.Index(reste, []byte("stream"))
		if i < 0 {
			break
		}
		debut := i + len("stream")
		for debut < len(reste) && (reste[debut] == '\r' || reste[debut] == '\n') {
			debut++
		}
		fin := bytes.Index(reste[debut:], []byte("endstream"))
		if fin < 0 {
			break
		}
		if r, err := zlib.NewReader(bytes.NewReader(reste[debut : debut+fin])); err == nil {
			if clair, err := io.ReadAll(r); err == nil {
				out.WriteString(chainesEntreParentheses(clair))
			}
			r.Close()
		}
		reste = reste[debut+fin:]
	}
	return out.String()
}

// chainesEntreParentheses rend le contenu des chaînes PDF.
func chainesEntreParentheses(b []byte) string {
	var out strings.Builder
	dans := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if dans && c == '\\' && i+1 < len(b) {
			i++
			out.WriteByte(b[i])
			continue
		}
		switch {
		case c == '(':
			dans = true
		case c == ')':
			dans = false
			out.WriteByte(' ')
		case dans:
			out.WriteByte(c)
		}
	}
	return out.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// La ponctuation typographique française ne doit pas sortir en « ? ».
//
// Le générateur PDF n'accepte que du Latin-1, où le tiret cadratin et
// l'apostrophe typographique n'existent pas. Sans translittération, un texte
// français ordinaire produit « Recettes ? CO art. 957 » — et un document dont
// la ponctuation est cassée fait douter des chiffres qu'il porte.
func TestLaPonctuationTypographiqueEstTranslitteree(t *testing.T) {
	cas := map[string]string{
		"a — b":      "a - b",
		"a – b":      "a - b",
		"l’exercice": "l'exercice",
		"“citation”": "\"citation\"",
		"suite…":     "suite...",
		"12 €":       "12 EUR",
		// Ce qui EST dans Latin-1 doit passer intact.
		"déjà payé":      "déjà payé",
		"« guillemets »": "« guillemets »",
	}
	for entree, attendu := range cas {
		got := latin1(entree)
		if got != latin1BrutAttendu(attendu) {
			t.Errorf("latin1(%q) = %q, attendu %q", entree, got, attendu)
		}
	}
}

// latin1BrutAttendu encode l'attendu de la même façon, pour comparer des
// octets à des octets.
func latin1BrutAttendu(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 0x100 {
			b = append(b, byte(r))
		}
	}
	return string(b)
}

// Le carnet sort dans la langue de l'interface au moment du clic.
//
// C'est la pièce que l'on tend à une administration cantonale : établie depuis
// un écran allemand, elle doit être en allemand. Et les références légales
// changent de NOM, pas seulement de langue — le Code des obligations est l'OR
// en allemand, la loi sur la TVA est la MWSTG. Un document remis à Zurich qui
// citerait « CO art. 957 » citerait une loi qui n'y porte pas ce nom.
func TestLeCarnetSortDansLaLangueDemandee(t *testing.T) {
	cas := []struct {
		lang     i18n.Lang
		attendus []string
		absents  []string
	}{
		{i18n.FR, []string{"Comptabilit", "RECETTES", "CO art. 957"}, []string{"EINNAHMEN", "RECEIPTS"}},
		{i18n.DE, []string{"Vereinfachte", "EINNAHMEN", "OR Art. 957", "MWSTG"}, []string{"RECETTES", "RECEIPTS"}},
		{i18n.IT, []string{"semplificata", "ENTRATE", "CO art. 957 cpv", "LIVA"}, []string{"RECETTES", "EINNAHMEN"}},
		{i18n.EN, []string{"Simplified", "RECEIPTS", "CO art. 957 para", "VAT Act"}, []string{"RECETTES", "EINNAHMEN"}},
	}
	for _, c := range cas {
		t.Run(string(c.lang), func(t *testing.T) {
			d := carnetExemple()
			d.Langue = c.lang
			b, err := GenerateCarnet(d)
			if err != nil {
				t.Fatalf("GenerateCarnet(%s): %v", c.lang, err)
			}
			texte := texteDuPDF(b)
			for _, a := range c.attendus {
				if !strings.Contains(texte, a) {
					t.Errorf("%s : le document ne porte pas %q", c.lang, a)
				}
			}
			for _, x := range c.absents {
				if strings.Contains(texte, x) {
					t.Errorf("%s : le document porte encore %q — une autre langue a fui", c.lang, x)
				}
			}
		})
	}
}

// Une langue inconnue retombe sur le français, la langue des sources.
func TestUneLangueInconnueRetombeSurLeFrancais(t *testing.T) {
	d := carnetExemple()
	d.Langue = i18n.Lang("es")
	b, err := GenerateCarnet(d)
	if err != nil {
		t.Fatalf("GenerateCarnet: %v", err)
	}
	if !strings.Contains(texteDuPDF(b), "RECETTES") {
		t.Error("une langue inconnue ne retombe pas sur le français")
	}
}

// L'avertissement du dépassement de seuil existe dans les quatre langues.
//
// C'est la mention qui protège l'utilisateur : la taire dans une langue
// laisserait quelqu'un remettre un document que la loi ne reconnaît pas.
func TestLAvertissementExisteDansLesQuatreLangues(t *testing.T) {
	motsAlerte := map[i18n.Lang]string{
		i18n.FR: "ATTENTION", i18n.DE: "ACHTUNG",
		i18n.IT: "ATTENZIONE", i18n.EN: "WARNING",
	}
	for lang, mot := range motsAlerte {
		d := carnetExemple()
		d.Langue = lang
		d.ChiffreAffaires = 620000
		d.Eligible = false
		b, err := GenerateCarnet(d)
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		if !strings.Contains(texteDuPDF(b), mot) {
			t.Errorf("%s : pas d'avertissement au-delà du seuil (attendu %q)", lang, mot)
		}
	}
}

// Le nom de fichier suit la langue : c'est par lui qu'on reconnaît le document
// dans un dossier de téléchargements.
func TestLeNomDeFichierSuitLaLangue(t *testing.T) {
	attendus := map[i18n.Lang]string{
		i18n.FR: "comptabilite-simplifiee",
		i18n.DE: "vereinfachte-buchhaltung",
		i18n.IT: "contabilita-semplificata",
		i18n.EN: "simplified-accounting",
	}
	for lang, attendu := range attendus {
		if got := NomFichierCarnet(lang); got != attendu {
			t.Errorf("%s : nom = %q, attendu %q", lang, got, attendu)
		}
	}
}

// Aucun libellé ne doit rester vide dans une langue : un champ oublié
// produirait un document troué, et personne ne le verrait avant de le remettre.
func TestAucunLibelleNEstVide(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.FR, i18n.DE, i18n.IT, i18n.EN} {
		L := libellés(lang)
		v := reflect.ValueOf(L)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).String() == "" {
				t.Errorf("%s : le libellé %q est vide", lang, v.Type().Field(i).Name)
			}
		}
	}
}

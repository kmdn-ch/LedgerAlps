package qrbill

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"runtime"
	"testing"
)

// Audit croisé Gemini, HIGH-12 : un PDF de 200 Ko faisait passer la mémoire du serveur de 35 à
// 820 Mo, un de 1,5 Mo à plus de 2 Go — pdfcpu décode chaque flux en mémoire, et le plafond
// d'envoi borne le fichier, pas son expansion.

// bombePDF fabrique un PDF VALIDE d'une page dont `flux` flux de contenu, compressés en Flate,
// se dilatent chacun en `dilate` octets de zéros. Valide : pdfcpu l'ouvre, et il décompresserait
// donc vraiment — un fichier malformé échouerait tôt et ne prouverait rien sur la mémoire.
func bombePDF(t *testing.T, flux int, dilate int) []byte {
	t.Helper()
	var comprime bytes.Buffer
	zw := zlib.NewWriter(&comprime)
	bloc := make([]byte, 1<<20)
	for ecrit := 0; ecrit < dilate; ecrit += len(bloc) {
		if _, err := zw.Write(bloc); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	var pdf bytes.Buffer
	var offsets []int
	objet := func(corps string) {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", len(offsets), corps)
	}
	pdf.WriteString("%PDF-1.4\n")
	objet("<< /Type /Catalog /Pages 2 0 R >>")
	objet("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	contenus := ""
	for i := 0; i < flux; i++ {
		contenus += fmt.Sprintf(" %d 0 R", 4+i)
	}
	objet("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents [" + contenus + " ] /Resources << >> >>")
	for i := 0; i < flux; i++ {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(offsets), comprime.Len())
		pdf.Write(comprime.Bytes())
		pdf.WriteString("\nendstream\nendobj\n")
	}
	debutXref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, debutXref)
	return pdf.Bytes()
}

// abaisser réduit les budgets pour la durée du test : les vraies valeurs (64 / 256 Mo) ne
// changent que l'échelle, et demanderaient de décompresser des centaines de méga-octets.
func abaisser(t *testing.T, flux, total int64) {
	t.Helper()
	f, tot := expansionFluxMax, expansionTotaleMax
	t.Cleanup(func() { expansionFluxMax, expansionTotaleMax = f, tot })
	expansionFluxMax, expansionTotaleMax = flux, total
}

func TestUnFluxQuiSeDilateDemesurementEstRefuse(t *testing.T) {
	abaisser(t, 8<<20, 20<<20)

	pdf := bombePDF(t, 1, 40<<20) // 40 Mo de zéros : quelques dizaines de Ko compressés
	if len(pdf) > 200<<10 {
		t.Fatalf("préambule : la bombe pèse %d Ko, elle devait rester petite", len(pdf)>>10)
	}

	var avant, apres runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&avant)
	_, err := DecodePDF(pdf)
	runtime.ReadMemStats(&apres)

	if !errors.Is(err, ErrExpansionDemesuree) {
		t.Fatalf("DecodePDF : %v, attendu ErrExpansionDemesuree", err)
	}
	// Refusé AVANT pdfcpu : la mémoire n'a pas suivi l'expansion (40 Mo ici, des centaines en vrai).
	if delta := int64(apres.TotalAlloc) - int64(avant.TotalAlloc); delta > 16<<20 {
		t.Errorf("%d Mo alloués pour refuser la bombe : la décompression n'est pas bornée", delta>>20)
	}
	if _, err := extractText(pdf); !errors.Is(err, ErrExpansionDemesuree) {
		t.Errorf("extractText : %v, attendu ErrExpansionDemesuree — le chemin texte n'est pas gardé", err)
	}
}

// Plusieurs flux admissibles dont la SOMME dépasse le budget : refusé aussi.
func TestLaSommeDesExpansionsEstBornee(t *testing.T) {
	abaisser(t, 8<<20, 20<<20)

	// Quatre flux de 6 Mo : chacun passe, les 24 Mo cumulés non.
	if _, err := DecodePDF(bombePDF(t, 4, 6<<20)); !errors.Is(err, ErrExpansionDemesuree) {
		t.Fatalf("quatre flux de 6 Mo : %v, attendu ErrExpansionDemesuree", err)
	}
	// Deux flux de 6 Mo : dans le budget, donc pas refusés pour ce motif (ils ne portent pas de
	// QR : la réponse normale est « aucun QR »).
	if _, err := DecodePDF(bombePDF(t, 2, 6<<20)); errors.Is(err, ErrExpansionDemesuree) {
		t.Fatalf("deux flux de 6 Mo refusés alors qu'ils tiennent dans le budget")
	}
}

// Un document ordinaire — une facture avec son QR — n'est pas touché.
func TestUnePDFOrdinaireNEstPasRefusee(t *testing.T) {
	pdf := pdfAvecImages(t, [][]byte{imageQR(t, 300)})
	if err := verifierExpansion(pdf); err != nil {
		t.Fatalf("une facture ordinaire est refusée : %v", err)
	}
	if bill, err := DecodePDF(pdf); err != nil || bill == nil {
		t.Fatalf("le QR d'une facture ordinaire n'est plus lu : %v", err)
	}
}

// « endstream » contient « stream » : la balise de fermeture n'est pas une ouverture.
func TestLeBalayageNeConfondPasEndstreamEtStream(t *testing.T) {
	if err := verifierExpansion([]byte("%PDF endstream endstream stream\nabc")); err != nil {
		t.Fatalf("document sans flux Flate refusé : %v", err)
	}
}

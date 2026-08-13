package pdf

// L'accent qui disparaît.
//
// Les factures portaient « B?n?ficiaire » au lieu de « Bénéficiaire ». La cause
// n'était pas une police manquante mais un encodage appliqué DEUX FOIS : le
// premier passage transforme « é » (U+00E9) en octet 0xE9, ce qui n'est plus de
// l'UTF-8 valide ; le second lit cet octet comme un caractère de remplacement,
// hors de la plage Latin-1, et écrit « ? ».
//
// Le défaut ne se voyait que sur le papier, et seulement sur les mots accentués.

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLEncodageNEstPasIdempotent(t *testing.T) {
	une := latin1("Bénéficiaire")
	deux := latin1(une)
	if !strings.Contains(deux, "?") {
		t.Skip("latin1 est devenue idempotente : ce test n'a plus d'objet")
	}
	// C'est bien le double appel qui casse, pas la fonction elle-même.
	if strings.Contains(une, "?") {
		t.Fatal("un seul passage abîme déjà le texte")
	}
}

// Le vrai contrôle : le code source ne doit plus encoder deux fois. Les
// fonctions d'affichage encodent déjà leurs arguments.
func TestAucunAppelNEncodeDeuxFois(t *testing.T) {
	src, err := os.ReadFile("service.go")
	if err != nil {
		t.Skipf("source illisible: %v", err)
	}
	for _, fautif := range []string{
		"metaRow(latin1(", "totalRow(latin1(", "line(latin1(", "cell(latin1(",
	} {
		if strings.Contains(string(src), fautif) {
			t.Errorf("%s… : le texte est encodé une seconde fois par la fonction appelée, "+
				"ce qui remplace chaque accent par « ? »", fautif)
		}
	}
}

// L'accent qui SURVIT — le contrôle que le test ci-dessus ne faisait pas.
//
// Interdire le double encodage ne dit rien de l'ABSENCE d'encodage. C'est ce
// trou qui a laissé « NÂ° facture: » et « Ã‰chÃ©ance: » sur toutes les
// factures : `metaRow` écrivait ses arguments bruts, et aucun test ne regardait
// le PDF produit.
//
// # Pourquoi il faut décompresser
//
// Ma première version cherchait les octets dans le fichier tel quel. Elle
// passait au vert alors que le défaut était réintroduit exprès : gofpdf
// comprime les flux de contenu, si bien qu'aucune séquence d'octets lisible
// n'y figure. Un test qui ne peut pas échouer ne prouve rien — celui-ci
// décompresse d'abord, et sa capacité à échouer a été vérifiée en retirant
// latin1() de metaRow.
func TestLesAccentsSortentEnLatin1(t *testing.T) {
	sortie, err := Generate(InvoiceData{
		InvoiceNumber: "FA-2026-0001",
		DocumentType:  "invoice",
		IssueDate:     time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
		DueDate:       time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Currency:      "CHF",
		TotalAmount:   100,
		Lines: []InvoiceLine{{
			Description: "Réparation d'été",
			Quantity:    1, UnitPrice: 100, LineTotal: 100,
		}},
		Company:  CompanyInfo{Name: "Alpes SA"},
		Customer: CustomerInfo{Name: "Müller AG"},
	})
	if err != nil {
		t.Fatalf("génération: %v", err)
	}

	contenu := fluxDécompressés(sortie)
	if len(contenu) == 0 {
		t.Fatal("aucun flux lisible dans le PDF — le test ne peut rien vérifier")
	}

	// Une paire UTF-8 dans le contenu signifie qu'un texte est passé sans
	// conversion : c'est exactement ce que le lecteur affichera en « Ã© ».
	for _, cas := range []struct {
		octets []byte
		mot    string
	}{
		{[]byte{0xC3, 0xA9}, "« é » (Ã©)"},
		{[]byte{0xC2, 0xB0}, "« ° » (Â°)"},
		{[]byte{0xC3, 0xBC}, "« ü » (Ã¼)"},
	} {
		if bytes.Contains(contenu, cas.octets) {
			t.Errorf("le PDF porte la paire UTF-8 de %s : un texte a échappé à latin1()", cas.mot)
		}
	}
}

// fluxDécompressés rend le contenu de tous les flux du PDF, dézippés.
func fluxDécompressés(pdf []byte) []byte {
	var out bytes.Buffer
	reste := pdf
	for {
		i := bytes.Index(reste, []byte("stream"))
		if i < 0 {
			break
		}
		j := bytes.Index(reste[i:], []byte("endstream"))
		if j < 0 {
			break
		}
		brut := bytes.TrimLeft(reste[i+len("stream"):i+j], "\r\n")
		if r, err := zlib.NewReader(bytes.NewReader(brut)); err == nil {
			if clair, err := io.ReadAll(r); err == nil {
				out.Write(clair)
			}
			_ = r.Close()
		} else {
			out.Write(brut)
		}
		reste = reste[i+j:]
	}
	return out.Bytes()
}

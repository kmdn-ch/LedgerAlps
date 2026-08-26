package qrbill

// Le chemin PDF, de bout en bout.
//
// C'est la porte la plus réaliste du produit pour une facture reçue par
// courriel : le comptable dépose le PDF, LedgerAlps y cherche le QR. Rien ne
// l'exerçait — les tests existants s'arrêtent à `DecodeImage`, qui reçoit déjà
// une image toute prête et ne passe donc ni par pdfcpu, ni par l'extraction
// page par page, ni par les bornes qui la protègent.

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	gofpdf "github.com/go-pdf/fpdf"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// imageQR rend un PNG portant le bulletin de test, à la taille demandée.
func imageQR(t *testing.T, cote int) []byte {
	t.Helper()
	enc := qrcode.NewQRCodeWriter()
	matrix, err := enc.Encode(bulletinQRR, gozxing.BarcodeFormat_QR_CODE, cote, cote, nil)
	if err != nil {
		t.Fatalf("encodage QR : %v", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, matrix); err != nil {
		t.Fatalf("encodage PNG : %v", err)
	}
	return buf.Bytes()
}

// pdfAvecImages fabrique un PDF d'une page par image fournie.
//
// Les images sont posées à leur taille naturelle : ce qui compte ici est
// qu'elles se retrouvent dans le document et en ressortent, pas la mise en page.
func pdfAvecImages(t *testing.T, pages [][]byte) []byte {
	t.Helper()
	doc := gofpdf.New("P", "mm", "A4", "")
	for i, img := range pages {
		doc.AddPage()
		nom := filepath.Join("img", string(rune('a'+i)))
		doc.RegisterImageOptionsReader(nom,
			gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: false},
			bytes.NewReader(img))
		doc.ImageOptions(nom, 20, 20, 60, 60,
			false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	}
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatalf("génération du PDF : %v", err)
	}
	return out.Bytes()
}

// Un PDF d'une page portant le bulletin QR se lit.
func TestUnQRDansUnPDFDUnePageSeLit(t *testing.T) {
	doc := pdfAvecImages(t, [][]byte{imageQR(t, 400)})

	b, err := DecodePDF(doc)
	if err != nil {
		t.Fatalf("DecodePDF : %v", err)
	}
	if b.CreditorIBAN != "CH4431999123000889012" {
		t.Errorf("IBAN du créancier = %q", b.CreditorIBAN)
	}
	if b.Amount != 1621.50 {
		t.Errorf("montant = %v, attendu 1621.50", b.Amount)
	}
}

// Le QR est trouvé même s'il n'est PAS sur la première page.
//
// C'est ce que l'extraction page par page pourrait casser si elle s'arrêtait
// trop tôt : le bulletin QR suisse est souvent sur la dernière page d'une
// facture de plusieurs pages, détachable.
func TestUnQRSurLaTroisiemePageSeLitQuandMeme(t *testing.T) {
	// Deux pages d'illustration quelconque, puis le bulletin.
	decor := imageQR(t, 80) // un QR minuscule fait un décor honnête et léger
	doc := pdfAvecImages(t, [][]byte{decor, decor, imageQR(t, 400)})

	b, err := DecodePDF(doc)
	if err != nil {
		t.Fatalf("DecodePDF : %v", err)
	}
	if b.CreditorIBAN != "CH4431999123000889012" {
		t.Errorf("IBAN du créancier = %q — le QR de la 3e page n'a pas été atteint", b.CreditorIBAN)
	}
}

// Un PDF sans aucune image le dit clairement, plutôt que d'échouer sur un
// message technique.
func TestUnPDFSansImageDitQuIlNYAPasDeQR(t *testing.T) {
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	doc.Cell(40, 10, "Facture sans bulletin")
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatal(err)
	}

	if _, err := DecodePDF(out.Bytes()); err == nil {
		t.Fatal("aucune erreur pour un PDF sans image")
	}
}

// Un fichier qui n'est pas un PDF est refusé.
func TestUnFichierQuiNEstPasUnPDFEstRefuse(t *testing.T) {
	if _, err := DecodePDF([]byte("ceci n'est pas un PDF")); err == nil {
		t.Fatal("aucune erreur pour un fichier qui n'est pas un PDF")
	}
}

// Un fichier au-delà du plafond est refusé AVANT toute extraction.
func TestUnPDFTropVolumineuxEstRefuseAvantExtraction(t *testing.T) {
	trop := make([]byte, MaxPDFBytes+1)
	if _, err := DecodePDF(trop); err == nil {
		t.Fatal("aucune erreur pour un fichier dépassant MaxPDFBytes")
	}
}

// tailleDuDossier somme bien ce qui est écrit, et ignore les sous-dossiers.
//
// C'est la mesure sur laquelle repose la borne disque : si elle rendait zéro,
// la borne ne se déclencherait jamais et la protection serait décorative.
func TestTailleDuDossierSommeLesFichiers(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), make([]byte, 2500), 0o600); err != nil {
		t.Fatal(err)
	}
	// Un sous-dossier, avec un fichier dedans : pdfcpu écrit à plat, et
	// descendre coûterait sans rien apporter.
	sous := filepath.Join(dir, "sous")
	if err := os.MkdirAll(sous, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sous, "c.bin"), make([]byte, 9999), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := tailleDuDossier(dir)
	if err != nil {
		t.Fatalf("tailleDuDossier : %v", err)
	}
	if got != 3500 {
		t.Errorf("taille = %d, attendu 3500 (les fichiers à plat, sans le sous-dossier)", got)
	}
}

// Un dossier vide mesure zéro — et non une erreur.
func TestTailleDUnDossierVideEstZero(t *testing.T) {
	got, err := tailleDuDossier(t.TempDir())
	if err != nil {
		t.Fatalf("tailleDuDossier : %v", err)
	}
	if got != 0 {
		t.Errorf("taille = %d, attendu 0", got)
	}
}

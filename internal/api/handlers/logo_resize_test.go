package handlers

// Ce que ce test protège.
//
// Le logo entre en base par une route ouverte, et il en ressort dans chaque
// réponse de la fiche société, dans les sauvegardes et dans l'archive légale.
// Un contrôle qui ne s'appliquerait qu'au navigateur ne serait pas un contrôle.
//
// Le cas qui compte est le RAPPORT : réduire en déformant transformerait un
// logo horizontal en logo écrasé, ce que personne ne remarque avant de voir sa
// propre facture.

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// imagePNG fabrique une adresse de données PNG de la taille demandée.
func imagePNG(t *testing.T, l, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, l, h))
	for y := 0; y < h; y++ {
		for x := 0; x < l; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 120, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func dimensions(t *testing.T, dataURL string) (int, int) {
	t.Helper()
	i := strings.IndexByte(dataURL, ',')
	brut, err := base64.StdEncoding.DecodeString(dataURL[i+1:])
	if err != nil {
		t.Fatalf("base64 : %v", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(brut))
	if err != nil {
		t.Fatalf("décodage : %v", err)
	}
	return cfg.Width, cfg.Height
}

// Une image déjà dans la limite ressort octet pour octet. La ré-encoder
// n'apporterait rien et retirerait ce qu'un PNG optimisé a de mieux.
func TestUnLogoAssezPetitNEstPasTouche(t *testing.T) {
	src := imagePNG(t, 200, 120)

	got, err := ajusterLogo(src)
	if err != nil {
		t.Fatalf("ajusterLogo : %v", err)
	}
	if got.DataURL != src {
		t.Error("l'image a été ré-encodée alors qu'elle tenait déjà dans la limite")
	}
	if got.Redimens {
		t.Error("Redimens = true sur une image qui n'a pas bougé")
	}
	if got.Largeur != 200 || got.Hauteur != 120 {
		t.Errorf("dimensions rendues = %dx%d, attendu 200x120", got.Largeur, got.Hauteur)
	}
}

// LE test : le rapport survit. Un logo deux fois plus large que haut le reste.
func TestUnLogoTropGrandEstReduitSansDeformation(t *testing.T) {
	got, err := ajusterLogo(imagePNG(t, 2000, 1000))
	if err != nil {
		t.Fatalf("ajusterLogo : %v", err)
	}
	if !got.Redimens {
		t.Error("Redimens = false alors que l'image faisait 2000 px de large")
	}

	l, h := dimensions(t, got.DataURL)
	if l != LogoTailleMax {
		t.Errorf("largeur = %d, attendu %d — le côté le plus long touche la limite", l, LogoTailleMax)
	}
	if h != LogoTailleMax/2 {
		t.Errorf("hauteur = %d, attendu %d — le rapport 2:1 n'a pas survécu", h, LogoTailleMax/2)
	}
	if l != got.Largeur || h != got.Hauteur {
		t.Errorf("les dimensions annoncées (%dx%d) ne sont pas celles de l'image (%dx%d)",
			got.Largeur, got.Hauteur, l, h)
	}
}

// Un logo plus haut que large : c'est la HAUTEUR qui touche la limite.
func TestUnLogoVerticalEstReduitParSaHauteur(t *testing.T) {
	got, err := ajusterLogo(imagePNG(t, 600, 1800))
	if err != nil {
		t.Fatalf("ajusterLogo : %v", err)
	}
	l, h := dimensions(t, got.DataURL)
	if h != LogoTailleMax {
		t.Errorf("hauteur = %d, attendu %d", h, LogoTailleMax)
	}
	if l != LogoTailleMax/3 {
		t.Errorf("largeur = %d, attendu %d", l, LogoTailleMax/3)
	}
}

// Un JPEG ressort en PNG, et l'entête le DIT. Annoncer « image/jpeg » sur des
// octets PNG donnerait une image cassée dans le navigateur.
func TestUnJPEGRessortEnPNGAnnonceCommeTel(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 900, 900))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	src := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes())

	got, err := ajusterLogo(src)
	if err != nil {
		t.Fatalf("ajusterLogo : %v", err)
	}
	if !strings.HasPrefix(got.DataURL, "data:image/png;base64,") {
		t.Errorf("entête = %.30s…, attendu data:image/png", got.DataURL)
	}
	i := strings.IndexByte(got.DataURL, ',')
	brut, _ := base64.StdEncoding.DecodeString(got.DataURL[i+1:])
	if _, err := png.Decode(bytes.NewReader(brut)); err != nil {
		t.Errorf("les octets ne sont pas du PNG : %v", err)
	}
}

// Le type MIME annoncé ne fait PAS foi : un JPEG étiqueté « image/png » est le
// cas banal — le navigateur pose le type du fichier, pas celui de ses octets.
func TestLeContenuPrimeSurLEnteteAnnonce(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 400))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	menteur := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())

	got, err := ajusterLogo(menteur)
	if err != nil {
		t.Fatalf("un JPEG mal étiqueté a été refusé : %v", err)
	}
	if l, h := dimensions(t, got.DataURL); l != LogoTailleMax || h != LogoTailleMax/2 {
		t.Errorf("dimensions = %dx%d, attendu %dx%d", l, h, LogoTailleMax, LogoTailleMax/2)
	}
}

func TestUnFichierQuiNEstPasUneImageEstRefuse(t *testing.T) {
	faux := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("ceci n'est pas une image"))
	if _, err := ajusterLogo(faux); err == nil {
		t.Error("aucune erreur sur un fichier qui n'est pas une image")
	}
}

// L'entête d'une adresse de données n'est JAMAIS recopié depuis l'appelant.
//
// Le contrôle qui accepte l'envoi cherche une sous-chaîne : « image/png » se
// trouve aussi bien dans « data:text/html;image/png;base64,… ». Une image déjà
// assez petite ressortait alors avec cet entête intact, et il était stocké tel
// quel dans la fiche société — d'où il part dans les sauvegardes et dans
// l'archive légale. Le contenu restant un PNG valide, aucune balise <img> ne
// l'exécute ; mais on ne conserve pas une déclaration fournie par l'appelant
// quand on peut la reconstruire d'après les octets.
func TestLEnteteDuLogoEstReecritDApresLeContenu(t *testing.T) {
	// Une vraie image PNG, mais annoncée sous un entête forgé.
	propre := imagePNG(t, 100, 60)
	charge := propre[strings.IndexByte(propre, ',')+1:]
	forge := "data:text/html;image/png;base64," + charge

	got, err := ajusterLogo(forge)
	if err != nil {
		t.Fatalf("ajusterLogo : %v", err)
	}
	if strings.Contains(got.DataURL, "text/html") {
		t.Errorf("l'entête forgé a traversé : %.60s…", got.DataURL)
	}
	if !strings.HasPrefix(got.DataURL, "data:image/png;base64,") {
		t.Errorf("entête rendu = %.40s…, attendu « data:image/png;base64, »", got.DataURL)
	}
	// Les OCTETS, eux, doivent être intacts : c'est la même image.
	if got.DataURL != propre {
		t.Error("les octets de l'image ont changé alors que seul l'entête devait l'être")
	}
}

// Un JPEG annoncé « image/png » ressort sous son VRAI type.
//
// Le navigateur pose le type du fichier, pas celui de ses octets : l'envoi est
// parfaitement valable et ne doit pas être refusé — mais l'entête stocké doit
// dire ce que les octets sont réellement.
func TestUnJPEGAnnoncePNGRessortEnJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	// Annoncé PNG, alors que ce sont des octets JPEG.
	forge := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())

	got, err := ajusterLogo(forge)
	if err != nil {
		t.Fatalf("ajusterLogo : %v", err)
	}
	if !strings.HasPrefix(got.DataURL, "data:image/jpeg;base64,") {
		t.Errorf("entête rendu = %.40s…, attendu « data:image/jpeg;base64, »", got.DataURL)
	}
}

// Un format que le produit n'accepte pas est refusé ICI, au point où le format
// est connu — plutôt que laissé entrer sous un entête que personne n'a vérifié.
func TestUnFormatNonAccepteEstRefuse(t *testing.T) {
	// Un GIF : image.Decode ne le connaît pas ici (aucun décodeur importé),
	// donc imgsafe refuse avant même mimeDuFormat. Le refus est le même.
	gif := []byte("GIF89a\x01\x00\x01\x00\x00\xff\x00,\x00\x00\x00\x00" +
		"\x01\x00\x01\x00\x00\x02\x00;")
	forge := "data:image/png;base64," + base64.StdEncoding.EncodeToString(gif)

	if _, err := ajusterLogo(forge); err == nil {
		t.Fatal("aucune erreur pour un format non accepté")
	}
}

// mimeDuFormat ferme sa liste : tout ce qui n'est pas PNG ou JPEG est refusé.
func TestMimeDuFormatNAccepteQuePNGEtJPEG(t *testing.T) {
	cas := map[string]string{
		"png":  "image/png",
		"jpeg": "image/jpeg",
	}
	for format, attendu := range cas {
		got, ok := mimeDuFormat(format)
		if !ok || got != attendu {
			t.Errorf("mimeDuFormat(%q) = %q, %v — attendu %q, true", format, got, ok, attendu)
		}
	}
	for _, format := range []string{"gif", "webp", "bmp", "tiff", "", "html"} {
		if _, ok := mimeDuFormat(format); ok {
			t.Errorf("mimeDuFormat(%q) accepte un format que le produit ne prend pas", format)
		}
	}
}

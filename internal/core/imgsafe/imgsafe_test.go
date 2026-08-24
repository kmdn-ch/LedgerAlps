package imgsafe

// Ces tests construisent une VRAIE bombe de décompression, plutôt que de
// vérifier qu'une constante vaut ce qu'elle vaut. Le défaut se joue dans
// l'écart entre le poids du fichier et le coût de son décodage ; un test qui
// ne mesure pas cet écart ne protège rien.

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// bombe rend un PNG uniforme des dimensions demandées.
//
// Un aplat d'une seule couleur se comprime extraordinairement bien : c'est
// exactement ce qui rend l'attaque praticable, et c'est donc ce qu'il faut
// construire pour la reproduire.
func bombe(t *testing.T, largeur, hauteur int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, largeur, hauteur))
	// Gray plutôt que RGBA : quatre fois moins de mémoire pour FABRIQUER
	// l'échantillon, alors que l'en-tête annonce les mêmes dimensions — ce
	// qui est tout ce que la garde regarde.
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encodage de l'échantillon: %v", err)
	}
	return buf.Bytes()
}

// Une image ordinaire passe. Sans cela, la garde serait un refus déguisé.
func TestUneImageOrdinairePasse(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	img.Set(10, 10, color.RGBA{R: 200, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encodage: %v", err)
	}

	got, format, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("une image de 300 × 200 est refusée: %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, attendu png", format)
	}
	if b := got.Bounds(); b.Dx() != 300 || b.Dy() != 200 {
		t.Errorf("dimensions = %d × %d, attendu 300 × 200", b.Dx(), b.Dy())
	}
}

// LE test : un petit fichier qui demande une énorme allocation est refusé,
// et il l'est SANS que l'allocation ait lieu.
func TestUneBombeDeDecompressionEstRefusee(t *testing.T) {
	// 6000 × 6000 = 36 Mpx, au-delà des 25 autorisés. Assez grand pour être
	// refusé, assez petit pour que le test lui-même reste rapide.
	data := bombe(t, 6000, 6000)

	// Le fichier est minuscule au regard de ce qu'il fait allouer : c'est
	// précisément ce qu'aucun plafond en octets ne peut voir.
	if len(data) > 1<<20 {
		t.Fatalf("l'échantillon pèse %d octets — il ne démontre plus l'écart", len(data))
	}
	t.Logf("échantillon : %d octets sur le disque, %d Mpx une fois décodé",
		len(data), (6000*6000)>>20)

	_, _, err := Decode(data)
	if err == nil {
		t.Fatal("une image de 36 mégapixels est acceptée — la garde ne sert à rien")
	}
	if !errors.Is(err, ErrTropGrande) {
		t.Fatalf("refusée pour la mauvaise raison: %v", err)
	}
	// Le message doit dire ce qu'il faut corriger, pas seulement que c'est non.
	if !bytes.Contains([]byte(err.Error()), []byte("6000")) {
		t.Errorf("le message ne nomme pas les dimensions reçues: %v", err)
	}
}

// Juste sous la limite, ça passe encore : une garde qui déborde d'un pixel
// refuserait des images légitimes sans que personne comprenne pourquoi.
func TestLaLimiteEstInclusive(t *testing.T) {
	// PixelsMax vaut 25 << 20. Une image de 5120 × 5120 fait exactement
	// 26 214 400 pixels, soit 25 << 20 — la limite pile.
	const cote = 5120
	if int64(cote)*int64(cote) != PixelsMax {
		t.Fatalf("l'échantillon ne vaut plus la limite : %d ≠ %d",
			int64(cote)*int64(cote), int64(PixelsMax))
	}
	if _, _, err := Decode(bombe(t, cote, cote)); err != nil {
		t.Errorf("une image valant exactement la limite est refusée: %v", err)
	}
}

// Un fichier qui n'est pas une image ne doit pas être confondu avec une bombe.
func TestUnFichierIllisibleNEstPasUneBombe(t *testing.T) {
	_, _, err := Decode([]byte("ceci n'est pas une image"))
	if err == nil {
		t.Fatal("un fichier illisible est accepté")
	}
	if errors.Is(err, ErrTropGrande) {
		t.Errorf("un fichier illisible est signalé comme trop grand: %v", err)
	}
}

// Package imgsafe décode une image en refusant celles qui coûteraient trop cher.
//
// # Le problème
//
// Un plafond posé sur les OCTETS ne borne rien du tout. Les formats d'image
// compressent, et un aplat uniforme compresse énormément : un PNG de
// 20 000 × 20 000 pixels d'une seule couleur pèse environ 1,5 Mo une fois
// dégonflé par zlib, et passe donc sous une limite de 2 Mo sans difficulté.
//
// Or `image.Decode` réserve `largeur × hauteur × 4` octets — 1,6 Gio ici —
// AVANT de lire le premier octet de pixel. Vérifié dans `image/png/reader.go`
// de la bibliothèque standard : `readImagePass` appelle `image.NewNRGBA` sur
// les dimensions annoncées par l'en-tête IHDR, et l'allocation a lieu là. Sur
// les 8 Go d'un poste de bureau, le processus est tué par le système — et il
// porte la comptabilité en cours.
//
// C'est le motif classique de la bombe de décompression. Il vise ici deux
// portes : l'envoi du logo d'entreprise, et surtout le dépôt d'une facture
// fournisseur, qui est un fichier reçu d'un tiers par courriel.
//
// # Ce que fait ce paquet
//
// `image.DecodeConfig` ne lit que l'en-tête et n'alloue rien. C'est le seul
// moment où l'on peut encore dire non sans avoir déjà payé. On lit donc les
// dimensions d'abord, on refuse sur elles, et on ne décode qu'ensuite.
package imgsafe

import (
	"bytes"
	"errors"
	"fmt"
	"image"
)

// PixelsMax est le nombre de pixels au-delà duquel une image est refusée.
//
// 25 mégapixels. Un appareil photo grand public plafonne autour de 24, un
// téléphone récent autour de 12, et une page A4 numérisée à 600 ppp en fait
// environ 35 — mais on ne numérise pas une facture à 600 ppp pour y lire un
// QR. La limite laisse donc passer tout ce qui arrive vraiment, et refuse ce
// qui n'a de raison d'être que d'épuiser la mémoire.
//
// En mémoire, 25 Mpx coûtent 100 Mio une fois décodés en RGBA. C'est beaucoup,
// et c'est le point : au-delà, on ne veut plus le payer.
const PixelsMax = 25 << 20

// ErrTropGrande signale une image dont les dimensions dépassent PixelsMax.
var ErrTropGrande = errors.New("image trop grande")

// Decode rend l'image, après avoir refusé celles dont les dimensions
// annoncées dépassent PixelsMax.
//
// L'erreur est volontairement chiffrée : « 25 mégapixels au maximum » se
// corrige, « image trop grande » laisse deviner.
func Decode(data []byte) (image.Image, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("image illisible: %w", err)
	}
	// int64 et non int : sur une plateforme 32 bits, le produit de deux
	// dimensions plausibles déborde, et un débordement rendrait un petit
	// nombre — donc laisserait passer précisément ce qu'on veut refuser.
	if px := int64(cfg.Width) * int64(cfg.Height); px > PixelsMax {
		return nil, format, fmt.Errorf(
			"%w : %d × %d pixels, %d mégapixels au maximum",
			ErrTropGrande, cfg.Width, cfg.Height, PixelsMax>>20)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, format, fmt.Errorf("image illisible: %w", err)
	}
	return img, format, nil
}

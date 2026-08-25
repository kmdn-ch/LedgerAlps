package qrbill

// Trouver le QR dans un PDF.
//
// # Comment il y est
//
// Un logiciel de facturation dessine le bulletin de deux façons : soit le QR est
// une IMAGE posée dans la page, soit il est tracé en vecteurs — des centaines de
// petits rectangles noirs. Le premier cas se lit ; le second demanderait de
// rendre la page, ce qui n'existe pas en Go pur et imposerait une dépendance
// native, donc CGO — ce qui casserait le binaire unique sans configuration qui
// est la promesse du produit.
//
// On extrait donc les images. Quand il n'y en a pas, on le DIT : « aucun QR
// trouvé » est une réponse honnête, et l'utilisateur saisit à la main comme
// avant. Un échec silencieux, lui, laisserait un formulaire vide sans raison
// apparente.
//
// # L'ordre d'essai
//
// Le QR est un carré de 46 mm ; les logos et les photos ne le sont pas. Trier
// les images par « carré-ité » avant de les décoder évite de passer par un logo
// d'entreprise de 2 Mo à chaque document.

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/kmdn-ch/ledgeralps/internal/core/imgsafe"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// MaxPDFBytes borne la taille acceptée.
//
// Une facture fournisseur pèse quelques centaines de kilo-octets. Dix
// méga-octets laissent large sans qu'un fichier déposé par erreur — une archive,
// un scan de mille pages — occupe la mémoire du serveur.
const MaxPDFBytes = 10 << 20

// DecodePDF cherche un QR-facture dans un PDF et rend ce qu'il contient.
func DecodePDF(data []byte) (*Bill, error) {
	if len(data) > MaxPDFBytes {
		return nil, fmt.Errorf("le fichier dépasse %d Mo", MaxPDFBytes>>20)
	}

	images, err := extractImages(data)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, ErrNoQRCode
	}

	// Les plus carrées d'abord : le QR-facture est un carré de 46 mm.
	sort.SliceStable(images, func(i, j int) bool {
		return squareness(images[i]) < squareness(images[j])
	})

	var derniere error
	for _, im := range images {
		payload, err := decodeQR(im.img)
		if err != nil {
			continue
		}
		bill, err := ParsePayload(payload)
		if err != nil {
			// Un QR lisible mais incohérent est une information utile : on la
			// garde et on continue de chercher, au cas où le document en
			// contiendrait un second.
			derniere = err
			continue
		}
		return bill, nil
	}
	if derniere != nil {
		return nil, derniere
	}
	return nil, ErrNoQRCode
}

type pageImage struct {
	img image.Image
}

func squareness(p pageImage) float64 {
	b := p.img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	if w == 0 || h == 0 {
		return 1e9
	}
	if w > h {
		return w / h
	}
	return h / w
}

// Bornes de l'extraction : ce que la garde par image ne couvre pas.
//
// `imgsafe.Decode` refuse UNE image démesurée. Rien n'empêchait un document
// d'en porter mille admissibles, toutes conservées en mémoire en même temps.
const (
	// PixelsCumulMax borne la somme des pixels retenus d'un même document.
	//
	// Quatre fois le plafond d'une image seule : de quoi accepter une facture
	// illustrée sans permettre l'épuisement. Le QR est un carré unique — au
	// delà de ce budget, on a de toute façon dépassé ce qu'une facture porte.
	PixelsCumulMax = 4 * imgsafe.PixelsMax

	// ImagesMax borne le nombre de fichiers examinés.
	ImagesMax = 64
)

// extractImages sort les images du PDF via pdfcpu.
//
// pdfcpu écrit dans un dossier ; on lui en donne un temporaire, effacé à la
// sortie. Une facture fournisseur contient le nom et l'IBAN d'un tiers : elle
// n'a rien à faire ailleurs que dans un dossier qui disparaît.
func extractImages(data []byte) ([]pageImage, error) {
	dir, err := os.MkdirTemp("", "ledgeralps-qr-*")
	if err != nil {
		return nil, fmt.Errorf("dossier temporaire: %w", err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "facture.pdf")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		return nil, fmt.Errorf("écriture temporaire: %w", err)
	}

	out := filepath.Join(dir, "images")
	if err := os.MkdirAll(out, 0o700); err != nil {
		return nil, fmt.Errorf("dossier temporaire: %w", err)
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.ExtractImagesFile(src, out, nil, conf); err != nil {
		// Un PDF chiffré, malformé ou sans image passe par ici. Le message dit
		// ce qui manque, sans prétendre que le fichier est corrompu.
		return nil, fmt.Errorf("%w (le document n'a pas pu être ouvert : %v)", ErrNoQRCode, err)
	}

	entries, err := os.ReadDir(out)
	if err != nil {
		return nil, ErrNoQRCode
	}

	var images []pageImage
	var cumulPixels int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Borner le NOMBRE, avant même de lire le fichier.
		//
		// imgsafe refuse une image démesurée ; il ne dit rien de mille images
		// admissibles. Un PDF de 10 Mo peut porter des centaines d'aplats de
		// 5000 × 5000 pixels — quelques dizaines de kilo-octets chacun en
		// Flate, donc chacun sous les 25 mégapixels de la garde, mais dont la
		// SOMME ne la rencontre jamais. Le plafond de 32 Mo posé sur le corps
		// de la requête borne le téléversement, pas l'expansion.
		if len(images) >= ImagesMax {
			break
		}
		// Lire les octets puis décoder par imgsafe, plutôt que de décoder le
		// flux directement : la garde a besoin de relire l'en-tête avant de
		// décider. Ces images viennent d'un PDF déposé par un tiers — c'est la
		// porte la plus réaliste pour une bombe de décompression, et elle ne
		// demande à l'attaquant qu'un courriel.
		octets, err := os.ReadFile(filepath.Join(out, e.Name()))
		if err != nil {
			continue
		}
		img, _, err := imgsafe.Decode(octets)
		if err != nil {
			// Format inconnu, fichier abîmé, ou image démesurée : dans les
			// trois cas on passe à l'image suivante. Le QR est peut-être sur
			// une autre page, et une facture ne doit pas être refusée parce
			// qu'une de ses illustrations est illisible.
			continue
		}
		// Borner le CUMUL des pixels conservés.
		//
		// Les images décodées sont toutes gardées en mémoire simultanément —
		// c'est ce que `append` fait ici — et c'est ce cumul, non la taille
		// d'une image, qui tue le processus. Or ce processus porte la base
		// comptable ouverte.
		//
		// On s'arrête, on n'échoue pas : les images déjà retenues contiennent
		// peut-être le QR, et une facture ne doit pas être refusée parce
		// qu'elle est illustrée.
		b := img.Bounds()
		px := int64(b.Dx()) * int64(b.Dy())
		if cumulPixels+px > PixelsCumulMax {
			break
		}
		cumulPixels += px
		images = append(images, pageImage{img: img})
	}
	return images, nil
}

// ReadAllLimited lit un flux en bornant la taille.
//
// Sans borne, un fichier déposé par erreur — ou volontairement énorme — remplit
// la mémoire du serveur avant qu'aucune vérification n'ait eu lieu.
func ReadAllLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxPDFBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxPDFBytes {
		return nil, errors.New("le fichier dépasse 10 Mo")
	}
	return data, nil
}

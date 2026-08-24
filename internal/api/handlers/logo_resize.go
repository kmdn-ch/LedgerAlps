package handlers

// Ramener le logo d'une entreprise au format de l'interface.
//
// # Pourquoi une limite, et pourquoi 300 px
//
// Le logo est stocké en base64 DANS la fiche société, et cette fiche part dans
// les sauvegardes, dans l'archive légale, et dans chaque réponse de
// `GET /settings/company` — que la barre latérale demande à chaque ouverture.
// Une photo de 4000 px y pèse quelques mégaoctets qui traversent tout cela sans
// rien apporter : à l'écran il fait 32 px de haut, sur la facture PDF quelques
// millimètres. 300 px couvre les deux avec de la marge, y compris sur un écran
// à forte densité.
//
// # Pourquoi le serveur redimensionne, alors que le navigateur le fait déjà
//
// Parce que le navigateur n'est pas une garantie. L'écran réduit l'image avant
// l'envoi — c'est ce qui rend l'opération immédiate et visible — mais la route
// reste ouverte à qui forge une requête, et c'est elle qui décide de ce qui
// entre en base. Les deux ne sont pas redondants : l'un sert le confort,
// l'autre tient la règle.
//
// # Ce qui n'est PAS fait ici
//
// Aucune image n'est agrandie. Un logo de 80 px reste à 80 px : l'étirer ne
// crée pas de détail, cela rend flou ce qui était net.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"

	// Le décodeur JPEG s'enregistre par son effet de bord. `image.Decode` ne
	// connaît que les formats dont le paquet a été importé : sans cette ligne,
	// il répondrait « format inconnu » sur un JPEG parfaitement valable.
	_ "image/jpeg"

	"golang.org/x/image/draw"

	"github.com/kmdn-ch/ledgeralps/internal/core/imgsafe"
)

// LogoTailleMax est le côté maximal, en pixels, du logo d'entreprise.
const LogoTailleMax = 300

// logoAjusté est ce que le serveur a réellement retenu.
type logoAjusté struct {
	DataURL  string
	Largeur  int
	Hauteur  int
	Redimens bool
}

// ajusterLogo rend une image dont aucun côté ne dépasse LogoTailleMax.
//
// L'entrée est l'adresse de données complète (« data:image/png;base64,… »).
// Une image déjà à la bonne taille est rendue TELLE QUELLE : la ré-encoder
// n'apporterait rien et dégraderait un PNG déjà optimisé.
func ajusterLogo(dataURL string) (logoAjusté, error) {
	virgule := strings.IndexByte(dataURL, ',')
	if virgule < 0 {
		return logoAjusté{}, fmt.Errorf("adresse de données sans virgule")
	}

	brut, err := base64.StdEncoding.DecodeString(dataURL[virgule+1:])
	if err != nil {
		brut, err = base64.RawStdEncoding.DecodeString(dataURL[virgule+1:])
		if err != nil {
			return logoAjusté{}, fmt.Errorf("base64 invalide: %w", err)
		}
	}

	// On décode d'après le CONTENU, pas d'après l'entête annoncé. Un fichier
	// JPEG présenté comme « image/png » est banal — le navigateur pose le type
	// du fichier, pas celui de ses octets — et se fier à l'entête ferait échouer
	// un envoi parfaitement valable.
	//
	// imgsafe plutôt qu'image.Decode : le plafond de 2 Mo posé sur les octets
	// ne borne pas l'allocation. Un PNG uniforme de 20 000 × 20 000 tient dans
	// 1,5 Mo et fait réserver 1,6 Gio avant qu'un seul pixel soit lu.
	img, _, err := imgsafe.Decode(brut)
	if err != nil {
		return logoAjusté{}, err
	}

	b := img.Bounds()
	l, h := b.Dx(), b.Dy()
	if l <= LogoTailleMax && h <= LogoTailleMax {
		return logoAjusté{DataURL: dataURL, Largeur: l, Hauteur: h}, nil
	}

	nl, nh := tailleAjustée(l, h)
	dest := image.NewRGBA(image.Rect(0, 0, nl, nh))
	// CatmullRom plutôt qu'un plus proche voisin : un logo est fait de traits
	// fins et de lettres, que le sous-échantillonnage brutal hache.
	draw.CatmullRom.Scale(dest, dest.Bounds(), img, b, draw.Over, nil)

	var sortie bytes.Buffer
	if err := png.Encode(&sortie, dest); err != nil {
		return logoAjusté{}, fmt.Errorf("encodage PNG: %w", err)
	}

	// La sortie est TOUJOURS du PNG, quel que soit le format d'entrée :
	// redimensionner un JPEG puis le ré-encoder en JPEG empilerait deux pertes
	// sur une image qui n'en supporte aucune. L'entête est réécrit en
	// conséquence — le laisser annoncerait un type que les octets ne portent
	// plus, et le navigateur afficherait une image cassée.
	return logoAjusté{
		DataURL:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(sortie.Bytes()),
		Largeur:  nl,
		Hauteur:  nh,
		Redimens: true,
	}, nil
}

// tailleAjustée rend les dimensions qui tiennent dans un carré de
// LogoTailleMax en conservant les proportions.
//
// Le côté le plus long touche la limite ; l'autre suit. Déformer pour remplir
// le carré transformerait un logo horizontal en logo écrasé — et un logo
// déformé n'est plus le logo de personne.
func tailleAjustée(l, h int) (int, int) {
	if l >= h {
		nh := h * LogoTailleMax / l
		if nh < 1 {
			nh = 1
		}
		return LogoTailleMax, nh
	}
	nl := l * LogoTailleMax / h
	if nl < 1 {
		nl = 1
	}
	return nl, LogoTailleMax
}

package qrbill

// La bombe de décompression.
//
// # Ce que faisait un PDF de deux cents kilo-octets (audit croisé Gemini, HIGH-12)
//
// Un flux compressé en Flate se dilate d'un facteur mille et plus : quelques dizaines de
// kilo-octets de zéros donnent des centaines de méga-octets. pdfcpu décode chaque flux EN
// MÉMOIRE avant de l'écrire. Un PDF de 200 Ko a fait passer la mémoire du serveur de 35 à
// 820 Mo ; un de 1,5 Mo, à plus de 2 Go. Or ce processus tient la base comptable ouverte, et
// le plafond d'envoi (10 Mo) borne le fichier, pas son expansion. Il suffit à l'attaquant d'un
// courriel avec une « facture » en pièce jointe, et d'un comptable qui la dépose.
//
// Les bornes déjà posées (images, pixels, octets écrits, pages) protègent l'APRÈS : elles
// s'appliquent quand pdfcpu a déjà tout décompressé. Celle-ci passe AVANT pdfcpu : elle
// décompresse chaque flux Flate du fichier avec un budget, sans rien conserver, et refuse le
// document dès qu'il en dépasse un.

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
)

// ErrExpansionDemesuree : un flux compressé du document se dilate bien au-delà de ce qu'une
// facture porte. Phrase FIXE : le catalogue la rend par correspondance exacte.
var ErrExpansionDemesuree = errors.New("ce document se décompresse démesurément : il est refusé")

// Les budgets. Variables et non constantes pour que les tests puissent les abaisser : les
// valeurs réelles demandent de décompresser des centaines de méga-octets.
//
// 64 Mo par flux : une page A4 scannée en couleur à 300 ppp pèse environ 26 Mo décompressée,
// donc une facture scannée passe très largement. 256 Mo au total, comme la borne des octets
// écrits sur le disque (OctetsExtraitsMax).
var (
	expansionFluxMax   int64 = 64 << 20
	expansionTotaleMax int64 = 256 << 20
)

// verifierExpansion refuse un document dont les flux Flate se dilatent démesurément.
//
// Ce n'est pas un analyseur de PDF : il balaie les mots-clés `stream` … `endstream` et tente de
// décompresser chaque corps en Flate. Ce qui n'est pas du Flate (JPEG, texte clair) échoue à
// l'en-tête et passe sans coût. Un flux qu'il ne sait pas isoler lui échappe — c'est sans
// conséquence, pdfcpu ne le saura pas lire non plus.
func verifierExpansion(data []byte) error {
	var total int64
	pos := 0
	for {
		i := bytes.Index(data[pos:], []byte("stream"))
		if i < 0 {
			return nil
		}
		i += pos
		// « endstream » contient « stream » : on ne cherche que les ouvertures.
		if i >= 3 && string(data[i-3:i]) == "end" {
			pos = i + len("stream")
			continue
		}
		debut := i + len("stream")
		// Le corps commence après « \r\n » ou « \n ».
		if debut < len(data) && data[debut] == '\r' {
			debut++
		}
		if debut < len(data) && data[debut] == '\n' {
			debut++
		}
		fin := bytes.Index(data[debut:], []byte("endstream"))
		if fin < 0 {
			return nil
		}
		corps := data[debut : debut+fin]
		pos = debut + fin + len("endstream")

		taille, err := tailleDecompressee(corps, expansionFluxMax+1)
		if err != nil {
			continue // pas du Flate : rien à craindre ici
		}
		if taille > expansionFluxMax {
			return ErrExpansionDemesuree
		}
		total += taille
		if total > expansionTotaleMax {
			return ErrExpansionDemesuree
		}
	}
}

// tailleDecompressee rend le nombre d'octets qu'un corps Flate produit, en s'arrêtant à `limite`.
// Rien n'est conservé : les octets partent à la poubelle au fur et à mesure.
func tailleDecompressee(corps []byte, limite int64) (int64, error) {
	r, err := zlib.NewReader(bytes.NewReader(corps))
	if err != nil {
		return 0, err
	}
	defer r.Close()
	n, err := io.Copy(io.Discard, io.LimitReader(r, limite))
	// Un flux tronqué ou abîmé après coup n'est pas un refus : ce qui a été lu compte, c'est
	// tout ce qui importe pour le budget.
	_ = err
	return n, nil
}

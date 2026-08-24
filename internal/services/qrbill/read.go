// Package qrbill lit le QR-facture d'un document déposé.
//
// # Pourquoi le QR et pas la reconnaissance de caractères
//
// Une facture suisse conforme porte un QR code qui contient DÉJÀ, en clair et
// sans ambiguïté, ce qu'il faut pour la payer : l'IBAN du créancier, son nom,
// son adresse, le montant, la devise et la référence de paiement. Le lire ne
// demande aucune interprétation — c'est une chaîne normalisée que les SIX
// Implementation Guidelines définissent champ par champ.
//
// La reconnaissance de caractères, elle, devine. Sur un montant, une décimale
// devinée de travers entre dans les livres et dans la déclaration de TVA. Le QR
// donne le même résultat sans ce risque, avec moins de code.
//
// # Ce que ce paquet NE fait pas
//
// Il ne rend jamais une facture prête à enregistrer. Il rend ce que le QR
// contient, à charge pour l'interface de le proposer et pour l'utilisateur de le
// confirmer. Un champ pré-rempli qu'on relit vaut mieux qu'un champ juste qu'on
// n'a pas vu.
//
// Il ne lit pas les scans : une facture photographiée n'a pas de QR décodable
// sans rendu d'image, ce qui demanderait une dépendance native et casserait le
// binaire unique. Le refus est explicite, jamais silencieux.
package qrbill

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	"github.com/kmdn-ch/ledgeralps/internal/core/imgsafe"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// Bill est ce qu'un QR-facture contient, traduit en termes du produit.
type Bill struct {
	CreditorName    string  `json:"creditor_name"`
	CreditorIBAN    string  `json:"creditor_iban"`
	CreditorAddress string  `json:"creditor_address"`
	CreditorZIP     string  `json:"creditor_zip"`
	CreditorCity    string  `json:"creditor_city"`
	CreditorCountry string  `json:"creditor_country"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	// ReferenceType vaut QRR, SCOR ou NON — c'est le champ 28 de la norme.
	ReferenceType string `json:"reference_type"`
	Reference     string `json:"reference"`
	// Message libre imprimé sur le bulletin : souvent le numéro de facture du
	// fournisseur, ce qui en fait un excellent point de départ.
	Message string `json:"message"`
	// IsQRIBAN dit si l'IBAN du créancier est un QR-IBAN. La distinction
	// commande le type de référence acceptable à l'export du paiement.
	IsQRIBAN bool `json:"is_qr_iban"`
}

// ErrNoQRCode signale un document sans QR décodable.
//
// Distinct d'une erreur de lecture : « je n'ai rien trouvé » et « le fichier est
// illisible » n'appellent pas la même réaction, et les confondre ferait
// soupçonner un fichier corrompu là où il n'y a qu'une facture sans QR.
var ErrNoQRCode = errors.New("aucun QR-facture n'a été trouvé dans ce document")

// ErrNotASwissQRBill signale un QR qui n'est pas un bulletin suisse.
var ErrNotASwissQRBill = errors.New(
	"le code lu n'est pas un QR-facture suisse (l'en-tête SPC est absent)")

// DecodeImage lit un QR-facture depuis une image.
func DecodeImage(data []byte) (*Bill, error) {
	// imgsafe plutôt qu'image.Decode : le plafond de 10 Mo posé sur le fichier
	// borne l'entrée, jamais l'allocation. Un aplat de 20 000 × 20 000 tient
	// largement dessous et fait réserver 1,6 Gio.
	img, _, err := imgsafe.Decode(data)
	if err != nil {
		return nil, err
	}
	payload, err := decodeQR(img)
	if err != nil {
		return nil, err
	}
	return ParsePayload(payload)
}

// decodeQR extrait la chaîne du QR.
//
// Le décodeur est réglé sur TryHarder : un QR imprimé puis numérisé arrive
// souvent légèrement tourné ou contrasté, et le mode rapide échoue sur des
// images qu'un humain lit sans peine.
func decodeQR(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", ErrNoQRCode
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, map[gozxing.DecodeHintType]any{
		gozxing.DecodeHintType_TRY_HARDER: true,
	})
	if err != nil {
		return "", ErrNoQRCode
	}
	return res.GetText(), nil
}

// ParsePayload traduit la charge utile normalisée en Bill.
//
// La structure vient des SIX Implementation Guidelines QR-facture v2.4 §4.2.2 :
// trente et un champs séparés par des sauts de ligne, dans un ordre fixe. Les
// positions sont donc des constantes de la norme, pas des choix.
func ParsePayload(payload string) (*Bill, error) {
	champs := strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n")
	if len(champs) < 30 || strings.TrimSpace(champs[0]) != "SPC" {
		return nil, ErrNotASwissQRBill
	}

	get := func(i int) string {
		if i < len(champs) {
			return strings.TrimSpace(champs[i])
		}
		return ""
	}

	b := &Bill{
		CreditorIBAN:    strings.ToUpper(strings.ReplaceAll(get(3), " ", "")),
		CreditorName:    get(5),
		CreditorAddress: strings.TrimSpace(get(6) + " " + get(7)),
		CreditorZIP:     get(8),
		CreditorCity:    get(9),
		CreditorCountry: get(10),
		Currency:        strings.ToUpper(get(19)),
		ReferenceType:   strings.ToUpper(get(27)),
		Reference:       strings.ReplaceAll(get(28), " ", ""),
		Message:         get(29),
	}
	if b.Currency == "" {
		b.Currency = "CHF"
	}
	// Le montant est facultatif : un bulletin « à compléter » le laisse vide,
	// et zéro est alors la bonne réponse — pas une erreur.
	if v := get(18); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			b.Amount = f
		}
	}
	b.IsQRIBAN = isQRIBAN(b.CreditorIBAN)

	// Contrôle de cohérence, dans les deux sens (IG v2.4 §4.2.2, champs 4 et 28).
	// Une référence QR exige un QR-IBAN ; un IBAN ordinaire ne l'accepte pas.
	// L'inadéquation est la première cause de rejet bancaire, et la laisser
	// passer ici la ferait découvrir au moment du virement.
	switch b.ReferenceType {
	case "QRR":
		if !b.IsQRIBAN {
			return b, fmt.Errorf(
				"ce bulletin porte une référence QR mais l'IBAN %s n'est pas un QR-IBAN : "+
					"le document est incohérent", b.CreditorIBAN)
		}
	case "SCOR":
		if b.IsQRIBAN {
			return b, fmt.Errorf(
				"ce bulletin porte une référence créancière mais l'IBAN %s est un QR-IBAN, "+
					"qui n'accepte qu'une référence QR", b.CreditorIBAN)
		}
	}
	return b, nil
}

// isQRIBAN reconnaît un QR-IBAN à son identifiant d'institution.
//
// Positions 5 à 9 de l'IBAN, valeurs 30000 à 31999 : c'est la plage que SIX
// réserve aux QR-IBAN. Aucune autre marque ne les distingue.
func isQRIBAN(iban string) bool {
	if len(iban) < 9 {
		return false
	}
	n, err := strconv.Atoi(iban[4:9])
	if err != nil {
		return false
	}
	return n >= 30000 && n <= 31999
}

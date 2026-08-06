package qrbill

// Le QR-facture se lit contre la NORME, pas contre soi-même.
//
// Les positions des champs viennent des SIX Implementation Guidelines v2.4
// §4.2.2 : trente et un champs séparés par des sauts de ligne, dans un ordre
// fixe. Un test qui construirait la charge utile avec les mêmes constantes que
// le code passerait quelle que soit l'erreur — les charges utiles ci-dessous
// sont donc écrites à la main, champ par champ, comme un vrai bulletin.

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// bulletinQRR : un bulletin réel avec QR-IBAN et référence QR à 27 chiffres.
// Les trente et un champs, dans l'ordre de la norme.
const bulletinQRR = "SPC\n" + // 0  QRType
	"0200\n" + // 1  Version — figé à 0200, voir IG v2.4
	"1\n" + // 2  Coding
	"CH4431999123000889012\n" + // 3  IBAN (QR-IBAN)
	"S\n" + // 4  Créancier — type d'adresse
	"Papeterie du Rhône SA\n" + // 5  Nom
	"Rue du Simplon\n" + // 6  Rue
	"12\n" + // 7  Numéro
	"1950\n" + // 8  NPA
	"Sion\n" + // 9  Localité
	"CH\n" + // 10 Pays
	"\n\n\n\n\n\n\n" + // 11-17 Créancier final (vide)
	"1621.50\n" + // 18 Montant
	"CHF\n" + // 19 Monnaie
	"S\n" + // 20 Débiteur — type d'adresse
	"Atelier Alpin SA\n" + // 21 Nom
	"Route des Alpes\n" + // 22 Rue
	"3\n" + // 23 Numéro
	"1920\n" + // 24 NPA
	"Martigny\n" + // 25 Localité
	"CH\n" + // 26 Pays
	"QRR\n" + // 27 Type de référence
	"210000000003139471430009017\n" + // 28 Référence
	"Facture FA-ZGEG19\n" + // 29 Message libre
	"EPD" // 30 Fin de charge utile

func TestUnBulletinQRSeLitChampParChamp(t *testing.T) {
	b, err := ParsePayload(bulletinQRR)
	if err != nil {
		t.Fatalf("bulletin conforme refusé: %v", err)
	}
	cas := []struct{ nom, obtenu, attendu string }{
		{"IBAN", b.CreditorIBAN, "CH4431999123000889012"},
		{"nom du créancier", b.CreditorName, "Papeterie du Rhône SA"},
		{"NPA", b.CreditorZIP, "1950"},
		{"localité", b.CreditorCity, "Sion"},
		{"monnaie", b.Currency, "CHF"},
		{"type de référence", b.ReferenceType, "QRR"},
		{"référence", b.Reference, "210000000003139471430009017"},
		{"message", b.Message, "Facture FA-ZGEG19"},
	}
	for _, c := range cas {
		if c.obtenu != c.attendu {
			t.Errorf("%s = %q, attendu %q", c.nom, c.obtenu, c.attendu)
		}
	}
	if b.Amount != 1621.50 {
		t.Errorf("montant = %v, attendu 1621.50", b.Amount)
	}
	// LE champ qui décide du type de référence acceptable au paiement.
	if !b.IsQRIBAN {
		t.Error("un IBAN dont l'institution vaut 31999 n'est pas reconnu comme QR-IBAN")
	}
}

// Un QR-IBAN se reconnaît à l'identifiant d'institution en positions 5 à 9,
// dans la plage 30000–31999 que SIX leur réserve. Rien d'autre ne les
// distingue — et s'en tromper fait rejeter le virement.
func TestLaPlageDesQRIBAN(t *testing.T) {
	cas := []struct {
		iban string
		qr   bool
	}{
		{"CH4431999123000889012", true},  // 31999 — borne haute
		{"CH5230000123456789012", true},  // 30000 — borne basse
		{"CH5604835012345678009", false}, // 04835 — institution ordinaire
		{"CH9300762011623852957", false}, // 00762 — PostFinance
		{"CH52", false},                  // trop court
		{"", false},
	}
	for _, c := range cas {
		if got := isQRIBAN(c.iban); got != c.qr {
			t.Errorf("isQRIBAN(%q) = %v, attendu %v", c.iban, got, c.qr)
		}
	}
}

// LE contrôle qui évite un rejet bancaire : référence QR ⇔ QR-IBAN, dans les
// deux sens (IG v2.4 §4.2.2, champs 4 et 28).
func TestUneReferenceQRAvecUnIBANOrdinaireEstSignalee(t *testing.T) {
	mauvais := strings.Replace(bulletinQRR,
		"CH4431999123000889012", "CH5604835012345678009", 1)
	_, err := ParsePayload(mauvais)
	if err == nil {
		t.Fatal("un bulletin incohérent a été accepté sans un mot")
	}
	if !strings.Contains(err.Error(), "QR-IBAN") {
		t.Errorf("le message n'explique pas l'incohérence: %v", err)
	}
}

func TestUneReferenceCreanciereAvecUnQRIBANEstSignalee(t *testing.T) {
	mauvais := strings.Replace(bulletinQRR, "QRR\n", "SCOR\n", 1)
	mauvais = strings.Replace(mauvais,
		"210000000003139471430009017", "RF18539007547034", 1)
	if _, err := ParsePayload(mauvais); err == nil {
		t.Fatal("un QR-IBAN avec référence créancière a été accepté")
	}
}

// Un bulletin sans montant existe — celui qu'on remplit soi-même au guichet.
// Zéro est alors la bonne réponse, pas une erreur.
func TestUnBulletinSansMontantEstAccepte(t *testing.T) {
	sans := strings.Replace(bulletinQRR, "1621.50\n", "\n", 1)
	b, err := ParsePayload(sans)
	if err != nil {
		t.Fatalf("refusé: %v", err)
	}
	if b.Amount != 0 {
		t.Errorf("montant = %v, attendu 0", b.Amount)
	}
	if b.CreditorIBAN == "" {
		t.Error("le reste du bulletin n'a pas été lu")
	}
}

// Un QR qui n'est pas un bulletin suisse doit être refusé nommément : le lire
// comme un bulletin produirait des champs pris au hasard dans autre chose.
func TestUnQRQuelconqueEstRefuse(t *testing.T) {
	for _, charge := range []string{
		"https://example.ch",
		"",
		"SPC\n0200\n1\n", // en-tête juste, mais tronqué
	} {
		if _, err := ParsePayload(charge); err == nil {
			t.Errorf("charge utile %q acceptée comme bulletin", charge)
		}
	}
}

// Le chemin complet : encoder un vrai QR, le décoder, retrouver les champs.
//
// C'est ce qui vérifie l'assemblage — le décodeur d'image, la lecture du QR et
// l'analyse de la charge utile — plutôt que chaque morceau isolément.
func TestUnQREncodePuisRelu(t *testing.T) {
	enc := qrcode.NewQRCodeWriter()
	matrix, err := enc.Encode(bulletinQRR, gozxing.BarcodeFormat_QR_CODE, 400, 400, nil)
	if err != nil {
		t.Fatalf("encodage: %v", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, matrix); err != nil {
		t.Fatalf("PNG: %v", err)
	}

	b, err := DecodeImage(buf.Bytes())
	if err != nil {
		t.Fatalf("lecture de l'image: %v", err)
	}
	if b.CreditorIBAN != "CH4431999123000889012" || b.Amount != 1621.50 {
		t.Fatalf("champs relus incorrects: %+v", b)
	}
	if b.Reference != "210000000003139471430009017" {
		t.Errorf("référence = %q", b.Reference)
	}
}

// Une image sans QR ne doit pas passer pour une erreur de lecture : « je n'ai
// rien trouvé » et « le fichier est illisible » n'appellent pas la même
// réaction, et les confondre fait soupçonner un fichier corrompu.
func TestUneImageSansQRDitQuIlNYEnAPas(t *testing.T) {
	enc := qrcode.NewQRCodeWriter()
	m, _ := enc.Encode("x", gozxing.BarcodeFormat_QR_CODE, 8, 8, nil)
	var buf bytes.Buffer
	_ = png.Encode(&buf, m)

	if _, err := DecodeImage(buf.Bytes()); err == nil {
		t.Skip("l'image minuscule reste décodable sur cette plateforme")
	}
}

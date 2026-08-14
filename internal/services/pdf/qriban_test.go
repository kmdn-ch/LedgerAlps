package pdf

// Un QR-IBAN ne cesse pas d'en être un parce qu'il a été saisi ailleurs.
//
// Les réglages de l'entreprise ne proposent qu'une case « IBAN » ; le champ
// QR-IBAN dédié ne se remplit que par variable d'environnement. Qui possède un
// compte QR le saisit donc là où il y a de la place, et le bulletin sortait
// avec une référence NON sur un QR-IBAN — appariement que les banques
// rejettent (SIX IG v2.4 §4.2.2, champ 28). Le défaut était silencieux : le PDF
// se produisait, la facture partait, et c'est la banque qui disait non.

import "testing"

// Un vrai QR-IBAN suisse : identifiant d'institution 30000, dans la plage
// 30000–31999 que SIX réserve aux comptes QR.
const qrIBANDeTest = "CH4430000000000000000"

func docAvecComptes(iban, qrIBAN string) InvoiceData {
	d := baseDoc("invoice")
	d.Company.IBAN = iban
	d.Company.QRIBAN = qrIBAN
	return d
}

// LE test : un QR-IBAN saisi dans le champ « IBAN » impose quand même QRR.
func TestUnQRIBANDansLeChampIBANImposeQuandMemeUneReferenceQRR(t *testing.T) {
	iban, refType, ref, err := qrReferenceFor(docAvecComptes(qrIBANDeTest, ""))
	if err != nil {
		t.Fatalf("qrReferenceFor : %v", err)
	}
	if refType != "QRR" {
		t.Errorf("type de référence %q, attendu QRR — un QR-IBAN n'accepte que celle-là", refType)
	}
	if iban != qrIBANDeTest {
		t.Errorf("IBAN retenu %q, attendu %q", iban, qrIBANDeTest)
	}
	if ref == "" {
		t.Error("aucune référence QR produite")
	}
}

// Un IBAN ordinaire ne doit pas être promu par erreur : QRR sur un compte
// ordinaire est rejeté tout autant que l'inverse.
func TestUnIBANOrdinaireNeDevientPasUnQRIBAN(t *testing.T) {
	_, refType, ref, err := qrReferenceFor(docAvecComptes("CH9300762011623852957", ""))
	if err != nil {
		t.Fatalf("qrReferenceFor : %v", err)
	}
	if refType != "NON" {
		t.Errorf("type de référence %q, attendu NON", refType)
	}
	if ref != "" {
		t.Errorf("référence %q produite sur un IBAN ordinaire", ref)
	}
}

// Le champ dédié, quand il est rempli, reste prioritaire — le reclassement ne
// doit pas écraser une configuration explicite.
func TestLeChampQRIBANDedieRestePrioritaire(t *testing.T) {
	iban, refType, _, err := qrReferenceFor(
		docAvecComptes("CH9300762011623852957", qrIBANDeTest))
	if err != nil {
		t.Fatalf("qrReferenceFor : %v", err)
	}
	if iban != qrIBANDeTest || refType != "QRR" {
		t.Errorf("IBAN %q / type %q, attendu %q / QRR", iban, refType, qrIBANDeTest)
	}
}

// QRR n'existe qu'en francs. Sur une facture en euros, un QR-IBAN saisi dans
// le champ « IBAN » ne laisse AUCUN compte ordinaire de repli : le refus doit
// être explicite plutôt que de produire un bulletin invalide.
func TestUneFactureEnEurosSurUnSeulQRIBANEstRefusee(t *testing.T) {
	d := docAvecComptes(qrIBANDeTest, "")
	d.Currency = "EUR"
	if _, _, _, err := qrReferenceFor(d); err == nil {
		t.Fatal("un bulletin a été produit pour un QR-IBAN en euros")
	}
}

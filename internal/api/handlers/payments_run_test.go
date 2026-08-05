package handlers

// Ce qui décide d'un virement, et ce qui le refuse.
//
// La règle vient des Implementation Guidelines SIX (QR-facture v2.4 §4.2.2,
// champs 28 et 29) et vaut dans les deux sens : une référence QR EXIGE un
// QR-IBAN, un IBAN ordinaire n'accepte PAS de référence QR. L'inadéquation
// entre les deux est la première cause de rejet bancaire — mieux vaut refuser
// ici que produire un fichier que la banque renverra.

import (
	"strings"
	"testing"
)

const (
	ibanOrdinaire = "CH5604835012345678009"
	qrIBAN        = "CH4431999123000889012" // institution 30000–31999 : QR-IBAN
	refQR         = "210000000003139471430009017"
	refSCOR       = "RF18539007547034"
)

func TestUneReferenceQRExigeUnQRIBAN(t *testing.T) {
	it := payableItem{SupplierName: "Papeterie", Amount: 100, PaymentReference: refQR}
	decidePayment(&it, ibanOrdinaire, "") // pas de QR-IBAN sur la fiche

	if it.BlockedReason == "" {
		t.Fatal("une référence QR a été acceptée avec un IBAN ordinaire")
	}
	// Le refus doit dire OÙ corriger : « QR-IBAN manquant » sans indiquer la
	// fiche du fournisseur laisse chercher.
	if !strings.Contains(it.BlockedReason, "QR-IBAN") ||
		!strings.Contains(it.BlockedReason, "Papeterie") {
		t.Fatalf("le refus n'oriente pas : %s", it.BlockedReason)
	}
}

func TestUneReferenceQRAvecQRIBANPasse(t *testing.T) {
	it := payableItem{SupplierName: "Papeterie", Amount: 100, PaymentReference: refQR}
	decidePayment(&it, ibanOrdinaire, qrIBAN)

	if it.BlockedReason != "" {
		t.Fatalf("refusé alors que tout est en règle : %s", it.BlockedReason)
	}
	if it.ReferenceType != "QRR" {
		t.Errorf("type de référence = %q, attendu QRR", it.ReferenceType)
	}
	// Le QR-IBAN prime : payer une facture à référence QR sur l'IBAN ordinaire
	// du fournisseur fait rejeter le virement.
	if it.IBAN != qrIBAN {
		t.Errorf("IBAN retenu = %s, attendu le QR-IBAN", it.IBAN)
	}
}

func TestUneReferenceISO11649EstUneSCOR(t *testing.T) {
	it := payableItem{SupplierName: "Fournisseur", Amount: 100, PaymentReference: refSCOR}
	decidePayment(&it, ibanOrdinaire, "")

	if it.BlockedReason != "" {
		t.Fatalf("refusé : %s", it.BlockedReason)
	}
	if it.ReferenceType != "SCOR" {
		t.Errorf("type = %q, attendu SCOR", it.ReferenceType)
	}
}

// Sans référence, le virement part avec un motif en texte libre. C'est le cas
// courant hors QR-facture, et le refuser bloquerait la moitié des paiements.
func TestSansReferenceUnIBANOrdinaireSuffit(t *testing.T) {
	it := payableItem{SupplierName: "Fournisseur", Amount: 100}
	decidePayment(&it, ibanOrdinaire, "")

	if it.BlockedReason != "" {
		t.Fatalf("refusé : %s", it.BlockedReason)
	}
	if it.ReferenceType != "" {
		t.Errorf("un type de référence est inventé : %q", it.ReferenceType)
	}
}

func TestSansIBANLaFactureEstBloqueeEtLeDit(t *testing.T) {
	it := payableItem{SupplierName: "Sans IBAN Sàrl", Amount: 100}
	decidePayment(&it, "", "")

	if it.BlockedReason == "" {
		t.Fatal("une facture sans IBAN a été acceptée")
	}
	if !strings.Contains(it.BlockedReason, "Sans IBAN Sàrl") {
		t.Errorf("le refus ne nomme pas le fournisseur : %s", it.BlockedReason)
	}
}

func TestUnIBANInvalideEstRefuse(t *testing.T) {
	it := payableItem{SupplierName: "Fournisseur", Amount: 100}
	decidePayment(&it, "CH00 0000 0000 0000 0000 0", "")

	if it.BlockedReason == "" {
		t.Fatal("un IBAN dont la clé de contrôle est fausse a été accepté")
	}
	if it.IBAN != "" {
		t.Errorf("un IBAN invalide reste dans la ligne : %s", it.IBAN)
	}
}

// Une référence qui n'est ni QR ni ISO 11649 est presque toujours le numéro de
// facture recopié dans le mauvais champ. L'envoyer comme référence structurée
// ferait rejeter le paiement.
func TestUneReferenceDeFormatInconnuEstRefusee(t *testing.T) {
	it := payableItem{SupplierName: "Fournisseur", Amount: 100, PaymentReference: "FA-2026-118"}
	decidePayment(&it, ibanOrdinaire, "")

	if it.BlockedReason == "" {
		t.Fatal("une référence de format inconnu a été acceptée")
	}
	if !strings.Contains(it.BlockedReason, "effacez") {
		t.Errorf("le refus ne dit pas comment s'en sortir : %s", it.BlockedReason)
	}
}

// La référence se recopie depuis un bulletin où elle est imprimée par groupes.
func TestLesEspacesDeLaReferenceSontTolerees(t *testing.T) {
	espacee := "21 00000 00003 13947 14300 09017"
	it := payableItem{SupplierName: "Papeterie", Amount: 100, PaymentReference: espacee}
	decidePayment(&it, ibanOrdinaire, qrIBAN)

	if it.BlockedReason != "" {
		t.Fatalf("une référence imprimée par groupes est refusée : %s", it.BlockedReason)
	}
	if it.PaymentReference != refQR {
		t.Errorf("référence normalisée = %q", it.PaymentReference)
	}
}

// L'identifiant de bout en bout sert à reconnaître le débit au relevé. Un
// numéro de facture contient parfois des caractères que la norme refuse : on
// nettoie plutôt que de bloquer un paiement pour une barre oblique.
func TestLIdentifiantDeBoutEnBoutEstNettoye(t *testing.T) {
	got := endToEndID("FA/2026 118°", "secours")
	for _, interdit := range []string{"/", " ", "°"} {
		if strings.Contains(got, interdit) {
			t.Errorf("%q subsiste dans %q", interdit, got)
		}
	}
	if got == "" {
		t.Fatal("identifiant vide")
	}
	// Une référence entièrement composée de caractères refusés retombe sur
	// l'identifiant interne, plutôt que de produire un champ vide.
	if endToEndID("///", "secours") != "secours" {
		t.Error("aucun repli quand la référence ne laisse rien")
	}
}

package pdf

import (
	"strings"
	"testing"
)

// Audit croisé Gemini, série 4 : le document (HIGH-05, LOW-02). Chaque cas a été constaté sur
// un serveur de bac à sable avant d'être corrigé.

func docMultiTaux() InvoiceData {
	d := baseDoc("invoice")
	d.Lines = []InvoiceLine{
		{Description: "Ligne remisée", Quantity: 2, UnitPrice: 100, DiscountPct: 10, VATRate: 8.1, LineTotal: 180},
		{Description: "Ligne réduite", Quantity: 1, UnitPrice: 100, VATRate: 2.6, LineTotal: 100},
	}
	d.SubtotalAmount, d.VATAmount, d.TotalAmount, d.VATRate = 280, 17.18, 297.18, 8.1
	return d
}

func generer(t *testing.T, d InvoiceData) string {
	t.Helper()
	out, err := Generate(d)
	if err != nil {
		t.Fatalf("Generate : %v", err)
	}
	return texteDuPDF(out)
}

// Un nom de client portant un retour à la ligne décalait d'UNE ligne les trente-et-un champs du
// payload : tous les champs suivants se lisaient dans la mauvaise case, et la banque refusait le
// QR. Constaté : 32 lignes au lieu de 31.
func TestLePayloadQRResteALTrenteEtUneLignes(t *testing.T) {
	for nom, client := range map[string]string{
		"retour à la ligne": "Client\nInjecte",
		"retour chariot":    "Client\r\nInjecte",
		"tabulation":        "Client\tInjecte",
		"espaces en série":  "Client      Injecte",
	} {
		t.Run(nom, func(t *testing.T) {
			d := baseDoc("invoice")
			d.Customer.Name = client
			payload, err := QRPayload(d)
			if err != nil {
				t.Fatalf("QRPayload : %v", err)
			}
			lignes := strings.Split(payload, "\n")
			if len(lignes) != 31 {
				t.Fatalf("%d lignes de payload, attendu 31", len(lignes))
			}
			if got := lignes[21]; got != "Client Injecte" {
				t.Errorf("nom du débiteur = %q, attendu %q", got, "Client Injecte")
			}
		})
	}
}

// Les longueurs de la norme : 70 pour un nom, 35 pour une localité, 16 pour un code postal.
func TestLesChampsDuPayloadSontBornesALaNorme(t *testing.T) {
	d := baseDoc("invoice")
	d.Customer.Name = strings.Repeat("N", 100)
	d.Customer.Address = strings.Repeat("R", 100)
	d.Customer.City = "1000 " + strings.Repeat("L", 60)
	payload, err := QRPayload(d)
	if err != nil {
		t.Fatalf("QRPayload : %v", err)
	}
	l := strings.Split(payload, "\n")
	if n := len([]rune(l[21])); n != 70 {
		t.Errorf("nom du débiteur : %d caractères, attendu 70", n)
	}
	if n := len([]rune(l[22])); n != 70 {
		t.Errorf("rue du débiteur : %d caractères, attendu 70", n)
	}
	if n := len([]rune(l[25])); n != 35 {
		t.Errorf("localité du débiteur : %d caractères, attendu 35", n)
	}
}

// LOW-02. « SW1A 1AA Londres » : le code postal contient une espace, le collage « code ville »
// ne peut pas être redécoupé à la première. L'adresse séparée, quand on l'a, fait foi.
func TestUnCodePostalEtrangerAvecEspaceNEstPasCoupeDeTravers(t *testing.T) {
	d := baseDoc("invoice")
	d.Customer.City = "SW1A 1AA Londres"
	d.Customer.PostalCode, d.Customer.Town, d.Customer.Country = "SW1A 1AA", "Londres", "GB"
	payload, err := QRPayload(d)
	if err != nil {
		t.Fatalf("QRPayload : %v", err)
	}
	l := strings.Split(payload, "\n")
	if l[24] != "SW1A 1AA" || l[25] != "Londres" {
		t.Errorf("code postal %q, localité %q — attendu %q et %q", l[24], l[25], "SW1A 1AA", "Londres")
	}
	// Sans l'adresse séparée, le repli reste l'ancien découpage.
	if p, v := postalAndTown("", "", "4001 Basel"); p != "4001" || v != "Basel" {
		t.Errorf("repli : %q %q", p, v)
	}
}

// HIGH-05. Le rabais entrait dans le total de la ligne et nulle part ailleurs : « 2 × 100.00 »
// face à « 180.00 », une ligne qui ne se recalcule pas.
func TestLeRabaisDUneLigneSImprime(t *testing.T) {
	texte := generer(t, docMultiTaux())
	for _, attendu := range []string{"Rabais", "10%"} {
		if !strings.Contains(texte, attendu) {
			t.Errorf("le PDF ne porte pas %q : la remise de la ligne n'est pas visible", attendu)
		}
	}
	// Sans rabais, pas de colonne qui ne dirait que « - » partout.
	if strings.Contains(generer(t, baseDoc("invoice")), "Rabais") {
		t.Error("la colonne Rabais s'affiche sur une facture sans rabais")
	}
}

// HIGH-05. 14.58 à 8,1 % et 2.60 à 2,6 % s'imprimaient « TVA 8.1 % : 17.18 », un montant qui n'est
// le produit d'aucun taux. L'impôt s'imprime taux par taux (LTVA art. 26 al. 2 let. g).
func TestLaTVASImprimeTauxParTaux(t *testing.T) {
	texte := generer(t, docMultiTaux())
	for _, attendu := range []string{"TVA 8.1%:", "TVA 2.6%:", "14.58", "2.60"} {
		if !strings.Contains(texte, attendu) {
			t.Errorf("le PDF ne porte pas %q", attendu)
		}
	}
	if strings.Contains(texte, "17.18") {
		t.Error("le PDF porte encore le total 17.18 face à un seul taux")
	}
	// Un seul taux : une seule ligne, comme avant.
	un := generer(t, baseDoc("invoice"))
	if strings.Count(un, "TVA 8.1%:") != 1 || strings.Contains(un, "TVA 2.6%") {
		t.Errorf("facture à un taux : %d ligne(s) de TVA", strings.Count(un, "TVA "))
	}
	// Une facture dont les lignes ne retombent pas sur la TVA enregistrée (version antérieure)
	// garde la ligne unique, exacte : deux lignes qui ne s'additionnent pas seraient pires.
	ancien := docMultiTaux()
	ancien.VATAmount = 17.00
	ancien.TotalAmount = 297
	if t2 := generer(t, ancien); strings.Contains(t2, "TVA 2.6%:") {
		t.Error("deux lignes de TVA qui ne s'additionnent pas à la TVA enregistrée")
	}
}

// HIGH-05. Les conditions de paiement saisies à l'écran et enregistrées ne sortaient sur aucun PDF.
func TestLesConditionsDePaiementSImprimentDansLaLangueDemandee(t *testing.T) {
	cond := "Paiement a 30 jours net"
	for lang, titre := range map[string]string{
		"fr": "Conditions de paiement", "de": "Zahlungsbedingungen", "it": "Condizioni di pagamento", "en": "Payment terms",
	} {
		d := baseDoc("invoice")
		d.Terms = &cond
		d.Lang = lang
		texte := generer(t, d)
		if !strings.Contains(texte, titre) || !strings.Contains(texte, cond) {
			t.Errorf("[%s] conditions absentes du PDF (titre %q)", lang, titre)
		}
	}
	if strings.Contains(generer(t, baseDoc("invoice")), "Conditions de paiement") {
		t.Error("un titre « Conditions de paiement » sans conditions")
	}
}

// HIGH-05. Le bloc « Paiement par virement bancaire » imprimait le QR-IBAN : un virement ordinaire
// vers un QR-IBAN est refusé. L'IBAN ordinaire s'y trouve, ou rien.
func TestLeBlocDeVirementNImprimeJamaisLeQRIBAN(t *testing.T) {
	// Les deux comptes : l'ordinaire s'imprime, le QR-IBAN non.
	d := baseDoc("invoice")
	d.Company.IBAN, d.Company.QRIBAN, d.Company.BankName = "CH9300762011623852957", qrIBANDeTest, "Banque Test"
	texte := generer(t, d)
	if !strings.Contains(texte, "CH93 0076 2011 6238 5295 7") {
		t.Error("l'IBAN ordinaire manque au bloc de virement")
	}
	// Le bulletin porte le QR-IBAN (avec sa référence QRR) ; le bloc de virement, lui, non.
	bloc := blocDeVirement(t, texte)
	if strings.Contains(bloc, "CH44 3000") {
		t.Errorf("le bloc de virement porte le QR-IBAN : %s", bloc)
	}

	// Un QR-IBAN saisi dans la seule case « IBAN » : aucune ligne IBAN dans ce bloc.
	d = baseDoc("invoice")
	d.Company.IBAN, d.Company.BankName = qrIBANDeTest, "Banque Test"
	texte = generer(t, d)
	bloc = blocDeVirement(t, texte)
	if strings.Contains(bloc, "IBAN :") || strings.Contains(bloc, "CH44 3000") {
		t.Errorf("le bloc de virement porte un QR-IBAN comme IBAN : %s", bloc)
	}
}

// blocDeVirement rend le texte du bloc « Paiement par virement bancaire », du titre au
// bénéficiaire. Le PDF est en Latin-1 : on cherche les mots tels qu'ils y sont écrits, sans quoi
// la recherche ne trouve rien et les assertions qui suivent ne vérifient plus rien.
func blocDeVirement(t *testing.T, texte string) string {
	t.Helper()
	debut := strings.Index(texte, "Paiement par virement")
	fin := strings.Index(texte, latin1("Bénéficiaire"))
	if debut < 0 || fin < debut {
		t.Fatalf("bloc de virement introuvable (début %d, fin %d) dans : %s", debut, fin, texte)
	}
	return texte[debut:fin]
}

// MED-03. Sur une période partielle, le document SUSPEND son verdict sur le régime (les seuils
// portent sur le dernier exercice) — mais un dépassement du seuil de la TVA reste concluant : 150 000
// sur un trimestre dépassent les 100 000, et ne peuvent que s'aggraver en s'allongeant.
func TestUnDepassementDuSeuilTVAAvertitSurUnTrimestre(t *testing.T) {
	d := carnetExemple()
	d.Du, d.Au = "2026-01-01", "2026-03-31"
	d.SurExerciceComplet = false
	d.ChiffreAffaires = 150000
	d.AssujettiTVA = true

	b, err := GenerateCarnet(d)
	if err != nil {
		t.Fatalf("GenerateCarnet : %v", err)
	}
	texte := texteDuPDF(b)
	if !strings.Contains(texte, "assujettissement") || !strings.Contains(texte, "100 000") {
		t.Errorf("le dépassement du seuil de la TVA n'est pas signalé sur un trimestre : %s", texte)
	}
	if !strings.Contains(texte, "riode inf") {
		t.Error("le refus de conclure sur le régime a disparu")
	}

	// Sous le seuil de la TVA : rien n'est affirmé (ni dépassement, ni libération).
	d.ChiffreAffaires, d.AssujettiTVA = 40000, false
	b, _ = GenerateCarnet(d)
	for _, interdit := range []string{"ration de l'assujettissement", "est admise"} {
		if strings.Contains(texteDuPDF(b), interdit) {
			t.Errorf("le document affirme %q sur un trimestre", interdit)
		}
	}
}

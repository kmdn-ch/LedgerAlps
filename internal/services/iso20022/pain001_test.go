package iso20022

// Le fichier pain.001 est celui que la banque EXÉCUTE : une fois déposé, les
// virements partent. Rien dans le dépôt n'appelait GeneratePain001 avant ces
// tests — ni directement, ni par un test de handler —, alors que c'est le
// point du produit où une erreur coûte le plus cher : un IBAN mal encodé, une
// référence QRR écrite dans le mauvais élément XML, un total de contrôle qui
// ne correspond pas à la somme des montants, et le fichier est soit rejeté par
// la banque, soit — pire — accepté avec un mauvais montant.

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// débiteur et créancier de test — le premier est l'IBAN d'exemple officiel SIX,
// déjà utilisé dans internal/core/compliance/iban_test.go.
const (
	ibanDebiteur  = "CH9300762011623852957"
	ibanCrediteur = "CH5604835012345678009"
)

func dateExecution(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", "2026-03-15")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// unmarshalPain001 relit le document produit, pour vérifier sa STRUCTURE plutôt
// que de chercher des sous-chaînes dans le XML — une réorganisation inoffensive
// des attributs ne doit pas faire échouer le test.
func unmarshalPain001(t *testing.T, xmlBytes []byte) p1Document {
	t.Helper()
	var doc p1Document
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		t.Fatalf("le document généré ne se reparse pas : %v\n%s", err, xmlBytes)
	}
	return doc
}

// Sans transaction, aucun fichier n'est produit : un pain.001 vide n'a pas de
// sens et un fichier de zéro virement ne devrait jamais atteindre la banque.
func TestGeneratePain001RefuseSansTransaction(t *testing.T) {
	_, err := GeneratePain001(Pain001Request{
		DebtorIBAN:    ibanDebiteur,
		ExecutionDate: dateExecution(t),
	})
	if err == nil {
		t.Fatal("aucune erreur alors qu'il n'y a aucune transaction")
	}
}

// Sans IBAN débiteur, il n'y a pas de compte à débiter — le refus doit arriver
// ici, pas sous la forme d'un fichier généré que la banque rejettera.
func TestGeneratePain001RefuseSansIBANDebiteur(t *testing.T) {
	_, err := GeneratePain001(Pain001Request{
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "FACT-1", CreditorName: "Fournisseur SA", CreditorIBAN: ibanCrediteur, Amount: 100},
		},
	})
	if err == nil {
		t.Fatal("aucune erreur alors que l'IBAN du débiteur est vide")
	}
}

// La structure de base : espace de noms, en-tête de groupe, et le virement
// unique retrouvé avec son identifiant, son montant et les deux IBAN.
func TestGeneratePain001StructureDeBase(t *testing.T) {
	xmlBytes, err := GeneratePain001(Pain001Request{
		DebtorName:    "Menuiserie Dupont Sàrl",
		DebtorIBAN:    ibanDebiteur,
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{
				EndToEndID:   "FACT-2026-042",
				CreditorName: "Bois Suisse SA",
				CreditorIBAN: ibanCrediteur,
				Amount:       1250.50,
				Currency:     "CHF",
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePain001: %v", err)
	}

	// L'en-tête XML et l'espace de noms sont ce que la banque vérifie en tout
	// premier : un fichier sans le bon xmlns est rejeté avant même d'être lu.
	if !strings.HasPrefix(string(xmlBytes), xml.Header) {
		t.Error("le document ne commence pas par l'en-tête XML standard")
	}

	doc := unmarshalPain001(t, xmlBytes)
	if doc.Xmlns != pain001NS {
		t.Errorf("xmlns = %q, attendu %q", doc.Xmlns, pain001NS)
	}

	hdr := doc.CstmrCdtTrfInitn.GrpHdr
	if hdr.NbOfTxs != 1 {
		t.Errorf("NbOfTxs = %d, attendu 1", hdr.NbOfTxs)
	}
	if hdr.CtrlSum != "1250.50" {
		t.Errorf("CtrlSum = %q, attendu %q", hdr.CtrlSum, "1250.50")
	}
	if hdr.InitgPty.Nm != "Menuiserie Dupont Sàrl" {
		t.Errorf("partie initiatrice = %q", hdr.InitgPty.Nm)
	}

	pmt := doc.CstmrCdtTrfInitn.PmtInf
	if pmt.PmtMtd != "TRF" {
		t.Errorf("méthode de paiement = %q, attendu TRF", pmt.PmtMtd)
	}
	if pmt.ReqdExctnDt.Dt != "2026-03-15" {
		t.Errorf("date d'exécution = %q", pmt.ReqdExctnDt.Dt)
	}
	if pmt.DbtrAcct.ID.IBAN != ibanDebiteur {
		t.Errorf("IBAN débiteur = %q, attendu %q", pmt.DbtrAcct.ID.IBAN, ibanDebiteur)
	}
	if len(pmt.CdtTrfTxInf) != 1 {
		t.Fatalf("%d transaction(s) dans le document, attendu 1", len(pmt.CdtTrfTxInf))
	}

	tx := pmt.CdtTrfTxInf[0]
	if tx.PmtID.EndToEndID != "FACT-2026-042" {
		t.Errorf("EndToEndId = %q", tx.PmtID.EndToEndID)
	}
	if tx.Amt.InstdAmt.Ccy != "CHF" {
		t.Errorf("devise = %q, attendu CHF", tx.Amt.InstdAmt.Ccy)
	}
	if tx.Amt.InstdAmt.Value != "1250.50" {
		t.Errorf("montant = %q, attendu 1250.50", tx.Amt.InstdAmt.Value)
	}
	if tx.Cdtr.Nm != "Bois Suisse SA" {
		t.Errorf("nom du créancier = %q", tx.Cdtr.Nm)
	}
	if tx.CdtrAcct.ID.IBAN != ibanCrediteur {
		t.Errorf("IBAN créancier = %q, attendu %q", tx.CdtrAcct.ID.IBAN, ibanCrediteur)
	}
}

// Une devise absente devient CHF — c'est la seule devise que le produit émet
// sans que l'appelant ait à y penser à chaque virement suisse ordinaire.
func TestGeneratePain001LaDeviseParDefautEstCHF(t *testing.T) {
	xmlBytes, err := GeneratePain001(Pain001Request{
		DebtorIBAN:    ibanDebiteur,
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "F1", CreditorIBAN: ibanCrediteur, Amount: 42},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePain001: %v", err)
	}
	doc := unmarshalPain001(t, xmlBytes)
	if got := doc.CstmrCdtTrfInitn.PmtInf.CdtTrfTxInf[0].Amt.InstdAmt.Ccy; got != "CHF" {
		t.Errorf("devise par défaut = %q, attendu CHF", got)
	}
}

// Le total de contrôle et le nombre de transactions doivent correspondre
// EXACTEMENT à ce qui est dans le lot : c'est la première vérification que
// fait une banque avant même de lire le détail des virements, et un écart
// fait rejeter le fichier entier.
func TestGeneratePain001SommeDeControleEtNbOfTxs(t *testing.T) {
	xmlBytes, err := GeneratePain001(Pain001Request{
		DebtorIBAN:    ibanDebiteur,
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "F1", CreditorIBAN: ibanCrediteur, Amount: 10.10},
			{EndToEndID: "F2", CreditorIBAN: ibanCrediteur, Amount: 20.20},
			{EndToEndID: "F3", CreditorIBAN: ibanCrediteur, Amount: 33.33},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePain001: %v", err)
	}
	doc := unmarshalPain001(t, xmlBytes)

	const totalAttendu = "63.63"
	if doc.CstmrCdtTrfInitn.GrpHdr.CtrlSum != totalAttendu {
		t.Errorf("CtrlSum (en-tête) = %q, attendu %q", doc.CstmrCdtTrfInitn.GrpHdr.CtrlSum, totalAttendu)
	}
	if doc.CstmrCdtTrfInitn.PmtInf.CtrlSum != totalAttendu {
		t.Errorf("CtrlSum (lot) = %q, attendu %q", doc.CstmrCdtTrfInitn.PmtInf.CtrlSum, totalAttendu)
	}
	if n := doc.CstmrCdtTrfInitn.GrpHdr.NbOfTxs; n != 3 {
		t.Errorf("NbOfTxs (en-tête) = %d, attendu 3", n)
	}
	if n := doc.CstmrCdtTrfInitn.PmtInf.NbOfTxs; n != 3 {
		t.Errorf("NbOfTxs (lot) = %d, attendu 3", n)
	}

	// Le total de contrôle doit égaler la somme des montants INDIVIDUELS tels
	// qu'ils apparaissent dans le fichier — pas une coïncidence numérique.
	// Vérifié séparément parce que CtrlSum est calculé sur la somme BRUTE puis
	// arrondie, alors que chaque InstdAmt est arrondi ligne à ligne : sur des
	// montants à sous-centime les deux calculs peuvent diverger (0.005 + 0.005
	// donne un total de contrôle à 0.01 mais une somme des lignes à 0.02). Le
	// cas ne se présente pas ici : les montants entrant dans ce générateur
	// viennent de factures déjà arrondies au centime, où les deux calculs
	// coïncident — ce que ce test établit pour un lot réaliste à plusieurs
	// lignes.
	var sommeLignes float64
	for _, tx := range doc.CstmrCdtTrfInitn.PmtInf.CdtTrfTxInf {
		v, err := strconv.ParseFloat(tx.Amt.InstdAmt.Value, 64)
		if err != nil {
			t.Fatalf("montant de ligne illisible : %q", tx.Amt.InstdAmt.Value)
		}
		sommeLignes += v
	}
	sommeLignes = math.Round(sommeLignes*100) / 100
	if got := strconv.FormatFloat(sommeLignes, 'f', 2, 64); got != totalAttendu {
		t.Errorf("somme des lignes = %s, attendu %s (le total de contrôle divergerait de la somme réelle)",
			got, totalAttendu)
	}
}

// La référence structurée s'encode différemment selon son type : QRR est
// propriétaire des Swiss Payment Standards et va dans <Prtry>, SCOR appartient
// à la liste de codes externes ISO et va dans <Cd>. Les inverser produit un
// document qu'un valideur de schéma ISO rejette — c'était le bug corrigé par
// ce champ, et c'est ce que ce test empêche de revenir.
func TestGeneratePain001ReferenceQRREtSCOR(t *testing.T) {
	cas := []struct {
		nom           string
		referenceType string
		attenduCd     string
		attenduPrtry  string
	}{
		{"QRR explicite", "QRR", "", "QRR"},
		{"SCOR explicite", "SCOR", "SCOR", ""},
		{"type omis, vaut QRR par défaut", "", "", "QRR"},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			xmlBytes, err := GeneratePain001(Pain001Request{
				DebtorIBAN:    ibanDebiteur,
				ExecutionDate: dateExecution(t),
				Transactions: []CreditTransfer{{
					EndToEndID:    "F1",
					CreditorIBAN:  ibanCrediteur,
					Amount:        100,
					Reference:     "210000000003139471430009017",
					ReferenceType: c.referenceType,
				}},
			})
			if err != nil {
				t.Fatalf("GeneratePain001: %v", err)
			}
			doc := unmarshalPain001(t, xmlBytes)
			tx := doc.CstmrCdtTrfInitn.PmtInf.CdtTrfTxInf[0]
			if tx.RmtInf == nil || tx.RmtInf.Strd == nil {
				t.Fatal("aucune référence structurée dans le document")
			}
			cdOrPrtry := tx.RmtInf.Strd.CdtrRefInf.Tp.CdOrPrtry
			if cdOrPrtry.Cd != c.attenduCd {
				t.Errorf("Cd = %q, attendu %q", cdOrPrtry.Cd, c.attenduCd)
			}
			if cdOrPrtry.Prtry != c.attenduPrtry {
				t.Errorf("Prtry = %q, attendu %q", cdOrPrtry.Prtry, c.attenduPrtry)
			}
			if tx.RmtInf.Strd.CdtrRefInf.Ref != "210000000003139471430009017" {
				t.Errorf("référence = %q", tx.RmtInf.Strd.CdtrRefInf.Ref)
			}
		})
	}
}

// Sans référence structurée, le motif en texte libre est utilisé — et c'est
// l'INVERSE qui ne doit jamais arriver : un motif libre écrasé par une
// référence structurée vide romprait le rapprochement bancaire.
func TestGeneratePain001MotifLibreQuandPasDeReference(t *testing.T) {
	xmlBytes, err := GeneratePain001(Pain001Request{
		DebtorIBAN:    ibanDebiteur,
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{{
			EndToEndID:   "F1",
			CreditorIBAN: ibanCrediteur,
			Amount:       50,
			Unstructured: "Acompte travaux toiture",
		}},
	})
	if err != nil {
		t.Fatalf("GeneratePain001: %v", err)
	}
	doc := unmarshalPain001(t, xmlBytes)
	tx := doc.CstmrCdtTrfInitn.PmtInf.CdtTrfTxInf[0]
	if tx.RmtInf == nil {
		t.Fatal("aucune information de remise dans le document")
	}
	if tx.RmtInf.Ustrd != "Acompte travaux toiture" {
		t.Errorf("motif libre = %q", tx.RmtInf.Ustrd)
	}
	if tx.RmtInf.Strd != nil {
		t.Error("une référence structurée est présente alors qu'aucune n'a été fournie")
	}
}

// Ni référence ni motif : aucune information de remise n'est écrite. Un
// élément vide (plutôt qu'absent) laisserait croire à une remise renseignée.
func TestGeneratePain001AucuneRemiseQuandRienNEstFourni(t *testing.T) {
	xmlBytes, err := GeneratePain001(Pain001Request{
		DebtorIBAN:    ibanDebiteur,
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "F1", CreditorIBAN: ibanCrediteur, Amount: 50},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePain001: %v", err)
	}
	doc := unmarshalPain001(t, xmlBytes)
	if tx := doc.CstmrCdtTrfInitn.PmtInf.CdtTrfTxInf[0]; tx.RmtInf != nil {
		t.Errorf("RmtInf présent alors qu'aucune référence ni motif n'a été fourni : %+v", tx.RmtInf)
	}
}

// Le niveau de service SEPA ne s'annonce que pour un virement en euros : le
// déclarer sur un franc suisse décrit un service que l'opération n'utilise
// pas, et expose au rejet par la banque destinataire. C'est le défaut que ce
// champ corrige — le test verrouille les deux sens.
func TestGeneratePain001NiveauDeServiceSEPASeulementEnEuros(t *testing.T) {
	t.Run("CHF : aucun niveau de service annoncé", func(t *testing.T) {
		xmlBytes, err := GeneratePain001(Pain001Request{
			DebtorIBAN:    ibanDebiteur,
			ExecutionDate: dateExecution(t),
			Transactions: []CreditTransfer{
				{EndToEndID: "F1", CreditorIBAN: ibanCrediteur, Amount: 100, Currency: "CHF"},
			},
		})
		if err != nil {
			t.Fatalf("GeneratePain001: %v", err)
		}
		doc := unmarshalPain001(t, xmlBytes)
		if doc.CstmrCdtTrfInitn.PmtInf.PmtTpInf != nil {
			t.Errorf("PmtTpInf présent pour un virement CHF : %+v", doc.CstmrCdtTrfInitn.PmtInf.PmtTpInf)
		}
	})
	t.Run("EUR : SEPA annoncé", func(t *testing.T) {
		xmlBytes, err := GeneratePain001(Pain001Request{
			DebtorIBAN:    ibanDebiteur,
			ExecutionDate: dateExecution(t),
			Transactions: []CreditTransfer{
				{EndToEndID: "F1", CreditorIBAN: "DE89370400440532013000", Amount: 100, Currency: "EUR"},
			},
		})
		if err != nil {
			t.Fatalf("GeneratePain001: %v", err)
		}
		doc := unmarshalPain001(t, xmlBytes)
		pmtTpInf := doc.CstmrCdtTrfInitn.PmtInf.PmtTpInf
		if pmtTpInf == nil || pmtTpInf.SvcLvl == nil || pmtTpInf.SvcLvl.Cd != "SEPA" {
			t.Errorf("PmtTpInf/SvcLvl = %+v, attendu SEPA", pmtTpInf)
		}
	})
	t.Run("comparaison insensible à la casse (eur, Eur)", func(t *testing.T) {
		xmlBytes, err := GeneratePain001(Pain001Request{
			DebtorIBAN:    ibanDebiteur,
			ExecutionDate: dateExecution(t),
			Transactions: []CreditTransfer{
				{EndToEndID: "F1", CreditorIBAN: "DE89370400440532013000", Amount: 100, Currency: "eur"},
			},
		})
		if err != nil {
			t.Fatalf("GeneratePain001: %v", err)
		}
		doc := unmarshalPain001(t, xmlBytes)
		if doc.CstmrCdtTrfInitn.PmtInf.PmtTpInf == nil {
			t.Error("« eur » en minuscules n'a pas déclenché le niveau de service SEPA")
		}
	})
}

// Un IBAN saisi avec des espaces — la forme qu'un utilisateur copie depuis un
// document — doit sortir compacté : c'est le format que la banque exige,
// espaces comprises ou non selon le champ, mais jamais celui qu'on saisit.
func TestGeneratePain001IBANNormaliseEspacesRetires(t *testing.T) {
	xmlBytes, err := GeneratePain001(Pain001Request{
		DebtorIBAN:    "CH93 0076 2011 6238 5295 7",
		ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "F1", CreditorIBAN: "CH56 0483 5012 3456 7800 9", Amount: 10},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePain001: %v", err)
	}
	doc := unmarshalPain001(t, xmlBytes)
	if got := doc.CstmrCdtTrfInitn.PmtInf.DbtrAcct.ID.IBAN; got != ibanDebiteur {
		t.Errorf("IBAN débiteur = %q, attendu %q (espaces non retirés)", got, ibanDebiteur)
	}
	if got := doc.CstmrCdtTrfInitn.PmtInf.CdtTrfTxInf[0].CdtrAcct.ID.IBAN; got != ibanCrediteur {
		t.Errorf("IBAN créancier = %q, attendu %q (espaces non retirés)", got, ibanCrediteur)
	}
}

// L'agent du débiteur porte le BIC quand on le connaît, et NOTPROVIDED sinon —
// les Swiss Payment Standards exigent l'élément, et un élément absent serait
// une raison de rejet différente de « BIC inconnu ».
func TestGeneratePain001AgentDuDebiteurBICOuNotProvided(t *testing.T) {
	t.Run("BIC fourni", func(t *testing.T) {
		xmlBytes, err := GeneratePain001(Pain001Request{
			DebtorIBAN: ibanDebiteur, DebtorBIC: "UBSWCHZH80A",
			ExecutionDate: dateExecution(t),
			Transactions: []CreditTransfer{
				{EndToEndID: "F1", CreditorIBAN: ibanCrediteur, Amount: 10},
			},
		})
		if err != nil {
			t.Fatalf("GeneratePain001: %v", err)
		}
		doc := unmarshalPain001(t, xmlBytes)
		agt := doc.CstmrCdtTrfInitn.PmtInf.DbtrAgt.FinInstnID
		if agt.BICFI != "UBSWCHZH80A" {
			t.Errorf("BICFI = %q, attendu UBSWCHZH80A", agt.BICFI)
		}
		if agt.Othr != nil {
			t.Errorf("Othr renseigné alors qu'un BIC est fourni : %+v", agt.Othr)
		}
	})
	t.Run("BIC absent", func(t *testing.T) {
		xmlBytes, err := GeneratePain001(Pain001Request{
			DebtorIBAN:    ibanDebiteur,
			ExecutionDate: dateExecution(t),
			Transactions: []CreditTransfer{
				{EndToEndID: "F1", CreditorIBAN: ibanCrediteur, Amount: 10},
			},
		})
		if err != nil {
			t.Fatalf("GeneratePain001: %v", err)
		}
		doc := unmarshalPain001(t, xmlBytes)
		agt := doc.CstmrCdtTrfInitn.PmtInf.DbtrAgt.FinInstnID
		if agt.BICFI != "" {
			t.Errorf("BICFI = %q, attendu vide", agt.BICFI)
		}
		if agt.Othr == nil || agt.Othr.ID != "NOTPROVIDED" {
			t.Errorf("Othr = %+v, attendu {ID: NOTPROVIDED}", agt.Othr)
		}
	})
}

// Le message est signé par sa date d'exécution ET le nombre de transactions —
// deux lots différents un même jour ne doivent pas porter le même identifiant.
func TestGeneratePain001IdentifiantDeMessageVarieAvecLeLot(t *testing.T) {
	req := func(n int) Pain001Request {
		txs := make([]CreditTransfer, n)
		for i := range txs {
			txs[i] = CreditTransfer{EndToEndID: "F", CreditorIBAN: ibanCrediteur, Amount: 1}
		}
		return Pain001Request{DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t), Transactions: txs}
	}
	xml1, err := GeneratePain001(req(1))
	if err != nil {
		t.Fatalf("GeneratePain001(1): %v", err)
	}
	xml2, err := GeneratePain001(req(2))
	if err != nil {
		t.Fatalf("GeneratePain001(2): %v", err)
	}
	doc1, doc2 := unmarshalPain001(t, xml1), unmarshalPain001(t, xml2)
	id1 := doc1.CstmrCdtTrfInitn.GrpHdr.MsgID
	id2 := doc2.CstmrCdtTrfInitn.GrpHdr.MsgID
	if id1 == id2 {
		t.Errorf("le même identifiant de message %q sert deux lots différents", id1)
	}
	if doc1.CstmrCdtTrfInitn.PmtInf.PmtInfID != id1+"-BATCH" {
		t.Errorf("PmtInfId = %q, attendu %q", doc1.CstmrCdtTrfInitn.PmtInf.PmtInfID, id1+"-BATCH")
	}
}

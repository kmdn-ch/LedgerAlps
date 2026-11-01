package iso20022

import (
	"strings"
	"testing"
	"time"
)

// Audit croisé Gemini, série 4, HIGH-06 : un lot mêlant euros et francs, et une somme de contrôle
// qui doit être celle des montants IMPRIMÉS.

// Un lot mêlant EUR et CHF, euro en tête, déclarait SEPA le paiement en francs : le niveau de
// service vaut pour le bloc entier. Chaque devise a désormais son bloc.
func TestUnLotMelantEurosEtFrancsFaitUnBlocParDevise(t *testing.T) {
	out, err := GeneratePain001(Pain001Request{
		DebtorName: "Debiteur SA", DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "A", CreditorName: "Fr SA", CreditorIBAN: "FR7630006000011234567890189", Amount: 100, Currency: "EUR"},
			{EndToEndID: "B", CreditorName: "Ch SA", CreditorIBAN: ibanCrediteur, Amount: 200.50, Currency: "CHF"},
			{EndToEndID: "C", CreditorName: "Ch2 SA", CreditorIBAN: ibanCrediteur, Amount: 0.10, Currency: "CHF"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := unmarshalPain001(t, out)
	blocs := doc.CstmrCdtTrfInitn.PmtInf
	if len(blocs) != 2 {
		t.Fatalf("%d bloc(s) de paiement, attendu 2 (un par devise)", len(blocs))
	}
	eur, chf := blocs[0], blocs[1]
	if eur.PmtTpInf == nil || eur.PmtTpInf.SvcLvl == nil || eur.PmtTpInf.SvcLvl.Cd != "SEPA" {
		t.Errorf("le bloc en euros n'est pas SEPA : %+v", eur.PmtTpInf)
	}
	if chf.PmtTpInf != nil {
		t.Errorf("le bloc en francs est marqué SEPA : %+v", chf.PmtTpInf)
	}
	if eur.NbOfTxs != 1 || chf.NbOfTxs != 2 || eur.CtrlSum != "100.00" || chf.CtrlSum != "200.60" {
		t.Errorf("blocs : %d/%s et %d/%s, attendu 1/100.00 et 2/200.60", eur.NbOfTxs, eur.CtrlSum, chf.NbOfTxs, chf.CtrlSum)
	}
	if eur.PmtInfID == chf.PmtInfID || len(eur.PmtInfID) > 35 || len(chf.PmtInfID) > 35 {
		t.Errorf("identifiants de bloc %q et %q : distincts et de 35 caractères au plus", eur.PmtInfID, chf.PmtInfID)
	}
	// L'en-tête additionne tout, en centimes.
	if h := doc.CstmrCdtTrfInitn.GrpHdr; h.NbOfTxs != 3 || h.CtrlSum != "300.60" {
		t.Errorf("en-tête : %d / %s, attendu 3 / 300.60 (toutes devises confondues, comme la norme le définit)", h.NbOfTxs, h.CtrlSum)
	}
}

// Deux virements de 10,004 s'impriment à 10.00 : la somme de contrôle de 20.01 ne correspondait à
// aucun fichier que la banque pouvait recompter.
func TestLaSommeDeControleEstCelleDesMontantsImprimes(t *testing.T) {
	out, err := GeneratePain001(Pain001Request{
		DebtorName: "D", DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{
			{EndToEndID: "A", CreditorName: "X", CreditorIBAN: ibanCrediteur, Amount: 10.004},
			{EndToEndID: "B", CreditorName: "X", CreditorIBAN: ibanCrediteur, Amount: 10.004},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := unmarshalPain001(t, out)
	if got := doc.CstmrCdtTrfInitn.GrpHdr.CtrlSum; got != "20.00" {
		t.Errorf("CtrlSum = %q, attendu 20.00 (deux montants de 10.00)", got)
	}
	if got := doc.CstmrCdtTrfInitn.PmtInf[0].CtrlSum; got != "20.00" {
		t.Errorf("CtrlSum du bloc = %q, attendu 20.00", got)
	}
}

// Un identifiant de bout en bout vide donnait <EndToEndId></EndToEndId> ; la valeur prévue par les
// Swiss Payment Standards quand l'ordre n'en porte pas est NOTPROVIDED.
func TestUnIdentifiantDeBoutEnBoutVideDevientNOTPROVIDED(t *testing.T) {
	out, err := GeneratePain001(Pain001Request{
		DebtorName: "D", DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t),
		Transactions: []CreditTransfer{{CreditorName: "X", CreditorIBAN: ibanCrediteur, Amount: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "<EndToEndId></EndToEndId>") || !strings.Contains(string(out), "<EndToEndId>NOTPROVIDED</EndToEndId>") {
		t.Errorf("identifiant de bout en bout vide mal traité :\n%s", out)
	}
}

// MED-10. <BookgDt> et <ValDt> admettent <Dt> OU <DtTm> : <DtTm> n'était jamais lu, et l'écriture
// d'une banque qui n'envoie que l'horodatage sortait sans date.
func TestLesDatesEnHorodatageAvecFuseauSontLues(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08"><BkToCstmrStmt><GrpHdr><MsgId>X</MsgId></GrpHdr>
<Stmt><Id>1</Id>
<Ntry><Amt Ccy="CHF">100.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Sts><Cd>BOOK</Cd></Sts>
<BookgDt><DtTm>2026-10-31T23:30:00+01:00</DtTm></BookgDt><ValDt><DtTm>2026-11-01T00:10:00.250+01:00</DtTm></ValDt>
<AcctSvcrRef>R1</AcctSvcrRef></Ntry>
<Ntry><Amt Ccy="CHF">50.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Sts><Cd>BOOK</Cd></Sts>
<BookgDt><Dt>2026-10-31</Dt></BookgDt><ValDt><Dt>2026-10-31</Dt></ValDt><AcctSvcrRef>R2</AcctSvcrRef></Ntry>
</Stmt></BkToCstmrStmt></Document>`
	es, err := ParseCamt053([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("%d écritures, attendu 2", len(es))
	}
	// La date est celle du calendrier de la banque, telle qu'écrite : 23:30 +01:00 le 31 octobre
	// est le 31 octobre, et 00:10 +01:00 le 1er novembre est le 1er novembre — quel que soit le
	// fuseau du serveur.
	if got := es[0].BookingDate.Format("2006-01-02"); got != "2026-10-31" {
		t.Errorf("date de comptabilisation en DtTm = %s, attendu 2026-10-31", got)
	}
	if got := es[0].ValueDate.Format("2006-01-02"); got != "2026-11-01" {
		t.Errorf("date de valeur en DtTm = %s, attendu 2026-11-01", got)
	}
	// Une date et une heure ne se distinguent pas dans la suite : tout est à minuit UTC.
	if es[0].BookingDate.Hour() != 0 || es[0].BookingDate.Location() != time.UTC {
		t.Errorf("la date n'est pas ramenée à minuit UTC : %v", es[0].BookingDate)
	}
	if es[1].BookingDate.Format("2006-01-02") != "2026-10-31" {
		t.Errorf("la forme <Dt> ne se lit plus : %v", es[1].BookingDate)
	}
}

// ─── Audit croisé Gemini, HIGH-06 (reliquat) : l'adresse postale structurée ───────────────────────
//
// Les banques suisses attendent une adresse STRUCTURÉE dans l'ordre de paiement — localité et pays
// obligatoires — et ne garantissent plus les adresses non structurées de bout en bout à partir de
// novembre 2026 (avis PostFinance, ZKB, UBS). Le texte des lignes directrices de SIX n'a pas pu être lu.

func ordreAdresse(t *testing.T, xml string, bloc string) string {
	t.Helper()
	i := strings.Index(xml, "<"+bloc+">")
	j := strings.Index(xml[i:], "</"+bloc+">")
	if i < 0 || j < 0 {
		t.Fatalf("bloc <%s> introuvable dans :\n%s", bloc, xml)
	}
	return xml[i : i+j]
}

func TestLesAdressesStructureesSontEmisesDansLOrdreDuSchema(t *testing.T) {
	out, err := GeneratePain001(Pain001Request{
		DebtorName: "Debiteur SA", DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t),
		DebtorAddress: PostalAddress{Street: "Rue du Lac 3", PostCode: "1003", Town: "Lausanne", Country: "ch"},
		Transactions: []CreditTransfer{{
			EndToEndID: "A", CreditorName: "Papeterie Alpes", CreditorIBAN: ibanCrediteur, Amount: 100,
			CreditorAddress: PostalAddress{Street: "Bahnhofstrasse 1", PostCode: "8001", Town: "Zürich", Country: "CH"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	xml := string(out)

	for nom, attendu := range map[string][]string{
		"Dbtr": {"<StrtNm>Rue du Lac 3</StrtNm>", "<PstCd>1003</PstCd>", "<TwnNm>Lausanne</TwnNm>", "<Ctry>CH</Ctry>"},
		"Cdtr": {"<StrtNm>Bahnhofstrasse 1</StrtNm>", "<PstCd>8001</PstCd>", "<TwnNm>Zürich</TwnNm>", "<Ctry>CH</Ctry>"},
	} {
		bloc := ordreAdresse(t, xml, nom)
		dernier := -1
		for _, el := range attendu {
			i := strings.Index(bloc, el)
			if i < 0 {
				t.Errorf("<%s> sans %s :\n%s", nom, el, bloc)
				continue
			}
			if i < dernier {
				t.Errorf("<%s> : %s n'est pas dans l'ordre du schéma (rue, code postal, localité, pays)", nom, el)
			}
			dernier = i
		}
		// Le nom précède l'adresse.
		if strings.Index(bloc, "<Nm>") > strings.Index(bloc, "<PstlAdr>") {
			t.Errorf("<%s> : <Nm> doit précéder <PstlAdr>", nom)
		}
	}
	// L'initiateur n'a que son nom : l'adresse suit le débiteur, pas l'en-tête.
	if strings.Contains(ordreAdresse(t, xml, "InitgPty"), "PstlAdr") {
		t.Error("l'initiateur du message porte une adresse")
	}
}

// Un élément à moitié rempli est refusé par la banque ; une adresse absente ne l'est pas. Sans localité
// ou sans pays valide, l'adresse n'est PAS émise — le fichier reste celui d'avant.
func TestUneAdresseSansLocaliteOuPaysValideNEstPasEmise(t *testing.T) {
	for nom, a := range map[string]PostalAddress{
		"vide":                   {},
		"sans localité":          {Street: "Rue 1", PostCode: "1000", Country: "CH"},
		"sans pays":              {Street: "Rue 1", PostCode: "1000", Town: "Lausanne"},
		"pays en toutes lettres": {Street: "Rue 1", Town: "Lausanne", Country: "Suisse"},
		"pays chiffré":           {Town: "Lausanne", Country: "12"},
		"localité de blancs":     {Street: "Rue 1", Town: "   ", Country: "CH"},
	} {
		out, err := GeneratePain001(Pain001Request{
			DebtorName: "D", DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t), DebtorAddress: a,
			Transactions: []CreditTransfer{{EndToEndID: "A", CreditorName: "C", CreditorIBAN: ibanCrediteur,
				Amount: 1, CreditorAddress: a}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "PstlAdr") {
			t.Errorf("%s : une adresse incomplète est émise\n%s", nom, out)
		}
	}
}

// Une rue ou une localité trop longue se tronque aux longueurs du schéma, en caractères et non en octets
// (« é » n'en vaut qu'un), et les blancs successifs se réduisent.
func TestLesLongueursDeLAdresseSontBornees(t *testing.T) {
	long := strings.Repeat("é", 100)
	out, err := GeneratePain001(Pain001Request{
		DebtorName: "D", DebtorIBAN: ibanDebiteur, ExecutionDate: dateExecution(t),
		DebtorAddress: PostalAddress{Street: long, PostCode: strings.Repeat("9", 30), Town: long + "  x", Country: "CH"},
		Transactions:  []CreditTransfer{{EndToEndID: "A", CreditorName: "C", CreditorIBAN: ibanCrediteur, Amount: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := unmarshalPain001(t, out)
	a := doc.CstmrCdtTrfInitn.PmtInf[0].Dbtr.PstlAdr
	if a == nil {
		t.Fatal("adresse absente")
	}
	if n := len([]rune(a.StrtNm)); n != 70 {
		t.Errorf("rue de %d caractères, attendu 70", n)
	}
	if n := len([]rune(a.TwnNm)); n != 35 {
		t.Errorf("localité de %d caractères, attendu 35", n)
	}
	if n := len(a.PstCd); n != 16 {
		t.Errorf("code postal de %d caractères, attendu 16", n)
	}
}

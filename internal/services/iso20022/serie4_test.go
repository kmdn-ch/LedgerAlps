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

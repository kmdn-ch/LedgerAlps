// Package iso20022 provides generation and parsing of Swiss ISO 20022 payment messages.
// pain.001.001.09 — Customer Credit Transfer Initiation (Six-Group / SIX Interbank Clearing)
// camt.053.001.08 — Bank to Customer Statement
package iso20022

import (
	"encoding/xml"
	"fmt"
	"math"
	"strings"
	"time"
)

// ─── pain.001.001.09 — Credit Transfer Initiation ────────────────────────────

const pain001NS = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"

// Pain001Request is the input to GeneratePain001.
type Pain001Request struct {
	// Debtor (the company initiating the payments)
	DebtorName  string
	DebtorIBAN  string
	DebtorBIC   string // optional — bank BIC/SWIFT

	// Execution date (ISO 8601 date)
	ExecutionDate time.Time

	// Individual transactions
	Transactions []CreditTransfer
}

// CreditTransfer represents one outgoing payment.
type CreditTransfer struct {
	EndToEndID   string // invoice number or unique reference
	CreditorName string
	CreditorIBAN string
	Amount       float64
	Currency     string // CHF or EUR
	Reference    string // référence structurée : QR à 27 chiffres, ou ISO 11649
	// ReferenceType dit COMMENT encoder la référence : « QRR » ou « SCOR ».
	//
	// La distinction n'est pas cosmétique. SCOR appartient à la liste de codes
	// externes ISO 20022 et s'écrit dans <Cd> ; QRR est propre aux Swiss Payment
	// Standards et s'écrit dans <Prtry>. Écrire QRR dans <Cd> — ce que faisait
	// cette fonction — produit un document que les schémas ISO rejettent, parce
	// que le code n'existe pas dans leur liste.
	//
	// Vide avec une Reference non vide vaut QRR, pour ne pas casser un appel
	// existant.
	ReferenceType string
	Unstructured  string // motif en texte libre, quand il n'y a pas de référence
}

// GeneratePain001 returns a pain.001.001.09 XML document as bytes.
func GeneratePain001(req Pain001Request) ([]byte, error) {
	if len(req.Transactions) == 0 {
		return nil, fmt.Errorf("pain.001: at least one transaction required")
	}
	if req.DebtorIBAN == "" {
		return nil, fmt.Errorf("pain.001: debtor IBAN required")
	}

	msgID := fmt.Sprintf("LEDGERALPS-%s-%04d",
		req.ExecutionDate.Format("20060102"),
		len(req.Transactions))

	var ctrlSum float64
	for _, t := range req.Transactions {
		ctrlSum += t.Amount
	}
	ctrlSum = math.Round(ctrlSum*100) / 100

	txInfos := make([]p1CdtTrfTxInf, 0, len(req.Transactions))
	for _, t := range req.Transactions {
		if t.Currency == "" {
			t.Currency = "CHF"
		}
		txInfo := p1CdtTrfTxInf{
			PmtID: p1PmtID{EndToEndID: t.EndToEndID},
			Amt: p1Amt{
				InstdAmt: p1InstdAmt{
					Ccy:   t.Currency,
					Value: fmt.Sprintf("%.2f", math.Round(t.Amount*100)/100),
				},
			},
			Cdtr:    p1Party{Nm: t.CreditorName},
			CdtrAcct: p1Acct{ID: p1AcctID{IBAN: normalizeIBAN(t.CreditorIBAN)}},
		}
		if t.Reference != "" {
			kind := t.ReferenceType
			if kind == "" {
				kind = "QRR"
			}
			cdOrPrtry := p1CdOrPrtry{Prtry: kind}
			if kind == "SCOR" {
				// SCOR figure dans la liste de codes externes ISO ; QRR n'y est
				// pas et reste propriétaire.
				cdOrPrtry = p1CdOrPrtry{Cd: kind}
			}
			txInfo.RmtInf = &p1RmtInf{
				Strd: &p1Strd{
					CdtrRefInf: p1CdtrRefInf{
						Tp:  p1RefTp{CdOrPrtry: cdOrPrtry},
						Ref: t.Reference,
					},
				},
			}
		} else if t.Unstructured != "" {
			txInfo.RmtInf = &p1RmtInf{Ustrd: t.Unstructured}
		}
		txInfos = append(txInfos, txInfo)
	}

	doc := p1Document{
		XMLName: xml.Name{Local: "Document"},
		Xmlns:   pain001NS,
		CstmrCdtTrfInitn: p1CstmrCdtTrfInitn{
			GrpHdr: p1GrpHdr{
				MsgID:    msgID,
				CreDtTm:  time.Now().UTC().Format("2006-01-02T15:04:05"),
				NbOfTxs:  len(req.Transactions),
				CtrlSum:  fmt.Sprintf("%.2f", ctrlSum),
				InitgPty: p1Party{Nm: req.DebtorName},
			},
			PmtInf: p1PmtInf{
				PmtInfID: msgID + "-BATCH",
				PmtMtd:   "TRF",
				NbOfTxs:  len(req.Transactions),
				CtrlSum:  fmt.Sprintf("%.2f", ctrlSum),
				PmtTpInf: paymentTypeInfo(req.Transactions),
				ReqdExctnDt: p1ReqdExctnDt{
					Dt: req.ExecutionDate.Format("2006-01-02"),
				},
				Dbtr:    p1Party{Nm: req.DebtorName},
				DbtrAcct: p1Acct{ID: p1AcctID{IBAN: normalizeIBAN(req.DebtorIBAN)}},
				DbtrAgt:  buildDbtrAgt(req.DebtorBIC),
				CdtTrfTxInf: txInfos,
			},
		},
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("pain.001: marshal: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// ─── XML structs (pain.001.001.09) ───────────────────────────────────────────

type p1Document struct {
	XMLName          xml.Name          `xml:"Document"`
	Xmlns            string            `xml:"xmlns,attr"`
	CstmrCdtTrfInitn p1CstmrCdtTrfInitn `xml:"CstmrCdtTrfInitn"`
}

type p1CstmrCdtTrfInitn struct {
	GrpHdr p1GrpHdr `xml:"GrpHdr"`
	PmtInf p1PmtInf `xml:"PmtInf"`
}

type p1GrpHdr struct {
	MsgID    string   `xml:"MsgId"`
	CreDtTm  string   `xml:"CreDtTm"`
	NbOfTxs  int      `xml:"NbOfTxs"`
	CtrlSum  string   `xml:"CtrlSum"`
	InitgPty p1Party  `xml:"InitgPty"`
}

type p1PmtInf struct {
	PmtInfID     string           `xml:"PmtInfId"`
	PmtMtd       string           `xml:"PmtMtd"`
	NbOfTxs      int              `xml:"NbOfTxs"`
	CtrlSum      string           `xml:"CtrlSum"`
	PmtTpInf     *p1PmtTpInf     `xml:"PmtTpInf,omitempty"`
	ReqdExctnDt  p1ReqdExctnDt   `xml:"ReqdExctnDt"`
	Dbtr         p1Party          `xml:"Dbtr"`
	DbtrAcct     p1Acct           `xml:"DbtrAcct"`
	DbtrAgt      p1DbtrAgt        `xml:"DbtrAgt"`
	CdtTrfTxInf  []p1CdtTrfTxInf `xml:"CdtTrfTxInf"`
}

// PmtTpInf est OMIS pour un virement suisse ordinaire.
//
// Le niveau de service « SEPA » ne concerne que les virements en euros dans
// l'espace SEPA. L'annoncer sur un paiement en francs vers un IBAN suisse
// décrit un service que l'opération n'utilise pas, et expose au rejet par la
// banque destinataire du fichier. Il n'est donc posé que pour un virement en
// euros.
type p1PmtTpInf struct {
	SvcLvl *p1SvcLvl `xml:"SvcLvl,omitempty"`
}

type p1SvcLvl struct {
	Cd string `xml:"Cd"`
}

type p1ReqdExctnDt struct {
	Dt string `xml:"Dt"`
}

type p1Party struct {
	Nm string `xml:"Nm"`
}

type p1Acct struct {
	ID p1AcctID `xml:"Id"`
}

type p1AcctID struct {
	IBAN string `xml:"IBAN"`
}

type p1DbtrAgt struct {
	FinInstnID p1FinInstnID `xml:"FinInstnId"`
}

type p1FinInstnID struct {
	BICFI string `xml:"BICFI,omitempty"`
	Othr  *p1Othr `xml:"Othr,omitempty"`
}

type p1Othr struct {
	ID string `xml:"Id"`
}

type p1CdtTrfTxInf struct {
	PmtID    p1PmtID  `xml:"PmtId"`
	Amt      p1Amt    `xml:"Amt"`
	Cdtr     p1Party  `xml:"Cdtr"`
	CdtrAcct p1Acct   `xml:"CdtrAcct"`
	RmtInf   *p1RmtInf `xml:"RmtInf,omitempty"`
}

type p1PmtID struct {
	EndToEndID string `xml:"EndToEndId"`
}

type p1Amt struct {
	InstdAmt p1InstdAmt `xml:"InstdAmt"`
}

type p1InstdAmt struct {
	Ccy   string `xml:"Ccy,attr"`
	Value string `xml:",chardata"`
}

type p1RmtInf struct {
	Ustrd string   `xml:"Ustrd,omitempty"`
	Strd  *p1Strd  `xml:"Strd,omitempty"`
}

type p1Strd struct {
	CdtrRefInf p1CdtrRefInf `xml:"CdtrRefInf"`
}

type p1CdtrRefInf struct {
	Tp  p1RefTp `xml:"Tp"`
	Ref string  `xml:"Ref"`
}

type p1RefTp struct {
	CdOrPrtry p1CdOrPrtry `xml:"CdOrPrtry"`
}

type p1CdOrPrtry struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// paymentTypeInfo n'annonce le niveau de service SEPA que s'il s'applique.
//
// Le lot est homogène en pratique — on paie ses fournisseurs suisses en francs —
// et la première devise décide. Un lot mêlant francs et euros relèverait de deux
// types de paiement distincts, ce qui demande deux fichiers ; le cas ne se
// présente pas dans le produit tel qu'il est, et le supposer réglé serait pire
// que ne pas le traiter.
func paymentTypeInfo(txs []CreditTransfer) *p1PmtTpInf {
	if len(txs) == 0 {
		return nil
	}
	if strings.EqualFold(txs[0].Currency, "EUR") {
		return &p1PmtTpInf{SvcLvl: &p1SvcLvl{Cd: "SEPA"}}
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func normalizeIBAN(iban string) string {
	out := make([]byte, 0, len(iban))
	for i := 0; i < len(iban); i++ {
		if iban[i] != ' ' {
			out = append(out, iban[i])
		}
	}
	return string(out)
}

func buildDbtrAgt(bic string) p1DbtrAgt {
	if bic != "" {
		return p1DbtrAgt{FinInstnID: p1FinInstnID{BICFI: bic}}
	}
	// Swiss Payment Standards require a FinInstnId — use NOTPROVIDED when BIC unknown
	return p1DbtrAgt{FinInstnID: p1FinInstnID{Othr: &p1Othr{ID: "NOTPROVIDED"}}}
}

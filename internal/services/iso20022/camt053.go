package iso20022

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ─── camt.053.001.08 — Bank to Customer Statement ─────────────────────────────

// BankEntry is a single parsed bank statement transaction.
type BankEntry struct {
	// Amounts
	Amount   float64
	Currency string
	IsCredit bool // true = CRDT (money in), false = DBIT (money out)

	// Dates
	BookingDate time.Time
	ValueDate   time.Time

	// References
	BankRef     string // AcctSvcrRef — bank's own reference
	EndToEndRef string // payment's EndToEndId
	QRReference string // QRR reference if present

	// Counterpart
	CounterpartName string
	CounterpartIBAN string

	// Narrative
	Unstructured string // free-text remittance info
}

// ParseCamt053 parses a camt.053.001.08 XML document and returns all booked entries.
// It tolerates minor namespace variations (some Swiss banks omit version suffix).
func ParseCamt053(xmlData []byte) ([]BankEntry, error) {
	var doc c53Document
	if err := xml.Unmarshal(xmlData, &doc); err != nil {
		return nil, fmt.Errorf("camt.053: parse XML: %w", err)
	}

	var entries []BankEntry
	for _, stmt := range doc.BkToCstmrStmt.Stmt {
		for _, ntry := range stmt.Ntry {
			// Only process booked entries (skip pending/information entries)
			if ntry.Sts.Cd != "BOOK" {
				continue
			}

			amount, err := strconv.ParseFloat(strings.TrimSpace(ntry.Amt.Value), 64)
			if err != nil {
				continue // skip malformed entries
			}

			be := BankEntry{
				Amount:   amount,
				Currency: ntry.Amt.Ccy,
				IsCredit: ntry.CdtDbtInd == "CRDT",
				BankRef:  ntry.AcctSvcrRef,
			}

			// Booking date
			if d := parseDate(ntry.BookgDt.Dt); !d.IsZero() {
				be.BookingDate = d
			}
			// Value date
			if d := parseDate(ntry.ValDt.Dt); !d.IsZero() {
				be.ValueDate = d
			}

			txs := ntry.NtryDtls.TxDtls
			switch {
			case len(txs) == 1:
				// Le cas courant : une opération, ses détails complètent l'écriture.
				entries = append(entries, withTxDetails(be, txs[0]))

			case len(txs) > 1:
				// Un lot (paiements groupés, remise d'encaissements) : l'écriture porte
				// le TOTAL, chaque TxDtls porte une opération. Ne garder que la
				// première faisait disparaître les autres sans un mot, et donnait au
				// premier le montant de tout le lot.
				if parts, ok := splitBatch(be, txs); ok {
					entries = append(entries, parts...)
				} else {
					// Les montants du détail manquent ou ne retombent pas sur le total :
					// on ne les invente pas. Une seule écriture, au total du lot, sans
					// la contrepartie d'une seule de ses opérations.
					entries = append(entries, be)
				}

			default:
				entries = append(entries, be)
			}
		}
	}
	return entries, nil
}

// withTxDetails complète une écriture avec le détail d'une opération :
// références, motif, contrepartie (qui dépend du sens du mouvement).
func withTxDetails(be BankEntry, tx c53TxDtls) BankEntry {
	be.EndToEndRef = tx.Refs.EndToEndID

	// Remittance info — structured (QRR ref) or unstructured
	if tx.RmtInf.Strd != nil {
		be.QRReference = strings.TrimSpace(tx.RmtInf.Strd.CdtrRefInf.Ref)
	}
	if tx.RmtInf.Ustrd != "" {
		be.Unstructured = strings.TrimSpace(tx.RmtInf.Ustrd)
	}

	if be.IsCredit {
		be.CounterpartName = tx.RltdPties.Dbtr.Pty.Nm
		be.CounterpartIBAN = normalizeIBAN(tx.RltdPties.DbtrAcct.ID.IBAN)
	} else {
		be.CounterpartName = tx.RltdPties.Cdtr.Pty.Nm
		be.CounterpartIBAN = normalizeIBAN(tx.RltdPties.CdtrAcct.ID.IBAN)
	}
	return be
}

// splitBatch répartit un lot en une écriture par opération.
//
// Elle ne réussit que si CHAQUE opération porte son montant et que leur somme
// retombe sur celui de l'écriture, au centime : c'est la seule preuve qu'on a
// bien lu le lot en entier. Sinon elle renonce, et l'appelant garde le total.
func splitBatch(be BankEntry, txs []c53TxDtls) ([]BankEntry, bool) {
	parts := make([]BankEntry, 0, len(txs))
	var sum float64
	for _, tx := range txs {
		amt := tx.Amt
		if strings.TrimSpace(amt.Value) == "" {
			amt = tx.AmtDtls.TxAmt.Amt
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(amt.Value), 64)
		if err != nil || v <= 0 {
			return nil, false
		}
		part := withTxDetails(be, tx)
		part.Amount = v
		if amt.Ccy != "" {
			part.Currency = amt.Ccy
		}
		sum += v
		parts = append(parts, part)
	}
	if math.Abs(sum-be.Amount) > 0.005 {
		return nil, false
	}
	return parts, true
}

// ─── XML structs (camt.053.001.08) ───────────────────────────────────────────

type c53Document struct {
	XMLName       xml.Name     `xml:"Document"`
	BkToCstmrStmt c53BkToCstmr `xml:"BkToCstmrStmt"`
}

type c53BkToCstmr struct {
	GrpHdr c53GrpHdr `xml:"GrpHdr"`
	Stmt   []c53Stmt `xml:"Stmt"`
}

type c53GrpHdr struct {
	MsgID   string `xml:"MsgId"`
	CreDtTm string `xml:"CreDtTm"`
}

type c53Stmt struct {
	ID   string    `xml:"Id"`
	Acct c53Acct   `xml:"Acct"`
	Ntry []c53Ntry `xml:"Ntry"`
}

type c53Acct struct {
	ID c53AcctID `xml:"Id"`
}

type c53AcctID struct {
	IBAN string `xml:"IBAN"`
}

type c53Ntry struct {
	Amt         c53Amt      `xml:"Amt"`
	CdtDbtInd   string      `xml:"CdtDbtInd"` // CRDT or DBIT
	Sts         c53Sts      `xml:"Sts"`
	BookgDt     c53Date     `xml:"BookgDt"`
	ValDt       c53Date     `xml:"ValDt"`
	AcctSvcrRef string      `xml:"AcctSvcrRef"`
	NtryDtls    c53NtryDtls `xml:"NtryDtls"`
}

type c53Amt struct {
	Ccy   string `xml:"Ccy,attr"`
	Value string `xml:",chardata"`
}

type c53Sts struct {
	Cd string `xml:"Cd"`
}

type c53Date struct {
	Dt   string `xml:"Dt"`
	DtTm string `xml:"DtTm"`
}

type c53NtryDtls struct {
	TxDtls []c53TxDtls `xml:"TxDtls"`
}

type c53TxDtls struct {
	Refs      c53Refs      `xml:"Refs"`
	Amt       c53Amt       `xml:"Amt"`
	AmtDtls   c53AmtDtls   `xml:"AmtDtls"`
	RmtInf    c53RmtInf    `xml:"RmtInf"`
	RltdPties c53RltdPties `xml:"RltdPties"`
}

type c53AmtDtls struct {
	TxAmt struct {
		Amt c53Amt `xml:"Amt"`
	} `xml:"TxAmt"`
}

type c53Refs struct {
	EndToEndID  string `xml:"EndToEndId"`
	AcctSvcrRef string `xml:"AcctSvcrRef"`
}

type c53RmtInf struct {
	Ustrd string   `xml:"Ustrd"`
	Strd  *c53Strd `xml:"Strd"`
}

type c53Strd struct {
	CdtrRefInf c53CdtrRefInf `xml:"CdtrRefInf"`
}

type c53CdtrRefInf struct {
	Ref string `xml:"Ref"`
}

type c53RltdPties struct {
	Dbtr     c53RltdPty  `xml:"Dbtr"`
	DbtrAcct c53RltdAcct `xml:"DbtrAcct"`
	Cdtr     c53RltdPty  `xml:"Cdtr"`
	CdtrAcct c53RltdAcct `xml:"CdtrAcct"`
}

type c53RltdPty struct {
	Pty c53Pty `xml:"Pty"`
}

type c53Pty struct {
	Nm string `xml:"Nm"`
}

type c53RltdAcct struct {
	ID c53AcctID `xml:"Id"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Try ISO 8601 date first, then datetime
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04:05", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

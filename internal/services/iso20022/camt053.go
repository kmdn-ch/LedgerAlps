package iso20022

import (
	"encoding/xml"
	"fmt"
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
	BankRef      string // AcctSvcrRef — bank's own reference
	EndToEndRef  string // payment's EndToEndId
	QRReference  string // QRR reference if present

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

			// Drill into transaction details
			for _, txDtls := range ntry.NtryDtls.TxDtls {
				be.EndToEndRef = txDtls.Refs.EndToEndID

				// Remittance info — structured (QRR ref) or unstructured
				if txDtls.RmtInf.Strd != nil {
					be.QRReference = strings.TrimSpace(txDtls.RmtInf.Strd.CdtrRefInf.Ref)
				}
				if txDtls.RmtInf.Ustrd != "" {
					be.Unstructured = strings.TrimSpace(txDtls.RmtInf.Ustrd)
				}

				// Related parties — counterpart depends on direction
				if be.IsCredit {
					be.CounterpartName = txDtls.RltdPties.Dbtr.Pty.Nm
					be.CounterpartIBAN = normalizeIBAN(txDtls.RltdPties.DbtrAcct.ID.IBAN)
				} else {
					be.CounterpartName = txDtls.RltdPties.Cdtr.Pty.Nm
					be.CounterpartIBAN = normalizeIBAN(txDtls.RltdPties.CdtrAcct.ID.IBAN)
				}

				// Only use first TxDtls (most Swiss bank files have one)
				break
			}

			entries = append(entries, be)
		}
	}
	return entries, nil
}

// ─── XML structs (camt.053.001.08) ───────────────────────────────────────────

type c53Document struct {
	XMLName       xml.Name      `xml:"Document"`
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
	CdtDbtInd   string      `xml:"CdtDbtInd"`  // CRDT or DBIT
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
	Refs     c53Refs     `xml:"Refs"`
	RmtInf   c53RmtInf   `xml:"RmtInf"`
	RltdPties c53RltdPties `xml:"RltdPties"`
}

type c53Refs struct {
	EndToEndID string `xml:"EndToEndId"`
	AcctSvcrRef string `xml:"AcctSvcrRef"`
}

type c53RmtInf struct {
	Ustrd string    `xml:"Ustrd"`
	Strd  *c53Strd  `xml:"Strd"`
}

type c53Strd struct {
	CdtrRefInf c53CdtrRefInf `xml:"CdtrRefInf"`
}

type c53CdtrRefInf struct {
	Ref string `xml:"Ref"`
}

type c53RltdPties struct {
	Dbtr    c53RltdPty `xml:"Dbtr"`
	DbtrAcct c53RltdAcct `xml:"DbtrAcct"`
	Cdtr    c53RltdPty `xml:"Cdtr"`
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

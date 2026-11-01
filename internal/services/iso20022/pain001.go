// Package iso20022 provides generation and parsing of Swiss ISO 20022 payment messages.
// pain.001.001.09 — Customer Credit Transfer Initiation (Six-Group / SIX Interbank Clearing)
// camt.053.001.08 — Bank to Customer Statement
package iso20022

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

// ─── pain.001.001.09 — Credit Transfer Initiation ────────────────────────────

const pain001NS = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"

// Pain001Request is the input to GeneratePain001.
type Pain001Request struct {
	// Debtor (the company initiating the payments)
	DebtorName string
	DebtorIBAN string
	DebtorBIC  string // optional — bank BIC/SWIFT
	// DebtorAddress est l'adresse postale du débiteur. Voir PostalAddress : sans ville ni pays, elle
	// n'est pas émise.
	DebtorAddress PostalAddress

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
	// CreditorAddress est l'adresse postale du bénéficiaire. Voir PostalAddress.
	CreditorAddress PostalAddress
}

// PostalAddress est une adresse STRUCTURÉE, telle que les banques suisses l'attendent dans un ordre
// de paiement (audit croisé Gemini, HIGH-06).
//
// Ce que les banques en disent, lu sur leurs avis aux clients (PostFinance, ZKB, UBS ; le texte des
// lignes directrices de SIX n'a pas pu être lu ici) : dans une adresse structurée la **localité** et
// le **pays** sont obligatoires ; la rue, le numéro et le code postal sont facultatifs ; les adresses
// non structurées ne sont plus garanties de bout en bout à partir de novembre 2026. Les banques suisses
// tolèrent « jusqu'à nouvel avis » le numéro de rue dans l'élément rue : `Street` est donc émise
// telle quelle, sans tenter de la scinder — une heuristique qui se tromperait sur « Rue du 1er-Août 3 »
// ferait pire que ne rien scinder.
//
// Une adresse sans localité ou sans pays valide (deux lettres) n'est PAS émise : un élément à moitié
// rempli est refusé, une adresse absente ne l'est pas.
type PostalAddress struct {
	Street   string
	PostCode string
	Town     string
	Country  string // code pays ISO 3166 à deux lettres
}

// structured rend l'élément XML, ou nil quand l'adresse ne peut pas être émise.
func (a PostalAddress) structured() *p1PstlAdr {
	town := tronque(a.Town, 35)
	country := strings.ToUpper(strings.TrimSpace(a.Country))
	if town == "" || len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		return nil
	}
	return &p1PstlAdr{
		StrtNm: tronque(a.Street, 70),
		PstCd:  tronque(a.PostCode, 16),
		TwnNm:  town,
		Ctry:   country,
	}
}

// tronque réduit les blancs successifs et borne la longueur, en caractères et non en octets.
func tronque(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

// newMessageID rend l'identifiant de message d'un fichier de virements.
//
// La banque rejette un MsgId déjà vu (code DUPL). L'ancien identifiant — date
// d'exécution et nombre de lignes — était identique pour deux fichiers de même
// taille le même jour : regénérer un lot après correction, ou en préparer deux
// de trois virements, donnait le même MsgId et la banque refusait le second.
//
// Il porte maintenant l'instant de création à la seconde et 32 bits d'aléa,
// donc deux appels n'entrent en collision que dans la même seconde ET avec le
// même tirage. Le tout tient en 26 caractères : l'ISO 20022 plafonne MsgId et
// PmtInfId à 35 (Max35Text), et PmtInfId y ajoute « -BATCH ».
func newMessageID(now time.Time) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand n'échoue pas en pratique ; à défaut, un compteur du processus
		// garde l'identifiant distinct d'un appel à l'autre.
		return fmt.Sprintf("LA-%s-%08x", now.Format("20060102150405"), msgIDFallback.Add(1))
	}
	return "LA-" + now.Format("20060102150405") + "-" + hex.EncodeToString(b[:])
}

// msgIDFallback numérote les identifiants produits quand crypto/rand a échoué.
var msgIDFallback atomic.Uint32

// GeneratePain001 returns a pain.001.001.09 XML document as bytes.
func GeneratePain001(req Pain001Request) ([]byte, error) {
	if len(req.Transactions) == 0 {
		return nil, fmt.Errorf("pain.001: at least one transaction required")
	}
	if req.DebtorIBAN == "" {
		return nil, fmt.Errorf("pain.001: debtor IBAN required")
	}

	msgID := newMessageID(time.Now().UTC())

	// Un bloc de paiement PAR DEVISE (audit croisé Gemini, HIGH-06). Le niveau de service
	// « SEPA » et la somme de contrôle valent pour un bloc entier : un lot mêlant euros et francs,
	// avec un euro en tête, déclarait SEPA le paiement en francs. Chaque devise a donc son
	// <PmtInf>, dans l'ordre où elle apparaît ; un lot d'une seule devise reste un seul bloc,
	// identique à ce qu'il était.
	type groupe struct {
		devise string
		txs    []CreditTransfer
		infos  []p1CdtTrfTxInf
		cents  int64
	}
	var groupes []*groupe
	parDevise := map[string]*groupe{}
	var totalCents int64

	for _, t := range req.Transactions {
		t.Currency = strings.ToUpper(strings.TrimSpace(t.Currency))
		if t.Currency == "" {
			t.Currency = "CHF"
		}
		// L'identifiant de bout en bout n'est jamais vide : « NOTPROVIDED » est la valeur que les
		// Swiss Payment Standards prévoient quand l'ordre n'en porte pas.
		if strings.TrimSpace(t.EndToEndID) == "" {
			t.EndToEndID = "NOTPROVIDED"
		}
		// Les montants se comptent en CENTIMES ENTIERS, et la somme de contrôle est celle des
		// montants IMPRIMÉS : sommer les montants bruts puis arrondir donnait 20.01 pour deux
		// virements imprimés à 10.00.
		cents := int64(math.Round(math.Round(t.Amount*1e6) / 1e4))
		g := parDevise[t.Currency]
		if g == nil {
			g = &groupe{devise: t.Currency}
			parDevise[t.Currency] = g
			groupes = append(groupes, g)
		}
		g.cents += cents
		totalCents += cents
		g.txs = append(g.txs, t)

		txInfo := p1CdtTrfTxInf{
			PmtID: p1PmtID{EndToEndID: t.EndToEndID},
			Amt: p1Amt{
				InstdAmt: p1InstdAmt{
					Ccy:   t.Currency,
					Value: centsToAmount(cents),
				},
			},
			Cdtr:     p1Party{Nm: t.CreditorName, PstlAdr: t.CreditorAddress.structured()},
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
		g.infos = append(g.infos, txInfo)
	}

	blocs := make([]p1PmtInf, 0, len(groupes))
	for i, g := range groupes {
		id := msgID + "-BATCH"
		if len(groupes) > 1 {
			// Max35Text : « -B1 » tient où « -BATCH-CHF » ne tenait pas.
			id = fmt.Sprintf("%s-B%d", msgID, i+1)
		}
		blocs = append(blocs, p1PmtInf{
			PmtInfID: id,
			PmtMtd:   "TRF",
			NbOfTxs:  len(g.txs),
			CtrlSum:  centsToAmount(g.cents),
			PmtTpInf: paymentTypeInfo(g.txs),
			ReqdExctnDt: p1ReqdExctnDt{
				Dt: req.ExecutionDate.Format("2006-01-02"),
			},
			Dbtr:        p1Party{Nm: req.DebtorName, PstlAdr: req.DebtorAddress.structured()},
			DbtrAcct:    p1Acct{ID: p1AcctID{IBAN: normalizeIBAN(req.DebtorIBAN)}},
			DbtrAgt:     buildDbtrAgt(req.DebtorBIC),
			CdtTrfTxInf: g.infos,
		})
	}

	doc := p1Document{
		XMLName: xml.Name{Local: "Document"},
		Xmlns:   pain001NS,
		CstmrCdtTrfInitn: p1CstmrCdtTrfInitn{
			GrpHdr: p1GrpHdr{
				MsgID:    msgID,
				CreDtTm:  time.Now().UTC().Format("2006-01-02T15:04:05"),
				NbOfTxs:  len(req.Transactions),
				CtrlSum:  centsToAmount(totalCents),
				InitgPty: p1Party{Nm: req.DebtorName},
			},
			PmtInf: blocs,
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
	XMLName          xml.Name           `xml:"Document"`
	Xmlns            string             `xml:"xmlns,attr"`
	CstmrCdtTrfInitn p1CstmrCdtTrfInitn `xml:"CstmrCdtTrfInitn"`
}

type p1CstmrCdtTrfInitn struct {
	GrpHdr p1GrpHdr   `xml:"GrpHdr"`
	PmtInf []p1PmtInf `xml:"PmtInf"`
}

type p1GrpHdr struct {
	MsgID    string  `xml:"MsgId"`
	CreDtTm  string  `xml:"CreDtTm"`
	NbOfTxs  int     `xml:"NbOfTxs"`
	CtrlSum  string  `xml:"CtrlSum"`
	InitgPty p1Party `xml:"InitgPty"`
}

type p1PmtInf struct {
	PmtInfID    string          `xml:"PmtInfId"`
	PmtMtd      string          `xml:"PmtMtd"`
	NbOfTxs     int             `xml:"NbOfTxs"`
	CtrlSum     string          `xml:"CtrlSum"`
	PmtTpInf    *p1PmtTpInf     `xml:"PmtTpInf,omitempty"`
	ReqdExctnDt p1ReqdExctnDt   `xml:"ReqdExctnDt"`
	Dbtr        p1Party         `xml:"Dbtr"`
	DbtrAcct    p1Acct          `xml:"DbtrAcct"`
	DbtrAgt     p1DbtrAgt       `xml:"DbtrAgt"`
	CdtTrfTxInf []p1CdtTrfTxInf `xml:"CdtTrfTxInf"`
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
	Nm      string     `xml:"Nm"`
	PstlAdr *p1PstlAdr `xml:"PstlAdr,omitempty"`
}

// p1PstlAdr : l'ordre des éléments est celui du schéma pain.001.001.09 — rue, code postal, localité, pays.
type p1PstlAdr struct {
	StrtNm string `xml:"StrtNm,omitempty"`
	PstCd  string `xml:"PstCd,omitempty"`
	TwnNm  string `xml:"TwnNm"`
	Ctry   string `xml:"Ctry"`
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
	BICFI string  `xml:"BICFI,omitempty"`
	Othr  *p1Othr `xml:"Othr,omitempty"`
}

type p1Othr struct {
	ID string `xml:"Id"`
}

type p1CdtTrfTxInf struct {
	PmtID    p1PmtID   `xml:"PmtId"`
	Amt      p1Amt     `xml:"Amt"`
	Cdtr     p1Party   `xml:"Cdtr"`
	CdtrAcct p1Acct    `xml:"CdtrAcct"`
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
	Ustrd string  `xml:"Ustrd,omitempty"`
	Strd  *p1Strd `xml:"Strd,omitempty"`
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

// centsToAmount écrit un montant en centimes entiers « 1250.50 », sans passer par un flottant.
func centsToAmount(c int64) string {
	signe := ""
	if c < 0 {
		signe, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", signe, c/100, c%100)
}

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

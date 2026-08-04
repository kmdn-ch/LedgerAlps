// Package pdf generates invoice PDFs with an embedded Swiss payment slip (QR-bill SPC 0200).
package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"time"

	gofpdf "github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
)

// ─── Data types ───────────────────────────────────────────────────────────────

// CompanyInfo holds the creditor/issuer details printed on the invoice.
type CompanyInfo struct {
	Name      string
	Address   string // street + nr
	City      string // postal code + city
	Country   string // ISO alpha-2, e.g. "CH"
	IBAN      string // QR-IBAN preferred; regular IBAN fallback
	QRIBAN    string
	VATNumber string // n° TVA, p. ex. "CHE-123.456.789 TVA"
	// UIDNumber est le numéro d'identification des entreprises (IDE), commun à
	// toutes les entreprises inscrites au registre — assujetties à la TVA ou
	// non. Il n'était pas rendu du tout, alors qu'il identifie l'émetteur.
	UIDNumber string // p. ex. "CHE-123.456.789"
	Phone     string
	Email     string
	LogoData  string // base64 data URL (data:image/png;base64,…) — optional
}

// InvoiceLine is a single line item rendered on the PDF.
type InvoiceLine struct {
	Description string
	Quantity    float64
	UnitPrice   float64
	VATRate     float64
	LineTotal   float64
}

// InvoiceData contains everything the PDF renderer needs.
type InvoiceData struct {
	// Invoice metadata
	InvoiceNumber string
	// DocumentType is "invoice", "quote" or "credit_note". Empty means invoice.
	// It decides the heading and whether a QR payment slip is drawn at all —
	// see documentTitle and renderPaymentSlip.
	DocumentType string
	// CorrectsInvoiceNumber names the invoice a credit note cancels. LTVA
	// art. 27 al. 4 defines a correction as "un document qui mentionne et
	// annule la facture d'origine", so the mention belongs on the page, not
	// only in the database.
	CorrectsInvoiceNumber string
	IssueDate             time.Time
	DueDate               time.Time
	Currency              string
	Status                string

	// Amounts (already calculated)
	SubtotalAmount float64
	VATAmount      float64
	TotalAmount    float64
	VATRate        float64

	// Notes / terms
	Notes *string
	Terms *string

	// Line items
	Lines []InvoiceLine

	// Parties
	Company  CompanyInfo
	Customer CustomerInfo
}

// CustomerInfo holds the debtor details.
type CustomerInfo struct {
	Name    string
	Address string
	City    string
	Country string
}

// ─── Generator ────────────────────────────────────────────────────────────────

// Generate renders the invoice as a PDF and returns the bytes.
// The PDF is A4 portrait with the Swiss QR payment slip at the bottom.
func Generate(inv InvoiceData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	// Pas de saut de page automatique : les sections gèrent leur propre
	// pagination, parce que le bulletin de versement occupe les 105 mm du bas
	// de la DERNIÈRE page et qu'un saut décidé par la bibliothèque le
	// recouvrirait.
	pdf.SetAutoPageBreak(false, 0)

	// Numérotation « Page n/N ». Sur une pièce comptable de plusieurs feuillets,
	// c'est ce qui permet de constater qu'il n'en manque pas — et le CO
	// art. 958f impose de la conserver complète dix ans.
	pdf.AliasNbPages("{nb}")
	pdf.SetFooterFunc(func() {
		if pdf.PageNo() == 1 && pdf.PageCount() == 1 {
			return // une facture d'une page n'a pas besoin qu'on le lui dise
		}
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 6, latin1(fmt.Sprintf("Page %d/{nb}", pdf.PageNo())),
			"", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()

	// ── Header: company + invoice title ──────────────────────────────────────
	renderHeader(pdf, inv)

	// ── Customer address block ────────────────────────────────────────────────
	renderCustomerBlock(pdf, inv)

	// ── Invoice metadata (number, dates) ─────────────────────────────────────
	renderMeta(pdf, inv)

	// ── Line items table ──────────────────────────────────────────────────────
	renderLines(pdf, inv)

	// ── Totals ────────────────────────────────────────────────────────────────
	renderTotals(pdf, inv)

	// ── Notes / terms ─────────────────────────────────────────────────────────
	renderNotes(pdf, inv)

	// ── Swiss QR payment slip (bottom 105 mm) ─────────────────────────────────
	//
	// Only an invoice gets one. A price offer carrying a QR slip and a VAT
	// amount is, to the recipient and to the AFC, indistinguishable from an
	// invoice: the prospect can pay it and can deduct the input tax shown on
	// it. LTVA art. 27 al. 2 then makes the issuer liable for tax stated
	// without entitlement. A credit note is excluded too — the money owed
	// flows the other way, so a slip asking the customer to pay is backwards.
	if wantsPaymentSlip(inv.DocumentType) {
		// Le bulletin occupe les 105 mm du bas. Si le contenu déborde dans cette
		// bande, il faut une page de plus : imprimer le bulletin par-dessus les
		// dernières lignes rendrait la facture illisible ET le bulletin
		// inutilisable par la banque, qui lit une zone à position fixe.
		if pdf.GetY() > slipTop-5 {
			pdf.AddPage()
		}
		if err := renderPaymentSlip(pdf, inv); err != nil {
			// Non-fatal: log but still return the PDF without slip
			_ = err
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf render: %w", err)
	}
	return buf.Bytes(), nil
}

// ─── Section renderers ────────────────────────────────────────────────────────

func renderHeader(pdf *gofpdf.Fpdf, inv InvoiceData) {
	// Render company logo if present (top-left, 22×16 mm reserved area).
	// Company text starts to the right of the logo when present, otherwise at x=15.
	textX := 15.0
	if inv.Company.LogoData != "" {
		if imgData, imgType, err := decodeLogoDataURL(inv.Company.LogoData); err == nil {
			imgKey := "company_logo"
			reader := bytes.NewReader(imgData)
			pdf.RegisterImageOptionsReader(imgKey, gofpdf.ImageOptions{ImageType: imgType}, reader)
			// Place logo at (15, 13), 22mm wide, 16mm tall (fixed box — proportions may vary)
			pdf.ImageOptions(imgKey, 15, 13, 22, 16, false, gofpdf.ImageOptions{ImageType: imgType}, 0, "")
			textX = 40 // company text starts after the logo
		}
	}

	// Nom de l'émetteur. En 14 points il concurrençait le titre du document et
	// débordait sur une raison sociale longue ; 11,5 le laisse lisible sans
	// écraser le reste de l'en-tête.
	pdf.SetFont("Helvetica", "B", 11.5)
	pdf.SetXY(textX, 15)
	pdf.CellFormat(115-textX+15, 6, latin1(inv.Company.Name), "", 1, "L", false, 0, "")

	// Adresse
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetX(textX)
	pdf.CellFormat(115-textX+15, 5, latin1(inv.Company.Address), "", 1, "L", false, 0, "")
	pdf.SetX(textX)
	pdf.CellFormat(115-textX+15, 5, latin1(inv.Company.City), "", 1, "L", false, 0, "")

	// IDE et numéro de TVA. Ce sont deux choses distinctes : l'IDE identifie
	// l'entreprise au registre, qu'elle soit assujettie ou non, tandis que le
	// numéro de TVA (le même IDE suivi de la mention « TVA ») n'existe que pour
	// un assujetti. La LTVA art. 26 al. 2 let. a exige ce dernier sur toute
	// facture portant de la TVA.
	//
	// Quand les deux sont renseignés et que le numéro de TVA contient déjà
	// l'IDE, une seule ligne suffit : les répéter donnerait deux fois le même
	// numéro à un lecteur qui y chercherait une différence.
	// Téléphone et courriel : une facture doit permettre de poser une question
	// sans chercher ailleurs. Ce n'est pas exigé par la LTVA art. 26, mais une
	// facture qu'on ne peut pas contester facilement se paie tard, ou pas.
	var contact []string
	if inv.Company.Phone != "" {
		contact = append(contact, inv.Company.Phone)
	}
	if inv.Company.Email != "" {
		contact = append(contact, inv.Company.Email)
	}
	if len(contact) > 0 {
		companyLine(pdf, textX, strings.Join(contact, "  ·  "))
	}

	uid, vat := inv.Company.UIDNumber, inv.Company.VATNumber
	switch {
	case vat != "" && uid != "" && strings.Contains(normaliseUID(vat), normaliseUID(uid)):
		companyLine(pdf, textX, "IDE / N° TVA : "+vat)
	default:
		if uid != "" {
			companyLine(pdf, textX, "IDE : "+uid)
		}
		if vat != "" {
			companyLine(pdf, textX, "N° TVA : "+vat)
		}
	}

	// Document title (right). It must name what the document actually is: a
	// price offer headed "FACTURE" is a document the recipient may pay and
	// deduct VAT from.
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetXY(130, 15)
	pdf.CellFormat(65, 12, latin1(documentTitle(inv.DocumentType)), "", 1, "R", false, 0, "")

	pdf.SetY(45)
}

// decodeLogoDataURL splits a base64 data URL into raw bytes and an fpdf image type string.
// Supported formats: PNG and JPEG.
func decodeLogoDataURL(dataURL string) ([]byte, string, error) {
	// Expected format: "data:image/png;base64,<b64data>"
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid data URL")
	}
	header := strings.ToLower(parts[0])
	imgType := "PNG"
	if strings.Contains(header, "jpeg") || strings.Contains(header, "jpg") {
		imgType = "JPEG"
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode: %w", err)
	}
	return decoded, imgType, nil
}

func renderCustomerBlock(pdf *gofpdf.Fpdf, inv InvoiceData) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetX(130)
	pdf.CellFormat(65, 6, latin1(inv.Customer.Name), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 10)
	if inv.Customer.Address != "" {
		pdf.SetX(130)
		pdf.CellFormat(65, 5, latin1(inv.Customer.Address), "", 1, "L", false, 0, "")
	}
	if inv.Customer.City != "" {
		pdf.SetX(130)
		pdf.CellFormat(65, 5, latin1(inv.Customer.City), "", 1, "L", false, 0, "")
	}
	pdf.SetY(pdf.GetY() + 5)
}

func renderMeta(pdf *gofpdf.Fpdf, inv InvoiceData) {
	pdf.SetFont("Helvetica", "", 10)
	y := 65.0
	col1, col2 := 15.0, 50.0

	metaRow := func(label, val string) {
		pdf.SetXY(col1, y)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(35, 6, label, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetX(col2)
		pdf.CellFormat(60, 6, val, "", 1, "L", false, 0, "")
		y += 6
	}

	metaRow(latin1(documentNumberLabel(inv.DocumentType)), inv.InvoiceNumber)
	metaRow("Date:", inv.IssueDate.Format("02.01.2006"))
	if inv.DocumentType == "credit_note" {
		// A credit note has no due date; what matters is what it cancels.
		if inv.CorrectsInvoiceNumber != "" {
			metaRow(latin1("Annule la facture:"), inv.CorrectsInvoiceNumber)
		}
	} else {
		metaRow(latin1(dueDateLabel(inv.DocumentType)), inv.DueDate.Format("02.01.2006"))
	}
	metaRow("Devise:", inv.Currency)

	pdf.SetY(y + 5)
}

func renderLines(pdf *gofpdf.Fpdf, inv InvoiceData) {
	// La colonne TVA ne s'affiche que s'il y a de la TVA quelque part. Une
	// entreprise non assujettie n'a pas le droit de faire figurer l'impôt sur
	// ses factures (LTVA art. 27 al. 1), et « 0.0 % » est une mention de
	// l'impôt : elle affirme un taux. Sa place est reprise par la description,
	// qui en a toujours l'usage.
	showVAT := inv.VATAmount != 0 || inv.VATRate != 0
	for _, l := range inv.Lines {
		if l.VATRate != 0 {
			showVAT = true
		}
	}

	wDesc, wQty, wPrice, wVAT, wTotal := 90.0, 20.0, 30.0, 15.0, 25.0
	if !showVAT {
		wDesc += wVAT
		wVAT = 0
	}

	header := func() {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetX(15)
		pdf.CellFormat(wDesc, 7, "Description", "1", 0, "L", true, 0, "")
		pdf.CellFormat(wQty, 7, latin1("Qt\u00e9"), "1", 0, "C", true, 0, "")
		pdf.CellFormat(wPrice, 7, "Prix unit.", "1", 0, "R", true, 0, "")
		if showVAT {
			pdf.CellFormat(wVAT, 7, "TVA%", "1", 0, "C", true, 0, "")
		}
		pdf.CellFormat(wTotal, 7, "Total", "1", 1, "R", true, 0, "")
		pdf.SetFont("Helvetica", "", 9)
	}
	header()

	pdf.SetFillColor(255, 255, 255)
	fill := false
	for _, line := range inv.Lines {
		desc := latin1(line.Description)

		// Une description longue s'enroule au lieu de déborder sur la colonne
		// voisine. La hauteur de ligne suit le nombre de lignes de texte, sans
		// quoi une justification détaillée écraserait le montant d'à côté.
		wrapped := pdf.SplitLines([]byte(desc), wDesc-2)
		if len(wrapped) == 0 {
			wrapped = [][]byte{{}}
		}
		rowH := float64(len(wrapped)) * 5
		if rowH < 6 {
			rowH = 6
		}

		// Saut de page avant la ligne, jamais au milieu : une ligne de facture
		// coupée en deux entre deux pages est illisible et se prête aux
		// contestations.
		if pdf.GetY()+rowH > contentBottom {
			pdf.AddPage()
			header()
		}

		bg := 255.0
		if fill {
			bg = 250
		}
		pdf.SetFillColor(int(bg), int(bg), int(bg))

		y := pdf.GetY()
		pdf.SetXY(15, y)
		pdf.MultiCell(wDesc, rowH/float64(len(wrapped)), desc, "1", "L", fill)

		pdf.SetXY(15+wDesc, y)
		pdf.CellFormat(wQty, rowH, fmtFloat(line.Quantity), "1", 0, "C", fill, 0, "")
		pdf.CellFormat(wPrice, rowH, fmtMoney(line.UnitPrice, inv.Currency), "1", 0, "R", fill, 0, "")
		if showVAT {
			pdf.CellFormat(wVAT, rowH, fmt.Sprintf("%.1f%%", line.VATRate), "1", 0, "C", fill, 0, "")
		}
		pdf.CellFormat(wTotal, rowH, fmtMoney(line.LineTotal, inv.Currency), "1", 1, "R", fill, 0, "")
		fill = !fill
	}

	pdf.SetY(pdf.GetY() + 3)
}

func renderTotals(pdf *gofpdf.Fpdf, inv InvoiceData) {
	x := 130.0
	w1, w2 := 35.0, 30.0

	// Le bloc des totaux ne se coupe pas : trois lignes et un trait, séparés
	// entre deux pages, se lisent comme deux totaux différents.
	if pdf.GetY()+20 > contentBottom {
		pdf.AddPage()
	}

	totalRow := func(label, val string, bold bool) {
		if bold {
			pdf.SetFont("Helvetica", "B", 10)
		} else {
			pdf.SetFont("Helvetica", "", 10)
		}
		pdf.SetX(x)
		pdf.CellFormat(w1, 6, label, "", 0, "L", false, 0, "")
		pdf.CellFormat(w2, 6, val, "", 1, "R", false, 0, "")
	}

	totalRow(latin1("Sous-total:"), fmtMoney(inv.SubtotalAmount, inv.Currency), false)

	// La ligne de TVA n'apparaît que s'il y a de la TVA. Elle était imprimée
	// même à 0 %, ce qu'une entreprise non assujettie n'a pas le droit de faire
	// figurer sur ses factures (LTVA art. 27 al. 1) — et qui la rendrait
	// redevable de l'impôt ainsi mentionné (al. 2).
	if inv.VATAmount != 0 || inv.VATRate != 0 {
		totalRow(fmt.Sprintf("TVA %.1f%%:", inv.VATRate), fmtMoney(inv.VATAmount, inv.Currency), false)
	}

	// Separator line
	y := pdf.GetY()
	pdf.Line(x, y, x+w1+w2, y)
	pdf.SetY(y + 1)

	totalRow("TOTAL "+inv.Currency+":", fmtMoney(inv.TotalAmount, inv.Currency), true)
	pdf.SetY(pdf.GetY() + 5)
}

func renderNotes(pdf *gofpdf.Fpdf, inv InvoiceData) {
	if inv.Notes != nil && *inv.Notes != "" {
		if pdf.GetY()+15 > contentBottom {
			pdf.AddPage()
		}
		pdf.SetFont("Helvetica", "I", 9)
		pdf.SetX(15)
		pdf.MultiCell(180, 5, latin1(*inv.Notes), "", "L", false)
		pdf.SetY(pdf.GetY() + 3)
	}
}

// ─── Swiss QR payment slip ────────────────────────────────────────────────────
// Layout dimensions (SPC 0200 v2.3 §3.5, BillLayout.java reference):
//
//	Slip height             = 105 mm (bottom of A4)
//	Receipt width           = 62 mm
//	Payment part width      = 148 mm
//	Margin (slip inner)     = 5 mm
//	QR code size            = 46 × 46 mm
//	QR left edge            = 67 mm (receiptWidth + margin)
//	QR top edge             = slipTop + 17 mm
//	Info column X           = 118 mm (62 + 46 + 2×5)
//	Amount section Y        = 260 mm (297 − 37)
//	Font: title 11pt bold; PP labels 8pt bold, PP values 10pt; RC labels 6pt bold, RC values 8pt

const (
	slipTop = 192.0 // 297 − 105 mm
	// contentBottom borne le contenu d'une page. Une marge de 15 mm en bas
	// laisse la place au numéro de page — qui, sur une pièce comptable de
	// plusieurs feuillets, est ce qui permet de constater qu'il n'en manque pas.
	contentBottom = 275.0
	receiptWidth  = 62.0
	pageWidth     = 210.0
)

// renderPaymentSlip draws the Swiss QR-bill payment slip at the bottom 105 mm.
// Uses SPC 0200 v2.3 structured address type S throughout.
func renderPaymentSlip(pdf *gofpdf.Fpdf, inv InvoiceData) error {
	// ── Determine IBAN and reference type ─────────────────────────────────────
	iban := inv.Company.IBAN
	if inv.Company.QRIBAN != "" {
		iban = inv.Company.QRIBAN
	}
	if iban == "" {
		return nil // no IBAN configured — skip slip silently
	}

	// Reference type must match the account type (IG v2.4 §4.2.2, field 28):
	// a QR-IBAN mandates QRR, a regular IBAN mandates SCOR or NON. The QR
	// reference is also restricted to CHF since IG v2.4.
	refType := "NON"
	var ref string
	if inv.Company.QRIBAN != "" {
		if inv.Currency != "CHF" {
			// QRR is CHF-only; a QR-IBAN cannot carry any other reference type,
			// so fall back to the regular IBAN rather than emit an invalid pairing.
			iban = inv.Company.IBAN
			if iban == "" {
				return fmt.Errorf("QR-IBAN cannot be used for %s invoices (QRR is CHF-only) and no regular IBAN is configured", inv.Currency)
			}
		} else {
			qrRef, err := compliance.GenerateQRRReference(extractDigits(inv.InvoiceNumber))
			if err != nil {
				// A QR-IBAN with a non-QRR reference is rejected by banks, so
				// surface the failure instead of silently emitting an invalid slip.
				return fmt.Errorf("QR-IBAN requires a QRR reference but generation failed for invoice %q: %w", inv.InvoiceNumber, err)
			}
			refType = "QRR"
			ref = qrRef
		}
	}

	// ── Split combined "postal town" strings into separate fields for S type ──
	// CompanyInfo.City is stored as "8001 Zürich"; the QR payload needs them split.
	credPostal, credTown := splitPostalCity(inv.Company.City)
	debtorPostal, debtorTown := splitPostalCity(inv.Customer.City)

	// Default country to CH when empty (required by structured address)
	credCountry := inv.Company.Country
	if credCountry == "" {
		credCountry = "CH"
	}
	debtorCountry := inv.Customer.Country
	if debtorCountry == "" {
		debtorCountry = "CH"
	}

	payload, err := compliance.GenerateQRBillPayload(compliance.QRBillData{
		// Creditor — structured address (S), building nr included in Street per §4.2.2
		CreditorIBAN:       iban,
		CreditorName:       inv.Company.Name,
		CreditorStreet:     inv.Company.Address, // "Bahnhofstrasse 1" — building nr allowed in StrtNm
		CreditorPostalCode: credPostal,
		CreditorTown:       credTown,
		CreditorCountry:    credCountry,
		// Amount
		Amount:   inv.TotalAmount,
		Currency: inv.Currency,
		// Debtor — only include when name is non-empty and address is complete enough
		DebtorName:       inv.Customer.Name,
		DebtorStreet:     inv.Customer.Address,
		DebtorPostalCode: debtorPostal,
		DebtorTown:       debtorTown,
		DebtorCountry:    debtorCountry,
		// Reference
		ReferenceType: refType,
		Reference:     ref,
		// Message — unstructured, max 140 chars
		Message:       inv.InvoiceNumber,
		InvoiceNumber: inv.InvoiceNumber,
		InvoiceDate:   inv.IssueDate,
	})
	if err != nil {
		return err
	}

	// ── Generate QR code (ECC Level M, 512 px for crisp print at 46 mm) ──────
	qrPNG, err := qrcode.Encode(payload, qrcode.Medium, 512)
	if err != nil {
		return fmt.Errorf("qr encode: %w", err)
	}

	// Overlay the Swiss cross (7×7 mm centred) — required by SPC 0200 v2.3 §6.4.2
	qrPNG = addSwissCross(qrPNG)

	// ── Layout constants (mm) ─────────────────────────────────────────────────
	const (
		margin     = 5.0
		rcWidth    = 52.0                  // receipt text area (62 − 2×5)
		qrSize     = 46.0                  // QR code printed size
		qrLeft     = receiptWidth + margin // 67 mm
		qrTop      = slipTop + 17.0        // 209 mm
		infoX      = 118.0                 // 62 + 46 + 2×5
		amountY    = 260.0                 // 297 − 37
		amountValY = 265.0
		ppX        = receiptWidth + margin // 67 mm
	)
	infoW := pageWidth - margin - infoX // 87 mm

	// ── Separator lines ───────────────────────────────────────────────────────
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.3)
	pdf.Line(0, slipTop, pageWidth, slipTop)
	pdf.SetFont("Helvetica", "", 6)
	pdf.SetXY(1, slipTop-2.5)
	pdf.CellFormat(10, 4, "- - -", "", 0, "L", false, 0, "")
	pdf.Line(receiptWidth, slipTop, receiptWidth, 297)

	// ── Receipt section ───────────────────────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(margin, slipTop+margin)
	pdf.CellFormat(rcWidth, 6, latin1("R\u00e9c\u00e9piss\u00e9"), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 6)
	pdf.SetX(margin)
	pdf.CellFormat(rcWidth, 3.5, latin1("Compte / Payable \u00e0"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	for _, line := range companyLines(iban, inv.Company) {
		pdf.SetX(margin)
		pdf.CellFormat(rcWidth, 4, latin1(line), "", 1, "L", false, 0, "")
	}

	if refType != "NON" {
		pdf.SetFont("Helvetica", "B", 6)
		pdf.SetX(margin)
		pdf.CellFormat(rcWidth, 3.5, latin1("R\u00e9f\u00e9rence"), "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetX(margin)
		pdf.CellFormat(rcWidth, 4, compliance.FormatQRRReference(ref), "", 1, "L", false, 0, "")
	}

	pdf.SetFont("Helvetica", "B", 6)
	pdf.SetX(margin)
	pdf.CellFormat(rcWidth, 3.5, "Payable par", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	for _, line := range customerLines(inv.Customer) {
		pdf.SetX(margin)
		pdf.CellFormat(rcWidth, 4, latin1(line), "", 1, "L", false, 0, "")
	}

	// Receipt amount
	pdf.SetFont("Helvetica", "B", 6)
	pdf.SetXY(margin, amountY)
	pdf.CellFormat(20, 3.5, "Monnaie", "", 0, "L", false, 0, "")
	pdf.SetX(margin + 22)
	pdf.CellFormat(28, 3.5, "Montant", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(margin, amountValY)
	pdf.CellFormat(20, 5, inv.Currency, "", 0, "L", false, 0, "")
	pdf.SetX(margin + 22)
	pdf.CellFormat(28, 5, fmtMoney(inv.TotalAmount, ""), "", 1, "L", false, 0, "")

	// ── Payment part ──────────────────────────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(ppX, slipTop+margin)
	pdf.CellFormat(qrSize+infoW+margin, 6, "Partie paiement", "", 1, "L", false, 0, "")

	// QR code image (with Swiss cross already embedded)
	imgKey := "qr_" + inv.InvoiceNumber
	reader := bytes.NewReader(qrPNG)
	pdf.RegisterImageOptionsReader(imgKey, gofpdf.ImageOptions{ImageType: "PNG"}, reader)
	pdf.ImageOptions(imgKey, qrLeft, qrTop, qrSize, qrSize, false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, "")

	// Info column — creditor
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(infoX, slipTop+margin+7)
	pdf.CellFormat(infoW, 4.5, latin1("Compte / Payable \u00e0"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	for _, line := range companyLines(iban, inv.Company) {
		pdf.SetX(infoX)
		pdf.CellFormat(infoW, 4.5, latin1(line), "", 1, "L", false, 0, "")
	}

	if refType != "NON" {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetX(infoX)
		pdf.CellFormat(infoW, 4.5, latin1("R\u00e9f\u00e9rence"), "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetX(infoX)
		pdf.CellFormat(infoW, 4.5, compliance.FormatQRRReference(ref), "", 1, "L", false, 0, "")
	}

	if inv.InvoiceNumber != "" {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetX(infoX)
		pdf.CellFormat(infoW, 4.5, "Message", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetX(infoX)
		pdf.CellFormat(infoW, 4.5, inv.InvoiceNumber, "", 1, "L", false, 0, "")
	}

	if inv.Customer.Name != "" {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetX(infoX)
		pdf.CellFormat(infoW, 4.5, "Payable par", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		for _, line := range customerLines(inv.Customer) {
			pdf.SetX(infoX)
			pdf.CellFormat(infoW, 4.5, latin1(line), "", 1, "L", false, 0, "")
		}
	}

	// Payment part amount
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(ppX, amountY)
	pdf.CellFormat(20, 4, "Monnaie", "", 0, "L", false, 0, "")
	pdf.SetX(ppX + 22)
	pdf.CellFormat(30, 4, "Montant", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(ppX, amountValY)
	pdf.CellFormat(20, 5, inv.Currency, "", 0, "L", false, 0, "")
	pdf.SetX(ppX + 22)
	pdf.CellFormat(30, 5, fmtMoney(inv.TotalAmount, ""), "", 1, "L", false, 0, "")

	return nil
}

// companyLines returns the creditor display lines for the payment slip.
func companyLines(iban string, c CompanyInfo) []string {
	var lines []string
	if iban != "" {
		lines = append(lines, formatIBAN(iban))
	}
	if c.Name != "" {
		lines = append(lines, c.Name)
	}
	if c.Address != "" {
		lines = append(lines, c.Address)
	}
	if c.City != "" {
		lines = append(lines, c.City)
	}
	return lines
}

// customerLines returns the debtor display lines for the payment slip.
func customerLines(c CustomerInfo) []string {
	var lines []string
	if c.Name != "" {
		lines = append(lines, c.Name)
	}
	if c.Address != "" {
		lines = append(lines, c.Address)
	}
	if c.City != "" {
		lines = append(lines, c.City)
	}
	return lines
}

// splitPostalCity splits a combined "4001 Basel" string into ("4001", "Basel").
// Swiss postal codes are the first whitespace-delimited token.
func splitPostalCity(s string) (postalCode, town string) {
	s = strings.TrimSpace(s)
	idx := strings.IndexByte(s, ' ')
	if idx <= 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
}

// addSwissCross overlays the mandatory Swiss cross logo (SPC 0200 v2.3 §6.4.2)
// centred on the QR code image. Dimensions: 7×7 mm at 46×46 mm printed size.
//
// Cross geometry (from SIX-Group reference implementation):
//
//	Outer square  = 7.0 mm (black)
//	White border  = 0.5 mm on each side
//	Cross arm width = 1.276 mm (white, centred in 6×6 mm inner area)
//
// Falls back to the original image on any decode/encode error.
func addSwissCross(qrPNG []byte) []byte {
	src, err := png.Decode(bytes.NewReader(qrPNG))
	if err != nil {
		return qrPNG
	}

	bounds := src.Bounds()
	w := bounds.Dx()

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// Scale factor: QR image width covers 46 mm
	pxPerMm := float64(w) / 46.0

	crossPx := iround(7.0 * pxPerMm)  // outer black square
	borderPx := iround(0.5 * pxPerMm) // white border
	armPx := iround(1.276 * pxPerMm)  // cross arm width
	if armPx < 2 {
		armPx = 2
	}

	// Top-left of centred square
	cx := (w - crossPx) / 2
	cy := (bounds.Dy() - crossPx) / 2

	black := color.RGBA{0, 0, 0, 255}
	white := color.RGBA{255, 255, 255, 255}

	// 1. Black outer square
	fillRect(dst, cx, cy, crossPx, crossPx, black)

	// 2. White cross arms centred in the inner area (after border)
	innerX := cx + borderPx
	innerY := cy + borderPx
	innerSz := crossPx - 2*borderPx
	if innerSz <= 0 {
		innerSz = crossPx
		innerX = cx
		innerY = cy
	}
	armOffset := (innerSz - armPx) / 2
	if armOffset < 0 {
		armOffset = 0
	}
	// Horizontal arm
	fillRect(dst, innerX, innerY+armOffset, innerSz, armPx, white)
	// Vertical arm
	fillRect(dst, innerX+armOffset, innerY, armPx, innerSz, white)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return qrPNG
	}
	return out.Bytes()
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	b := img.Bounds()
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if px >= b.Min.X && px < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
				img.Set(px, py, c)
			}
		}
	}
}

func iround(f float64) int {
	return int(math.Round(f))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func fmtMoney(amount float64, currency string) string {
	if currency != "" {
		return fmt.Sprintf("%s %s", currency, formatAmount(amount))
	}
	return formatAmount(amount)
}

func formatAmount(amount float64) string {
	// Format with 2 decimal places and thousands separator
	rounded := math.Round(amount*100) / 100
	return fmt.Sprintf("%.2f", rounded)
}

func fmtFloat(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}

// formatIBAN inserts spaces every 4 chars for readability: CHxx xxxx xxxx ...
func formatIBAN(iban string) string {
	clean := ""
	for _, ch := range iban {
		if ch != ' ' {
			clean += string(ch)
		}
	}
	var parts []string
	for i := 0; i < len(clean); i += 4 {
		end := i + 4
		if end > len(clean) {
			end = len(clean)
		}
		parts = append(parts, clean[i:end])
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

// extractDigits strips non-digit characters (for QRR reference generation from invoice numbers).
func extractDigits(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// latin1 converts a UTF-8 string to ISO-8859-1 (Latin-1) bytes so that fpdf's
// standard Core fonts (Helvetica, Times, Courier) render accented characters
// correctly. Unicode code points U+0000–U+00FF map one-to-one to Latin-1 byte
// values; code points above U+00FF are replaced with '?'.
func latin1(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 0x100 {
			b = append(b, byte(r))
		} else {
			b = append(b, '?')
		}
	}
	return string(b)
}

// ─── Document type ────────────────────────────────────────────────────────────

// documentTitle names the document on the page. The heading, the number label
// and the presence of a payment slip are the only things distinguishing a
// quote from an invoice on paper, so they must agree with document_type.
func documentTitle(docType string) string {
	switch docType {
	case "quote":
		return "OFFRE DE PRIX"
	case "credit_note":
		return "NOTE DE CRÉDIT"
	default:
		return "FACTURE"
	}
}

func documentNumberLabel(docType string) string {
	switch docType {
	case "quote":
		return "N° offre:"
	case "credit_note":
		return "N° note de crédit:"
	default:
		return "N° facture:"
	}
}

// dueDateLabel names what the second date means. On an offer nothing is due —
// the date says how long the price stands, and calling it "Échéance" invites
// the reader to treat the document as payable.
func dueDateLabel(docType string) string {
	if docType == "quote" {
		return "Valable jusqu'au:"
	}
	return "Échéance:"
}

// wantsPaymentSlip reports whether a Swiss QR payment slip belongs on this
// document. Only an invoice asks the recipient for money.
func wantsPaymentSlip(docType string) bool {
	return docType == "" || docType == "invoice"
}

// companyLine écrit une ligne d'identification de l'émetteur sous l'adresse.
func companyLine(pdf *gofpdf.Fpdf, x float64, text string) {
	pdf.SetX(x)
	pdf.CellFormat(115-x+15, 5, latin1(text), "", 1, "L", false, 0, "")
}

// normaliseUID retire ponctuation, espaces et mentions pour comparer un IDE à
// un numéro de TVA : « CHE-123.456.789 » et « CHE-123.456.789 TVA » désignent
// la même entreprise, et les afficher tous les deux ferait chercher une
// différence qui n'existe pas.
func normaliseUID(v string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(v) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	for _, suffix := range []string{"TVA", "MWST", "IVA", "VAT"} {
		out = strings.TrimSuffix(out, suffix)
	}
	return out
}

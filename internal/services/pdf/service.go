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
	VATNumber string // e.g. "CHE-123.456.789 MWST"
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
	IssueDate     time.Time
	DueDate       time.Time
	Currency      string
	Status        string

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
	pdf.SetAutoPageBreak(false, 0)
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
	if err := renderPaymentSlip(pdf, inv); err != nil {
		// Non-fatal: log but still return the PDF without slip
		_ = err
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

	// Company name (large)
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetXY(textX, 15)
	pdf.CellFormat(115-textX+15, 7, latin1(inv.Company.Name), "", 1, "L", false, 0, "")

	// Company address (small)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetX(textX)
	pdf.CellFormat(115-textX+15, 5, latin1(inv.Company.Address), "", 1, "L", false, 0, "")
	pdf.SetX(textX)
	pdf.CellFormat(115-textX+15, 5, latin1(inv.Company.City), "", 1, "L", false, 0, "")
	if inv.Company.VATNumber != "" {
		pdf.SetX(textX)
		pdf.CellFormat(115-textX+15, 5, latin1("TVA/MwSt: "+inv.Company.VATNumber), "", 1, "L", false, 0, "")
	}

	// "FACTURE" title (right)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetXY(130, 15)
	pdf.CellFormat(65, 12, "FACTURE", "", 1, "R", false, 0, "")

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

	metaRow(latin1("N\u00b0 facture:"), inv.InvoiceNumber)
	metaRow("Date:", inv.IssueDate.Format("02.01.2006"))
	metaRow(latin1("\u00c9ch\u00e9ance:"), inv.DueDate.Format("02.01.2006"))
	metaRow("Devise:", inv.Currency)

	pdf.SetY(y + 5)
}

func renderLines(pdf *gofpdf.Fpdf, inv InvoiceData) {
	// Table header
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetX(15)
	pdf.CellFormat(90, 7, "Description", "1", 0, "L", true, 0, "")
	pdf.CellFormat(20, 7, latin1("Qt\u00e9"), "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 7, "Prix unit.", "1", 0, "R", true, 0, "")
	pdf.CellFormat(15, 7, "TVA%", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 7, "Total", "1", 1, "R", true, 0, "")

	// Table rows
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetFillColor(255, 255, 255)
	fill := false
	for _, line := range inv.Lines {
		pdf.SetFillColor(255, 255, 255)
		if fill {
			pdf.SetFillColor(250, 250, 250)
		}
		pdf.SetX(15)
		pdf.CellFormat(90, 6, latin1(line.Description), "1", 0, "L", fill, 0, "")
		pdf.CellFormat(20, 6, fmtFloat(line.Quantity), "1", 0, "C", fill, 0, "")
		pdf.CellFormat(30, 6, fmtMoney(line.UnitPrice, inv.Currency), "1", 0, "R", fill, 0, "")
		pdf.CellFormat(15, 6, fmt.Sprintf("%.1f%%", line.VATRate), "1", 0, "C", fill, 0, "")
		pdf.CellFormat(25, 6, fmtMoney(line.LineTotal, inv.Currency), "1", 1, "R", fill, 0, "")
		fill = !fill
	}

	pdf.SetY(pdf.GetY() + 3)
}

func renderTotals(pdf *gofpdf.Fpdf, inv InvoiceData) {
	x := 130.0
	w1, w2 := 35.0, 30.0

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
	totalRow(fmt.Sprintf("TVA %.1f%%:", inv.VATRate), fmtMoney(inv.VATAmount, inv.Currency), false)

	// Separator line
	y := pdf.GetY()
	pdf.Line(x, y, x+w1+w2, y)
	pdf.SetY(y + 1)

	totalRow("TOTAL "+inv.Currency+":", fmtMoney(inv.TotalAmount, inv.Currency), true)
	pdf.SetY(pdf.GetY() + 5)
}

func renderNotes(pdf *gofpdf.Fpdf, inv InvoiceData) {
	if inv.Notes != nil && *inv.Notes != "" {
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
	slipTop      = 192.0 // 297 − 105 mm
	receiptWidth = 62.0
	pageWidth    = 210.0
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

	refType := "NON"
	var ref string
	if inv.Company.QRIBAN != "" {
		qrRef, err := compliance.GenerateQRRReference(extractDigits(inv.InvoiceNumber))
		if err == nil {
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
		rcWidth    = 52.0  // receipt text area (62 − 2×5)
		qrSize     = 46.0  // QR code printed size
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

	crossPx  := iround(7.0 * pxPerMm)   // outer black square
	borderPx := iround(0.5 * pxPerMm)   // white border
	armPx    := iround(1.276 * pxPerMm) // cross arm width
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
	innerX  := cx + borderPx
	innerY  := cy + borderPx
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

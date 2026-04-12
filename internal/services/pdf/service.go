// Package pdf generates invoice PDFs with an embedded Swiss payment slip (QR-bill SPC 0200).
package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
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
// Layout per SIX-Group Swiss Payment Standards:
// - Slip height: 105 mm from bottom of A4 (297 mm)
// - Receipt section: 62 mm wide (left)
// - Separator: vertical line
// - Payment part: 148 mm wide (right)
// - QR code: 46×46 mm starting at x=67, y=297-105+17

const (
	slipTop      = 192.0 // 297 - 105
	receiptWidth = 62.0
	pageWidth    = 210.0
)

// renderPaymentSlip draws the Swiss QR-bill payment slip (SPC 0200) at the bottom
// 105 mm of the page, following the exact layout from manuelbl/SwissQRBill BillLayout.java:
//
//   MARGIN                = 5 mm
//   RECEIPT_WIDTH         = 62 mm
//   RECEIPT_TEXT_WIDTH    = 52 mm   (RECEIPT_WIDTH – 2×MARGIN)
//   QR code               = 46×46 mm, left edge at x=67, top at y=slipTop+17
//   QR_CODE_BOTTOM        = 42 mm from slip bottom → top at 105-42-46 = 17 mm from slip top
//   PP_DETAIL_TEXT_X      = 118 mm  (62 + 46 + 2×5)
//   AMOUNT_SECTION_TOP    = 37 mm from slip bottom → y = 297-37 = 260 mm
//   Font: title=11pt; PP label=8pt, PP body=10pt; RC label=6pt, RC body=8pt
func renderPaymentSlip(pdf *gofpdf.Fpdf, inv InvoiceData) error {
	// Determine which IBAN and reference to use
	iban := inv.Company.IBAN
	if inv.Company.QRIBAN != "" {
		iban = inv.Company.QRIBAN
	}
	if iban == "" {
		return nil // no IBAN configured — skip slip
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

	payload, err := compliance.GenerateQRBillPayload(compliance.QRBillData{
		CreditorIBAN:    iban,
		CreditorName:    inv.Company.Name,
		CreditorAddress: inv.Company.Address,
		CreditorCity:    inv.Company.City,
		CreditorCountry: inv.Company.Country,
		Amount:          inv.TotalAmount,
		Currency:        inv.Currency,
		DebtorName:      inv.Customer.Name,
		DebtorAddress:   inv.Customer.Address,
		DebtorCity:      inv.Customer.City,
		DebtorCountry:   inv.Customer.Country,
		ReferenceType:   refType,
		Reference:       ref,
		Message:         inv.InvoiceNumber,
		InvoiceNumber:   inv.InvoiceNumber,
		InvoiceDate:     inv.IssueDate,
	})
	if err != nil {
		return err
	}

	// Generate QR code at 512 px for crisp print output (SPC 0200 requires ECC Level M)
	qrPNG, err := qrcode.Encode(payload, qrcode.Medium, 512)
	if err != nil {
		return fmt.Errorf("qr encode: %w", err)
	}

	// ── Layout constants (all mm, matching BillLayout.java) ───────────────
	const (
		margin      = 5.0  // slip inner margin
		rcWidth     = 52.0 // receipt text width  (62 - 2×5)
		qrSize      = 46.0 // QR code side length
		qrLeft      = receiptWidth + margin // 67 mm from page left
		qrTop       = slipTop + 17.0        // 209 mm from page top (QR_CODE_BOTTOM=42 mm from slip bottom)
		infoX       = 118.0                 // receipt(62)+qr(46)+2×margin(10)
		amountY     = 260.0                 // 297 - AMOUNT_SECTION_TOP(37)
		amountValY  = 265.0                 // value row 5 mm below labels
		ppX         = receiptWidth + margin // 67 mm — payment part text left edge
	)
	infoW := pageWidth - margin - infoX // 87 mm  (matches PP_INFO_SECTION_WIDTH)

	// ── Separator lines ───────────────────────────────────────────────────
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.3)
	pdf.Line(0, slipTop, pageWidth, slipTop)

	pdf.SetFont("Helvetica", "", 6)
	pdf.SetXY(1, slipTop-2.5)
	pdf.CellFormat(10, 4, "- - -", "", 0, "L", false, 0, "")

	pdf.Line(receiptWidth, slipTop, receiptWidth, 297)

	// ── Receipt section (x = 5…57 mm) ────────────────────────────────────
	// Title
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(margin, slipTop+margin)
	pdf.CellFormat(rcWidth, 6, latin1("R\u00e9c\u00e9piss\u00e9"), "", 1, "L", false, 0, "")

	// "Compte / Payable à" block — RC labels 6pt, RC body 8pt
	pdf.SetFont("Helvetica", "B", 6)
	pdf.SetX(margin)
	pdf.CellFormat(rcWidth, 3.5, latin1("Compte / Payable \u00e0"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	for _, line := range []string{
		formatIBAN(iban),
		inv.Company.Name,
		inv.Company.Address,
		inv.Company.City,
	} {
		if line == "" {
			continue
		}
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
	for _, line := range []string{inv.Customer.Name, inv.Customer.Address, inv.Customer.City} {
		if line == "" {
			continue
		}
		pdf.SetX(margin)
		pdf.CellFormat(rcWidth, 4, latin1(line), "", 1, "L", false, 0, "")
	}

	// Receipt amount (AMOUNT_SECTION_TOP = 37 mm from slip bottom → y=260)
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

	// ── Payment part (x ≥ 62 mm) ──────────────────────────────────────────
	// Title
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(ppX, slipTop+margin)
	pdf.CellFormat(qrSize+infoW+margin, 6, "Partie paiement", "", 1, "L", false, 0, "")

	// QR code — top at slipTop+17 mm (QR_CODE_BOTTOM = 42 mm from slip bottom)
	imgKey := "qr_" + inv.InvoiceNumber
	_ = png.Decode // ensure image/png is registered
	reader := bytes.NewReader(qrPNG)
	pdf.RegisterImageOptionsReader(imgKey, gofpdf.ImageOptions{ImageType: "PNG"}, reader)
	pdf.ImageOptions(imgKey, qrLeft, qrTop, qrSize, qrSize, false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, "")

	// Creditor info column (x = 118 mm) — PP labels 8pt bold, PP body 10pt
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(infoX, slipTop+margin+7) // start just below title
	pdf.CellFormat(infoW, 4.5, latin1("Compte / Payable \u00e0"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	for _, line := range []string{
		formatIBAN(iban),
		inv.Company.Name,
		inv.Company.Address,
		inv.Company.City,
	} {
		if line == "" {
			continue
		}
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
		for _, line := range []string{inv.Customer.Name, inv.Customer.Address, inv.Customer.City} {
			if line == "" {
				continue
			}
			pdf.SetX(infoX)
			pdf.CellFormat(infoW, 4.5, latin1(line), "", 1, "L", false, 0, "")
		}
	}

	// Payment part amount (same AMOUNT_SECTION_TOP = y=260)
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

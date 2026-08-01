package pdf

import (
	"strings"
	"testing"
	"time"
)

// baseDoc is a complete, payable document. Tests vary only DocumentType so any
// difference in the output is attributable to that field alone.
func baseDoc(docType string) InvoiceData {
	issued := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return InvoiceData{
		InvoiceNumber:  "FA-2026-0001",
		DocumentType:   docType,
		IssueDate:      issued,
		DueDate:        issued.AddDate(0, 0, 30),
		Currency:       "CHF",
		Status:         "sent",
		SubtotalAmount: 1000,
		VATAmount:      81,
		TotalAmount:    1081,
		VATRate:        8.1,
		Lines: []InvoiceLine{
			{Description: "Prestation", Quantity: 1, UnitPrice: 1000, VATRate: 8.1, LineTotal: 1000},
		},
		Company: CompanyInfo{
			Name:      "Alpes SA",
			Address:   "Rue du Test 1",
			City:      "1200 Genève",
			Country:   "CH",
			IBAN:      "CH9300762011623852957",
			VATNumber: "CHE-123.456.789 MWST",
		},
		Customer: CustomerInfo{Name: "Client SA", Address: "Av. Client 2", City: "1000 Lausanne", Country: "CH"},
	}
}

func generate(t *testing.T, docType string) []byte {
	t.Helper()
	out, err := Generate(baseDoc(docType))
	if err != nil {
		t.Fatalf("Generate(%q): %v", docType, err)
	}
	if !strings.HasPrefix(string(out[:5]), "%PDF-") {
		t.Fatalf("Generate(%q) did not return a PDF", docType)
	}
	return out
}

// The defect this guards: the renderer ignored document_type entirely, so a
// price offer came out headed "FACTURE" and carrying a Swiss QR payment slip.
// The prospect could pay it and deduct the VAT printed on it, and LTVA
// art. 27 al. 2 makes the issuer liable for tax stated without entitlement.
func TestQuoteCarriesNoPaymentSlip(t *testing.T) {
	invoice := generate(t, "invoice")
	quote := generate(t, "quote")

	// The slip embeds a rendered QR PNG, by far the largest object on the page.
	// Its absence is what makes the quote document materially smaller.
	if len(quote) >= len(invoice) {
		t.Errorf("quote PDF is %d bytes vs invoice %d — the QR payment slip appears to still be drawn",
			len(quote), len(invoice))
	}
}

func TestCreditNoteCarriesNoPaymentSlip(t *testing.T) {
	invoice := generate(t, "invoice")
	credit := generate(t, "credit_note")

	if len(credit) >= len(invoice) {
		t.Errorf("credit note PDF is %d bytes vs invoice %d — a slip asking the customer to pay does not belong on a credit note",
			len(credit), len(invoice))
	}
}

// An empty document_type must keep behaving exactly like an invoice: rows
// predating the column default to "" and must still produce a payable bill.
func TestEmptyDocumentTypeBehavesAsInvoice(t *testing.T) {
	if len(generate(t, "")) != len(generate(t, "invoice")) {
		t.Error("an empty document_type must render identically to an invoice")
	}
}

func TestDocumentTitleAndNumberLabel(t *testing.T) {
	cases := []struct{ docType, title, label string }{
		{"invoice", "FACTURE", "N° facture:"},
		{"quote", "OFFRE DE PRIX", "N° offre:"},
		{"credit_note", "NOTE DE CRÉDIT", "N° note de crédit:"},
		{"", "FACTURE", "N° facture:"},
		{"unexpected", "FACTURE", "N° facture:"},
	}
	for _, c := range cases {
		if got := documentTitle(c.docType); got != c.title {
			t.Errorf("documentTitle(%q) = %q, want %q", c.docType, got, c.title)
		}
		if got := documentNumberLabel(c.docType); got != c.label {
			t.Errorf("documentNumberLabel(%q) = %q, want %q", c.docType, got, c.label)
		}
	}
}

func TestWantsPaymentSlip(t *testing.T) {
	for docType, want := range map[string]bool{
		"invoice": true, "": true, "quote": false, "credit_note": false,
	} {
		if got := wantsPaymentSlip(docType); got != want {
			t.Errorf("wantsPaymentSlip(%q) = %v, want %v", docType, got, want)
		}
	}
}

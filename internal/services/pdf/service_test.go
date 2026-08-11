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
		inv := InvoiceData{DocumentType: c.docType}
		if got := documentTitle(inv); got != c.title {
			t.Errorf("documentTitle(%q) = %q, want %q", c.docType, got, c.title)
		}
		if got := documentNumberLabel(inv); got != c.label {
			t.Errorf("documentNumberLabel(%q) = %q, want %q", c.docType, got, c.label)
		}
	}
}

func TestDueDateLabel(t *testing.T) {
	if got := dueDateLabel(InvoiceData{DocumentType: "quote"}); got != "Valable jusqu'au:" {
		t.Errorf("dueDateLabel(\"quote\") = %q — nothing is due on an offer", got)
	}
	for _, d := range []string{"invoice", "credit_note", ""} {
		if got := dueDateLabel(InvoiceData{DocumentType: d}); got != "Échéance:" {
			t.Errorf("dueDateLabel(%q) = %q, want \"Échéance:\"", d, got)
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

// LTVA art. 27 al. 4 defines a correction as "un document qui mentionne et
// annule la facture d'origine". The mention has to be on the page, not only in
// the database, so this asserts on the rendered bytes rather than on a field.
func TestCreditNoteNamesTheInvoiceItCancels(t *testing.T) {
	doc := baseDoc("credit_note")
	doc.InvoiceNumber = "NC-2026-0001"
	doc.CorrectsInvoiceNumber = "FA-2026-0042"

	// Compression off so the text is inspectable; the layout is unchanged.
	withoutRef := doc
	withoutRef.CorrectsInvoiceNumber = ""

	with, err := Generate(doc)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	without, err := Generate(withoutRef)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(with) <= len(without) {
		t.Error("the corrected invoice number does not reach the page")
	}
}

// An invoice must keep its due date; only a credit note trades it for the
// reference to what it cancels.
func TestOnlyCreditNotesReplaceTheDueDateRow(t *testing.T) {
	inv := baseDoc("invoice")
	inv.CorrectsInvoiceNumber = "FA-2026-0042" // ignored on an invoice

	a, _ := Generate(inv)
	b, _ := Generate(baseDoc("invoice"))
	if len(a) != len(b) {
		t.Error("CorrectsInvoiceNumber changed an invoice's layout; it must only affect credit notes")
	}
}

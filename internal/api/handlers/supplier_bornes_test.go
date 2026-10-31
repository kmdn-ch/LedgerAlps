package handlers

import (
	"math"
	"strings"
	"testing"
)

// Audit 9, A-04 : une facture REÇUE était arrondie à 5 centimes comme une facture
// émise. 1234.56 devenait 1234.55 ; l'ordre de paiement versait 1234.55 au
// fournisseur, un centime de moins que son bulletin QR.
func TestUneFactureRecueGardeSesCentimes(t *testing.T) {
	for _, c := range []struct {
		prix, taux, ht, tva, ttc float64
	}{
		{1234.56, 0, 1234.56, 0, 1234.56},
		{12.34, 0, 12.34, 0, 12.34},
		{99.99, 0, 99.99, 0, 99.99},
		{12.34, 0.081, 12.34, 1.00, 13.34}, // 12.34 × 8.1 % = 0.99954
		{1000, 0.081, 1000, 81, 1081},
	} {
		sub, vat, total, _ := computeTotals([]supplierInvoiceLine{{Quantity: 1, UnitPrice: c.prix, VATRate: c.taux}})
		if math.Abs(sub-c.ht) > 1e-9 || math.Abs(vat-c.tva) > 1e-9 || math.Abs(total-c.ttc) > 1e-9 {
			t.Errorf("%.2f à %.3f : HT %.4f / TVA %.4f / TTC %.4f, attendu %.2f / %.2f / %.2f",
				c.prix, c.taux, sub, vat, total, c.ht, c.tva, c.ttc)
		}
	}
}

func TestLesBornesDUneFactureRecue(t *testing.T) {
	bonne := func() createSupplierInvoiceRequest {
		return createSupplierInvoiceRequest{
			SupplierID: "s", SupplierReference: "R-1", IssueDate: "2026-05-01", DueDate: "2026-05-31",
			Lines: []supplierInvoiceLine{{Description: "Achat", Quantity: 1, UnitPrice: 100, VATRate: 0.081}},
		}
	}
	req := bonne()
	if err := validateSupplierBounds(&req); err != nil {
		t.Fatalf("facture ordinaire refusée : %v", err)
	}
	if req.Currency != "CHF" {
		t.Errorf("devise par défaut = %q, attendu CHF", req.Currency)
	}

	for nom, mod := range map[string]func(r *createSupplierInvoiceRequest){
		"émission en l'an 26":  func(r *createSupplierInvoiceRequest) { r.IssueDate = "0026-10-30" },
		"échéance en 9999":     func(r *createSupplierInvoiceRequest) { r.DueDate = "9999-12-31" },
		"devise XXX":           func(r *createSupplierInvoiceRequest) { r.Currency = "XXX" },
		"prix négatif":         func(r *createSupplierInvoiceRequest) { r.Lines[0].UnitPrice = -10 },
		"prix 1e300":           func(r *createSupplierInvoiceRequest) { r.Lines[0].UnitPrice = 1e300 },
		"quantité négative":    func(r *createSupplierInvoiceRequest) { r.Lines[0].Quantity = -1 },
		"TVA de 810 %":         func(r *createSupplierInvoiceRequest) { r.Lines[0].VATRate = 8.1 },
		"TVA négative":         func(r *createSupplierInvoiceRequest) { r.Lines[0].VATRate = -0.081 },
		"libellé de 3000 car.": func(r *createSupplierInvoiceRequest) { r.Lines[0].Description = strings.Repeat("x", 3000) },
	} {
		r := bonne()
		mod(&r)
		if err := validateSupplierBounds(&r); err == nil {
			t.Errorf("%s : accepté", nom)
		}
	}

	r := bonne()
	r.Currency = "EUR"
	if err := validateSupplierBounds(&r); err != nil {
		t.Errorf("une facture fournisseur en EUR (devise du bulletin QR) est refusée : %v", err)
	}
}

package invoicing

// Bornes d'un document avant qu'il n'entre en base (audit 9, A-03, A-05, M-05).
//
// Elles vivent dans le SERVICE et non seulement dans la route : la conversion
// d'une offre, la note de crédit et toute modification passent aussi ici, et un
// document comptabilisé ne se corrige plus. Les valeurs limites sont dans
// compliance/bornes.go.

import (
	"context"
	"fmt"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ErrCurrencyMismatch : le document n'est pas dans la devise de la comptabilité.
//
// Le produit est mono-devise : un document en EUR dans des livres tenus en CHF
// était comptabilisé 1:1, c'est-à-dire avec un produit faux (M-05). Il n'y a pas
// de cours de change ; refuser vaut mieux que d'inventer un taux.
type ErrCurrencyMismatch struct{ Got, Books string }

func (e ErrCurrencyMismatch) Error() string {
	return fmt.Sprintf("la devise du document (%s) n'est pas celle de la comptabilité (%s) : "+
		"la tenue en plusieurs devises n'est pas prise en charge", e.Got, e.Books)
}

// bookCurrency est la devise de la comptabilité : celle de la fiche société, CHF
// à défaut (installation en cours de création).
func (s *Service) bookCurrency(ctx context.Context) string {
	var cur string
	q := db.Rebind(`SELECT COALESCE(currency, '') FROM company_settings LIMIT 1`, s.usePostgres)
	if err := s.db.QueryRowContext(ctx, q).Scan(&cur); err != nil || cur == "" {
		return "CHF"
	}
	return cur
}

// validateDocument contrôle dates, devise et lignes d'un document, et complète la
// devise absente. Les montants totaux se contrôlent après computeTotals.
func (s *Service) validateDocument(ctx context.Context, req *CreateInvoiceRequest) error {
	if err := compliance.CheckBookingDate(req.IssueDate); err != nil {
		return err
	}
	if !req.DueDate.IsZero() {
		if err := compliance.CheckBookingDate(req.DueDate); err != nil {
			return err
		}
	}
	books := s.bookCurrency(ctx)
	if req.Currency == "" {
		req.Currency = books
	} else if req.Currency != books {
		return ErrCurrencyMismatch{Got: req.Currency, Books: books}
	}
	return validateLines(req.Lines)
}

// validateLines refuse les lignes que le produit ne sait pas facturer : voir
// compliance.CheckDocumentLine.
func validateLines(lines []LineInput) error {
	if len(lines) > compliance.MaxDocumentLines {
		return fmt.Errorf("un document ne peut pas comporter plus de %d lignes", compliance.MaxDocumentLines)
	}
	for i, l := range lines {
		if err := compliance.CheckDocumentLine(i+1, l.Description, l.Quantity, l.UnitPrice, l.DiscountPct, l.VATRate); err != nil {
			return err
		}
	}
	return nil
}

// checkTotals borne les totaux calculés : une somme de lignes admissibles peut
// encore dépasser la borne.
func checkTotals(subtotal, vat, total float64) error {
	for _, v := range []float64{subtotal, vat, total} {
		if err := compliance.CheckAmount(v); err != nil {
			return err
		}
	}
	return nil
}

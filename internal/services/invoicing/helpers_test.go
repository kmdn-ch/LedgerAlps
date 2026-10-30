package invoicing

import (
	"context"

	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// Variantes SANS AUTEUR, réservées aux tests.
//
// Elles vivaient dans le code de production, sans autre appelant que ces
// tests : une action exécutée avec un Actor vide n'écrit aucun maillon d'audit,
// et le premier code qui les aurait appelées aurait modifié des pièces
// comptables sans que la chaîne d'empreintes le dise. Les garder ici les rend
// inatteignables hors des tests, et leur nom dit ce qu'elles font : pas de trace.

func (s *Service) Transition(ctx context.Context, invoiceID string, to models.InvoiceStatus) error {
	return s.TransitionBy(ctx, invoiceID, to, Actor{})
}

func (s *Service) UpdateInvoice(ctx context.Context, invoiceID string, req CreateInvoiceRequest) (*models.Invoice, error) {
	return s.UpdateInvoiceBy(ctx, invoiceID, req, Actor{})
}

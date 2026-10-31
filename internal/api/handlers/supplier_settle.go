package handlers

// Le règlement d'une facture fournisseur, et les transitions qui y mènent.
//
// # Ce que faisait « payée » (audit croisé Gemini, CRIT-06)
//
// La route de transition changeait le statut et n'écrivait rien d'autre :
//
//   - passer une facture comptabilisée à « payée » ne passait AUCUNE écriture de
//     règlement : la dette (2000) gonflait indéfiniment au bilan, `amount_paid`
//     restait à zéro, le paiement n'avait aucune trace au journal ;
//   - passer un BROUILLON à « payée » faisait entrer sa TVA préalable dans la
//     déclaration (le filtre lit `status IN ('booked', 'paid')`) sans qu'aucune
//     charge ne soit jamais comptabilisée ;
//   - une facture comptabilisée repassait en brouillon, se modifiait, et se
//     SUPPRIMAIT — son écriture restant au journal, orpheline de sa pièce ;
//   - elle passait à « annulée » sans extourne : charge et dette restaient.
//
// # La machine à états
//
//	brouillon   → comptabilisée | annulée
//	comptabilisée → payée | annulée (par extourne)
//
// Rien d'autre. Une pièce comptabilisée ne redevient pas un brouillon (CO art. 957a) :
// elle s'annule, ce qui passe l'extourne, ou se corrige par une écriture. Une pièce
// payée ne s'annule pas : le décaissement est parti, c'est un remboursement du
// fournisseur qui se saisit comme tel.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Comptes de trésorerie du plan comptable PME suisse.
const (
	accountSupplierBank = "1020" // Banque (compte courant)
	accountSupplierCash = "1000" // Caisse
)

// supplierTransitions est la machine à états d'une facture fournisseur.
var supplierTransitions = map[string][]string{
	"draft":  {"booked", "cancelled"},
	"booked": {"paid", "cancelled"},
}

func supplierTransitionAllowed(from, to string) bool {
	for _, t := range supplierTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Refus de transition. Phrases FIXES : le catalogue les rend par correspondance exacte.
var (
	errSupplierTransition = errors.New(
		"cette transition n'est pas permise pour une facture fournisseur : une pièce comptabilisée " +
			"ne redevient pas un brouillon, elle s'annule (extourne) ou se corrige par une écriture")
	errSupplierPayBeforeBook = errors.New(
		"une facture fournisseur doit être comptabilisée avant d'être payée")
	errSupplierNotBooked = errors.New(
		"seule une facture fournisseur comptabilisée se règle")
)

// supplierSettleMu sérialise les règlements du processus : la lecture du statut, l'écriture
// et le changement de statut forment une suite qu'un second appel ne doit pas croiser.
var supplierSettleMu sync.Mutex

// settleSupplierInvoice règle une facture comptabilisée : Débit 2000 Créanciers / Crédit
// banque (ou caisse), puis statut « payée » et `amount_paid`. Rend l'identifiant de l'écriture,
// vide quand il n'y avait rien à régler.
//
// L'écriture précède le changement de statut : si elle échoue (exercice clos, compte absent),
// rien n'a changé et l'erreur est franche — même ordre que les encaissements clients.
func (h *SupplierInvoicesHandler) settleSupplierInvoice(
	ctx context.Context, id, userID, ip string, date time.Time, method string,
) (string, error) {
	supplierSettleMu.Lock()
	defer supplierSettleMu.Unlock()

	var (
		status, reference, supplierName string
		total, paid                     float64
	)
	q := db.Rebind(`
		SELECT si.status, si.supplier_reference, COALESCE(c.name, ''), si.total_amount, si.amount_paid
		FROM supplier_invoices si LEFT JOIN contacts c ON c.id = si.supplier_id
		WHERE si.id = ?`, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, q, id).Scan(&status, &reference, &supplierName, &total, &paid); err == sql.ErrNoRows {
		return "", fmt.Errorf("facture fournisseur introuvable")
	} else if err != nil {
		return "", fmt.Errorf("lecture de la facture: %w", err)
	}
	if status != "booked" {
		return "", errSupplierNotBooked
	}
	if err := compliance.CheckBookingDate(date); err != nil {
		return "", err
	}

	due := round2(total - paid)
	entryID := ""
	if due > 0.004 && h.accountingSvc != nil {
		bankCode := accountSupplierBank
		if method == "cash" {
			bankCode = accountSupplierCash
		}
		payablesID, err := h.accountID(ctx, accountPayables)
		if err != nil {
			return "", err
		}
		bankID, err := h.accountID(ctx, bankCode)
		if err != nil {
			return "", err
		}
		entry, err := h.accountingSvc.CreateEntry(ctx, userID, accounting.CreateEntryRequest{
			Date:        date,
			Description: fmt.Sprintf("Règlement facture fournisseur %s — %s", reference, supplierName),
			Lines: []accounting.LineInput{
				{AccountID: payablesID, DebitAmount: ptr(due), Description: "Dette soldée — " + reference, Sequence: 1},
				{AccountID: bankID, CreditAmount: ptr(due), Description: "Règlement " + method + " — " + reference, Sequence: 2},
			},
		})
		if err != nil {
			return "", fmt.Errorf("création de l'écriture de règlement: %w", err)
		}
		if err := h.accountingSvc.PostEntry(ctx, userID, entry.ID, ip); err != nil {
			return "", fmt.Errorf("comptabilisation de l'écriture de règlement %s: %w", entry.ID, err)
		}
		entryID = entry.ID
	}

	var entry any
	if entryID != "" {
		entry = entryID
	}
	upd := db.Rebind(`
		UPDATE supplier_invoices
		SET status = 'paid', amount_paid = ?, payment_entry_id = ?, updated_at = ?
		WHERE id = ? AND status = 'booked'`, h.usePostgres)
	res, err := h.db.ExecContext(ctx, upd, round2(total), entry, time.Now().UTC(), id)
	if err != nil {
		return entryID, fmt.Errorf("l'écriture de règlement %s est passée mais le statut n'a pas pu être changé: %w", entryID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return entryID, fmt.Errorf("l'écriture de règlement %s est passée mais la facture n'est plus comptabilisée", entryID)
	}
	return entryID, nil
}

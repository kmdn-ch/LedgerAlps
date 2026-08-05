package handlers

// Écriture au journal d'une facture fournisseur à sa comptabilisation.
//
// Le statut « comptabilisée » l'annonçait depuis l'origine — le schéma dit
// « booked : posted to the journal, counts for VAT » — mais rien ne l'écrivait.
// La charge n'entrait donc au journal que si quelqu'un la saisissait à la main,
// et la TVA déductible alimentait la déclaration sans contrepartie comptable :
// les livres et la déclaration racontaient deux histoires différentes.
//
// # Ce qui est écrit
//
//	Débit  <compte de charge>   montant hors taxe
//	Débit  2262 TVA déductible  montant de TVA        (omis s'il n'y en a pas)
//	Crédit 2000 Créanciers      montant TTC
//
// C'est le miroir exact de l'émission d'une facture client (invoicing/posting).
//
// # Idempotent par le lien
//
// Une facture déjà rattachée à une écriture n'en produit pas une seconde. Sans
// cela, un aller-retour « comptabilisée → brouillon → comptabilisée » doublerait
// la charge et la TVA déductible.

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Comptes du plan PME suisse utilisés à la comptabilisation d'un achat.
const (
	accountPayables     = "2000" // Créanciers (fournisseurs)
	accountInputVAT     = "2262" // TVA déductible (créance fiscale)
	accountDefaultSpend = "6500" // Charges d'administration — filet, jamais un choix
)

// postSupplierInvoice écrit et comptabilise l'achat, puis lie l'écriture.
//
// Renvoie l'identifiant de l'écriture, ou une chaîne vide quand il n'y a rien à
// écrire (montant nul). Une erreur ici EMPÊCHE la comptabilisation : contrairement
// à l'émission d'une facture client — où le document est déjà parti et où nier
// l'envoi serait pire — rien n'est encore engagé vis-à-vis d'un tiers. Laisser
// passer le statut sans l'écriture recréerait exactement le défaut corrigé ici.
func (h *SupplierInvoicesHandler) postSupplierInvoice(
	ctx context.Context, invoiceID, userID, ipAddress string,
) (string, error) {
	if h.accountingSvc == nil {
		return "", nil
	}

	var (
		existing     string
		reference    string
		issueDate    time.Time
		subtotal     float64
		vat          float64
		total        float64
		expenseCode  string
		supplierName string
	)
	q := db.Rebind(`
		SELECT COALESCE(si.journal_entry_id, ''), si.supplier_reference, si.issue_date,
		       si.subtotal_amount, si.vat_amount, si.total_amount,
		       COALESCE(si.expense_account_code, ''), COALESCE(c.name, '')
		FROM supplier_invoices si
		LEFT JOIN contacts c ON c.id = si.supplier_id
		WHERE si.id = ?`, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, q, invoiceID).Scan(
		&existing, &reference, &issueDate, &subtotal, &vat, &total,
		&expenseCode, &supplierName); err == sql.ErrNoRows {
		return "", fmt.Errorf("facture fournisseur introuvable")
	} else if err != nil {
		return "", fmt.Errorf("lecture de la facture: %w", err)
	}

	if existing != "" {
		return existing, nil // déjà comptabilisée
	}
	if total == 0 {
		return "", nil
	}

	if expenseCode == "" {
		expenseCode = accountDefaultSpend
	}
	expenseID, err := h.accountID(ctx, expenseCode)
	if err != nil {
		return "", err
	}
	payablesID, err := h.accountID(ctx, accountPayables)
	if err != nil {
		return "", err
	}

	lines := []accounting.LineInput{
		{AccountID: expenseID, DebitAmount: ptr(round2(subtotal)),
			Description: "Achat " + reference, Sequence: 0},
	}
	if round2(vat) != 0 {
		vatID, err := h.accountID(ctx, accountInputVAT)
		if err != nil {
			return "", err
		}
		lines = append(lines, accounting.LineInput{
			AccountID: vatID, DebitAmount: ptr(round2(vat)),
			Description: "TVA déductible", Sequence: 1})
	}
	lines = append(lines, accounting.LineInput{
		AccountID: payablesID, CreditAmount: ptr(round2(total)),
		Description: supplierName, Sequence: 2})

	entry, err := h.accountingSvc.CreateEntry(ctx, userID, accounting.CreateEntryRequest{
		Date:        issueDate,
		Description: fmt.Sprintf("Facture fournisseur %s — %s", reference, supplierName),
		Lines:       lines,
	})
	if err != nil {
		return "", fmt.Errorf("création de l'écriture: %w", err)
	}
	if err := h.accountingSvc.PostEntry(ctx, userID, entry.ID, ipAddress); err != nil {
		return "", fmt.Errorf("comptabilisation de l'écriture %s: %w", entry.ID, err)
	}

	// Le lien vient en dernier, comme à l'émission : une écriture non liée reste
	// corrigeable à la main, alors qu'un lien vers une écriture inexistante
	// rendrait la facture impossible à comptabiliser ensuite.
	setQ := db.Rebind(
		`UPDATE supplier_invoices SET journal_entry_id = ?, updated_at = ? WHERE id = ?`,
		h.usePostgres)
	if _, err := h.db.ExecContext(ctx, setQ, entry.ID, time.Now().UTC(), invoiceID); err != nil {
		return entry.ID, fmt.Errorf(
			"l'écriture %s a été passée mais n'a pas pu être liée à la facture: %w", entry.ID, err)
	}
	return entry.ID, nil
}

// accountID résout un numéro de compte, en nommant celui qui manque.
func (h *SupplierInvoicesHandler) accountID(ctx context.Context, number string) (string, error) {
	var id string
	q := db.Rebind(`SELECT id FROM accounts WHERE code = ? AND is_active = 1`, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, q, number).Scan(&id); err == sql.ErrNoRows {
		return "", fmt.Errorf(
			"le compte %s est absent du plan comptable : l'achat ne peut pas être comptabilisé",
			number)
	} else if err != nil {
		return "", fmt.Errorf("lecture du compte %s: %w", number, err)
	}
	return id, nil
}

func ptr(v float64) *float64 { return &v }

func round2(v float64) float64 {
	return float64(int64(v*100+copySign(0.5, v))) / 100
}

func copySign(mag, sign float64) float64 {
	if sign < 0 {
		return -mag
	}
	return mag
}

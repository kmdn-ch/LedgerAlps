package invoicing

// Comptabilisation d'une facture ou d'une note de crédit à son émission.
//
// Jusqu'ici aucun document ne passait d'écriture : seuls les paiements étaient
// automatisés. Une facture envoyée n'apparaissait donc au journal qu'au moment
// de son encaissement, ce qui décale le produit d'un exercice à l'autre et
// contredit le principe de l'exercice auquel l'opération se rapporte
// (CO art. 958b).
//
// # L'ordre était imposé, et c'est pour ça que ce point attendait
//
// La note de crédit ne pouvait pas être comptabilisée en premier : contrepasser
// le produit d'une note alors que la facture n'a jamais été passée créerait un
// produit négatif sans contrepartie. La facture vient donc d'abord, la note
// suit le même mécanisme en sens inverse.
//
// # Pourquoi le réglage est éteint sur les installations existantes
//
// Qui tenait une comptabilité complète saisissait ces écritures à la main.
// Activer l'automatisme d'office doublerait le produit et la TVA due. Le
// réglage arrive donc actif pour les installations neuves et éteint pour les
// autres, qui l'allument quand elles ont vérifié.
//
// # Ce qui est écrit
//
//	Facture            Débit  1100 Débiteurs      total TTC
//	                   Crédit 3200 Produits       hors taxe
//	                   Crédit 2261 TVA due        montant de TVA
//
//	Note de crédit     exactement l'inverse
//
// La ligne de TVA est omise quand il n'y en a pas — une ligne à zéro n'est pas
// une information, c'est du bruit dans le grand livre.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Numéros du plan comptable PME suisse utilisés à l'émission.
const (
	accountReceivables = "1100" // Débiteurs suisses
	accountRevenue     = "3200" // Produits des services
	accountVATDue      = "2261" // TVA due (dette fiscale)
)

// ErrAccountMissing signale un plan comptable amputé. Le message nomme le
// compte : « impossible de comptabiliser » sans dire lequel manque oblige à
// fouiller le plan à la main.
type ErrAccountMissing struct{ Number string }

func (e ErrAccountMissing) Error() string {
	return fmt.Sprintf("le compte %s est absent du plan comptable : la facture ne peut pas être comptabilisée", e.Number)
}

// autoPostEnabled reports whether this installation posts documents on issue.
//
// Une fiche société absente — installation en cours de création — vaut « non » :
// comptabiliser avant que l'entreprise soit décrite produirait des écritures
// que personne n'a demandées.
func (s *Service) autoPostEnabled(ctx context.Context) bool {
	q := db.Rebind(`SELECT COALESCE(auto_post_invoices, 0) FROM company_settings LIMIT 1`, s.usePostgres)
	var on int
	if err := s.db.QueryRowContext(ctx, q).Scan(&on); err != nil {
		return false
	}
	return on == 1
}

// accountIDByNumber resolves a chart-of-accounts number to its identifier.
func (s *Service) accountIDByNumber(ctx context.Context, number string) (string, error) {
	q := db.Rebind(`SELECT id FROM accounts WHERE code = ? AND is_active = 1`, s.usePostgres)
	var id string
	if err := s.db.QueryRowContext(ctx, q, number).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return "", ErrAccountMissing{Number: number}
	} else if err != nil {
		return "", fmt.Errorf("lecture du compte %s: %w", number, err)
	}
	return id, nil
}

// documentAmounts loads what is needed to build the entry.
type documentAmounts struct {
	Subtotal float64
	VAT      float64
	Total    float64
}

func (s *Service) documentAmounts(ctx context.Context, invoiceID string) (documentAmounts, error) {
	q := db.Rebind(`SELECT subtotal_amount, vat_amount, total_amount FROM invoices WHERE id = ?`, s.usePostgres)
	var a documentAmounts
	if err := s.db.QueryRowContext(ctx, q, invoiceID).Scan(&a.Subtotal, &a.VAT, &a.Total); err != nil {
		return a, fmt.Errorf("lecture des montants: %w", err)
	}
	return a, nil
}

// PostIssuedDocument comptabilise le document et lie l'écriture à la facture.
//
// Idempotent par le lien : un document déjà rattaché à une écriture n'en produit
// pas une seconde. C'est ce qui permet d'appeler cette fonction depuis la
// transition de statut sans craindre qu'un double clic double le produit.
func (s *Service) PostIssuedDocument(
	ctx context.Context, invoiceID, documentType, invoiceNumber, userID string,
	issueDate time.Time, ipAddress string,
) (string, error) {
	if s.accountingSvc == nil {
		return "", nil
	}
	if documentType == DocumentTypeQuote {
		// Une offre n'est pas une opération : personne ne doit rien tant qu'elle
		// n'est pas acceptée.
		return "", nil
	}

	// Déjà comptabilisé ? Le lien fait foi.
	linkQ := db.Rebind(`SELECT COALESCE(journal_entry_id, '') FROM invoices WHERE id = ?`, s.usePostgres)
	var existing string
	if err := s.db.QueryRowContext(ctx, linkQ, invoiceID).Scan(&existing); err != nil {
		return "", fmt.Errorf("lecture du lien comptable: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	amounts, err := s.documentAmounts(ctx, invoiceID)
	if err != nil {
		return "", err
	}
	if amounts.Total == 0 {
		// Un document à zéro n'a pas de contrepartie à écrire. Ce n'est pas une
		// erreur — une facture de régularisation à zéro existe — mais il n'y a
		// rien à porter au journal.
		return "", nil
	}

	arID, err := s.accountIDByNumber(ctx, accountReceivables)
	if err != nil {
		return "", err
	}
	revID, err := s.accountIDByNumber(ctx, accountRevenue)
	if err != nil {
		return "", err
	}

	credit := documentType == DocumentTypeCreditNote
	lines, err := s.buildIssueLines(ctx, credit, amounts, arID, revID, invoiceNumber)
	if err != nil {
		return "", err
	}

	label := "Facture"
	if credit {
		label = "Note de crédit"
	}
	entry, err := s.accountingSvc.CreateEntry(ctx, userID, accounting.CreateEntryRequest{
		Date:        issueDate,
		Description: fmt.Sprintf("%s %s — émission", label, invoiceNumber),
		Lines:       lines,
	})
	if err != nil {
		return "", fmt.Errorf("création de l'écriture: %w", err)
	}
	if err := s.accountingSvc.PostEntry(ctx, userID, entry.ID, ipAddress); err != nil {
		return "", fmt.Errorf("comptabilisation de l'écriture %s: %w", entry.ID, err)
	}

	// Le lien vient en dernier : une écriture non liée reste corrigeable à la
	// main, alors qu'un lien vers une écriture inexistante rendrait le document
	// impossible à comptabiliser ensuite.
	setQ := db.Rebind(`UPDATE invoices SET journal_entry_id = ?, updated_at = ? WHERE id = ?`, s.usePostgres)
	if _, err := s.db.ExecContext(ctx, setQ, entry.ID, time.Now().UTC(), invoiceID); err != nil {
		return entry.ID, fmt.Errorf("l'écriture %s a été passée mais n'a pas pu être liée au document: %w", entry.ID, err)
	}
	return entry.ID, nil
}

// buildIssueLines assembles the debit/credit lines for one issued document.
func (s *Service) buildIssueLines(
	ctx context.Context, credit bool, a documentAmounts, arID, revID, number string,
) ([]accounting.LineInput, error) {
	total, subtotal, vat := a.Total, a.Subtotal, a.VAT

	arLine := accounting.LineInput{
		AccountID:   arID,
		Description: fmt.Sprintf("Créance client — %s", number),
		Sequence:    1,
	}
	revLine := accounting.LineInput{
		AccountID:   revID,
		Description: fmt.Sprintf("Produit — %s", number),
		Sequence:    2,
	}
	if credit {
		arLine.CreditAmount = &total
		revLine.DebitAmount = &subtotal
	} else {
		arLine.DebitAmount = &total
		revLine.CreditAmount = &subtotal
	}
	lines := []accounting.LineInput{arLine, revLine}

	// La ligne de TVA n'existe que s'il y a de la TVA. Une ligne à zéro
	// encombrerait le grand livre sans rien apprendre.
	if vat != 0 {
		vatID, err := s.accountIDByNumber(ctx, accountVATDue)
		if err != nil {
			return nil, err
		}
		vatLine := accounting.LineInput{
			AccountID:   vatID,
			Description: fmt.Sprintf("TVA due — %s", number),
			Sequence:    3,
		}
		if credit {
			vatLine.DebitAmount = &vat
		} else {
			vatLine.CreditAmount = &vat
		}
		lines = append(lines, vatLine)
	}
	return lines, nil
}

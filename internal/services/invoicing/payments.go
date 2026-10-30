package invoicing

// Encaissements : un paiement, qu'il solde la facture ou non, entre dans les
// livres.
//
// # Ce qui était faux jusqu'à la 1.6.4
//
// Deux chemins existaient, et aucun ne tenait la comptabilité :
//
//   - « Marquer payée » changeait le statut et rien d'autre, alors que l'écran
//     promettait « passé au journal (banque / débiteurs) ». La créance de la
//     facture n'était jamais soldée au grand livre.
//   - POST /payments ne passait d'écriture que si le versement SEUL couvrait la
//     facture. Deux versements de 600 et 400 sur 1000 ne laissaient aucune
//     écriture, aucun cumul, et une facture « envoyée » à vie. L'interface,
//     elle, n'appelait jamais cette route.
//
// # Ce que fait RecordPayment
//
// Un seul chemin pour les deux. Chaque versement :
//
//  1. est plafonné au solde restant (un trop-perçu n'est pas une créance soldée) ;
//  2. passe Débit banque (ou caisse) / Crédit 1100, ou l'inverse pour une note
//     de crédit — un remboursement —, SI l'émission de la facture a été
//     comptabilisée. Sinon la comptabilité de cette installation est tenue à la
//     main, et passer l'encaissement seul porterait 1100 au crédit sans le
//     débit correspondant ;
//  3. s'ajoute à `amount_paid` ;
//  4. solde la facture quand le cumul atteint le total.
//
// L'écriture est passée AVANT l'enregistrement du paiement : si elle échoue
// (exercice clos, compte absent), rien n'a changé et l'erreur est franche.
// L'inverse laissait un paiement sans écriture, c'est-à-dire exactement le
// défaut décrit plus haut.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	accsvc "github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

var (
	ErrInvoiceNotPayable = fmt.Errorf("seule une facture envoyée accepte un paiement")
	// Un encaissement qui dépasse le solde ne solde pas « un peu plus » : il
	// laisse un trop-perçu que ni la facture ni le journal ne savent porter.
	ErrPaymentExceedsBalance = fmt.Errorf("le paiement dépasse le solde restant de la facture")
	// Annuler une facture encaissée en tout ou partie extournerait l'écriture
	// d'émission en laissant les encaissements, et 1100 ne se réconcilierait plus.
	// La correction prévue est la note de crédit.
	ErrInvoiceHasPayments = fmt.Errorf("une facture déjà encaissée, en tout ou partie, ne s'annule pas : émettez une note de crédit")
)

// ErrPaymentAccountMissing nomme le compte absent, comme ErrAccountMissing, avec
// la bonne conséquence : c'est l'encaissement qui ne peut pas être passé.
type ErrPaymentAccountMissing struct{ Number string }

func (e ErrPaymentAccountMissing) Error() string {
	return fmt.Sprintf("le compte %s est absent du plan comptable : l'encaissement ne peut pas être comptabilisé", e.Number)
}

// Comptes de trésorerie du plan comptable PME suisse.
const (
	accountCash = "1000" // Caisse
	accountBank = "1020" // Banque (compte courant)
)

// PaymentRequest décrit un encaissement.
type PaymentRequest struct {
	InvoiceID string
	Amount    float64
	Date      time.Time
	Method    string  // bank_transfer | cash | card | check | other
	Reference *string // libre
	// BankAccountID force le compte débité. Vide : la caisse pour des espèces, la
	// banque pour le reste.
	BankAccountID string
}

// PaymentResult dit ce que l'encaissement a changé.
type PaymentResult struct {
	Payment        models.Payment
	JournalEntryID string // vide quand l'émission n'était pas comptabilisée
	Status         models.InvoiceStatus
	AmountPaid     float64
	Balance        float64
}

// payMu sérialise les encaissements de TOUT le processus. Il est global parce
// que deux gestionnaires construisent chacun leur Service (factures et
// paiements) : un verrou porté par l'instance ne les ferait pas s'attendre.
var payMu sync.Mutex

// cents arrondit à la pièce de 1 centime. Un paiement est de l'argent réel : la
// règle des 5 centimes ne vaut que pour les montants facturés.
func cents(v float64) float64 { return math.Round(v*100) / 100 }

// soldeTolerance absorbe l'erreur de représentation des flottants, pas un
// centime : 0.005 est la moitié de la plus petite unité.
const soldeTolerance = 0.005

// RecordPayment enregistre un encaissement : voir l'en-tête du fichier.
func (s *Service) RecordPayment(ctx context.Context, actor Actor, req PaymentRequest) (*PaymentResult, error) {
	// Un seul encaissement à la fois dans le processus : la lecture du solde, le
	// plafonnement et l'écriture forment une suite qu'un second appel ne doit pas
	// croiser. SQLite sérialise les écritures, mais pas cette suite.
	payMu.Lock()
	defer payMu.Unlock()

	amount := cents(req.Amount)
	if amount <= 0 {
		return nil, fmt.Errorf("le montant d'un paiement doit être positif")
	}

	var inv struct {
		status, number, docType, journalEntryID, createdByID string
		total, paid                                          float64
	}
	q := db.Rebind(`
		SELECT status, invoice_number, COALESCE(document_type, 'invoice'),
		       COALESCE(journal_entry_id, ''), created_by_id, total_amount, amount_paid
		FROM invoices WHERE id = ?`, s.usePostgres)
	if err := s.db.QueryRowContext(ctx, q, req.InvoiceID).Scan(
		&inv.status, &inv.number, &inv.docType, &inv.journalEntryID, &inv.createdByID, &inv.total, &inv.paid,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load invoice: %w", err)
	}
	if inv.docType == DocumentTypeQuote || models.InvoiceStatus(inv.status) != models.InvoiceStatusSent {
		return nil, ErrInvoiceNotPayable
	}

	balance := cents(inv.total - inv.paid)
	if amount > balance+soldeTolerance {
		return nil, ErrPaymentExceedsBalance
	}

	// Qui agit. Sans auteur connu (appel interne, test), la pièce reste
	// rattachée à celui qui a créé la facture : created_by_id est obligatoire.
	userID := actor.UserID
	if userID == "" {
		userID = inv.createdByID
	}

	paymentID := db.NewID()
	date := req.Date
	if date.IsZero() {
		date = time.Now()
	}

	// L'écriture d'abord (voir l'en-tête).
	var entryID string
	if inv.journalEntryID != "" && s.accountingSvc != nil {
		bankID, err := s.paymentBankAccount(ctx, req)
		if err != nil {
			return nil, err
		}
		arID, err := s.accountIDByNumber(ctx, accountReceivables)
		if err != nil {
			return nil, ErrPaymentAccountMissing{Number: accountReceivables}
		}
		entryID, err = s.postPaymentEntry(ctx, userID, actor.IP, inv.docType == DocumentTypeCreditNote,
			amount, date, inv.number, paymentID, req, bankID, arID)
		if err != nil {
			return nil, err
		}
	}

	newPaid := cents(inv.paid + amount)
	settled := newPaid >= cents(inv.total)-soldeTolerance
	status := models.InvoiceStatusSent
	if settled {
		status = models.InvoiceStatusPaid
		newPaid = cents(inv.total) // pas de dérive : un solde exact est le total
	}

	now := time.Now().UTC()
	if err := s.storePayment(ctx, paymentID, req, amount, date, userID, entryID, newPaid, status, now); err != nil {
		// L'écriture est passée, et un journal comptabilisé ne se défait pas : on
		// la contrepasse, pour que les livres ne portent pas un encaissement que
		// la facture ignore.
		if entryID != "" {
			if revErr := s.createReversalEntry(ctx, userID, inv.number, entryID, date); revErr != nil {
				log.Printf("ERROR: encaissement %s : écriture %s passée, paiement non enregistré (%v) et contrepassation impossible (%v) — à corriger au journal",
					paymentID, entryID, err, revErr)
			}
		}
		return nil, fmt.Errorf("enregistrement du paiement: %w", err)
	}

	// Les traces viennent après : voir audit.go sur ce que cela garantit.
	s.recordOn(ctx, actor, accsvc.TablePayments, accsvc.ActionPaymentRecorded, paymentID,
		accsvc.Creation(map[string]any{
			"invoice_id":   req.InvoiceID,
			"amount":       amount,
			"payment_date": date.Format("2006-01-02"),
			"method":       req.Method,
			"reference":    req.Reference,
		}))
	if settled {
		s.record(ctx, actor, accsvc.ActionDocumentTransition, req.InvoiceID,
			accsvc.Modification(
				map[string]any{"document_type": inv.docType, "number": inv.number, "status": string(models.InvoiceStatusSent)},
				map[string]any{"document_type": inv.docType, "number": inv.number, "status": string(models.InvoiceStatusPaid)},
			))
	}

	res := &PaymentResult{
		Payment: models.Payment{
			ID: paymentID, InvoiceID: req.InvoiceID, Amount: amount, PaymentDate: date,
			Method: models.PaymentMethod(req.Method), Reference: req.Reference,
			CreatedByID: userID, CreatedAt: now, UpdatedAt: now,
		},
		JournalEntryID: entryID,
		Status:         status,
		AmountPaid:     newPaid,
		Balance:        cents(inv.total - newPaid),
	}
	if entryID != "" {
		res.Payment.JournalEntryID = &entryID
	}
	return res, nil
}

// paymentBankAccount résout le compte de trésorerie débité.
func (s *Service) paymentBankAccount(ctx context.Context, req PaymentRequest) (string, error) {
	if req.BankAccountID != "" {
		q := db.Rebind(`SELECT id FROM accounts WHERE id = ? AND account_type = 'asset' AND is_active = 1`, s.usePostgres)
		var id string
		if err := s.db.QueryRowContext(ctx, q, req.BankAccountID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("le compte bancaire indiqué est introuvable ou n'est pas un compte d'actif actif")
		} else if err != nil {
			return "", fmt.Errorf("lecture du compte: %w", err)
		}
		return id, nil
	}
	code := accountBank
	if req.Method == "cash" {
		code = accountCash
	}
	id, err := s.accountIDByNumber(ctx, code)
	if err != nil {
		var missing ErrAccountMissing
		if errors.As(err, &missing) {
			return "", ErrPaymentAccountMissing{Number: missing.Number}
		}
		return "", err
	}
	return id, nil
}

// postPaymentEntry crée et comptabilise l'écriture d'encaissement.
//
//	Facture         Débit trésorerie  / Crédit 1100 Débiteurs
//	Note de crédit  Débit 1100        / Crédit trésorerie        (remboursement)
func (s *Service) postPaymentEntry(
	ctx context.Context, userID, ip string, refund bool, amount float64, date time.Time,
	number, paymentID string, req PaymentRequest, bankID, arID string,
) (string, error) {
	debit, credit := amount, amount
	bankLine := accsvc.LineInput{AccountID: bankID, Sequence: 1}
	arLine := accsvc.LineInput{AccountID: arID, Sequence: 2}
	label := "Encaissement facture"
	if refund {
		label = "Remboursement note de crédit"
		arLine.DebitAmount, bankLine.CreditAmount = &debit, &credit
		arLine.Sequence, bankLine.Sequence = 1, 2
	} else {
		bankLine.DebitAmount, arLine.CreditAmount = &debit, &credit
	}
	bankLine.Description = fmt.Sprintf("%s — %s %s", label, req.Method, ptrString(req.Reference))
	arLine.Description = fmt.Sprintf("Créance client soldée — %s", number)

	entry, err := s.accountingSvc.CreateEntry(ctx, userID, accsvc.CreateEntryRequest{
		Date:        date,
		Description: fmt.Sprintf("%s %s (paiement %s)", label, number, paymentID),
		Lines:       []accsvc.LineInput{bankLine, arLine},
	})
	if err != nil {
		return "", fmt.Errorf("création de l'écriture d'encaissement: %w", err)
	}
	if err := s.accountingSvc.PostEntry(ctx, userID, entry.ID, ip); err != nil {
		return "", fmt.Errorf("comptabilisation de l'écriture %s: %w", entry.ID, err)
	}
	return entry.ID, nil
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// storePayment écrit le paiement et met la facture à jour dans UNE transaction.
func (s *Service) storePayment(
	ctx context.Context, paymentID string, req PaymentRequest, amount float64, date time.Time,
	userID, entryID string, newPaid float64, status models.InvoiceStatus, now time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var entry any
	if entryID != "" {
		entry = entryID
	}
	insQ := db.Rebind(`
		INSERT INTO payments (id, invoice_id, amount, payment_date, method, reference, journal_entry_id, created_by_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insQ, paymentID, req.InvoiceID, amount, date.Format("2006-01-02"),
		req.Method, req.Reference, entry, userID, now, now); err != nil {
		return err
	}
	updQ := db.Rebind(`UPDATE invoices SET amount_paid = ?, status = ?, updated_at = ? WHERE id = ?`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, updQ, newPaid, string(status), now, req.InvoiceID); err != nil {
		return err
	}
	return tx.Commit()
}

// settleBalance solde une facture « envoyée » à la date du jour : c'est ce que
// fait « Marquer payée ». Le solde restant est encaissé par virement.
//
// Rendu : false quand il n'y avait rien à encaisser (facture à zéro, ou déjà
// couverte) — l'appelant se contente alors de changer le statut.
func (s *Service) settleBalance(ctx context.Context, actor Actor, invoiceID string) (bool, error) {
	var total, paid float64
	q := db.Rebind(`SELECT total_amount, amount_paid FROM invoices WHERE id = ?`, s.usePostgres)
	if err := s.db.QueryRowContext(ctx, q, invoiceID).Scan(&total, &paid); err != nil {
		return false, fmt.Errorf("load invoice: %w", err)
	}
	balance := cents(total - paid)
	if balance <= soldeTolerance {
		return false, nil
	}
	_, err := s.RecordPayment(ctx, actor, PaymentRequest{
		InvoiceID: invoiceID,
		Amount:    balance,
		Date:      time.Now(),
		Method:    "bank_transfer",
	})
	return true, err
}

// Package banking rapproche les écritures d'un relevé bancaire des factures.
//
// L'import camt.053 existait déjà et ne gardait rien : le relevé était analysé,
// renvoyé au navigateur, puis oublié. On ne pouvait donc pas savoir ce qui avait
// déjà été traité, et réimporter le relevé du mois obligeait à tout revoir.
//
// # Ce que ce paquet fait, et ce qu'il ne fait pas
//
// Il conserve les écritures, propose des rapprochements et enregistre la
// décision de l'utilisateur. Il ne crée AUCUN paiement et ne touche pas au
// journal : c'est le chemin de paiement existant, déjà éprouvé, qui s'en charge
// quand l'utilisateur confirme depuis la facture.
//
// La séparation est délibérée. Une suggestion n'est pas un rapprochement. Un
// logiciel qui solderait des factures parce qu'un montant correspond ferait
// passer pour réglées des créances que personne n'a vérifiées — et c'est
// exactement le genre d'erreur qu'on ne découvre qu'en relançant un client qui
// a déjà payé, ou en ne relançant jamais celui qui n'a pas payé.

package banking

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/iso20022"
)

type Service struct {
	db          *sql.DB
	usePostgres bool
}

func New(database *sql.DB, usePostgres bool) *Service {
	return &Service{db: database, usePostgres: usePostgres}
}

// fingerprint identifie une opération bancaire de façon stable.
//
// La référence bancaire seule ne suffit pas : toutes les banques ne
// renseignent pas AcctSvcrRef. Le montant et la date seuls ne suffisent pas non
// plus : deux versements identiques du même client le même jour existent, et
// les fondre en un seul ferait disparaître un encaissement. La combinaison de
// tout ce qui identifie l'opération est le compromis qui ne perd rien et ne
// duplique pas.
func fingerprint(e iso20022.BankEntry) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%.2f|%s|%t|%s|%s|%s",
		e.BookingDate.Format("2006-01-02"), e.Amount, e.Currency, e.IsCredit,
		strings.TrimSpace(e.BankRef), strings.TrimSpace(e.EndToEndRef),
		strings.TrimSpace(e.QRReference))
	return hex.EncodeToString(h.Sum(nil))
}

// ImportResult décrit ce qu'un import a produit, pour que l'interface le dise
// au lieu d'afficher un succès muet.
type ImportResult struct {
	Imported  int `json:"imported"`
	Duplicate int `json:"duplicate"`
}

// Import conserve les écritures d'un relevé. Les doublons sont comptés, pas
// écrits : réimporter le relevé du mois est une opération courante, et elle ne
// doit rien casser.
func (s *Service) Import(ctx context.Context, entries []iso20022.BankEntry) (ImportResult, error) {
	var res ImportResult
	q := db.Rebind(`
		INSERT INTO bank_entries
		  (fingerprint, amount, currency, is_credit, booking_date, value_date,
		   bank_ref, end_to_end_ref, qr_reference, counterparty, remittance)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, s.usePostgres)

	for _, e := range entries {
		var valueDate any
		if !e.ValueDate.IsZero() {
			valueDate = e.ValueDate
		}
		_, err := s.db.ExecContext(ctx, q,
			fingerprint(e), e.Amount, currencyOr(e.Currency), boolInt(e.IsCredit),
			e.BookingDate, valueDate,
			e.BankRef, e.EndToEndRef, e.QRReference, counterpartyOf(e), remittanceOf(e))
		if err != nil {
			if isUnique(err) {
				res.Duplicate++
				continue
			}
			return res, fmt.Errorf("enregistrement d'une écriture bancaire: %w", err)
		}
		res.Imported++
	}
	return res, nil
}

// Entry est une écriture telle que l'interface la montre, avec sa suggestion.
type Entry struct {
	ID           string    `json:"id"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	IsCredit     bool      `json:"is_credit"`
	BookingDate  time.Time `json:"booking_date"`
	QRReference  string    `json:"qr_reference,omitempty"`
	Counterparty string    `json:"counterparty,omitempty"`
	Remittance   string    `json:"remittance,omitempty"`
	Ignored      bool      `json:"ignored"`

	// Rapprochement confirmé.
	InvoiceID     *string `json:"invoice_id,omitempty"`
	InvoiceNumber string  `json:"invoice_number,omitempty"`

	// Suggestion, quand rien n'est encore confirmé.
	Suggestion *Suggestion `json:"suggestion,omitempty"`
}

// Suggestion propose une facture, avec la raison qui l'a désignée.
//
// La raison compte autant que la proposition : « même montant » et « référence
// QR du bulletin » n'engagent pas la même confiance, et l'utilisateur doit
// pouvoir en tenir compte sans ouvrir la facture.
type Suggestion struct {
	InvoiceID     string  `json:"invoice_id"`
	InvoiceNumber string  `json:"invoice_number"`
	ContactName   string  `json:"contact_name"`
	TotalAmount   float64 `json:"total_amount"`
	// Confidence : "certaine" | "probable" | "possible".
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

// List rend les écritures, la plus récente d'abord, avec leurs suggestions.
func (s *Service) List(ctx context.Context, includeSettled bool) ([]Entry, error) {
	where := "WHERE be.ignored = 0 AND be.invoice_id IS NULL"
	if includeSettled {
		where = ""
	}
	q := db.Rebind(`
		SELECT be.id, be.amount, be.currency, be.is_credit, be.booking_date,
		       be.qr_reference, be.counterparty, be.remittance, be.ignored,
		       be.invoice_id, COALESCE(i.invoice_number, '')
		FROM bank_entries be
		LEFT JOIN invoices i ON i.id = be.invoice_id
		`+where+`
		ORDER BY be.booking_date DESC, be.id`, s.usePostgres)

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("lecture des écritures bancaires: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var isCredit, ignored int
		if err := rows.Scan(&e.ID, &e.Amount, &e.Currency, &isCredit, &e.BookingDate,
			&e.QRReference, &e.Counterparty, &e.Remittance, &ignored,
			&e.InvoiceID, &e.InvoiceNumber); err != nil {
			return nil, err
		}
		e.IsCredit = isCredit == 1
		e.Ignored = ignored == 1
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Les suggestions se calculent après la lecture : les faire pendant
	// laisserait un curseur ouvert le temps de plusieurs requêtes, ce que
	// SQLite supporte mal.
	for i := range out {
		if out[i].InvoiceID != nil || !out[i].IsCredit {
			continue // déjà tranché, ou sortie d'argent : rien à encaisser
		}
		sug, err := s.suggest(ctx, out[i])
		if err != nil {
			return nil, err
		}
		out[i].Suggestion = sug
	}
	return out, nil
}

// suggest désigne au plus une facture, avec la raison qui l'a désignée.
//
// L'ordre des règles suit la confiance qu'on peut leur accorder, et il s'arrête
// à la première qui répond. Proposer plusieurs candidats ferait porter à
// l'utilisateur un arbitrage que le logiciel a les moyens de trancher pour les
// cas nets, et ne l'aiderait pas pour les autres.
func (s *Service) suggest(ctx context.Context, e Entry) (*Suggestion, error) {
	// 1. La référence QR. Elle est portée par le bulletin, recopiée par la
	//    banque : c'est une correspondance, pas une ressemblance.
	if ref := normaliseRef(e.QRReference); ref != "" {
		if sug, err := s.findByReference(ctx, ref); err != nil || sug != nil {
			if sug != nil {
				sug.Confidence = "certaine"
				sug.Reason = "référence du bulletin de versement"
			}
			return sug, err
		}
	}

	// 2. Le montant exact sur une facture envoyée et non soldée. Fréquent, et
	//    fiable tant qu'un seul candidat répond — d'où le refus de proposer
	//    quand il y en a plusieurs.
	sug, count, err := s.findByAmount(ctx, e.Amount)
	if err != nil {
		return nil, err
	}
	if count == 1 && sug != nil {
		sug.Confidence = "probable"
		sug.Reason = "montant exact, une seule facture ouverte correspond"
		return sug, nil
	}
	if count > 1 {
		// Plusieurs factures au même montant : désigner la première serait un
		// tirage au sort présenté comme une analyse.
		return nil, nil
	}
	return nil, nil
}

func (s *Service) findByReference(ctx context.Context, ref string) (*Suggestion, error) {
	q := db.Rebind(`
		SELECT i.id, i.invoice_number, COALESCE(c.name,''), i.total_amount
		FROM invoices i
		LEFT JOIN contacts c ON c.id = i.contact_id
		WHERE REPLACE(COALESCE(i.qr_reference,''), ' ', '') = ?
		LIMIT 1`, s.usePostgres)
	var sug Suggestion
	err := s.db.QueryRowContext(ctx, q, ref).Scan(
		&sug.InvoiceID, &sug.InvoiceNumber, &sug.ContactName, &sug.TotalAmount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("recherche par référence: %w", err)
	}
	return &sug, nil
}

func (s *Service) findByAmount(ctx context.Context, amount float64) (*Suggestion, int, error) {
	// La tolérance d'un centime absorbe l'arrondi du flottant, pas une
	// différence réelle : au-delà, ce n'est plus la même facture.
	q := db.Rebind(`
		SELECT i.id, i.invoice_number, COALESCE(c.name,''), i.total_amount
		FROM invoices i
		LEFT JOIN contacts c ON c.id = i.contact_id
		WHERE i.status = 'sent'
		  AND COALESCE(i.document_type,'invoice') = 'invoice'
		  AND ABS(i.total_amount - ?) < 0.011
		LIMIT 5`, s.usePostgres)
	rows, err := s.db.QueryContext(ctx, q, amount)
	if err != nil {
		return nil, 0, fmt.Errorf("recherche par montant: %w", err)
	}
	defer rows.Close()

	var first *Suggestion
	count := 0
	for rows.Next() {
		var sug Suggestion
		if err := rows.Scan(&sug.InvoiceID, &sug.InvoiceNumber, &sug.ContactName, &sug.TotalAmount); err != nil {
			return nil, 0, err
		}
		count++
		if first == nil {
			s := sug
			first = &s
		}
	}
	return first, count, rows.Err()
}

// Match enregistre la décision de l'utilisateur : cette écriture correspond à
// cette facture.
//
// Il n'en découle AUCUN paiement ni écriture au journal. Le rapprochement dit
// « j'ai identifié ce versement » ; encaisser reste un geste distinct, fait
// depuis la facture, par le chemin déjà éprouvé.
func (s *Service) Match(ctx context.Context, entryID, invoiceID, userID string) error {
	q := db.Rebind(`
		UPDATE bank_entries
		SET invoice_id = ?, matched_at = ?, matched_by_id = ?, ignored = 0, updated_at = ?
		WHERE id = ?`, s.usePostgres)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, q, invoiceID, now, userID, now, entryID)
	if err != nil {
		return fmt.Errorf("rapprochement: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("écriture bancaire introuvable")
	}
	return nil
}

// Unmatch défait un rapprochement. Se tromper de facture arrive, et une
// décision qu'on ne peut pas reprendre pousse à ne pas la prendre.
func (s *Service) Unmatch(ctx context.Context, entryID string) error {
	q := db.Rebind(`
		UPDATE bank_entries
		SET invoice_id = NULL, matched_at = NULL, matched_by_id = NULL, updated_at = ?
		WHERE id = ?`, s.usePostgres)
	_, err := s.db.ExecContext(ctx, q, time.Now().UTC(), entryID)
	return err
}

// Ignore écarte une écriture qui ne concerne aucune facture — frais bancaires,
// virement interne.
//
// Distinct de « pas encore regardé » : sans cette distinction la liste ne se
// vide jamais, et une liste qui ne se vide jamais cesse d'être lue.
func (s *Service) Ignore(ctx context.Context, entryID string, ignored bool) error {
	q := db.Rebind(`UPDATE bank_entries SET ignored = ?, updated_at = ? WHERE id = ?`, s.usePostgres)
	_, err := s.db.ExecContext(ctx, q, boolInt(ignored), time.Now().UTC(), entryID)
	return err
}

// ── petites aides ───────────────────────────────────────────────────────────

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func currencyOr(c string) string {
	if c == "" {
		return "CHF"
	}
	return c
}

func normaliseRef(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), " ", "") }

func isUnique(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique")
}

func counterpartyOf(e iso20022.BankEntry) string { return strings.TrimSpace(e.CounterpartName) }
func remittanceOf(e iso20022.BankEntry) string   { return strings.TrimSpace(e.Unstructured) }

package accounting

// Réparer une clôture scellée déséquilibrée.
//
// # Le défaut, et pourquoi une écriture ordinaire n'y peut rien
//
// Avant la 1.6.6, la clôture d'un exercice ignorait les produits à solde DÉBITEUR (rabais,
// avoirs) et les charges à solde CRÉDITEUR (ristournes) : elles étaient sommées dans le résultat
// sans recevoir de ligne. L'écriture scellée faisait par exemple 100 000 au débit pour 97 000
// au crédit, la balance de vérification ne s'équilibrait plus, et ces comptes restaient ouverts
// sur l'exercice suivant (audit croisé Gemini, CRIT-04).
//
// Le journal est immuable, et une écriture rectificative ORDINAIRE est équilibrée : elle ne
// rétablit pas l'équilibre d'une écriture qui ne l'est pas. Extourner la clôture ne fait pas
// mieux — l'extourne d'une écriture déséquilibrée l'est aussi, et ne passe pas la validation.
//
// # Ce que fait cette réparation
//
// Elle passe le COMPLÉMENT de la clôture : une seule écriture, datée de la fin de l'exercice,
// portant exactement les lignes qui manquaient — celles qui soldent les comptes restés ouverts.
// Le compte de résultat n'y figure pas : sa ligne, dans la clôture d'origine, comptait déjà tout.
// Le complément est donc déséquilibré, du MÊME écart et en sens inverse ; les deux écritures
// ensemble s'équilibrent, et la balance de vérification aussi.
//
// Rien n'est modifié ni supprimé : la clôture d'origine reste telle quelle, le complément s'y
// ajoute, chaînée au journal d'audit comme toute écriture, avec le condensé de ses lignes. La
// réparation est explicite (un administrateur la demande, après avoir vu ce qu'elle va passer),
// et elle ne s'applique que si l'écart s'explique EXACTEMENT par les comptes restés ouverts —
// sinon elle refuse, et renvoie à la fiduciaire.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ComplementSuffix termine la référence du complément : « CLOTURE-2024 » → « CLOTURE-2024-C ».
// Il porte le préfixe des clôtures, donc les rapports l'excluent comme la clôture elle-même.
const ComplementSuffix = "-C"

// Refus de la réparation. Phrases FIXES : le catalogue les rend par correspondance exacte.
var (
	ErrReparationExerciceOuvert = errors.New(
		"cet exercice n'est pas clos : il n'y a rien à réparer")
	ErrReparationSansCloture = errors.New(
		"aucune écriture de clôture n'a été trouvée pour cet exercice")
	ErrReparationClotureEquilibree = errors.New(
		"la clôture de cet exercice s'équilibre : il n'y a rien à réparer")
	ErrReparationDejaFaite = errors.New(
		"cette clôture a déjà été complétée")
	ErrReparationInexpliquee = errors.New(
		"l'écart de cette clôture ne s'explique pas par des comptes de produits ou de charges restés ouverts : " +
			"la réparation automatique est refusée, faites examiner l'écriture par votre fiduciaire")
)

// ClosingRepairLine est une ligne du complément, lisible par la personne qui doit la valider.
type ClosingRepairLine struct {
	AccountCode string  `json:"account_code"`
	AccountName string  `json:"account_name"`
	Debit       float64 `json:"debit"`
	Credit      float64 `json:"credit"`
}

// ClosingRepairPlan est ce que la réparation passerait — ou a passé.
type ClosingRepairPlan struct {
	FiscalYearID     string              `json:"fiscal_year_id"`
	FiscalYear       string              `json:"fiscal_year"`
	ClosingReference string              `json:"closing_reference"`
	ClosingDebit     float64             `json:"closing_debit"`
	ClosingCredit    float64             `json:"closing_credit"`
	Gap              float64             `json:"gap"` // débit − crédit de la clôture d'origine
	Reference        string              `json:"reference"`
	Date             string              `json:"date"`
	Lines            []ClosingRepairLine `json:"lines"`
	EntryID          string              `json:"entry_id,omitempty"` // renseigné une fois passée
}

// PlanClosingRepair dit ce que la réparation passerait, sans rien écrire.
func (s *FiscalYearService) PlanClosingRepair(ctx context.Context, fiscalYearID string) (*ClosingRepairPlan, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	plan, _, err := s.computeRepair(ctx, tx, fiscalYearID)
	return plan, err
}

// RepairClosing passe le complément de la clôture. Une seule transaction : le complément, son
// condensé et son maillon d'audit entrent ensemble ou pas du tout.
func (s *FiscalYearService) RepairClosing(ctx context.Context, fiscalYearID, userID string) (*ClosingRepairPlan, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	plan, lines, err := s.computeRepair(ctx, tx, fiscalYearID)
	if err != nil {
		return nil, err
	}

	entryID := db.NewID()
	insertEntry := db.Rebind(`
		INSERT INTO journal_entries (id, reference, date, description, status, fiscal_year_id, created_by_id)
		VALUES (?, ?, ?, ?, 'draft', ?, ?)`, s.usePostgres)
	desc := fmt.Sprintf("Complément de la clôture %s : comptes de produits ou de charges à solde inversé "+
		"que la clôture d'origine n'avait pas soldés", plan.FiscalYear)
	if _, err := tx.ExecContext(ctx, insertEntry, entryID, plan.Reference, plan.Date, desc, fiscalYearID, userID); err != nil {
		return nil, fmt.Errorf("insert closing complement: %w", err)
	}
	insertLine := db.Rebind(`
		INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount, description, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for _, l := range lines {
		if _, err := tx.ExecContext(ctx, insertLine, db.NewID(), entryID, l.accountID,
			l.debitAmount, l.creditAmount, l.description, l.sequence); err != nil {
			return nil, fmt.Errorf("insert complement line: %w", err)
		}
	}

	digest, err := LinesDigest(ctx, tx, s.usePostgres, entryID)
	if err != nil {
		return nil, err
	}
	chainedHash, now, err := AppendAuditEntry(ctx, tx, s.usePostgres,
		userID, "close_repair", entryID, "",
		map[string]any{
			"entry_id":          entryID,
			"status":            "posted",
			"fiscal_year":       plan.FiscalYear,
			"repairs":           plan.ClosingReference,
			"closing_gap":       plan.Gap,
			"complement_debit":  sumLines(plan.Lines, true),
			"complement_credit": sumLines(plan.Lines, false),
			"lines_digest":      digest,
		})
	if err != nil {
		return nil, fmt.Errorf("chain closing complement: %w", err)
	}
	postQ := db.Rebind(
		`UPDATE journal_entries SET status = 'posted', integrity_hash = ?, updated_at = ? WHERE id = ?`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, postQ, chainedHash, now, entryID); err != nil {
		return nil, fmt.Errorf("post closing complement: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	plan.EntryID = entryID
	return plan, nil
}

func sumLines(lines []ClosingRepairLine, debit bool) float64 {
	var c int64
	for _, l := range lines {
		if debit {
			c += compliance.Cents(l.Debit)
		} else {
			c += compliance.Cents(l.Credit)
		}
	}
	return float64(c) / 100
}

// computeRepair établit le complément et vérifie qu'il explique l'écart EXACTEMENT.
func (s *FiscalYearService) computeRepair(ctx context.Context, tx *sql.Tx, fiscalYearID string) (*ClosingRepairPlan, []closingLine, error) {
	var (
		name    string
		endDate time.Time
		closed  int
	)
	if err := tx.QueryRowContext(ctx, db.Rebind(
		`SELECT name, end_date, is_closed FROM fiscal_years WHERE id = ?`, s.usePostgres), fiscalYearID,
	).Scan(&name, &endDate, &closed); err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("fiscal year %q not found", fiscalYearID)
	} else if err != nil {
		return nil, nil, fmt.Errorf("load fiscal year: %w", err)
	}
	if closed != 1 {
		return nil, nil, ErrReparationExerciceOuvert
	}

	closingRef := fmt.Sprintf("%s%d", ClosingReferencePrefix, endDate.Year())
	complementRef := closingRef + ComplementSuffix

	var existing int
	if err := tx.QueryRowContext(ctx, db.Rebind(
		`SELECT COUNT(*) FROM journal_entries WHERE reference = ? AND status = 'posted'`, s.usePostgres), complementRef,
	).Scan(&existing); err != nil {
		return nil, nil, fmt.Errorf("check complement: %w", err)
	}
	if existing > 0 {
		return nil, nil, ErrReparationDejaFaite
	}

	var debit, credit float64
	var found int
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT COUNT(DISTINCT je.id), COALESCE(SUM(jl.debit_amount), 0), COALESCE(SUM(jl.credit_amount), 0)
		FROM journal_entries je JOIN journal_lines jl ON jl.entry_id = je.id
		WHERE je.reference = ? AND je.fiscal_year_id = ? AND je.status = 'posted'`, s.usePostgres),
		closingRef, fiscalYearID,
	).Scan(&found, &debit, &credit); err != nil {
		return nil, nil, fmt.Errorf("load closing entry: %w", err)
	}
	if found == 0 {
		return nil, nil, ErrReparationSansCloture
	}
	gapCents := compliance.Cents(debit) - compliance.Cents(credit)
	if gapCents == 0 {
		return nil, nil, ErrReparationClotureEquilibree
	}

	// Les comptes de produits et de charges encore ouverts sur l'exercice, clôture comprise.
	balances, err := s.resultBalances(ctx, tx, fiscalYearID)
	if err != nil {
		return nil, nil, err
	}
	var lines []closingLine
	var plan []ClosingRepairLine
	var debitCents, creditCents int64
	seq := 1
	for _, b := range balances {
		net := compliance.Cents(b.netDebit)
		if net == 0 {
			continue
		}
		cl := closingLine{accountID: b.accountID, sequence: seq}
		pl := ClosingRepairLine{}
		if net > 0 { // solde débiteur : on le crédite pour le solder
			cl.creditAmount = ptrFloat(float64(net) / 100)
			cl.description = "Complément de clôture (compte resté ouvert, solde débiteur)"
			pl.Credit = float64(net) / 100
			creditCents += net
		} else { // solde créditeur : on le débite
			cl.debitAmount = ptrFloat(float64(-net) / 100)
			cl.description = "Complément de clôture (compte resté ouvert, solde créditeur)"
			pl.Debit = float64(-net) / 100
			debitCents += -net
		}
		if err := tx.QueryRowContext(ctx, db.Rebind(`SELECT code, name FROM accounts WHERE id = ?`, s.usePostgres), b.accountID).
			Scan(&pl.AccountCode, &pl.AccountName); err != nil {
			return nil, nil, fmt.Errorf("load account: %w", err)
		}
		lines = append(lines, cl)
		plan = append(plan, pl)
		seq++
	}
	// Le complément doit compenser l'écart EXACTEMENT : crédit − débit du complément = débit − crédit
	// de la clôture. Sinon l'écart a une autre cause, et on ne le « répare » pas en l'enjambant.
	if len(lines) == 0 || creditCents-debitCents != gapCents {
		return nil, nil, ErrReparationInexpliquee
	}

	return &ClosingRepairPlan{
		FiscalYearID:     fiscalYearID,
		FiscalYear:       name,
		ClosingReference: closingRef,
		ClosingDebit:     debit,
		ClosingCredit:    credit,
		Gap:              float64(gapCents) / 100,
		Reference:        complementRef,
		Date:             endDate.Format("2006-01-02"),
		Lines:            plan,
	}, lines, nil
}

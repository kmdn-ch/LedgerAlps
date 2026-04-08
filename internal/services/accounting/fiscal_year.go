package accounting

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// FiscalYearService handles fiscal year lifecycle operations (CO art. 958).
type FiscalYearService struct {
	db          *sql.DB
	usePostgres bool
}

func NewFiscalYearService(database *sql.DB, usePostgres bool) *FiscalYearService {
	return &FiscalYearService{db: database, usePostgres: usePostgres}
}

// accountBalance holds an account's aggregated net balance for closure.
type accountBalance struct {
	accountID string
	netDebit  float64 // SUM(debit_amount) - SUM(credit_amount) over posted lines
}

// CloseYear effectue la clôture comptable de l'exercice fiscal (CO art. 958).
//
// Étapes :
//  1. Vérifier que l'exercice existe et n'est pas déjà clôturé
//  2. Vérifier l'absence d'écritures en statut 'draft' pour cet exercice
//  3. Calculer le résultat net = SUM(produits revenue) - SUM(charges expense)
//  4. Créer l'écriture de clôture (status='posted') qui ramène les soldes à 0
//  5. Marquer fiscal_year.is_closed = 1
//  6. Créer l'exercice suivant si inexistant
func (s *FiscalYearService) CloseYear(ctx context.Context, fiscalYearID, userID string) error {
	// ── 1. Load fiscal year ───────────────────────────────────────────────────
	fyQ := db.Rebind(`SELECT name, end_date, is_closed FROM fiscal_years WHERE id = ?`, s.usePostgres)
	var fyName string
	var fyEndDate time.Time
	var isClosed int
	if err := s.db.QueryRowContext(ctx, fyQ, fiscalYearID).Scan(&fyName, &fyEndDate, &isClosed); err == sql.ErrNoRows {
		return fmt.Errorf("fiscal year %q not found", fiscalYearID)
	} else if err != nil {
		return fmt.Errorf("load fiscal year: %w", err)
	}
	if isClosed == 1 {
		return fmt.Errorf("fiscal year %q is already closed", fyName)
	}

	// ── 2. No draft entries allowed ───────────────────────────────────────────
	draftQ := db.Rebind(`SELECT COUNT(*) FROM journal_entries WHERE fiscal_year_id = ? AND status = 'draft'`, s.usePostgres)
	var draftCount int
	if err := s.db.QueryRowContext(ctx, draftQ, fiscalYearID).Scan(&draftCount); err != nil {
		return fmt.Errorf("check draft entries: %w", err)
	}
	if draftCount > 0 {
		return fmt.Errorf("cannot close fiscal year %q: %d draft journal entries must be posted or deleted first", fyName, draftCount)
	}

	// ── 3. Compute revenue/expense account balances ───────────────────────────
	// Returns one row per account: net debit = SUM(debit) - SUM(credit)
	// A revenue account normally has net_debit < 0 (credit balance).
	// An expense account normally has net_debit > 0 (debit balance).
	balanceQ := db.Rebind(`
		SELECT
			jl.account_id,
			COALESCE(SUM(jl.debit_amount), 0) - COALESCE(SUM(jl.credit_amount), 0) AS net_debit
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.entry_id
		JOIN accounts ac ON ac.id = jl.account_id
		WHERE je.fiscal_year_id = ?
		  AND je.status = 'posted'
		  AND ac.account_type IN ('revenue', 'expense')
		GROUP BY jl.account_id
		HAVING COALESCE(SUM(jl.debit_amount), 0) - COALESCE(SUM(jl.credit_amount), 0) <> 0
	`, s.usePostgres)

	rows, err := s.db.QueryContext(ctx, balanceQ, fiscalYearID)
	if err != nil {
		return fmt.Errorf("compute account balances: %w", err)
	}
	defer rows.Close()

	type accountTypeBalance struct {
		accountBalance
		accountType string
	}
	var balances []accountTypeBalance

	for rows.Next() {
		var ab accountTypeBalance
		if err := rows.Scan(&ab.accountID, &ab.netDebit); err != nil {
			return fmt.Errorf("scan account balance: %w", err)
		}
		balances = append(balances, ab)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("account balance rows: %w", err)
	}

	// Fetch account_type for each account found
	if len(balances) > 0 {
		for i := range balances {
			typeQ := db.Rebind(`SELECT account_type FROM accounts WHERE id = ?`, s.usePostgres)
			if err := s.db.QueryRowContext(ctx, typeQ, balances[i].accountID).Scan(&balances[i].accountType); err != nil {
				return fmt.Errorf("load account type for %s: %w", balances[i].accountID, err)
			}
		}
	}

	// Calculate net result: SUM(revenue credits) - SUM(expense debits)
	// net_debit for revenue accounts is negative (they have credit balances).
	// net_debit for expense accounts is positive (they have debit balances).
	var totalRevenue, totalExpense float64
	for _, b := range balances {
		switch b.accountType {
		case "revenue":
			// revenue credit balance → net_debit is negative; revenue = abs
			totalRevenue += -b.netDebit
		case "expense":
			// expense debit balance → net_debit is positive
			totalExpense += b.netDebit
		}
	}
	netResult := totalRevenue - totalExpense // positive = profit, negative = loss

	// ── 4. Load compte 5900 (compte de résultat) ─────────────────────────────
	var resultAccountID string
	accQ := db.Rebind(`SELECT id FROM accounts WHERE code = '5900' LIMIT 1`, s.usePostgres)
	if err := s.db.QueryRowContext(ctx, accQ).Scan(&resultAccountID); err == sql.ErrNoRows {
		return fmt.Errorf("account 5900 (compte de résultat) not found in chart of accounts")
	} else if err != nil {
		return fmt.Errorf("load account 5900: %w", err)
	}

	// ── 5. Build closing entry lines ──────────────────────────────────────────
	// To zero out each account:
	//   revenue account has credit balance (net_debit < 0) → debit it to zero
	//   expense account has debit balance  (net_debit > 0) → credit it to zero
	// Then balance the result account (5900):
	//   profit (net > 0) → credit 5900
	//   loss   (net < 0) → debit  5900 (absolute value)

	type closingLine struct {
		accountID    string
		debitAmount  *float64
		creditAmount *float64
		description  string
		sequence     int
	}

	var lines []closingLine
	seq := 1

	for _, b := range balances {
		cl := closingLine{accountID: b.accountID, sequence: seq}
		switch b.accountType {
		case "revenue":
			// net_debit < 0 means credit balance; debit the account to zero it
			amount := -b.netDebit // positive amount
			if amount > 0.001 {
				cl.debitAmount = ptrFloat(amount)
				cl.description = "Clôture compte produits"
				lines = append(lines, cl)
				seq++
			}
		case "expense":
			// net_debit > 0 means debit balance; credit the account to zero it
			if b.netDebit > 0.001 {
				cl.creditAmount = ptrFloat(b.netDebit)
				cl.description = "Clôture compte charges"
				lines = append(lines, cl)
				seq++
			}
		}
	}

	// Add the result account line (5900)
	resultLine := closingLine{accountID: resultAccountID, sequence: seq, description: "Résultat net de l'exercice"}
	if abs(netResult) > 0.001 {
		if netResult > 0 {
			// profit → credit 5900
			resultLine.creditAmount = ptrFloat(netResult)
		} else {
			// loss → debit 5900
			resultLine.debitAmount = ptrFloat(-netResult)
		}
		lines = append(lines, resultLine)
	}

	// ── 6. Execute within a single atomic transaction ─────────────────────────
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Only create the closing entry if there are revenue/expense lines to close.
	if len(lines) > 0 {
		fyYear := fyEndDate.Year()
		closingRef := fmt.Sprintf("CLOTURE-%d", fyYear)
		closingDesc := fmt.Sprintf("Clôture exercice %s", fyName)
		closingDate := fyEndDate.Format("2006-01-02")
		entryID := db.NewID()

		insertEntry := db.Rebind(`
			INSERT INTO journal_entries (id, reference, date, description, status, fiscal_year_id, created_by_id)
			VALUES (?, ?, ?, ?, 'posted', ?, ?)
		`, s.usePostgres)
		if _, err := tx.ExecContext(ctx, insertEntry, entryID, closingRef, closingDate, closingDesc, fiscalYearID, userID); err != nil {
			return fmt.Errorf("insert closing entry: %w", err)
		}

		insertLine := db.Rebind(`
			INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount, description, sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, s.usePostgres)
		for _, l := range lines {
			if _, err := tx.ExecContext(ctx, insertLine,
				db.NewID(), entryID, l.accountID,
				l.debitAmount, l.creditAmount,
				l.description, l.sequence,
			); err != nil {
				return fmt.Errorf("insert closing line: %w", err)
			}
		}
	}

	// ── 7. Mark fiscal year as closed ─────────────────────────────────────────
	closeQ := db.Rebind(`UPDATE fiscal_years SET is_closed = 1, updated_at = ? WHERE id = ?`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, closeQ, time.Now().UTC(), fiscalYearID); err != nil {
		return fmt.Errorf("close fiscal year: %w", err)
	}

	// ── 8. Create next fiscal year if not already existing ────────────────────
	nextYear := fyEndDate.Year() + 1
	nextName := fmt.Sprintf("%d", nextYear)
	var existingCount int
	checkNextQ := db.Rebind(`SELECT COUNT(*) FROM fiscal_years WHERE name = ?`, s.usePostgres)
	if err := tx.QueryRowContext(ctx, checkNextQ, nextName).Scan(&existingCount); err != nil {
		return fmt.Errorf("check next fiscal year: %w", err)
	}
	if existingCount == 0 {
		nextStart := fyEndDate.AddDate(0, 0, 1)
		nextEnd := fyEndDate.AddDate(0, 0, 365)
		insertNext := db.Rebind(`
			INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed)
			VALUES (?, ?, ?, ?, 0)
		`, s.usePostgres)
		if _, err := tx.ExecContext(ctx, insertNext,
			db.NewID(), nextName,
			nextStart.Format("2006-01-02"),
			nextEnd.Format("2006-01-02"),
		); err != nil {
			return fmt.Errorf("create next fiscal year: %w", err)
		}
	}

	return tx.Commit()
}

// ptrFloat returns a pointer to the given float64 value.
func ptrFloat(v float64) *float64 { return &v }

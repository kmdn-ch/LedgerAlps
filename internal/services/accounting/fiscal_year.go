package accounting

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
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

// ClosingReferencePrefix préfixe la référence de l'écriture de clôture d'un
// exercice (`CLOTURE-2025`). Aucun autre chemin ne produit une référence de ce
// nom : les écritures saisies reçoivent `JN-…` (voir nextReference).
//
// C'est ce préfixe qui permet de distinguer l'écriture qui VIRE le résultat de
// l'exercice de celles qui le PRODUISENT. Les rapports de performance (compte de
// résultat, carnet du lait, chiffre d'affaires de l'éligibilité) doivent
// l'exclure : elle remet à zéro les comptes de produits et de charges, et un
// exercice clôturé affichait alors zéro partout (audit croisé Gemini, CRIT-01 et
// CRIT-02).
const ClosingReferencePrefix = "CLOTURE-"

// SQLNotClosingEntry est le fragment de condition qui écarte l'écriture de
// clôture. L'alias de la table des écritures est `je`, partout.
const SQLNotClosingEntry = "je.reference NOT LIKE 'CLOTURE-%'"

// CloseYear effectue la clôture comptable de l'exercice fiscal (CO art. 958).
//
// Étapes :
//  1. Vérifier que l'exercice existe et n'est pas déjà clôturé
//  2. Vérifier l'absence d'écritures en statut 'draft' pour cet exercice
//  3. Lire le solde net de chaque compte de produits et de charges
//  4. Créer l'écriture de clôture (status='posted') qui ramène chaque solde à 0
//  5. Marquer fiscal_year.is_closed = 1
//  6. Créer l'exercice suivant si inexistant
//
// # Ce que cette version corrige (audit croisé Gemini, CRIT-04)
//
//   - Tout se lit et s'écrit dans UNE transaction. Les vérifications (exercice
//     clos ? brouillons ?) et la lecture des soldes précédaient BeginTx : une
//     écriture comptabilisée entre-temps était ignorée par la clôture, et deux
//     clôtures simultanées passaient toutes deux.
//   - Chaque compte reçoit sa ligne de clôture QUEL QUE SOIT le sens de son solde.
//     Un produit à solde débiteur (rabais, avoirs) ou une charge à solde créditeur
//     (ristourne) était sommé dans le résultat mais n'avait pas de ligne : avec
//     5 000 de rabais et 2 000 de ristourne, l'écriture scellée faisait
//     100 000 au débit pour 97 000 au crédit, et ces deux comptes restaient
//     ouverts sur l'exercice suivant.
//   - Le compte de résultat (5900) équilibre l'écriture PAR CONSTRUCTION : son
//     montant est la différence des deux côtés, en centimes entiers, et non un
//     résultat recalculé à part.
//   - L'écriture est refusée plutôt que scellée si elle ne s'équilibre pas : elle
//     ne passe pas par `CreateEntry`, donc par aucune autre vérification.
//   - La trace d'audit porte le débit ET le crédit réels. Elle écrivait deux fois
//     le débit.
func (s *FiscalYearService) CloseYear(ctx context.Context, fiscalYearID, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// ── 1. Load fiscal year ───────────────────────────────────────────────────
	fyQ := db.Rebind(`SELECT name, end_date, is_closed FROM fiscal_years WHERE id = ?`, s.usePostgres)
	var fyName string
	var fyEndDate time.Time
	var isClosed int
	if err := tx.QueryRowContext(ctx, fyQ, fiscalYearID).Scan(&fyName, &fyEndDate, &isClosed); err == sql.ErrNoRows {
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
	if err := tx.QueryRowContext(ctx, draftQ, fiscalYearID).Scan(&draftCount); err != nil {
		return fmt.Errorf("check draft entries: %w", err)
	}
	if draftCount > 0 {
		return fmt.Errorf("cannot close fiscal year %q: %d draft journal entries must be posted or deleted first", fyName, draftCount)
	}

	// ── 3. Net balance of every revenue / expense account ─────────────────────
	// net debit = SUM(debit) - SUM(credit). Normally negative for a revenue
	// account and positive for an expense account — but not always.
	balances, err := s.resultBalances(ctx, tx, fiscalYearID)
	if err != nil {
		return err
	}

	// ── 4. Load compte 5900 (compte de résultat) ─────────────────────────────
	var resultAccountID string
	accQ := db.Rebind(`SELECT id FROM accounts WHERE code = '5900' LIMIT 1`, s.usePostgres)
	if err := tx.QueryRowContext(ctx, accQ).Scan(&resultAccountID); err == sql.ErrNoRows {
		return fmt.Errorf("account 5900 (compte de résultat) not found in chart of accounts")
	} else if err != nil {
		return fmt.Errorf("load account 5900: %w", err)
	}

	lines, debitCents, creditCents := buildClosingLines(balances, resultAccountID)

	// ── 5. Execute the closing within the same transaction ────────────────────
	// Only create the closing entry if there are revenue/expense lines to close.
	if len(lines) > 0 {
		// Garde-fou : cette écriture est insérée directement, sans passer par
		// validateDoubleEntry. Une écriture qui ne s'équilibre pas ne se scelle pas.
		if debitCents != creditCents {
			return fmt.Errorf("la clôture de l'exercice %s ne s'équilibre pas (débit %.2f, crédit %.2f) : "+
				"elle n'est pas passée", fyName, float64(debitCents)/100, float64(creditCents)/100)
		}

		// La référence vient de l'ANNÉE DE FIN, et deux exercices peuvent finir la même
		// année (un exercice court, un changement de date de clôture) : la seconde
		// clôture tombait sur `UNIQUE(reference)` et répondait 500 — l'exercice ne pouvait
		// plus être clos (audit croisé Gemini, MED-04).
		closingRef, err := closingReferenceFor(ctx, tx, s.usePostgres, fyEndDate.Year())
		if err != nil {
			return err
		}
		closingDesc := fmt.Sprintf("Clôture exercice %s", fyName)
		closingDate := fyEndDate.Format("2006-01-02")
		entryID := db.NewID()

		// L'écriture de clôture entre au journal en 'draft', puis reçoit son
		// empreinte et son maillon d'audit comme n'importe quelle autre. Elle
		// était auparavant insérée directement en 'posted', sans empreinte ni
		// entrée d'audit : l'écriture qui vire le résultat de l'exercice était
		// la seule à échapper à la chaîne du CO art. 957a. Le contrôle de
		// cohérence la signalait déjà, sans que le lien soit fait.
		insertEntry := db.Rebind(`
			INSERT INTO journal_entries (id, reference, date, description, status, fiscal_year_id, created_by_id)
			VALUES (?, ?, ?, ?, 'draft', ?, ?)
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

		// Le condensé des lignes, scellé avec les totaux (audit croisé Gemini, CRIT-10).
		linesDigest, err := LinesDigest(ctx, tx, s.usePostgres, entryID)
		if err != nil {
			return fmt.Errorf("digest closing lines: %w", err)
		}

		// Chaînage et comptabilisation, dans la même transaction que la clôture :
		// une panne entre les deux laisserait un exercice clos sur une écriture
		// de clôture restée en brouillon.
		chainedHash, now, err := AppendAuditEntry(ctx, tx, s.usePostgres,
			userID, "close", entryID, clientIPFrom(ctx),
			map[string]any{
				"entry_id":     entryID,
				"status":       "posted",
				"fiscal_year":  fyName,
				"debit":        float64(debitCents) / 100,
				"credit":       float64(creditCents) / 100,
				"lines_digest": linesDigest,
			})
		if err != nil {
			return fmt.Errorf("chain closing entry: %w", err)
		}
		postQ := db.Rebind(
			`UPDATE journal_entries SET status = 'posted', integrity_hash = ?, updated_at = ? WHERE id = ?`,
			s.usePostgres)
		if _, err := tx.ExecContext(ctx, postQ, chainedHash, now, entryID); err != nil {
			return fmt.Errorf("post closing entry: %w", err)
		}
	}

	// ── 6. Mark fiscal year as closed ─────────────────────────────────────────
	// `AND is_closed = 0` : la clôture ne vaut qu'une fois, même si deux requêtes
	// ont franchi la vérification du début en même temps.
	closeQ := db.Rebind(`UPDATE fiscal_years SET is_closed = 1, updated_at = ? WHERE id = ? AND is_closed = 0`, s.usePostgres)
	res, err := tx.ExecContext(ctx, closeQ, time.Now().UTC(), fiscalYearID)
	if err != nil {
		return fmt.Errorf("close fiscal year: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("fiscal year %q is already closed", fyName)
	}

	// ── 7. Create next fiscal year if the period is not already covered ────────
	//
	// Un an civil après le début, moins un jour — et non « fin + 365 jours » :
	// l'exercice suivant d'un 31.12.2027 est 2028, bissextile, et 365 jours
	// arrêtaient la nouvelle année au 30.12.2028. Le 31 n'appartenait alors à
	// aucun exercice, et toute écriture de ce jour était refusée.
	//
	// « Déjà couverte » se juge sur les DATES, pas sur le nom (audit croisé Gemini,
	// MED-04). Le nom seul laissait créer un exercice « 2032 » du 1.7.2031 au 30.6.2032
	// par-dessus un exercice existant du 1.7 au 31.12.2031, ou par-dessus un exercice
	// suivant que l'utilisateur avait nommé autrement : deux exercices se
	// chevauchaient, et le rattachement d'une écriture devenait arbitraire.
	nextStart := fyEndDate.AddDate(0, 0, 1)
	nextEnd := nextStart.AddDate(1, 0, 0).AddDate(0, 0, -1)
	var covered int
	overlapQ := db.Rebind(`SELECT COUNT(*) FROM fiscal_years WHERE start_date <= ? AND end_date >= ?`, s.usePostgres)
	if err := tx.QueryRowContext(ctx, overlapQ,
		nextEnd.Format("2006-01-02"), nextStart.Format("2006-01-02")).Scan(&covered); err != nil {
		return fmt.Errorf("check next fiscal year: %w", err)
	}
	if covered == 0 {
		nextName, err := freeFiscalYearName(ctx, tx, s.usePostgres, nextEnd.Year(), nextStart)
		if err != nil {
			return err
		}
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

// closingReferenceFor choisit la référence de l'écriture de clôture d'un exercice qui
// finit l'année `endYear` : « CLOTURE-2031 », puis « CLOTURE-2031-2 » si un autre exercice
// finissant la même année a déjà pris la première. Elle commence toujours par le préfixe
// des clôtures (les rapports l'écartent par ce préfixe) et ne finit jamais par « -C »
// (suffixe du complément).
func closingReferenceFor(ctx context.Context, tx *sql.Tx, usePostgres bool, endYear int) (string, error) {
	base := fmt.Sprintf("%s%d", ClosingReferencePrefix, endYear)
	ref := base
	for n := 2; n < 100; n++ {
		var used int
		if err := tx.QueryRowContext(ctx,
			db.Rebind(`SELECT COUNT(*) FROM journal_entries WHERE reference = ?`, usePostgres), ref,
		).Scan(&used); err != nil {
			return "", fmt.Errorf("check closing reference: %w", err)
		}
		if used == 0 {
			return ref, nil
		}
		ref = fmt.Sprintf("%s-%d", base, n)
	}
	return "", fmt.Errorf("aucune référence de clôture libre pour l'année %d", endYear)
}

// freeFiscalYearName donne son nom à l'exercice créé à la clôture : l'année de sa fin, ou
// — si un exercice la porte déjà sans chevaucher celui-ci — l'année suivie du mois de début.
func freeFiscalYearName(ctx context.Context, tx *sql.Tx, usePostgres bool, endYear int, start time.Time) (string, error) {
	for _, name := range []string{
		fmt.Sprintf("%d", endYear),
		fmt.Sprintf("%d-%s", endYear, start.Format("01")),
		fmt.Sprintf("%d-%s", endYear, start.Format("0102")),
	} {
		var n int
		if err := tx.QueryRowContext(ctx,
			db.Rebind(`SELECT COUNT(*) FROM fiscal_years WHERE name = ?`, usePostgres), name,
		).Scan(&n); err != nil {
			return "", fmt.Errorf("check next fiscal year name: %w", err)
		}
		if n == 0 {
			return name, nil
		}
	}
	return "", fmt.Errorf("aucun nom libre pour l'exercice suivant (%d)", endYear)
}

// closingLine est une ligne de l'écriture de clôture.
type closingLine struct {
	accountID    string
	debitAmount  *float64
	creditAmount *float64
	description  string
	sequence     int
}

// resultBalances lit, dans la transaction donnée, le solde net de chaque compte de
// produits et de charges de l'exercice, au centime.
func (s *FiscalYearService) resultBalances(ctx context.Context, tx *sql.Tx, fiscalYearID string) ([]accountBalance, error) {
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
		ORDER BY ac.code
	`, s.usePostgres)

	rows, err := tx.QueryContext(ctx, balanceQ, fiscalYearID)
	if err != nil {
		return nil, fmt.Errorf("compute account balances: %w", err)
	}
	defer rows.Close()

	var balances []accountBalance
	for rows.Next() {
		var ab accountBalance
		if err := rows.Scan(&ab.accountID, &ab.netDebit); err != nil {
			return nil, fmt.Errorf("scan account balance: %w", err)
		}
		balances = append(balances, ab)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("account balance rows: %w", err)
	}
	return balances, nil
}

// buildClosingLines construit l'écriture de clôture et rend ses totaux en centimes.
//
// Chaque compte à solde non nul reçoit la ligne qui le ramène à zéro : crédité s'il
// est débiteur, débité s'il est créditeur — que ce soit un produit ou une charge.
// Le compte de résultat reçoit la différence : au crédit si les produits l'emportent
// (bénéfice), au débit sinon (perte). L'écriture s'équilibre donc par construction.
func buildClosingLines(balances []accountBalance, resultAccountID string) (lines []closingLine, debitCents, creditCents int64) {
	seq := 1
	for _, b := range balances {
		net := compliance.Cents(b.netDebit) // centimes entiers : pas de dérive
		if net == 0 {
			continue
		}
		cl := closingLine{accountID: b.accountID, sequence: seq}
		if net > 0 {
			// solde débiteur : on le crédite pour le solder
			cl.creditAmount = ptrFloat(float64(net) / 100)
			cl.description = "Clôture compte de résultat (charge)"
			creditCents += net
		} else {
			// solde créditeur : on le débite pour le solder
			cl.debitAmount = ptrFloat(float64(-net) / 100)
			cl.description = "Clôture compte de résultat (produit)"
			debitCents += -net
		}
		lines = append(lines, cl)
		seq++
	}
	if len(lines) == 0 {
		return nil, 0, 0
	}

	// Le résultat net est ce qui sort des produits moins ce qui sort des charges :
	// débit des lignes (produits soldés) - crédit des lignes (charges soldées).
	switch diff := debitCents - creditCents; {
	case diff > 0: // bénéfice → crédit 5900
		lines = append(lines, closingLine{accountID: resultAccountID, sequence: seq,
			creditAmount: ptrFloat(float64(diff) / 100), description: "Résultat net de l'exercice"})
		creditCents += diff
	case diff < 0: // perte → débit 5900
		lines = append(lines, closingLine{accountID: resultAccountID, sequence: seq,
			debitAmount: ptrFloat(float64(-diff) / 100), description: "Résultat net de l'exercice"})
		debitCents += -diff
	}
	return lines, debitCents, creditCents
}

// ptrFloat returns a pointer to the given float64 value.
func ptrFloat(v float64) *float64 { return &v }

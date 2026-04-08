package accounting

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ErrNotDoubleEntry is returned when debit total ≠ credit total.
type ErrNotDoubleEntry struct {
	Debit  float64
	Credit float64
}

func (e ErrNotDoubleEntry) Error() string {
	return fmt.Sprintf("double-entry violation: debit %.2f ≠ credit %.2f (CO art. 957)", e.Debit, e.Credit)
}

// ErrAlreadyPosted is returned when trying to post an already-posted entry.
var ErrAlreadyPosted = fmt.Errorf("journal entry is already posted")

// ErrEntryNotFound is returned when the entry does not exist.
var ErrEntryNotFound = fmt.Errorf("journal entry not found")

// ErrIntegrityViolation is returned when the stored integrity_hash of a posted
// journal entry does not match the hash recomputed from the audit log.
// This signals tampering or data corruption (CO art. 957a).
type ErrIntegrityViolation struct {
	EntryID  string
	Expected string
	Got      string
}

func (e ErrIntegrityViolation) Error() string {
	return fmt.Sprintf("integrity violation for entry %s: expected hash %s, got %s", e.EntryID, e.Expected, e.Got)
}

// Service implements the double-entry accounting engine.
type Service struct {
	db          *sql.DB
	usePostgres bool
}

func New(database *sql.DB, usePostgres bool) *Service {
	return &Service{db: database, usePostgres: usePostgres}
}

// ─── CreateEntry ──────────────────────────────────────────────────────────────

type LineInput struct {
	AccountID    string
	DebitAmount  *float64
	CreditAmount *float64
	Description  string
	Sequence     int
}

type CreateEntryRequest struct {
	Date        time.Time
	Description string
	Lines       []LineInput
}

// CreateEntry inserts a draft journal entry with its lines.
// Returns ErrNotDoubleEntry if sum(debit) ≠ sum(credit).
func (s *Service) CreateEntry(ctx context.Context, userID string, req CreateEntryRequest) (*models.JournalEntry, error) {
	if err := validateDoubleEntry(req.Lines); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	entryID := db.NewID()
	ref, err := s.nextReference(ctx, tx, req.Date)
	if err != nil {
		return nil, fmt.Errorf("next reference: %w", err)
	}

	insertEntry := db.Rebind(`
		INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
		VALUES (?, ?, ?, ?, 'draft', ?)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insertEntry, entryID, ref, req.Date.Format("2006-01-02"), req.Description, userID); err != nil {
		return nil, fmt.Errorf("insert entry: %w", err)
	}

	insertLine := db.Rebind(`
		INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount, description, sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	for _, l := range req.Lines {
		if _, err := tx.ExecContext(ctx, insertLine, db.NewID(), entryID, l.AccountID, l.DebitAmount, l.CreditAmount, l.Description, l.Sequence); err != nil {
			return nil, fmt.Errorf("insert line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &models.JournalEntry{
		ID:          entryID,
		Reference:   ref,
		Date:        req.Date,
		Description: req.Description,
		Status:      models.JournalEntryStatusDraft,
		CreatedByID: userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// ─── PostEntry ────────────────────────────────────────────────────────────────

// PostEntry validates, marks the entry as posted, computes integrity_hash,
// and appends an audit log record with the CO art. 957a hash chain.
func (s *Service) PostEntry(ctx context.Context, userID, entryID, ipAddress string) error {
	// 1. Load entry
	getQ := db.Rebind("SELECT status FROM journal_entries WHERE id = ?", s.usePostgres)
	var status string
	if err := s.db.QueryRowContext(ctx, getQ, entryID).Scan(&status); err == sql.ErrNoRows {
		return ErrEntryNotFound
	} else if err != nil {
		return fmt.Errorf("load entry: %w", err)
	}
	if status == string(models.JournalEntryStatusPosted) {
		return ErrAlreadyPosted
	}

	// 2. Re-validate double-entry from stored lines
	sumQ := db.Rebind(`
		SELECT COALESCE(SUM(debit_amount), 0), COALESCE(SUM(credit_amount), 0)
		FROM journal_lines WHERE entry_id = ?`, s.usePostgres)
	var totalDebit, totalCredit float64
	if err := s.db.QueryRowContext(ctx, sumQ, entryID).Scan(&totalDebit, &totalCredit); err != nil {
		return fmt.Errorf("sum lines: %w", err)
	}
	if abs(totalDebit-totalCredit) > 0.001 {
		return ErrNotDoubleEntry{Debit: totalDebit, Credit: totalCredit}
	}

	// 3. Compute integrity hash (covers entry state at posting time)
	afterState := fmt.Sprintf(`{"entry_id":%q,"debit":%.4f,"credit":%.4f,"posted_at":%q}`,
		entryID, totalDebit, totalCredit, time.Now().UTC().Format(time.RFC3339))
	now := time.Now().UTC()
	entryHash := security.ComputeEntryHash(userID, "post", "journal_entries", entryID, "", afterState, ipAddress, now)

	// 4. Get prev_hash and sequence for audit chain
	prevQ := db.Rebind("SELECT entry_hash, sequence_number FROM audit_logs ORDER BY sequence_number DESC LIMIT 1", s.usePostgres)
	var prevHash string
	var lastSeq int64
	if err := s.db.QueryRowContext(ctx, prevQ).Scan(&prevHash, &lastSeq); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load prev hash: %w", err)
	}
	chainedHash := security.ChainHash(prevHash, entryHash)
	nextSeq := lastSeq + 1

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 5. Mark entry as posted with integrity hash
	updateQ := db.Rebind("UPDATE journal_entries SET status = 'posted', integrity_hash = ?, updated_at = ? WHERE id = ?", s.usePostgres)
	if _, err := tx.ExecContext(ctx, updateQ, chainedHash, now, entryID); err != nil {
		return fmt.Errorf("update entry: %w", err)
	}

	// 6. Write audit log record with CO art. 957a hash chain
	afterJSON, _ := json.Marshal(map[string]any{
		"entry_id": entryID,
		"status":   "posted",
		"debit":    totalDebit,
		"credit":   totalCredit,
	})
	insertAudit := db.Rebind(`
		INSERT INTO audit_logs (id, user_id, action, table_name, record_id, after_state, ip_address, entry_hash, prev_hash, sequence_number)
		VALUES (?, ?, 'post', 'journal_entries', ?, ?, ?, ?, ?, ?)`, s.usePostgres)
	var prevHashPtr *string
	if prevHash != "" {
		prevHashPtr = &prevHash
	}
	if _, err := tx.ExecContext(ctx, insertAudit,
		db.NewID(), userID, entryID, string(afterJSON), ipAddress,
		entryHash, prevHashPtr, nextSeq); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return tx.Commit()
}

// ─── VerifyEntryIntegrity ─────────────────────────────────────────────────────

// VerifyEntryIntegrity vérifie que l'integrity_hash d'une écriture postée
// correspond au hash recalculé depuis l'audit log (CO art. 957a).
// Retourne nil si intègre, ErrIntegrityViolation si corrompu.
// Retourne nil sans erreur si l'écriture n'est pas encore postée (pas de hash attendu).
func (s *Service) VerifyEntryIntegrity(ctx context.Context, entryID string) error {
	// 1. Charger l'écriture : status et integrity_hash
	entryQ := db.Rebind("SELECT status, COALESCE(integrity_hash, '') FROM journal_entries WHERE id = ?", s.usePostgres)
	var status, storedHash string
	if err := s.db.QueryRowContext(ctx, entryQ, entryID).Scan(&status, &storedHash); err == sql.ErrNoRows {
		return ErrEntryNotFound
	} else if err != nil {
		return fmt.Errorf("load entry: %w", err)
	}

	// 2. Pas encore postée → pas de hash, intégrité non vérifiable
	if status != string(models.JournalEntryStatusPosted) {
		return nil
	}

	// 3. Charger l'audit log correspondant à l'action 'post'
	auditQ := db.Rebind(`
		SELECT
			COALESCE(user_id, ''),
			action,
			table_name,
			record_id,
			COALESCE(before_state, ''),
			COALESCE(after_state, ''),
			COALESCE(ip_address, ''),
			entry_hash,
			created_at
		FROM audit_logs
		WHERE table_name = 'journal_entries'
		  AND record_id = ?
		  AND action = 'post'
		LIMIT 1`, s.usePostgres)

	var (
		userID, action, tableName, recordID string
		beforeState, afterState, ipAddress  string
		auditEntryHash                      string
		createdAt                           time.Time
	)
	if err := s.db.QueryRowContext(ctx, auditQ, entryID).Scan(
		&userID, &action, &tableName, &recordID,
		&beforeState, &afterState, &ipAddress,
		&auditEntryHash, &createdAt,
	); err == sql.ErrNoRows {
		return fmt.Errorf("audit log not found for posted entry %s", entryID)
	} else if err != nil {
		return fmt.Errorf("load audit log: %w", err)
	}

	// 4. Recalculer l'entry_hash depuis les champs de l'audit log
	recomputed := security.ComputeEntryHash(userID, action, tableName, recordID, beforeState, afterState, ipAddress, createdAt)

	// 5. Comparer avec l'entry_hash stocké dans audit_logs
	if recomputed != auditEntryHash {
		return ErrIntegrityViolation{
			EntryID:  entryID,
			Expected: auditEntryHash,
			Got:      recomputed,
		}
	}

	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// nextReference generates the next sequential reference for a given year: JN-2026-001
func (s *Service) nextReference(ctx context.Context, tx *sql.Tx, date time.Time) (string, error) {
	year := date.Format("2006")
	countQ := db.Rebind(`
		SELECT COUNT(*) FROM journal_entries
		WHERE reference LIKE ?`, s.usePostgres)
	var count int
	if err := tx.QueryRowContext(ctx, countQ, "JN-"+year+"-%").Scan(&count); err != nil {
		return "", fmt.Errorf("count references: %w", err)
	}
	return fmt.Sprintf("JN-%s-%03d", year, count+1), nil
}

// validateDoubleEntry returns ErrNotDoubleEntry if sum(debit) ≠ sum(credit).
func validateDoubleEntry(lines []LineInput) error {
	var debit, credit float64
	for _, l := range lines {
		if l.DebitAmount != nil {
			debit += *l.DebitAmount
		}
		if l.CreditAmount != nil {
			credit += *l.CreditAmount
		}
	}
	if abs(debit-credit) > 0.001 {
		return ErrNotDoubleEntry{Debit: debit, Credit: credit}
	}
	if len(lines) < 2 {
		return fmt.Errorf("a journal entry must have at least two lines")
	}
	return nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

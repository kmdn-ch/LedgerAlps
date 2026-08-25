package accounting

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ErrNotDoubleEntry is returned when debit total ≠ credit total.
type ErrNotDoubleEntry struct {
	Debit  float64
	Credit float64
}

// Error dit l'ÉCART, pas seulement le refus.
//
// « vérifiez la partie double » oblige à recompter à la main une écriture de
// dix lignes. Donner les deux totaux et leur différence désigne presque
// toujours la faute de frappe : un écart de 90.00 sur un montant de 100.00 est
// un zéro oublié, un écart de 9.00 une décimale décalée.
func (e ErrNotDoubleEntry) Error() string {
	return fmt.Sprintf(
		"l'écriture n'est pas équilibrée : débit %.2f, crédit %.2f, écart %.2f (CO art. 957)",
		e.Debit, e.Credit, abs(e.Debit-e.Credit))
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

// ErrIntegrityNotVerifiable signale une entrée dont l'empreinte ne peut pas
// être recalculée — et non une entrée altérée. La distinction est le fond du
// sujet : « je ne peux pas vérifier » et « quelqu'un a modifié vos livres » ne
// s'adressent pas au même destinataire ni au même problème.
type ErrIntegrityNotVerifiable struct {
	EntryID     string
	HashVersion int
}

func (e ErrIntegrityNotVerifiable) Error() string {
	return fmt.Sprintf(
		"entry %s was written with hash version %d, whose inputs were not persisted: its own hash cannot be recomputed (the chain links around it still can)",
		e.EntryID, e.HashVersion)
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

	// Rattachement à l'exercice, dans la même transaction que l'insertion :
	// une écriture sans exercice est invisible à la clôture, qui filtre dessus.
	// EnsureFiscalPeriod refuse aussi les exercices clos (CO art. 958f).
	period, err := EnsureFiscalPeriod(ctx, tx, s.usePostgres, req.Date)
	if err != nil {
		return nil, err
	}

	insertEntry := db.Rebind(`
		INSERT INTO journal_entries (id, reference, date, description, status, fiscal_year_id, created_by_id)
		VALUES (?, ?, ?, ?, 'draft', ?, ?)`, s.usePostgres)
	if _, err := tx.ExecContext(ctx, insertEntry, entryID, ref, req.Date.Format("2006-01-02"), req.Description, period.ID, userID); err != nil {
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
	getQ := db.Rebind("SELECT status, date FROM journal_entries WHERE id = ?", s.usePostgres)
	var status string
	var entryDate time.Time
	if err := s.db.QueryRowContext(ctx, getQ, entryID).Scan(&status, &entryDate); err == sql.ErrNoRows {
		return ErrEntryNotFound
	} else if err != nil {
		return fmt.Errorf("load entry: %w", err)
	}
	if status == string(models.JournalEntryStatusPosted) {
		return ErrAlreadyPosted
	}

	// 1 bis. Verrouillage de période. Un brouillon peut avoir été créé avant la
	// clôture et comptabilisé après : c'est précisément le chemin qu'il faut
	// fermer, sans quoi l'exercice bouclé continuerait de bouger (CO art. 958f,
	// Olico art. 3). La correction se passe dans l'exercice ouvert.
	if period, found, err := LookupFiscalPeriod(ctx, s.db, s.usePostgres, entryDate); err != nil {
		return err
	} else if found && period.Closed {
		return ErrPeriodClosed{FiscalYear: period.Name, Date: entryDate}
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

	// 3. Ouvrir la transaction AVANT de lire le maillon précédent : lire à
	//    l'extérieur laissait deux comptabilisations concurrentes obtenir le
	//    même prédécesseur et fourcher la chaîne.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 4. Ajouter le maillon d'audit (CO art. 957a) et récupérer l'empreinte
	//    chaînée à porter sur l'écriture.
	chainedHash, now, err := AppendAuditEntry(ctx, tx, s.usePostgres,
		userID, "post", entryID, ipAddress,
		map[string]any{
			"entry_id": entryID,
			"status":   "posted",
			"debit":    totalDebit,
			"credit":   totalCredit,
		})
	if err != nil {
		return err
	}

	// 5. Marquer l'écriture comme comptabilisée, empreinte comprise.
	updateQ := db.Rebind("UPDATE journal_entries SET status = 'posted', integrity_hash = ?, updated_at = ? WHERE id = ?", s.usePostgres)
	if _, err := tx.ExecContext(ctx, updateQ, chainedHash, now, entryID); err != nil {
		return fmt.Errorf("update entry: %w", err)
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
			created_at,
			hash_version
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
		hashVersion                         int
	)
	if err := s.db.QueryRowContext(ctx, auditQ, entryID).Scan(
		&userID, &action, &tableName, &recordID,
		&beforeState, &afterState, &ipAddress,
		&auditEntryHash, &createdAt, &hashVersion,
	); err == sql.ErrNoRows {
		return fmt.Errorf("aucun maillon d'audit pour l'écriture comptabilisée %s", entryID)
	} else if err != nil {
		return fmt.Errorf("load audit log: %w", err)
	}

	// 3 bis. Entrée antérieure au correctif d'empreinte (voir migration 0012) :
	// les valeurs ayant servi au calcul n'ont pas été persistées, l'empreinte
	// n'est donc pas recalculable. Retourner ErrIntegrityViolation reviendrait
	// à accuser l'utilisateur d'une altération qui n'a pas eu lieu.
	if hashVersion < 2 {
		return ErrIntegrityNotVerifiable{EntryID: entryID, HashVersion: hashVersion}
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

// ─── nLPD data minimisation ───────────────────────────────────────────────────

// sensitiveFieldRe reconnaît les paires clé-valeur JSON dont la clé désigne une
// donnée personnelle (nLPD art. 6 — minimisation des données dans les journaux).
//
// # Pourquoi les variantes composées comptent
//
// La règle ne reconnaissait que les clés EXACTES : `name`, `address`, `iban`.
// Elle laissait donc passer `company_name`, `legal_name`, `supplier_name`,
// `address_street`, `address_city` — c'est-à-dire, chez un indépendant, son
// propre nom et son adresse privée, écrits en clair dans une table conservée
// dix ans (CO art. 958f).
//
// Le motif accepte maintenant le terme sensible comme MOT, délimité par des
// tirets bas : préfixé (`company_name`), suffixé (`address_street`), ou les
// deux. Les clés voisines restent intactes — `number` ne contient pas `name`,
// et `document_type` ne contient aucun terme sensible.
//
// Élargir le masquage ne remet en cause aucun maillon existant : l'empreinte
// porte sur ce qui a été RÉELLEMENT stocké, et les anciennes lignes conservent
// leurs valeurs comme leur empreinte. Seuls les maillons écrits ensuite sont
// plus discrets.
var sensitiveFieldRe = regexp.MustCompile(
	`("(?:[a-z0-9]+_)*(?:email|name|address|phone|iban|qr_iban|password_hash)(?:_[a-z0-9]+)*"\s*:\s*)"[^"]*"`,
)

// maskPersonalData remplace la valeur des champs sensibles par "[MASKED]" dans
// une chaîne JSON (nLPD art. 6 — minimisation des données dans les journaux).
//
// Champs couverts : email, name, address, phone, iban, qr_iban, password_hash,
// et leurs variantes composées (company_name, address_street, …).
//
// # Conservée pour les chaînes déjà encodées, PAS pour la piste d'audit
//
// La piste passe désormais par `masquerEtat`, qui opère sur la structure. Cette
// fonction-ci reste pour les appelants qui n'ont qu'un texte JSON en main, avec
// la limite que le motif ne peut pas dépasser : il termine la valeur au premier
// guillemet ÉCHAPPÉ, donc casse sur `{"company_name":"Au \"Bon\" Vin Sàrl"}`.
func maskPersonalData(jsonData string) string {
	return sensitiveFieldRe.ReplaceAllString(jsonData, `${1}"[MASKED]"`)
}

// estChampSensible reconnaît un terme personnel comme MOT dans un nom de clé.
//
// Les mots sont délimités par des tirets bas et comparés hors casse :
// `company_name`, `address_street`, `Email`, `customerName` — cette dernière
// via la césure sur les majuscules — sont reconnus, `number` et `document_type`
// ne le sont pas.
func estChampSensible(cle string) bool {
	for _, mot := range motsDeLaCle(cle) {
		switch mot {
		case "email", "name", "address", "phone", "iban", "hash", "password":
			return true
		}
	}
	return false
}

// motsDeLaCle découpe un nom de clé en mots, puis les met en minuscules.
//
// `customer_name`, `customerName` et `CustomerName` donnent les mêmes mots : le
// masquage ne doit pas dépendre de la convention d'écriture de l'appelant.
//
// # Les acronymes comptent
//
// Couper à CHAQUE majuscule réduirait `IBAN` à « i », « b », « a », « n » — et
// la clé la plus sensible du produit traverserait le masquage intacte. La
// césure n'a donc lieu qu'aux VRAIES frontières : une majuscule qui suit une
// minuscule ou un chiffre (`customerName` → customer·Name), et une majuscule
// suivie d'une minuscule au sein d'une suite de majuscules (`IBANNumber` →
// IBAN·Number). `IBAN` reste ainsi un seul mot.
func motsDeLaCle(cle string) []string {
	runes := []rune(cle)
	var mots []string
	courant := strings.Builder{}
	vider := func() {
		if courant.Len() > 0 {
			mots = append(mots, strings.ToLower(courant.String()))
			courant.Reset()
		}
	}
	for i, r := range runes {
		if r == '_' || r == '-' {
			vider()
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			prec := runes[i-1]
			finDeMot := unicode.IsLower(prec) || unicode.IsDigit(prec)
			debutDApresAcronyme := unicode.IsUpper(prec) &&
				i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if finDeMot || debutDApresAcronyme {
				vider()
			}
		}
		courant.WriteRune(r)
	}
	vider()
	return mots
}

// masquerEtat remplace les valeurs sensibles DANS LA STRUCTURE, avant tout
// encodage (nLPD art. 6).
//
// # Pourquoi pas l'expression régulière
//
// Le masquage textuel terminait la valeur sur `[^"]*"`, donc au premier
// guillemet échappé produit par `json.Marshal`. Une raison sociale suisse
// banale — *Au « Bon » Vin Sàrl* — donnait :
//
//	{"company_name":"[MASKED]"Bon\" Vin Sarl","iban":"[MASKED]"}
//
// c'est-à-dire du JSON INVALIDE, avec un fragment du nom de l'indépendant resté
// EN CLAIR dans une table conservée dix ans (CO art. 958f). L'écran d'audit,
// qui fait `JSON.parse` puis `catch`, affichait alors « aucun champ modifié »
// sur un changement d'IBAN — le cas exact pour lequel l'audit différentiel
// existe. Et la chaîne d'empreintes restait valide, puisqu'elle porte sur la
// chaîne telle que stockée : le maillon corrompu se vérifiait, ce qui rendait
// le défaut invisible.
//
// On ne lit pas du JSON avec une expression régulière ; ici on n'a pas à
// essayer, la structure est disponible.
//
// # Ce qu'il couvre en plus
//
// Les valeurs NON textuelles (`"phone": 41791234567`) et les objets imbriqués,
// que le motif laissait traverser intacts parce qu'il n'accrochait que `"…"`.
func masquerEtat(etat map[string]any) map[string]any {
	if etat == nil {
		return nil
	}
	out := make(map[string]any, len(etat))
	for k, v := range etat {
		if estChampSensible(k) {
			out[k] = "[MASKED]"
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			out[k] = masquerEtat(t)
		case []any:
			out[k] = masquerListe(t)
		default:
			out[k] = v
		}
	}
	return out
}

// masquerListe applique le masquage aux objets d'une liste.
func masquerListe(l []any) []any {
	out := make([]any, len(l))
	for i, v := range l {
		switch t := v.(type) {
		case map[string]any:
			out[i] = masquerEtat(t)
		case []any:
			out[i] = masquerListe(t)
		default:
			out[i] = v
		}
	}
	return out
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
		return fmt.Errorf("une écriture comptable porte au moins deux lignes")
	}
	return nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

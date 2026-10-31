package accounting

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// L'empreinte des LIGNES d'une écriture.
//
// # Ce que la chaîne ne couvrait pas (audit croisé Gemini, CRIT-10)
//
// Le maillon d'audit d'une écriture comptabilisée portait ses TOTAUX (débit, crédit) et rien
// des lignes. Modifier une ligne — changer le compte 6000 en 6200 d'une charge, sans toucher au
// montant — ne changeait donc ni les totaux ni l'empreinte : la vérification de la chaîne
// répondait « intègre » sur une écriture dont le contenu avait changé, et seul un montant
// modifié se voyait, par le contrôle d'équilibre.
//
// Le condensé des lignes entre désormais dans l'état du maillon (`lines_digest`), donc dans
// l'empreinte chaînée : il ne se réécrit pas sans rompre la chaîne. Les écritures comptabilisées
// avant cette version n'en portent pas ; leurs lignes ne sont pas vérifiables, et on le dit au
// lieu de laisser croire le contraire.

// queryer : ce dont le calcul a besoin, que *sql.DB et *sql.Tx fournissent.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// LinesDigest rend le condensé SHA-256 des lignes de l'écriture : pour chaque ligne, dans
// l'ordre de leur séquence, le compte, le débit et le crédit EN CENTIMES, la séquence et le
// libellé. Le format est figé — il est recalculé tel quel des années plus tard.
func LinesDigest(ctx context.Context, q queryer, usePostgres bool, entryID string) (string, error) {
	rows, err := q.QueryContext(ctx, db.Rebind(`
		SELECT account_id, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0),
		       COALESCE(sequence, 0), COALESCE(description, '')
		FROM journal_lines WHERE entry_id = ? ORDER BY sequence, id`, usePostgres), entryID)
	if err != nil {
		return "", fmt.Errorf("lecture des lignes pour l'empreinte: %w", err)
	}
	defer rows.Close()

	h := sha256.New()
	for rows.Next() {
		var account, description string
		var debit, credit float64
		var sequence int
		if err := rows.Scan(&account, &debit, &credit, &sequence, &description); err != nil {
			return "", fmt.Errorf("lecture d'une ligne pour l'empreinte: %w", err)
		}
		fmt.Fprintf(h, "%s|%d|%d|%d|%s\n", account, compliance.Cents(debit), compliance.Cents(credit), sequence, description)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("lecture des lignes pour l'empreinte: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ErrLinesAltered : les lignes d'une écriture ne sont plus celles que le maillon d'audit a scellées.
type ErrLinesAltered struct {
	EntryID  string
	Expected string
	Got      string
}

func (e ErrLinesAltered) Error() string {
	return fmt.Sprintf("les lignes de l'écriture %s ne correspondent plus au maillon d'audit : condensé scellé %s, condensé actuel %s",
		e.EntryID, e.Expected, e.Got)
}

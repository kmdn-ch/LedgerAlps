package accounting

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Rattachement d'une écriture à son exercice comptable.
//
// Jusqu'à la v1.4.6, `fiscal_year_id` n'était renseigné nulle part : ni à la
// création d'une écriture, ni à celle d'une facture. Seule la clôture en posait
// un, sur l'écriture de clôture qu'elle créait elle-même.
//
// Les conséquences étaient silencieuses et graves. `CloseYear` sélectionne les
// soldes à virer avec `WHERE je.fiscal_year_id = ?` : ne trouvant rien, il
// clôturait l'exercice **sans produire aucune écriture de clôture**, tout en
// répondant « closed ». Le contrôle des brouillons, filtré de la même façon,
// laissait passer un exercice contenant des écritures non comptabilisées. Et
// l'archive légale filtrée par exercice revenait vide.
//
// Une clôture qui ne fait rien en annonçant le succès est pire qu'une clôture
// absente : personne ne va vérifier.

// ErrPeriodClosed est renvoyée lorsqu'une écriture viserait un exercice clos.
// L'exercice bouclé ne se corrige pas en le rouvrant : on passe une écriture
// dans l'exercice courant (CO art. 958f, Olico art. 3).
type ErrPeriodClosed struct {
	FiscalYear string
	Date       time.Time
}

func (e ErrPeriodClosed) Error() string {
	return fmt.Sprintf(
		"fiscal year %q is closed: no entry can be created or posted at %s — book the correction in the open period instead (CO art. 958f)",
		e.FiscalYear, e.Date.Format("2006-01-02"))
}

// execQuerier couvre *sql.DB comme *sql.Tx : le rattachement doit pouvoir se
// faire dans la transaction qui insère l'écriture, sans quoi une panne entre
// les deux laisserait une écriture orpheline de tout exercice.
type execQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// FiscalPeriod décrit l'exercice couvrant une date.
type FiscalPeriod struct {
	ID     string
	Name   string
	Closed bool
}

// LookupFiscalPeriod retourne l'exercice couvrant `date`, ou ok=false si aucun
// ne la couvre. Les dates sont comparées sous forme ISO `YYYY-MM-DD`, dont
// l'ordre lexicographique est l'ordre chronologique — vrai sur les deux moteurs.
func LookupFiscalPeriod(ctx context.Context, q execQuerier, usePostgres bool, date time.Time) (FiscalPeriod, bool, error) {
	d := date.Format("2006-01-02")
	query := db.Rebind(`
		SELECT id, name, is_closed
		FROM fiscal_years
		WHERE start_date <= ? AND end_date >= ?
		ORDER BY start_date DESC
		LIMIT 1`, usePostgres)

	var p FiscalPeriod
	var closed int
	err := q.QueryRowContext(ctx, query, d, d).Scan(&p.ID, &p.Name, &closed)
	if err == sql.ErrNoRows {
		return FiscalPeriod{}, false, nil
	}
	if err != nil {
		return FiscalPeriod{}, false, fmt.Errorf("lookup fiscal period: %w", err)
	}
	p.Closed = closed == 1
	return p, true, nil
}

// EnsureFiscalPeriod retourne l'exercice couvrant `date`, en créant l'année
// civile correspondante si aucun ne la couvre.
//
// Refuser l'écriture faute d'exercice serait plus pur, mais rendrait le produit
// inutilisable : aucun exercice n'est créé à l'installation et aucune route ne
// permettait d'en créer un. L'année civile est le choix par défaut de la grande
// majorité des PME suisses ; un exercice décalé se définit explicitement via
// `POST /fiscal-years`, **avant** d'y comptabiliser quoi que ce soit.
//
// La fonction renvoie ErrPeriodClosed si l'exercice trouvé est clos : c'est le
// verrouillage de période, et il vaut aussi pour une écriture antidatée.
func EnsureFiscalPeriod(ctx context.Context, q execQuerier, usePostgres bool, date time.Time) (FiscalPeriod, error) {
	p, found, err := LookupFiscalPeriod(ctx, q, usePostgres, date)
	if err != nil {
		return FiscalPeriod{}, err
	}
	if found {
		if p.Closed {
			return p, ErrPeriodClosed{FiscalYear: p.Name, Date: date}
		}
		return p, nil
	}

	year := date.Year()
	name := fmt.Sprintf("%d", year)
	id := db.NewID()
	insert := db.Rebind(`
		INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed)
		VALUES (?, ?, ?, ?, 0)`, usePostgres)

	if _, err := q.ExecContext(ctx, insert, id, name,
		fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year)); err != nil {
		// `name` est UNIQUE : une création concurrente, ou un exercice portant
		// déjà ce nom sur d'autres dates, nous ramène ici. On relit plutôt que
		// d'échouer — l'objectif est d'attacher l'écriture, pas de gagner la course.
		if p, found, lookupErr := LookupFiscalPeriod(ctx, q, usePostgres, date); lookupErr == nil && found {
			if p.Closed {
				return p, ErrPeriodClosed{FiscalYear: p.Name, Date: date}
			}
			return p, nil
		}
		return FiscalPeriod{}, fmt.Errorf("create fiscal year %s: %w", name, err)
	}

	return FiscalPeriod{ID: id, Name: name, Closed: false}, nil
}

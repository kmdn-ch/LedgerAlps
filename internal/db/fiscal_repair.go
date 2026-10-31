package db

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
)

// RepairFiscalYearDates remet sur quatre chiffres les bornes d'exercice que la
// base ne sait plus relire.
//
// # Le défaut réparé (audit 9, A-03)
//
// La création automatique d'un exercice écrivait `fmt.Sprintf("%d-01-01", année)`.
// Pour une année saisie « 26 » — un champ date HTML laisse taper deux chiffres
// dans le segment de l'année — cela donnait `26-01-01`, que le pilote ne relit
// plus dans un time.Time : l'écran des exercices et l'archive légale (CO
// art. 958f) répondaient 500. L'écriture comptabilisée à cette date ne se supprime
// pas (CO art. 957a) ; l'application ne pouvait donc rien réparer.
//
// Le correctif de création (`%04d` et bornes de date) empêche que cela se
// reproduise. Cette réparation corrige les bases où cela est déjà arrivé, en
// complétant l'année par des zéros : `26-01-01` devient `0026-01-01`. L'exercice
// reste celui de la date saisie, il redevient simplement lisible. L'écriture
// elle-même n'est pas touchée.
//
// Idempotente : une base saine ne subit qu'un SELECT. SQLite seulement — le
// moteur PostgreSQL n'est pas pris en charge par cette version.
func RepairFiscalYearDates(database *sql.DB, usePostgres bool) error {
	if usePostgres {
		return nil
	}
	todo, err := exercicesARepaires(database)
	if err != nil {
		return err
	}
	for _, r := range todo {
		if _, err := database.Exec(
			`UPDATE fiscal_years SET start_date = ?, end_date = ? WHERE id = ?`, r.start, r.end, r.id); err != nil {
			return fmt.Errorf("réparation de l'exercice %s: %w", r.id, err)
		}
	}
	if len(todo) > 0 {
		log.Printf("[migration] %d exercice(s) aux bornes illisibles réparé(s) (années complétées sur quatre chiffres)", len(todo))
	}
	return nil
}

type reparation struct{ id, start, end string }

// exercicesARepaires relit les exercices et rend ceux dont une borne est à réécrire.
// Le curseur se ferme par defer : il doit être fermé AVANT les UPDATE, SQLite
// n'aimant pas un curseur ouvert pendant une écriture.
func exercicesARepaires(database *sql.DB) ([]reparation, error) {
	rows, err := database.Query(
		`SELECT id, CAST(start_date AS TEXT), CAST(end_date AS TEXT) FROM fiscal_years`)
	if err != nil {
		return nil, fmt.Errorf("lecture des exercices: %w", err)
	}
	defer rows.Close()
	var todo []reparation
	for rows.Next() {
		var id, start, end string
		if err := rows.Scan(&id, &start, &end); err != nil {
			return nil, fmt.Errorf("lecture d'un exercice: %w", err)
		}
		s, e := padYear(start), padYear(end)
		if s != start || e != end {
			todo = append(todo, reparation{id, s, e})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lecture des exercices: %w", err)
	}
	return todo, nil
}

var reAnneeCourte = regexp.MustCompile(`^(\d{1,3})(-\d{2}-\d{2}.*)$`)

// padYear complète sur quatre chiffres une année écrite sur un à trois.
func padYear(s string) string {
	m := reAnneeCourte.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	return fmt.Sprintf("%04s%s", m[1], m[2])
}

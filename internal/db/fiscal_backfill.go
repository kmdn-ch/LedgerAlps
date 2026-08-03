package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// BackfillFiscalYears rattache à un exercice les écritures et factures qui n'en
// ont pas.
//
// Jusqu'à la v1.4.6, `fiscal_year_id` n'était jamais renseigné. Toute base
// existante contient donc des écritures orphelines, invisibles à la clôture qui
// filtre dessus — c'est ce qui faisait qu'un exercice se clôturait sans produire
// la moindre écriture de clôture, en répondant « closed ».
//
// Ce rattrapage est en Go et non dans une migration SQL parce qu'il doit
// extraire l'année d'une date : `strftime` n'existe pas sur PostgreSQL et
// `substr` ne s'applique pas à une colonne DATE. Le faire ici garde une seule
// implémentation valable sur les deux moteurs.
//
// Idempotent : seules les lignes à NULL sont touchées. Sur une base à jour, il
// exécute deux SELECT et s'arrête.
func BackfillFiscalYears(database *sql.DB, usePostgres bool) error {
	years, err := collectYears(database, usePostgres)
	if err != nil {
		return err
	}
	if len(years) == 0 {
		return nil
	}

	created := 0
	for _, y := range years {
		ok, err := ensureCalendarYear(database, usePostgres, y)
		if err != nil {
			return err
		}
		if ok {
			created++
		}
	}

	entries, err := attachOrphans(database, usePostgres, "journal_entries", "date")
	if err != nil {
		return err
	}
	invoices, err := attachOrphans(database, usePostgres, "invoices", "issue_date")
	if err != nil {
		return err
	}

	if entries > 0 || invoices > 0 || created > 0 {
		log.Printf("[migration] rattachement à l'exercice : %d écriture(s), %d document(s), %d exercice(s) créé(s)",
			entries, invoices, created)
	}
	return nil
}

// collectYears retourne les années civiles concernées par des lignes orphelines.
func collectYears(database *sql.DB, usePostgres bool) ([]int, error) {
	seen := map[int]bool{}
	for _, q := range []struct{ table, col string }{
		{"journal_entries", "date"},
		{"invoices", "issue_date"},
	} {
		query := fmt.Sprintf(
			"SELECT %s FROM %s WHERE fiscal_year_id IS NULL", q.col, q.table)
		rows, err := database.Query(query)
		if err != nil {
			return nil, fmt.Errorf("collect %s: %w", q.table, err)
		}
		func() {
			defer rows.Close()
			for rows.Next() {
				var d time.Time
				if err := rows.Scan(&d); err != nil {
					// Une date illisible ne doit pas empêcher le démarrage :
					// la ligne restera orpheline et le contrôle de cohérence
					// la signalera, ce qui vaut mieux qu'un serveur qui refuse
					// de se lancer.
					log.Printf("[migration] date illisible dans %s, ligne ignorée: %v", q.table, err)
					continue
				}
				seen[d.Year()] = true
			}
			err = rows.Err()
		}()
		if err != nil {
			return nil, fmt.Errorf("collect %s: %w", q.table, err)
		}
	}

	out := make([]int, 0, len(seen))
	for y := range seen {
		out = append(out, y)
	}
	return out, nil
}

// ensureCalendarYear crée l'exercice couvrant l'année civile `year` s'il
// n'existe aucun exercice couvrant le 1ᵉʳ janvier de cette année. Retourne
// true s'il a été créé.
//
// Le test porte sur la *couverture*, pas sur le nom : une PME ayant défini un
// exercice juillet–juin ne doit pas se voir ajouter une année civile qui le
// chevaucherait.
func ensureCalendarYear(database *sql.DB, usePostgres bool, year int) (bool, error) {
	jan := fmt.Sprintf("%d-01-01", year)
	dec := fmt.Sprintf("%d-12-31", year)

	var n int
	checkQ := Rebind(
		"SELECT COUNT(*) FROM fiscal_years WHERE start_date <= ? AND end_date >= ?", usePostgres)
	if err := database.QueryRow(checkQ, dec, jan).Scan(&n); err != nil {
		return false, fmt.Errorf("check fiscal year %d: %w", year, err)
	}
	if n > 0 {
		return false, nil
	}

	insertQ := Rebind(`
		INSERT INTO fiscal_years (id, name, start_date, end_date, is_closed)
		VALUES (?, ?, ?, ?, 0)`, usePostgres)
	if _, err := database.Exec(insertQ, NewID(), fmt.Sprintf("%d", year), jan, dec); err != nil {
		return false, fmt.Errorf("create fiscal year %d: %w", year, err)
	}
	return true, nil
}

// attachOrphans rattache les lignes sans exercice à celui qui couvre leur date.
func attachOrphans(database *sql.DB, usePostgres bool, table, dateCol string) (int64, error) {
	// Sous-requête corrélée : portable SQLite/PostgreSQL, et sans effet sur une
	// ligne dont la date n'est couverte par aucun exercice — elle reste NULL
	// plutôt que d'être rattachée au hasard.
	q := Rebind(fmt.Sprintf(`
		UPDATE %s
		SET fiscal_year_id = (
			SELECT f.id FROM fiscal_years f
			WHERE f.start_date <= %s.%s AND f.end_date >= %s.%s
			ORDER BY f.start_date DESC
			LIMIT 1
		)
		WHERE fiscal_year_id IS NULL`, table, table, dateCol, table, dateCol), usePostgres)

	res, err := database.Exec(q)
	if err != nil {
		return 0, fmt.Errorf("attach %s: %w", table, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil // certains pilotes ne le rapportent pas ; sans conséquence
	}
	return n, nil
}

package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
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

// ─── Liste blanche des tables rattachables à un exercice ────────────────────
//
// Les requêtes de ce fichier interpolent un NOM DE TABLE et un NOM DE COLONNE
// avec `fmt.Sprintf` — ce que les paramètres liés ne peuvent pas faire : un `?`
// porte une valeur, jamais un identifiant.
//
// Aujourd'hui aucune entrée externe n'atteint ces paramètres : les deux sites
// d'appel passent des littéraux, et `BackfillFiscalYears` n'est appelée qu'une
// fois au démarrage du serveur, jamais depuis une route HTTP. Le risque est
// celui d'une réutilisation future — un `?table=` branché là un jour, et
// l'injection serait immédiate et invisible aux tests actuels.
//
// La liste blanche ferme la porte par construction plutôt que par vigilance :
// un nom hors de cet ensemble est refusé, il ne peut pas atteindre le SQL.
type tableRattachable struct{ table, dateCol string }

var tablesRattachables = []tableRattachable{
	{"journal_entries", "date"},
	{"invoices", "issue_date"},
}

// resoudreTable refuse tout couple qui n'est pas dans la liste blanche.
//
// Elle rend le couple issu de la CONSTANTE, jamais celui reçu en argument :
// même si l'appelant fournissait une chaîne équivalente construite autrement,
// c'est le littéral du programme qui part dans la requête.
func resoudreTable(table, dateCol string) (tableRattachable, error) {
	for _, t := range tablesRattachables {
		if t.table == table && t.dateCol == dateCol {
			return t, nil
		}
	}
	return tableRattachable{}, fmt.Errorf(
		"table non rattachable à un exercice: %q/%q — cette fonction n'accepte que "+
			"des identifiants figés dans le programme, jamais une valeur reçue", table, dateCol)
}

// collectYears retourne les années civiles concernées par des lignes orphelines.
func collectYears(database *sql.DB, usePostgres bool) ([]int, error) {
	seen := map[int]bool{}
	for _, q := range tablesRattachables {
		query := fmt.Sprintf(
			"SELECT %s FROM %s WHERE fiscal_year_id IS NULL", q.dateCol, q.table)
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
	// Les identifiants passent par la liste blanche AVANT d'atteindre le SQL.
	t, err := resoudreTable(table, dateCol)
	if err != nil {
		return 0, err
	}
	table, dateCol = t.table, t.dateCol

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

// BackfillInvoiceRecipients renseigne l'identité du destinataire sur les
// factures qui n'en portent pas.
//
// Avant la v1.4.6 une facture ne stockait que `contact_id`, si bien que le PDF
// relisait le contact vivant : renommer un client réécrivait rétroactivement
// toutes ses factures passées. Ces factures-là ne peuvent pas être restituées
// telles qu'elles ont été imprimées — la seule source disponible est le contact
// d'aujourd'hui. Elles sont donc marquées `recipient_backfilled = 1`, parce
// qu'une reconstitution et une pièce d'origine ne se valent pas devant un
// réviseur, et que confondre les deux serait le vrai défaut.
//
// Idempotent : seules les lignes sans identité sont touchées.
func BackfillInvoiceRecipients(database *sql.DB, usePostgres bool) error {
	q := Rebind(`
		UPDATE invoices
		SET recipient_name        = COALESCE((SELECT c.name        FROM contacts c WHERE c.id = invoices.contact_id), ''),
		    recipient_address     = COALESCE((SELECT c.address     FROM contacts c WHERE c.id = invoices.contact_id), ''),
		    recipient_postal_code = COALESCE((SELECT c.postal_code FROM contacts c WHERE c.id = invoices.contact_id), ''),
		    recipient_city        = COALESCE((SELECT c.city        FROM contacts c WHERE c.id = invoices.contact_id), ''),
		    recipient_country     = COALESCE((SELECT c.country     FROM contacts c WHERE c.id = invoices.contact_id), ''),
		    recipient_vat_number  = COALESCE((SELECT c.vat_number  FROM contacts c WHERE c.id = invoices.contact_id), ''),
		    recipient_backfilled  = 1
		WHERE recipient_name IS NULL OR recipient_name = ''`, usePostgres)

	res, err := database.Exec(q)
	if err != nil {
		return fmt.Errorf("backfill invoice recipients: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		log.Printf("[migration] identité du destinataire reconstituée sur %d facture(s) — "+
			"depuis la fiche contact actuelle, faute d'instantané d'époque", n)
	}
	return nil
}

// BackfillSupplierReversalMarkers marque comme extourne les écritures
// d'annulation FOURNISSEUR passées avant le troisième audit, qui n'étaient
// pas conformes : `is_reversal` restait à 0 et `reversal_of_id` à NULL, alors
// que le chemin des factures CLIENTES renseignait déjà les deux.
//
// Ces deux colonnes partent dans l'archive légale (CO art. 958f) remise à la
// fiduciaire. Une extourne fournisseur non marquée s'y lisait comme une
// écriture ordinaire — bien qu'elle se décrive elle-même comme une extourne
// dans son libellé — sans qu'aucun lien ne pointe vers l'écriture qu'elle
// annule.
//
// La migration 0028 ouvre l'exception, étroite et à sens unique, qui rend
// cette réparation possible malgré le déclencheur d'immuabilité
// (CO art. 957a). Le code applicatif est corrigé séparément
// (supplier_cancel.go) : ce rattrapage ne concerne que ce qui a déjà été
// écrit avec le défaut.
//
// # Comment une extourne candidate est identifiée
//
// Le libellé d'une extourne fournisseur suit un format fixe
// (supplier_cancel.go) : « Extourne facture fournisseur <référence> —
// <motif> ». Aucun autre chemin du dépôt ne produit ce préfixe — celui des
// factures clientes écrit « Contrepassation facture <numéro> ».
//
// # Comment elle est rattachée à SON écriture d'origine
//
// La comparaison se fait en Go, par préfixe exact — jamais par SQL LIKE sur
// une référence non échappée, qui contiendrait des caractères génériques
// (`%`, `_`) si le fournisseur les avait utilisés dans sa numérotation.
// `supplier_invoices.journal_entry_id` continue de pointer vers l'écriture
// D'ORIGINE après l'annulation (jamais réécrit) : c'est la source du
// rattachement.
//
// Une candidate qui ne trouve AUCUNE origine, ou qui en trouve PLUSIEURS
// (deux fournisseurs différents ayant utilisé la même référence et le même
// motif), n'est pas touchée : deviner un lien comptable serait pire que n'en
// écrire aucun, et le cas est journalisé pour être traité à la main.
//
// Idempotent : une candidate corrigée porte `is_reversal = 1` et disparaît de
// la sélection au passage suivant.
func BackfillSupplierReversalMarkers(database *sql.DB, usePostgres bool) error {
	if usePostgres {
		// L'exception posée par la migration 0028 est écrite en syntaxe de
		// déclencheur SQLite. PostgreSQL n'est pas le moteur exercé par ce
		// rattrapage : ne rien faire plutôt que deviner une syntaxe non
		// vérifiée sur une opération qui touche des écritures comptabilisées.
		return nil
	}

	origines, err := collectCancelledSupplierEntries(database)
	if err != nil {
		return fmt.Errorf("collect cancelled supplier invoices: %w", err)
	}
	if len(origines) == 0 {
		return nil
	}

	candidates, err := collectLegacySupplierReversals(database)
	if err != nil {
		return err
	}

	fixed, ambiguous, orphelines := 0, 0, 0
	for _, c := range candidates {
		trouve := ""
		multiple := false
		for _, o := range origines {
			prefixe := "Extourne facture fournisseur " + o.reference + " — "
			if strings.HasPrefix(c.description, prefixe) {
				if trouve != "" && trouve != o.entryID {
					multiple = true
					break
				}
				trouve = o.entryID
			}
		}
		switch {
		case multiple:
			ambiguous++
			log.Printf("[migration] extourne fournisseur %s : plusieurs origines possibles, "+
				"laissée telle quelle — %q", c.id, c.description)
		case trouve == "":
			orphelines++
			log.Printf("[migration] extourne fournisseur %s : aucune origine trouvée, "+
				"laissée telle quelle — %q", c.id, c.description)
		default:
			updQ := Rebind(
				`UPDATE journal_entries SET is_reversal = 1, reversal_of_id = ? WHERE id = ?`,
				usePostgres)
			if _, err := database.Exec(updQ, trouve, c.id); err != nil {
				return fmt.Errorf("mark supplier reversal %s: %w", c.id, err)
			}
			fixed++
		}
	}

	if fixed > 0 || ambiguous > 0 || orphelines > 0 {
		log.Printf("[migration] extournes fournisseur marquées : %d corrigée(s), "+
			"%d ambiguë(s), %d sans origine trouvée", fixed, ambiguous, orphelines)
	}
	return nil
}

type legacySupplierReversal struct{ id, description string }

// collectLegacySupplierReversals rend les écritures d'extourne fournisseur
// non marquées — celles au libellé caractéristique et à `is_reversal = 0`.
func collectLegacySupplierReversals(database *sql.DB) ([]legacySupplierReversal, error) {
	rows, err := database.Query(`
		SELECT id, description FROM journal_entries
		WHERE is_reversal = 0 AND description LIKE 'Extourne facture fournisseur %'`)
	if err != nil {
		return nil, fmt.Errorf("collect legacy supplier reversals: %w", err)
	}
	defer rows.Close()

	var out []legacySupplierReversal
	for rows.Next() {
		var c legacySupplierReversal
		if err := rows.Scan(&c.id, &c.description); err != nil {
			return nil, fmt.Errorf("scan legacy supplier reversal: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("collect legacy supplier reversals: %w", err)
	}
	return out, nil
}

type supplierOrigine struct{ reference, entryID string }

// collectCancelledSupplierEntries rend, pour chaque facture fournisseur
// annulée, la référence telle qu'elle a été saisie et l'écriture d'origine
// que la comptabilisation avait laissée — jamais réécrite par l'annulation.
func collectCancelledSupplierEntries(database *sql.DB) ([]supplierOrigine, error) {
	rows, err := database.Query(`
		SELECT supplier_reference, journal_entry_id FROM supplier_invoices
		WHERE status = 'cancelled' AND journal_entry_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []supplierOrigine
	for rows.Next() {
		var o supplierOrigine
		if err := rows.Scan(&o.reference, &o.entryID); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

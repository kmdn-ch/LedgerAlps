package handlers

// Journal général, grand livre et balance, en CSV.
//
// Ces trois exports figuraient dans l'écran Rapports depuis l'origine, sous une
// pastille « Bientôt disponible », avec un bouton désactivé et un gestionnaire
// vide. Ce n'étaient pas des fonctions en panne : c'étaient des maquettes que
// rien n'avait jamais reliées à quoi que ce soit.
//
// # Pourquoi ces trois-là et pas d'autres
//
// Ce sont les trois documents qu'une fiduciaire réclame, et les trois que le CO
// impose de pouvoir présenter (art. 957a al. 2 ch. 2 et 3, art. 958f). Le
// journal dit ce qui s'est passé dans l'ordre, le grand livre le range par
// compte, la balance prouve que les deux s'équilibrent. Aucun ne remplace les
// autres.
//
// # Seules les écritures COMPTABILISÉES y figurent
//
// Un brouillon n'est scellé par rien et reste modifiable : le porter dans un
// document remis à un tiers reviendrait à lui présenter un chiffre qui peut
// encore changer.
//
// # Le point-virgule et le BOM
//
// Excel en configuration suisse ou française lit un CSV séparé par des
// point-virgules ; la virgule y produit une seule colonne. Le BOM UTF-8 en tête
// est ce qui fait qu'Excel affiche « Genève » plutôt que « GenÃ¨ve ». Les deux
// sont des concessions à l'outil qui ouvrira le fichier — c'est lui qui compte,
// pas la pureté du format.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

type AccountingExportHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewAccountingExportHandler(database *sql.DB, usePostgres bool) *AccountingExportHandler {
	return &AccountingExportHandler{db: database, usePostgres: usePostgres}
}

// writeCSV envoie le fichier avec ce qu'il faut pour qu'Excel l'ouvre droit.
func writeCSV(c *gin.Context, filename string, rows [][]string) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // BOM UTF-8 : sans lui, Excel casse les accents
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "écriture du CSV: " + err.Error()})
			return
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "écriture du CSV: " + err.Error()})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// money rend un montant avec un point décimal et deux décimales.
//
// Sans séparateur de milliers : un CSV se recalcule, il ne se lit pas. Une
// apostrophe suisse dans un nombre en fait du texte pour tout tableur.
func money(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// periodFromQuery lit la période, en refusant une date mal formée plutôt que de
// l'ignorer : un export silencieusement non filtré est plus trompeur qu'une
// erreur, parce qu'il a l'air complet.
func periodFromQuery(c *gin.Context) (from, to string, ok bool) {
	from, to = c.Query("from"), c.Query("to")
	for _, d := range []string{from, to} {
		if d == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "les dates doivent être au format AAAA-MM-JJ"})
			return "", "", false
		}
	}
	return from, to, true
}

func periodWhere(from, to string, args *[]any) string {
	where := ""
	if from != "" {
		where += " AND e.date >= ?"
		*args = append(*args, from)
	}
	if to != "" {
		where += " AND e.date <= ?"
		*args = append(*args, to)
	}
	return where
}

func periodSuffix(from, to string) string {
	switch {
	case from != "" && to != "":
		return from + "_" + to
	case from != "":
		return "depuis-" + from
	case to != "":
		return "jusqu-" + to
	default:
		return time.Now().Format("2006-01-02")
	}
}

// ─── Journal général ─────────────────────────────────────────────────────────

// ExportJournalCSV GET /api/v1/exports/journal.csv?from=&to=
//
// Une ligne par ligne d'écriture, dans l'ordre chronologique. La référence et la
// description sont répétées sur chaque ligne du même document : un tableur trie
// et filtre, et une valeur laissée vide « parce qu'elle est au-dessus »
// disparaît au premier tri.
func (h *AccountingExportHandler) ExportJournalCSV(c *gin.Context) {
	from, to, ok := periodFromQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	args := []any{}
	q := `
		SELECT e.reference, e.date, e.description,
		       a.code, a.name,
		       COALESCE(l.debit_amount, 0), COALESCE(l.credit_amount, 0),
		       COALESCE(l.description, ''), COALESCE(u.name, ''),
		       COALESCE(e.integrity_hash, '')
		FROM journal_entries e
		JOIN journal_lines l ON l.entry_id = e.id
		JOIN accounts a ON a.id = l.account_id
		LEFT JOIN users u ON u.id = e.created_by_id
		WHERE e.status = 'posted'` + periodWhere(from, to, &args) + `
		ORDER BY e.date, e.reference, l.sequence, a.code`

	rows, err := h.db.QueryContext(ctx, db.Rebind(q, h.usePostgres), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	out := [][]string{{
		"Reference", "Date", "Description", "Compte", "Libelle compte",
		"Debit CHF", "Credit CHF", "Libelle ligne", "Auteur", "Empreinte",
	}}
	var totalD, totalC float64
	for rows.Next() {
		var ref, date, desc, code, aname, ldesc, author, hash string
		var d, cr float64
		if err := rows.Scan(&ref, &date, &desc, &code, &aname, &d, &cr,
			&ldesc, &author, &hash); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		totalD += d
		totalC += cr
		out = append(out, []string{
			ref, firstTen(date), desc, code, aname,
			money(d), money(cr), ldesc, author, hash,
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}
	// La ligne de total est ce qui permet de contrôler que le fichier est
	// complet : un export tronqué se repère à un total qui ne s'équilibre plus.
	out = append(out, []string{"TOTAL", "", "", "", "", money(totalD), money(totalC), "", "", ""})

	nom := "journal_" + periodSuffix(from, to) + ".csv"
	traceExport(c, h.db, h.usePostgres, nom, "csv",
		map[string]any{"du": from, "au": to, "lignes": len(out)})
	writeCSV(c, nom, out)
}

// ─── Grand livre ─────────────────────────────────────────────────────────────

// ExportLedgerCSV GET /api/v1/exports/ledger.csv?from=&to=
//
// Les mêmes mouvements, rangés par compte, avec le solde cumulé après chaque
// ligne. C'est ce cumul qui fait la différence avec le journal : il permet de
// suivre l'évolution d'un compte et de pointer un solde à une date donnée.
func (h *AccountingExportHandler) ExportLedgerCSV(c *gin.Context) {
	from, to, ok := periodFromQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	args := []any{}
	q := `
		SELECT a.code, a.name, a.account_type, e.date, e.reference, e.description,
		       COALESCE(l.debit_amount, 0), COALESCE(l.credit_amount, 0)
		FROM journal_lines l
		JOIN journal_entries e ON e.id = l.entry_id
		JOIN accounts a ON a.id = l.account_id
		WHERE e.status = 'posted'` + periodWhere(from, to, &args) + `
		ORDER BY a.code, e.date, e.reference`

	rows, err := h.db.QueryContext(ctx, db.Rebind(q, h.usePostgres), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	out := [][]string{{
		"Compte", "Libelle compte", "Type", "Date", "Reference", "Description",
		"Debit CHF", "Credit CHF", "Solde cumule CHF",
	}}
	current := ""
	var solde float64
	for rows.Next() {
		var code, aname, atype, date, ref, desc string
		var d, cr float64
		if err := rows.Scan(&code, &aname, &atype, &date, &ref, &desc, &d, &cr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		// Le cumul repart à zéro à chaque compte : un solde qui traverserait
		// deux comptes ne voudrait rien dire.
		if code != current {
			current, solde = code, 0
		}
		solde += d - cr
		out = append(out, []string{
			code, aname, accountTypeLabel(atype), firstTen(date), ref, desc,
			money(d), money(cr), money(solde),
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	nom := "grand-livre_" + periodSuffix(from, to) + ".csv"
	traceExport(c, h.db, h.usePostgres, nom, "csv",
		map[string]any{"du": from, "au": to, "lignes": len(out)})
	writeCSV(c, nom, out)
}

// ─── Balance de vérification ─────────────────────────────────────────────────

// ExportTrialBalanceCSV GET /api/v1/exports/trial-balance.csv?to=
//
// Totaux par compte à une date. C'est le document de contrôle : si débit et
// crédit ne s'équilibrent pas, les livres ont un problème, et il vaut mieux le
// voir sur une ligne de total que le découvrir six mois plus tard.
func (h *AccountingExportHandler) ExportTrialBalanceCSV(c *gin.Context) {
	from, to, ok := periodFromQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	args := []any{}
	// Le CASE, et non la seule condition de jointure : celle-ci décide si
	// l'écriture est rattachée, pas si la ligne est retenue. Sans lui, les
	// brouillons entreraient dans la balance — le défaut corrigé en v1.4.8.
	q := `
		SELECT a.code, a.name, a.account_type,
		       COALESCE(SUM(CASE WHEN e.id IS NOT NULL THEN l.debit_amount  END), 0),
		       COALESCE(SUM(CASE WHEN e.id IS NOT NULL THEN l.credit_amount END), 0)
		FROM accounts a
		LEFT JOIN journal_lines l ON l.account_id = a.id
		LEFT JOIN journal_entries e ON e.id = l.entry_id AND e.status = 'posted'` +
		// La periode appartient a la condition de JOINTURE, pas au WHERE : dans
		// le WHERE elle eliminerait les comptes sans mouvement sur la periode,
		// alors qu'ils doivent apparaitre a zero.
		periodWhere(from, to, &args) + `
		WHERE a.is_active = 1
		GROUP BY a.id, a.code, a.name, a.account_type
		ORDER BY a.code`

	rows, err := h.db.QueryContext(ctx, db.Rebind(q, h.usePostgres), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	out := [][]string{{"Compte", "Libelle", "Type", "Debit CHF", "Credit CHF", "Solde CHF"}}
	var totalD, totalC float64
	for rows.Next() {
		var code, name, atype string
		var d, cr float64
		if err := rows.Scan(&code, &name, &atype, &d, &cr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		// Les comptes sans mouvement sont omis : soixante-dix-huit lignes à zéro
		// noient le contrôle, et un compte à zéro n'apprend rien.
		if d == 0 && cr == 0 {
			continue
		}
		totalD += d
		totalC += cr
		out = append(out, []string{
			code, name, accountTypeLabel(atype), money(d), money(cr), money(d - cr),
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}
	out = append(out, []string{"TOTAL", "", "", money(totalD), money(totalC), money(totalD - totalC)})

	nom := "balance_" + periodSuffix(from, to) + ".csv"
	traceExport(c, h.db, h.usePostgres, nom, "csv",
		map[string]any{"du": from, "au": to, "lignes": len(out)})
	writeCSV(c, nom, out)
}

// accountTypeLabel traduit le type de compte. Un CSV destiné à une fiduciaire
// suisse ne doit pas dire « asset ».
func accountTypeLabel(t string) string {
	switch t {
	case "asset":
		return "Actif"
	case "liability":
		return "Passif"
	case "equity":
		return "Capitaux propres"
	case "revenue":
		return "Produits"
	case "expense":
		return "Charges"
	default:
		return t
	}
}

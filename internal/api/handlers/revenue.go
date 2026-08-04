package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Chiffre d'affaires, groupable par année, par mois ou par client.
//
// La série mensuelle du tableau de bord est figée à six mois glissants et sert
// à dessiner une courbe. Elle ne répond pas aux questions qu'on se pose au
// moment de préparer une déclaration ou de relancer quelqu'un : « combien
// ai-je facturé en 2025 ? », « quel client représente quelle part ? ».
//
// Ce que ces chiffres comptent, et pourquoi :
//
//   - Les **brouillons** sont exclus : un document non émis ne représente aucun
//     produit, et l'inclure gonflerait le chiffre d'affaires d'un montant que
//     personne ne doit.
//   - Les **annulées** aussi, pour la même raison.
//   - Les **offres de prix** aussi : personne ne doit rien sur une offre.
//   - Les **notes de crédit** sont soustraites (LTVA art. 41 — modification
//     ultérieure de la dette d'impôt). Les montants sont stockés non signés
//     pour que le document se lise naturellement ; le signe s'applique ici.
//     Les additionner reviendrait à compter deux fois une vente annulée.
//
// C'est la même convention que la déclaration TVA. Deux règles différentes
// donneraient deux chiffres d'affaires pour la même période, et l'utilisateur
// n'aurait aucun moyen de savoir lequel croire.

type revenueRow struct {
	Key      string  `json:"key"`   // "2026", "2026-03", ou l'identifiant du contact
	Label    string  `json:"label"` // ce qui s'affiche
	Invoiced float64 `json:"invoiced"`
	Paid     float64 `json:"paid"`
	Count    int     `json:"count"`
}

type revenueResponse struct {
	GroupBy  string       `json:"group_by"`
	From     string       `json:"from,omitempty"`
	To       string       `json:"to,omitempty"`
	Rows     []revenueRow `json:"rows"`
	Invoiced float64      `json:"total_invoiced"`
	Paid     float64      `json:"total_paid"`
	// Basis énonce la convention retenue. Un total sans sa définition invite à
	// le comparer à un autre calculé autrement.
	Basis string `json:"basis"`
}

// Revenue GET /api/v1/reports/revenue?group_by=year|month|contact&from=&to=
func (h *ReportsHandler) Revenue(c *gin.Context) {
	groupBy := c.DefaultQuery("group_by", "month")
	switch groupBy {
	case "year", "month", "contact":
	default:
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "group_by doit valoir year, month ou contact"})
		return
	}

	from, to := c.Query("from"), c.Query("to")
	for name, v := range map[string]string{"from": from, "to": to} {
		if v == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", v); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": name + " doit être au format AAAA-MM-JJ"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	rows, totals, err := h.revenueRows(ctx, groupBy, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, revenueResponse{
		GroupBy: groupBy, From: from, To: to,
		Rows:     rows,
		Invoiced: totals[0], Paid: totals[1],
		Basis: "Factures émises (hors brouillons et annulées), notes de crédit déduites. " +
			"Les offres de prix ne comptent pas : personne ne doit rien dessus. " +
			"Même convention que la déclaration TVA.",
	})
}

func (h *ReportsHandler) revenueRows(
	ctx context.Context, groupBy, from, to string,
) ([]revenueRow, [2]float64, error) {
	var totals [2]float64

	// Expression de regroupement. `strftime` n'existe pas sur PostgreSQL et
	// `to_char` pas sur SQLite : les deux moteurs sont donc traités
	// explicitement plutôt que par une astuce qui marcherait sur un seul.
	var keyExpr, labelJoin, groupCols, orderBy string
	switch groupBy {
	case "year":
		if h.usePostgres {
			keyExpr = "to_char(i.issue_date, 'YYYY')"
		} else {
			keyExpr = "strftime('%Y', i.issue_date)"
		}
		groupCols, orderBy = keyExpr, "1 DESC"
	case "month":
		if h.usePostgres {
			keyExpr = "to_char(i.issue_date, 'YYYY-MM')"
		} else {
			keyExpr = "strftime('%Y-%m', i.issue_date)"
		}
		groupCols, orderBy = keyExpr, "1 DESC"
	case "contact":
		keyExpr = "i.contact_id"
		// Le nom vient de la fiche contact, pas de l'identité figée sur la
		// facture : ce tableau sert à savoir avec QUI on travaille aujourd'hui.
		// Un contact anonymisé y apparaît donc sous son libellé d'anonymat, ce
		// qui est la réponse correcte à « qui est ce client ? » après un
		// effacement.
		labelJoin = " LEFT JOIN contacts ct ON ct.id = i.contact_id"
		groupCols, orderBy = "i.contact_id, ct.name", "invoiced DESC"
	}

	where := ` WHERE i.document_type IN ('invoice', 'credit_note')
	             AND i.status NOT IN ('draft', 'cancelled')`
	args := []any{}
	if from != "" {
		where += " AND i.issue_date >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND i.issue_date <= ?"
		args = append(args, to)
	}

	label := keyExpr
	if groupBy == "contact" {
		label = "COALESCE(ct.name, '')"
	}

	// Le signe négatif porté par les notes de crédit est appliqué ici, comme
	// dans la déclaration TVA : les montants sont stockés non signés.
	q := db.Rebind(fmt.Sprintf(`
		SELECT %s AS k,
		       %s AS lbl,
		       COALESCE(SUM(CASE WHEN i.document_type = 'credit_note'
		                         THEN -i.total_amount ELSE i.total_amount END), 0) AS invoiced,
		       COALESCE(SUM(CASE WHEN i.document_type = 'credit_note'
		                         THEN -i.amount_paid ELSE i.amount_paid END), 0)   AS paid,
		       COUNT(*) AS n
		FROM invoices i%s%s
		GROUP BY %s
		ORDER BY %s`, keyExpr, label, labelJoin, where, groupCols, orderBy), h.usePostgres)

	sqlRows, err := h.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, totals, err
	}
	defer sqlRows.Close()

	out := []revenueRow{}
	for sqlRows.Next() {
		var r revenueRow
		if err := sqlRows.Scan(&r.Key, &r.Label, &r.Invoiced, &r.Paid, &r.Count); err != nil {
			return nil, totals, err
		}
		if r.Label == "" {
			// Un contact supprimé de la base laisserait une ligne sans nom.
			// « (contact inconnu) » vaut mieux qu'une case vide, qui se lit
			// comme un défaut d'affichage.
			r.Label = "(contact inconnu)"
		}
		totals[0] += r.Invoiced
		totals[1] += r.Paid
		out = append(out, r)
	}
	return out, totals, sqlRows.Err()
}

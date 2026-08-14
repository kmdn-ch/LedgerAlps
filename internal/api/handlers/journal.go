package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// journalListItem est ce que la liste rend réellement.
//
// models.JournalEntry porte un tableau de lignes que la liste ne remplissait
// pas — l'interface était censée additionner des lignes absentes, et la colonne
// « Montant » restait vide. Un type propre à la liste dit ce qu'elle contient
// vraiment, plutôt que de promettre un champ qu'elle ne rend jamais.
type journalListItem struct {
	ID          string    `json:"id"`
	Reference   string    `json:"reference"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	IsReversal  bool      `json:"is_reversal"`
	// Total des débits. Dans une écriture équilibrée il vaut celui des crédits ;
	// additionner les deux doublerait le montant.
	Total  float64 `json:"total"`
	Author string  `json:"author"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// journalLineView est une ligne telle qu'on la lit, avec le compte en clair.
//
// L'identifiant du compte ne dit rien à personne : c'est le numéro et le nom
// qui permettent de contrôler une écriture. Les rendre ici évite à l'interface
// de recharger le plan comptable pour traduire chaque ligne.
type journalLineView struct {
	ID           string  `json:"id"`
	AccountID    string  `json:"account_id"`
	AccountCode  string  `json:"account_code"`
	AccountName  string  `json:"account_name"`
	DebitAmount  float64 `json:"debit_amount"`
	CreditAmount float64 `json:"credit_amount"`
	Description  string  `json:"description"`
	Sequence     int     `json:"sequence"`
}

type JournalHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewJournalHandler(database *sql.DB, usePostgres bool) *JournalHandler {
	return &JournalHandler{db: database, usePostgres: usePostgres}
}

// ListJournal godoc
// GET /api/v1/journal?page=1&page_size=20&date_from=&date_to=&status=&reference=
func (h *JournalHandler) ListJournal(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	status := c.Query("status")
	reference := c.Query("reference")

	where := " WHERE 1=1"
	args := []any{}

	// Le journal est UN registre, pas une boîte de réception.
	//
	// Ce filtre restreignait la liste aux écritures créées par le compte
	// connecté, sous couvert de minimisation des données. C'était un contresens
	// comptable : le journal doit être complet et se rapprocher de la balance
	// (CO art. 957a al. 2 ch. 2 et 3), et un comptable qui n'y voit que ses
	// propres écritures ne peut ni contrôler ni boucler. Deux personnes
	// travaillant sur les mêmes livres voyaient deux journaux différents, tous
	// deux en désaccord avec le bilan.
	//
	// Ce qui protège vraiment ici est le rôle : les écritures se lisent avec la
	// permission de lecture, s'écrivent avec celle d'écriture comptable, et un
	// compte en lecture seule ne peut rien y ajouter. La minimisation nLPD
	// porte sur les données personnelles, pas sur les pièces comptables que la
	// loi oblige justement à conserver dix ans (CO art. 958f).

	if dateFrom != "" {
		if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "date_from doit être au format AAAA-MM-JJ"})
			return
		}
		where += " AND e.date >= ?"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		if _, err := time.Parse("2006-01-02", dateTo); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "date_to doit être au format AAAA-MM-JJ"})
			return
		}
		where += " AND e.date <= ?"
		args = append(args, dateTo)
	}
	if status != "" {
		where += " AND e.status = ?"
		args = append(args, status)
	}
	if reference != "" {
		where += " AND e.reference LIKE ?"
		args = append(args, "%"+reference+"%")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Count total
	countQuery := db.Rebind("SELECT COUNT(*) FROM journal_entries e"+where, h.usePostgres)
	var total int
	if err := h.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	// Le montant part avec la ligne.
	//
	// La liste ne le rendait pas, et l'interface était censée l'additionner
	// depuis des lignes qu'elle ne recevait pas. Une colonne « Montant » vide
	// sur un journal n'est pas un défaut d'affichage : c'est la seule grandeur
	// qui permet de repérer une écriture au milieu des autres.
	//
	// Le total est celui des DÉBITS : dans une écriture équilibrée il vaut celui
	// des crédits, et additionner les deux doublerait le montant.
	listQuery := db.Rebind(`
		SELECT e.id, e.reference, e.date, e.description, e.status, e.is_reversal,
		       e.created_at, e.updated_at,
		       COALESCE((SELECT SUM(l.debit_amount) FROM journal_lines l
		                 WHERE l.entry_id = e.id), 0) AS total,
		       COALESCE(u.name, '') AS author
		FROM journal_entries e
		LEFT JOIN users u ON u.id = e.created_by_id`+where+
		" ORDER BY e.date DESC, e.created_at DESC LIMIT ? OFFSET ?",
		h.usePostgres,
	)
	offset := (page - 1) * pageSize
	rows, err := h.db.QueryContext(ctx, listQuery, append(args, pageSize, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	entries := []journalListItem{}
	for rows.Next() {
		var e journalListItem
		var isReversal int
		if err := rows.Scan(&e.ID, &e.Reference, &e.Date, &e.Description, &e.Status,
			&isReversal, &e.CreatedAt, &e.UpdatedAt, &e.Total, &e.Author); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		e.IsReversal = isReversal == 1
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	pages := (total + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     entries,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     pages,
	})
}

// GetJournalEntry GET /api/v1/journal/:id
//
// Le détail avec ses lignes. Sans lui, une écriture ne se contrôle pas : la
// liste ne montre qu'un total, et vérifier qu'on a bien débité le bon compte
// obligeait à ouvrir la base.
func (h *JournalHandler) GetJournalEntry(c *gin.Context) {
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var e journalListItem
	var isReversal int
	var integrityHash sql.NullString
	head := db.Rebind(`
		SELECT e.id, e.reference, e.date, e.description, e.status, e.is_reversal,
		       e.created_at, e.updated_at, COALESCE(u.name, ''), e.integrity_hash
		FROM journal_entries e
		LEFT JOIN users u ON u.id = e.created_by_id
		WHERE e.id = ?`, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, head, id).Scan(
		&e.ID, &e.Reference, &e.Date, &e.Description, &e.Status, &isReversal,
		&e.CreatedAt, &e.UpdatedAt, &e.Author, &integrityHash); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "écriture introuvable"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	e.IsReversal = isReversal == 1

	linesQ := db.Rebind(`
		SELECT l.id, l.account_id, a.code, a.name,
		       COALESCE(l.debit_amount, 0), COALESCE(l.credit_amount, 0),
		       COALESCE(l.description, ''), l.sequence
		FROM journal_lines l
		JOIN accounts a ON a.id = l.account_id
		WHERE l.entry_id = ?
		ORDER BY l.sequence, a.code`, h.usePostgres)
	rows, err := h.db.QueryContext(ctx, linesQ, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	defer rows.Close()

	lines := []journalLineView{}
	for rows.Next() {
		var l journalLineView
		if err := rows.Scan(&l.ID, &l.AccountID, &l.AccountCode, &l.AccountName,
			&l.DebitAmount, &l.CreditAmount, &l.Description, &l.Sequence); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		e.Total += l.DebitAmount
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entry": e,
		"lines": lines,
		// L'empreinte n'existe qu'après comptabilisation : c'est elle qui scelle
		// l'écriture dans la chaîne du CO art. 957a. La rendre visible permet de
		// la citer, et son absence dit clairement qu'un brouillon n'est encore
		// scellé par rien.
		"integrity_hash": integrityHash.String,
	})
}

func queryInt(c *gin.Context, key string, fallback int) int {
	v := c.Query(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

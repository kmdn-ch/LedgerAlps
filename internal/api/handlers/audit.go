package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// ─── AuditHandler ────────────────────────────────────────────────────────────

type AuditHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewAuditHandler(database *sql.DB, usePostgres bool) *AuditHandler {
	return &AuditHandler{db: database, usePostgres: usePostgres}
}

// ─── GET /api/v1/audit-logs ──────────────────────────────────────────────────

// ListAuditLogs returns audit log entries matching optional filter parameters.
// Query params: table_name, record_id, from (YYYY-MM-DD), to (YYYY-MM-DD),
//
//	limit (default 50, max 200), offset (default 0).
//
// Access: admin only.
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	// La garde qui lisait le drapeau administrateur DU JETON a ete retiree.
	//
	// Deux defauts en un. Elle lisait un drapeau fige a la connexion : rétrograder
	// quelqu'un le laissait agir jusqu'a l'expiration de son jeton. Et elle
	// reservait a l'administrateur le journal d'audit, qui est
	// le metier du COMPTABLE — il devait demander a quelqu'un dont le role est de
	// gerer des mots de passe.
	//
	// La permission est desormais declaree sur la route (authz.PermManage) et lue
	// dans la base a chaque requete.

	tableName := c.Query("table_name")
	recordID := c.Query("record_id")
	fromStr := c.Query("from")
	toStr := c.Query("to")
	limit := queryInt(c, "limit", 50)
	offset := queryInt(c, "offset", 0)

	if limit < 1 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	if fromStr != "" {
		if _, err := time.Parse("2006-01-02", fromStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
			return
		}
	}
	if toStr != "" {
		if _, err := time.Parse("2006-01-02", toStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Build dynamic WHERE clause. Columns are qualified with the "a" alias so
	// the same clause serves both the count and the joined listing below.
	where := " WHERE 1=1"
	args := []any{}

	if tableName != "" {
		where += " AND a.table_name = ?"
		args = append(args, tableName)
	}
	if recordID != "" {
		where += " AND a.record_id = ?"
		args = append(args, recordID)
	}
	if fromStr != "" {
		where += " AND DATE(a.created_at) >= ?"
		args = append(args, fromStr)
	}
	if toStr != "" {
		where += " AND DATE(a.created_at) <= ?"
		args = append(args, toStr)
	}

	// Total count.
	countQ := db.Rebind("SELECT COUNT(*) FROM audit_logs a"+where, h.usePostgres)
	var total int
	if err := h.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Ordered by sequence_number. Ascending stays the default: it is the order
	// the chain was written in, and the order any integrity reading needs.
	// The screen asks for desc, where the most recent entry is what you look at
	// first. Only these two values are accepted — the parameter is concatenated
	// into the query, so it must never carry caller-supplied text.
	direction := "ASC"
	if c.Query("order") == "desc" {
		direction = "DESC"
	}
	// LEFT JOIN sur users : un identifiant technique ne dit rien à qui lit
	// l'écran. La jointure est à gauche pour qu'une entrée survive à la
	// disparition de son auteur — une ligne de la chaîne ne s'efface pas.
	listQ := db.Rebind(`
		SELECT a.id, a.user_id, a.action, a.table_name, a.record_id,
		       a.before_state, a.after_state, a.ip_address,
		       a.entry_hash, a.prev_hash, a.sequence_number, a.created_at,
		       COALESCE(u.name, '') AS user_name
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id`+where+`
		ORDER BY a.sequence_number `+direction+`
		LIMIT ? OFFSET ?`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, listQ, append(args, limit, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	items := []models.AuditLog{}
	for rows.Next() {
		var al models.AuditLog
		if err := rows.Scan(
			&al.ID, &al.UserID, &al.Action, &al.TableName, &al.RecordID,
			&al.BeforeState, &al.AfterState, &al.IPAddress,
			&al.EntryHash, &al.PrevHash, &al.SequenceNumber, &al.CreatedAt,
			&al.UserName,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		items = append(items, al)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"pages":  pages,
	})
}

// ─── GET /api/v1/audit-logs/:id/verify ───────────────────────────────────────

// VerifyAuditLog recomputes the entry_hash for a single audit log record and
// compares it to the stored value, confirming data integrity (CO art. 957a).
// Returns 200 with verified=true if the hash matches, 409 with verified=false
// if it does not (indicating possible tampering or corruption).
// Access: admin only.
func (h *AuditHandler) VerifyAuditLog(c *gin.Context) {

	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT
		    user_id,
		    action,
		    table_name,
		    record_id,
		    COALESCE(before_state, '') AS before_state,
		    COALESCE(after_state,  '') AS after_state,
		    COALESCE(ip_address,   '') AS ip_address,
		    entry_hash,
		    created_at,
		    hash_version
		FROM audit_logs WHERE id = ?`, h.usePostgres)

	var (
		userID, action, tableName, recordID string
		beforeState, afterState, ipAddress  string
		storedHash                          string
		createdAt                           time.Time
		hashVersion                         int
	)
	err := h.db.QueryRowContext(ctx, q, id).Scan(
		&userID, &action, &tableName, &recordID,
		&beforeState, &afterState, &ipAddress,
		&storedHash, &createdAt, &hashVersion,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit log entry not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Entrée antérieure au correctif d'empreinte (migration 0012) : les valeurs
	// ayant servi au calcul n'ont jamais été persistées, l'empreinte n'est donc
	// pas recalculable. Répondre 409 « intégrité en échec » accuserait à tort.
	if hashVersion < 2 {
		c.JSON(http.StatusOK, gin.H{
			"id":           id,
			"verifiable":   false,
			"hash_version": hashVersion,
			"hash":         storedHash,
			"detail":       "Entrée écrite avant la v1.4.6 : son empreinte propre n'est pas recalculable. Son chaînage avec les entrées voisines reste vérifiable.",
		})
		return
	}

	// Recompute the hash using the same algorithm as the accounting service.
	recomputed := security.ComputeEntryHash(
		userID, action, tableName, recordID,
		beforeState, afterState, ipAddress,
		createdAt,
	)

	if recomputed == storedHash {
		c.JSON(http.StatusOK, gin.H{
			"id":       id,
			"verified": true,
			"hash":     storedHash,
		})
		return
	}

	// Hash mismatch — potential tampering or data corruption.
	c.JSON(http.StatusConflict, gin.H{
		"id":              id,
		"verified":        false,
		"stored_hash":     storedHash,
		"recomputed_hash": recomputed,
		"error":           "integrity check failed: stored hash does not match recomputed hash (CO art. 957a)",
	})
}

// ─── GET /api/v1/audit-logs/verify-chain ─────────────────────────────────────

// ChainBreak est une rupture constatée dans la chaîne d'empreintes.
type ChainBreak struct {
	SequenceNumber int64     `json:"sequence_number"`
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	Kind           string    `json:"kind"`
	Detail         string    `json:"detail"`
}

// ChainReport est le résultat du parcours complet de la chaîne.
type ChainReport struct {
	Verified bool `json:"verified"`
	Entries  int  `json:"entries"`
	// Legacy compte les entrées dont l'empreinte propre n'est pas recalculable
	// (écrites avant la v1.4.6). Leur chaînage, lui, est bien vérifié.
	Legacy    int          `json:"legacy_entries"`
	FirstSeq  int64        `json:"first_sequence"`
	LastSeq   int64        `json:"last_sequence"`
	Breaks    []ChainBreak `json:"breaks"`
	Truncated bool         `json:"truncated"`
	CheckedAt time.Time    `json:"checked_at"`
	// HeadHash est l'empreinte de la dernière entrée. Elle résume l'état de
	// toute la chaîne : la noter aujourd'hui permet de prouver demain qu'aucune
	// des entrées d'aujourd'hui n'a bougé depuis.
	HeadHash string `json:"head_hash,omitempty"`
}

// maxChainBreaks borne la réponse : au-delà, la chaîne est manifestement
// rompue et énumérer chaque maillon n'apprend plus rien.
const maxChainBreaks = 100

// VerifyAuditChain parcourt la totalité de la chaîne d'empreintes et vérifie
// les trois propriétés qui, ensemble, prouvent que les livres n'ont pas été
// altérés (CO art. 957a al. 2 ch. 5, Olico art. 9).
//
// Vérifier une entrée isolée — ce que fait VerifyAuditLog — ne suffit pas :
// cela détecte la modification du contenu d'une ligne, mais **pas sa
// suppression**. Effacer une ligne laisse l'empreinte propre de toutes les
// autres parfaitement valide. Seuls le chaînage et la continuité des numéros
// de séquence rendent une suppression visible :
//
//	entry_altered   — l'empreinte recalculée diffère de celle stockée :
//	                  le contenu de la ligne a changé
//	link_broken     — prev_hash ne pointe pas sur l'empreinte de la ligne
//	                  précédente : une ligne a été retirée, insérée ou déplacée
//	sequence_gap    — un numéro de séquence manque : une ligne a été supprimée
//	anchor_invalid  — la première ligne porte un prev_hash, ou ne commence pas
//	                  à 1 : le **début** de la chaîne a été supprimé, le seul
//	                  cas où tous les maillons restants restent cohérents
//
// Accès : administrateur uniquement.
func (h *AuditHandler) VerifyAuditChain(c *gin.Context) {

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	report, err := h.ComputeChainReport(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// 200 quand la chaîne est intacte, 409 sinon — même convention que la
	// vérification d'une entrée isolée, pour qu'un client puisse traiter les
	// deux points d'entrée de la même manière.
	if report.Verified {
		c.JSON(http.StatusOK, report)
		return
	}
	c.JSON(http.StatusConflict, report)
}

// ComputeChainReport parcourt la chaîne et retourne le rapport. Extrait du
// handler pour que l'attestation Olico repose exactement sur le même parcours :
// deux implémentations finiraient par diverger, et c'est l'attestation — le
// document qu'on présente à un tiers — qui aurait tort.
func (h *AuditHandler) ComputeChainReport(ctx context.Context) (ChainReport, error) {
	// COALESCE aligné sur VerifyAuditLog : les deux points d'entrée doivent
	// recalculer exactement la même empreinte, sans quoi ils se contrediraient
	// sur la même ligne.
	q := db.Rebind(`
		SELECT id, user_id, action, table_name, record_id,
		       COALESCE(before_state, '') AS before_state,
		       COALESCE(after_state,  '') AS after_state,
		       COALESCE(ip_address,   '') AS ip_address,
		       entry_hash,
		       COALESCE(prev_hash, '') AS prev_hash,
		       sequence_number, created_at, hash_version
		FROM audit_logs
		ORDER BY sequence_number ASC`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		return ChainReport{}, err
	}
	defer rows.Close()

	report := ChainReport{Breaks: []ChainBreak{}, CheckedAt: time.Now().UTC()}

	var (
		prevEntryHash string
		prevSeq       int64
		headHash      string
		first         = true
	)

	add := func(b ChainBreak) {
		if len(report.Breaks) < maxChainBreaks {
			report.Breaks = append(report.Breaks, b)
			return
		}
		report.Truncated = true
	}

	for rows.Next() {
		var (
			id, userID, action, tableName, recordID string
			beforeState, afterState, ipAddress      string
			entryHash, prevHash                     string
			seq                                     int64
			createdAt                               time.Time
			hashVersion                             int
		)
		if err := rows.Scan(
			&id, &userID, &action, &tableName, &recordID,
			&beforeState, &afterState, &ipAddress,
			&entryHash, &prevHash, &seq, &createdAt, &hashVersion,
		); err != nil {
			return ChainReport{}, err
		}

		report.Entries++
		if first {
			report.FirstSeq = seq
		}
		report.LastSeq = seq

		// 1. L'empreinte propre de la ligne — seulement si elle est recalculable.
		//    Les entrées en version 1 ont été écrites avec des valeurs qui n'ont
		//    pas été persistées (migration 0012). Les compter comme rompues
		//    afficherait « vos livres ont été altérés » à tout utilisateur ayant
		//    comptabilisé avant la v1.4.6, ce qui serait faux — et une alerte
		//    fausse n'est crue qu'une fois.
		if hashVersion < 2 {
			report.Legacy++
		} else {
			recomputed := security.ComputeEntryHash(
				userID, action, tableName, recordID,
				beforeState, afterState, ipAddress, createdAt,
			)
			if recomputed != entryHash {
				add(ChainBreak{
					SequenceNumber: seq, ID: id, CreatedAt: createdAt, Kind: "entry_altered",
					Detail: "Le contenu de cette entrée ne correspond plus à son empreinte.",
				})
			}
		}

		if first {
			// 2. L'ancrage. Une chaîne intacte commence à 1 sans antécédent ;
			//    supprimer les premières lignes est la seule altération qui
			//    laisse tous les maillons suivants mutuellement cohérents.
			if prevHash != "" {
				add(ChainBreak{
					SequenceNumber: seq, ID: id, CreatedAt: createdAt, Kind: "anchor_invalid",
					Detail: "La première entrée référence une entrée antérieure qui n'existe plus.",
				})
			}
			if seq != 1 {
				add(ChainBreak{
					SequenceNumber: seq, ID: id, CreatedAt: createdAt, Kind: "anchor_invalid",
					Detail: fmt.Sprintf("La chaîne commence au numéro %d au lieu de 1 : %d entrée(s) manquent au début.", seq, seq-1),
				})
			}
			first = false
			prevEntryHash, prevSeq, headHash = entryHash, seq, entryHash
			continue
		}

		// 3. Le chaînage.
		if prevHash != prevEntryHash {
			add(ChainBreak{
				SequenceNumber: seq, ID: id, CreatedAt: createdAt, Kind: "link_broken",
				Detail: "Cette entrée ne pointe pas sur l'entrée qui la précède.",
			})
		}

		// 4. La continuité des numéros.
		if seq != prevSeq+1 {
			add(ChainBreak{
				SequenceNumber: seq, ID: id, CreatedAt: createdAt, Kind: "sequence_gap",
				Detail: fmt.Sprintf("Le numéro %d suit le numéro %d : %d entrée(s) ont été supprimées.", seq, prevSeq, seq-prevSeq-1),
			})
		}

		prevEntryHash, prevSeq, headHash = entryHash, seq, entryHash
	}
	if err := rows.Err(); err != nil {
		return ChainReport{}, err
	}

	report.Verified = len(report.Breaks) == 0 && !report.Truncated
	report.HeadHash = headHash
	return report, nil
}

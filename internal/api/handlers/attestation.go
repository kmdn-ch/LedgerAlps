package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/version"
)

// Attestation d'intégrité — Olico (RS 221.431) art. 9.
//
// L'art. 9 autorise la conservation sur support **modifiable** à deux
// conditions : que des procédés techniques garantissent l'intégrité des données
// enregistrées, et que le moment de l'enregistrement puisse être établi.
//
// La chaîne d'empreintes SHA-256 des écritures répond à la première : chaque
// entrée dérive de la précédente, une modification ou une suppression se voit.
// Restait à pouvoir le **présenter** — à une fiduciaire, à un réviseur, à
// l'AFC. C'est l'objet de ce document.
//
// Ce qu'il n'affirme pas. L'horodatage vient de l'horloge du poste, pas d'une
// autorité tierce. Il établit l'ordre des enregistrements et leur cohérence,
// pas une date opposable au sens d'un horodatage qualifié (RFC 3161). Le dire
// est le seul choix tenable : une attestation qui promettrait plus que ce
// qu'elle peut tenir serait détruite au premier examen sérieux, et emporterait
// avec elle le crédit de tout le reste.

type attestationPeriod struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Closed    bool   `json:"closed"`
	Entries   int    `json:"posted_entries"`
}

type attestationScope struct {
	PostedEntries int                 `json:"posted_entries"`
	Invoices      int                 `json:"invoices"`
	Periods       []attestationPeriod `json:"fiscal_years"`
}

type attestationChain struct {
	Verified       bool         `json:"verified"`
	Entries        int          `json:"entries"`
	FirstSequence  int64        `json:"first_sequence"`
	LastSequence   int64        `json:"last_sequence"`
	HeadHash       string       `json:"head_hash"`
	Breaks         int          `json:"breaks"`
	LegacyEntries  int          `json:"entries_not_recomputable"`
	Algorithm      string       `json:"algorithm"`
	ChainedOn      string       `json:"chained_on"`
	VerifiedAtUTC  string       `json:"verified_at_utc"`
	BreakDetails   []ChainBreak `json:"break_details,omitempty"`
	LegacyExplains string       `json:"entries_not_recomputable_explanation,omitempty"`
}

// Attestation est le document remis à un tiers.
type Attestation struct {
	Document   string           `json:"document"`
	Product    string           `json:"product"`
	Version    string           `json:"version"`
	IssuedAt   string           `json:"issued_at_utc"`
	IssuedBy   string           `json:"issued_by"`
	LegalBasis []string         `json:"legal_basis"`
	Scope      attestationScope `json:"scope"`
	Chain      attestationChain `json:"chain"`
	Statement  []string         `json:"statement"`
	Limits     []string         `json:"limits"`
	// SelfHash scelle tout ce qui précède. Modifier une ligne du document sans
	// recalculer cette empreinte se voit ; la recalculer suppose de disposer de
	// l'outil, et l'empreinte de tête reste comparable à la base d'origine.
	SelfHash string `json:"self_hash"`
}

// ─── GET /api/v1/audit-logs/attestation ──────────────────────────────────────

// IntegrityAttestation produit l'attestation d'intégrité, en pièce jointe.
// Accès : administrateur uniquement.
func (h *AuditHandler) IntegrityAttestation(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin privileges required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	report, err := h.ComputeChainReport(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	scope, err := h.attestationScope(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Le nom plutôt que l'identifiant technique : le document part chez un tiers,
	// à qui un UUID n'apprend rien.
	issuedBy := currentUserID(c)
	var name string
	if err := h.db.QueryRowContext(ctx,
		db.Rebind(`SELECT COALESCE(name, '') FROM users WHERE id = ?`, h.usePostgres), issuedBy,
	).Scan(&name); err == nil && name != "" {
		issuedBy = name
	}

	att := Attestation{
		Document: "Attestation d'intégrité de la comptabilité",
		Product:  "LedgerAlps",
		Version:  version.Version(),
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
		IssuedBy: issuedBy,
		LegalBasis: []string{
			"CO art. 957a al. 2 ch. 5 — traçabilité des écritures",
			"CO art. 958f — conservation dix ans",
			"Olico (RS 221.431) art. 9 — conservation sur support modifiable",
		},
		Scope: scope,
		Chain: attestationChain{
			Verified:      report.Verified,
			Entries:       report.Entries,
			FirstSequence: report.FirstSeq,
			LastSequence:  report.LastSeq,
			HeadHash:      report.HeadHash,
			Breaks:        len(report.Breaks),
			LegacyEntries: report.Legacy,
			Algorithm:     "SHA-256",
			ChainedOn:     "user_id, action, table_name, record_id, before_state, after_state, ip_address, created_at",
			VerifiedAtUTC: report.CheckedAt.Format(time.RFC3339),
			BreakDetails:  report.Breaks,
		},
	}

	if report.Legacy > 0 {
		att.Chain.LegacyExplains = "Entrées écrites avant la v1.4.6, dont l'empreinte propre n'est pas recalculable : " +
			"elle portait alors sur des valeurs qui n'étaient pas enregistrées. Leur chaînage est vérifié comme les autres, " +
			"donc une suppression y reste détectable ; seule la question « le contenu de cette ligne a-t-il changé ? » est sans réponse."
	}

	if report.Verified {
		att.Statement = []string{
			fmt.Sprintf("Au %s, la chaîne d'empreintes couvrant %d écriture(s) comptabilisée(s) est intacte.",
				att.IssuedAt, report.Entries),
			"Chaque entrée dérive par SHA-256 de la précédente. Aucune entrée n'a été modifiée, retirée ou réordonnée entre la première et la dernière.",
			"L'empreinte de tête ci-dessus résume l'état de la chaîne : la conserver permet d'établir ultérieurement qu'aucune des écritures couvertes n'a bougé depuis l'émission de cette attestation.",
		}
	} else {
		att.Statement = []string{
			fmt.Sprintf("Au %s, la chaîne d'empreintes présente %d anomalie(s) sur %d écriture(s).",
				att.IssuedAt, len(report.Breaks), report.Entries),
			"Cette attestation ne certifie donc PAS l'intégrité de la comptabilité. Elle constate et documente une rupture.",
			"Le détail des ruptures figure ci-dessus. Une sauvegarde antérieure à la rupture est nécessaire pour rétablir les livres.",
		}
	}

	att.Limits = []string{
		"L'horodatage provient de l'horloge du poste, non d'une autorité d'horodatage tierce. La chaîne établit l'ORDRE des enregistrements et leur cohérence, pas une date opposable au sens d'un horodatage qualifié (RFC 3161).",
		"Une troncature en FIN de chaîne n'est pas détectable : rien ne distingue des écritures effacées après la dernière d'écritures jamais passées. Seule la comparaison avec une sauvegarde répond à cette question.",
		"Cette attestation est produite par le logiciel lui-même. Elle documente l'état d'un mécanisme technique ; elle ne remplace ni un contrôle de révision, ni l'avis d'une fiduciaire.",
	}

	// Le sceau se calcule sur le document sans son propre champ, sérialisé de
	// façon déterministe par encoding/json (ordre des champs de la structure).
	att.SelfHash = ""
	body, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encoding error"})
		return
	}
	sum := sha256.Sum256(body)
	att.SelfHash = hex.EncodeToString(sum[:])

	final, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encoding error"})
		return
	}

	filename := fmt.Sprintf("ledgeralps-attestation-integrite-%s.json", time.Now().UTC().Format("2006-01-02"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", final)
}

// attestationScope décrit ce que l'attestation couvre : sans périmètre, un
// « tout est intact » ne veut rien dire.
func (h *AuditHandler) attestationScope(ctx context.Context) (attestationScope, error) {
	var sc attestationScope

	countOne := func(query string) (int, error) {
		var n int
		err := h.db.QueryRowContext(ctx, db.Rebind(query, h.usePostgres)).Scan(&n)
		return n, err
	}

	var err error
	if sc.PostedEntries, err = countOne(
		`SELECT COUNT(*) FROM journal_entries WHERE status = 'posted'`); err != nil {
		return sc, err
	}
	if sc.Invoices, err = countOne(
		`SELECT COUNT(*) FROM invoices WHERE document_type = 'invoice'`); err != nil {
		return sc, err
	}

	rows, err := h.db.QueryContext(ctx, db.Rebind(`
		SELECT f.name, f.start_date, f.end_date, f.is_closed,
		       (SELECT COUNT(*) FROM journal_entries e
		        WHERE e.fiscal_year_id = f.id AND e.status = 'posted')
		FROM fiscal_years f
		ORDER BY f.start_date`, h.usePostgres))
	if err != nil {
		return sc, err
	}
	defer rows.Close()

	sc.Periods = []attestationPeriod{}
	for rows.Next() {
		var p attestationPeriod
		var start, end time.Time
		var closed int
		if err := rows.Scan(&p.Name, &start, &end, &closed, &p.Entries); err != nil {
			return sc, err
		}
		p.StartDate = start.Format("2006-01-02")
		p.EndDate = end.Format("2006-01-02")
		p.Closed = closed == 1
		sc.Periods = append(sc.Periods, p)
	}
	return sc, rows.Err()
}

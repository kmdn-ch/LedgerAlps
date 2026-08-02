package handlers

// Maintenance & Système — intégrité des données et diagnostic.
//
// Ces contrôles ne réparent rien. Une comptabilité incohérente se corrige par
// une écriture, pas par un bouton : réparer en silence effacerait la trace de ce
// qui s'est passé, ce que le CO art. 957a al. 2 ch. 5 interdit précisément. Le
// rôle de cette page est de *montrer*, avec assez de contexte pour agir.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/core/tlsutil"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/version"
)

type MaintenanceHandler struct {
	db          *sql.DB
	cfg         *config.Config
	usePostgres bool
}

func NewMaintenanceHandler(database *sql.DB, cfg *config.Config) *MaintenanceHandler {
	return &MaintenanceHandler{db: database, cfg: cfg, usePostgres: cfg.UsePostgres()}
}

// Finding is one thing worth a human's attention.
type Finding struct {
	// Severity is "error" when the books are wrong, "warning" when something
	// looks unintended, "info" when it is merely worth knowing. Nothing is
	// reported as an error unless a figure is actually incorrect — crying wolf
	// here would make the whole page ignorable.
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	// Action tells the user what to do. A finding they cannot act on is noise.
	Action string `json:"action,omitempty"`
	Count  int    `json:"count"`
}

// IntegrityCheck GET /api/v1/maintenance/integrity
func (h *MaintenanceHandler) IntegrityCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	findings := []Finding{}
	add := func(f Finding) {
		if f.Count > 0 {
			findings = append(findings, f)
		}
	}

	// ── Débit ≠ crédit ───────────────────────────────────────────────────────
	// L'invariant fondamental de la partie double. Une écriture déséquilibrée
	// fausse le bilan : c'est la seule chose ici qui soit franchement une erreur.
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM (
			SELECT jl.entry_id
			FROM journal_lines jl
			GROUP BY jl.entry_id
			HAVING ABS(COALESCE(SUM(jl.debit_amount), 0) - COALESCE(SUM(jl.credit_amount), 0)) > 0.005
		)`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "error", Check: "journal_balance", Count: n,
			Title:  "Écritures déséquilibrées",
			Detail: fmt.Sprintf("%d écriture(s) dont le total des débits diffère du total des crédits.", n),
			Action: "Ouvrez ces écritures dans le journal et corrigez-les par une écriture rectificative.",
		})
	}

	// ── Écritures sans ligne ─────────────────────────────────────────────────
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM journal_entries je
		WHERE NOT EXISTS (SELECT 1 FROM journal_lines jl WHERE jl.entry_id = je.id)`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "warning", Check: "journal_empty", Count: n,
			Title:  "Écritures sans ligne",
			Detail: fmt.Sprintf("%d écriture(s) ne contiennent aucune ligne.", n),
			Action: "Complétez-les ou supprimez-les si elles sont restées à l'état de brouillon.",
		})
	}

	// ── Chaîne d'empreintes rompue ───────────────────────────────────────────
	// La chaîne SHA-256 est ce qui rend les écritures postées vérifiables
	// (CO art. 957a). Un maillon manquant n'est pas nécessairement une fraude —
	// une migration interrompue suffit — mais cela doit se voir.
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM journal_entries
		WHERE status = 'posted' AND (integrity_hash IS NULL OR integrity_hash = '')`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "error", Check: "hash_chain", Count: n,
			Title:  "Écritures postées sans empreinte d'intégrité",
			Detail: fmt.Sprintf("%d écriture(s) postées n'ont pas d'empreinte SHA-256.", n),
			Action: "Ces écritures ne sont pas vérifiables au sens du CO art. 957a. Signalez-le avant votre prochaine clôture.",
		})
	}

	// ── Factures sans ligne ──────────────────────────────────────────────────
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM invoices i
		WHERE NOT EXISTS (SELECT 1 FROM invoice_lines l WHERE l.invoice_id = i.id)`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "warning", Check: "invoice_empty", Count: n,
			Title:  "Documents sans ligne",
			Detail: fmt.Sprintf("%d facture(s), offre(s) ou note(s) de crédit ne contiennent aucune ligne.", n),
			Action: "Complétez-les, ou annulez-les si elles ont été créées par erreur.",
		})
	}

	// ── Totaux incohérents ───────────────────────────────────────────────────
	// Tolérance de 5 centimes : les totaux sont arrondis au 5 rappen, donc un
	// écart inférieur est le fonctionnement normal, pas un défaut.
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM (
			SELECT i.id
			FROM invoices i
			JOIN invoice_lines l ON l.invoice_id = i.id
			GROUP BY i.id, i.subtotal_amount
			HAVING ABS(i.subtotal_amount - SUM(l.line_total)) > 0.05
		)`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "error", Check: "invoice_totals", Count: n,
			Title:  "Totaux ne correspondant pas aux lignes",
			Detail: fmt.Sprintf("%d document(s) dont le sous-total diffère de la somme des lignes de plus de 5 centimes.", n),
			Action: "Rouvrez ces documents et enregistrez-les à nouveau pour recalculer les totaux.",
		})
	}

	// ── Contacts orphelins ───────────────────────────────────────────────────
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM invoices i
		WHERE NOT EXISTS (SELECT 1 FROM contacts co WHERE co.id = i.contact_id)`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "error", Check: "orphan_contacts", Count: n,
			Title:  "Documents rattachés à un contact inexistant",
			Detail: fmt.Sprintf("%d document(s) référencent un contact supprimé.", n),
			Action: "Le nom du client n'apparaîtra plus sur ces documents. Recréez le contact ou réaffectez-les.",
		})
	}

	// ── Notes de crédit dépassant leur facture ───────────────────────────────
	// Le garde-fou existe depuis la v1.4.4 ; ce contrôle rattrape les documents
	// créés avant, quand rien ne l'empêchait.
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM (
			SELECT orig.id
			FROM invoices orig
			JOIN invoices cn ON cn.corrects_invoice_id = orig.id AND cn.status <> 'cancelled'
			GROUP BY orig.id, orig.total_amount
			HAVING SUM(cn.total_amount) > orig.total_amount + 0.01
		)`); err != nil {
		h.fail(c, err)
		return
	} else {
		add(Finding{
			Severity: "error", Check: "over_credited", Count: n,
			Title:  "Factures créditées au-delà de leur montant",
			Detail: fmt.Sprintf("%d facture(s) portent des notes de crédit dont le total dépasse la facture.", n),
			Action: "Créées avant la v1.4.4, quand rien ne l'empêchait. Annulez la note de crédit en trop : votre TVA est sous-évaluée.",
		})
	}

	// ── Offres restées sans suite ────────────────────────────────────────────
	if n, err := h.count(ctx, `
		SELECT COUNT(*) FROM invoices
		WHERE document_type = 'quote' AND status = 'sent' AND quote_outcome IS NULL
		  AND due_date < date('now', '-90 day')`); err != nil {
		// Une syntaxe de date propre à SQLite : sur PostgreSQL, on n'en fait
		// pas un échec du diagnostic entier.
		if !h.usePostgres {
			h.fail(c, err)
			return
		}
	} else {
		add(Finding{
			Severity: "info", Check: "stale_quotes", Count: n,
			Title:  "Offres de prix sans réponse",
			Detail: fmt.Sprintf("%d offre(s) envoyées depuis plus de 90 jours n'ont ni été acceptées, ni refusées.", n),
			Action: "Marquez-les refusées ou expirées pour qu'elles cessent d'encombrer vos listes.",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"checked_at": time.Now().UTC(),
		"findings":   findings,
		"clean":      len(findings) == 0,
	})
}

// SystemHealth GET /api/v1/maintenance/health
//
// Ce que l'utilisateur doit pouvoir constater sans ouvrir un terminal : la base
// répond, les sauvegardes tournent, et ce qui est chiffré l'est.
func (h *MaintenanceHandler) SystemHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	resp := gin.H{
		"version":  version.Version(),
		"database": gin.H{"engine": "SQLite"},
	}
	if h.usePostgres {
		resp["database"] = gin.H{"engine": "PostgreSQL"}
	}

	// Volumétrie : utile pour juger si une lenteur est normale.
	counts := gin.H{}
	for label, q := range map[string]string{
		"invoices":        "SELECT COUNT(*) FROM invoices",
		"journal_entries": "SELECT COUNT(*) FROM journal_entries",
		"contacts":        "SELECT COUNT(*) FROM contacts",
		"audit_logs":      "SELECT COUNT(*) FROM audit_logs",
	} {
		if n, err := h.count(ctx, q); err == nil {
			counts[label] = n
		}
	}
	resp["counts"] = counts

	// Sauvegardes : la question que l'utilisateur se pose vraiment est « suis-je
	// protégé ? », pas « le service tourne-t-il ».
	backups, err := db.ListBackups(db.BackupDir())
	if err == nil {
		encrypted := 0
		for _, b := range backups {
			if enc, _ := db.IsEncrypted(b.Path); enc {
				encrypted++
			}
		}
		info := gin.H{"count": len(backups), "encrypted": encrypted, "directory": db.BackupDir()}
		if len(backups) > 0 {
			info["newest"] = backups[0].CreatedAt
			info["newest_name"] = backups[0].Name
		}
		resp["backups"] = info
	}

	// Sécurité du transport : dire ce qui est, pas ce qui devrait être.
	resp["network"] = gin.H{
		"host":            h.cfg.Host,
		"tls":             !tlsutil.IsLoopback(h.cfg.Host) && !h.cfg.AllowInsecureHTTP,
		"loopback":        tlsutil.IsLoopback(h.cfg.Host),
		"insecure_opt_in": h.cfg.AllowInsecureHTTP,
	}

	// Ce que le produit sait faire — la même table que celle qui garde les avis
	// de conformité honnêtes.
	caps := gin.H{}
	for name, present := range compliance.Capabilities {
		caps[string(name)] = present
	}
	resp["capabilities"] = caps

	// Verrous de sécurité récents : visibles ici plutôt que dans un fichier log.
	if n, err := h.count(ctx, `SELECT COUNT(*) FROM security_events WHERE event_type = 'login_lockout'`); err == nil {
		resp["login_lockouts"] = n
	}

	c.JSON(http.StatusOK, resp)
}

func (h *MaintenanceHandler) count(ctx context.Context, q string) (int, error) {
	var n int
	if err := h.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (h *MaintenanceHandler) fail(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "le contrôle d'intégrité n'a pas pu s'exécuter: " + err.Error()})
}

package handlers

// Retirer des factures de la liste des paiements, sans mentir aux livres.
//
// # Le besoin, et pourquoi « supprimer » n'est pas la réponse
//
// La liste des paiements accumule tout ce qui est comptabilisé et non réglé :
// une facture réglée hors LedgerAlps, une saisie d'essai, un double, y restent
// indéfiniment. Une liste qu'on ne peut pas vider cesse d'être lue, et le jour
// où elle porte une vraie facture en retard, personne ne la voit.
//
// Mais une facture COMPTABILISÉE est une charge dans les livres, et sa
// contrepartie est une dette au compte créanciers. L'effacer ferait disparaître
// la charge d'un exercice déjà tenu, et le CO art. 958f impose de conserver la
// pièce dix ans. Le geste juste n'est donc pas la suppression : c'est
// l'ANNULATION, qui laisse la facture visible, marquée annulée, et passe une
// écriture d'EXTOURNE — l'exact inverse de la comptabilisation.
//
// Après extourne, la charge et la TVA déductible sont neutralisées, la dette
// disparaît du compte créanciers, et les deux écritures restent lisibles. C'est
// ce qu'un réviseur attend de voir : pas un trou, une correction.
//
// # Le défaut que ceci corrige
//
// Passer une facture comptabilisée à « annulée » ne faisait que changer son
// statut. L'écriture restait dans les livres : la charge et la TVA déductible
// continuaient d'alimenter le résultat et la déclaration, pendant que l'écran
// affichait « annulée ». Le décompte trimestriel aurait fini par le révéler,
// des mois plus tard, sans que rien ne pointe vers la cause.
//
// # Ce qui reste refusé
//
// Une facture déjà PAYÉE, même partiellement. L'argent est parti ; l'annuler
// laisserait un décaissement sans dette en face. Le cas réel est un
// remboursement du fournisseur, qui se saisit comme tel.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// MaxCancelBatch borne un lot.
//
// Sans borne, un appel forgé pourrait demander l'annulation de toute la base en
// une transaction. La limite est haute pour un usage normal — on ne nettoie pas
// deux cents lignes à la main — et basse devant ce qu'un abus tenterait.
const MaxCancelBatch = 200

type cancelSupplierInvoicesRequest struct {
	IDs    []string `json:"ids"`
	Reason string   `json:"reason"`
}

// cancelResult dit ce qu'il est advenu de CHAQUE facture.
//
// Un lot partiel est le cas normal : on coche quatre lignes, deux sont des
// brouillons, une est déjà payée. Rendre un seul « ok » ou une seule erreur
// obligerait à recharger pour comprendre, et laisserait croire à un échec
// total là où trois lignes sur quatre ont été traitées.
type cancelResult struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"` // "deleted", "cancelled", "skipped", "refused"
	Detail  string `json:"detail,omitempty"`
	EntryID string `json:"reversal_entry_id,omitempty"`

	// StatutAvant et StatutApres alimentent la piste d'audit, pas la réponse
	// HTTP : ils disent ce que la facture ÉTAIT et ce qu'elle est devenue.
	//
	// Sur un refus, les deux sont égaux — et c'est l'information utile : elle
	// nomme le statut qui a résisté. Une trace qui ne dirait que « refusé »
	// laisserait sans réponse la question qui se pose ensuite, à savoir
	// pourquoi la pièce est encore là.
	StatutAvant string `json:"-"`
	StatutApres string `json:"-"`
}

// CancelSupplierInvoices POST /api/v1/supplier-invoices/cancel
//
// Réservé à PermWriteAccounting : tenir les livres, donc administrateur et
// comptable. La lecture seule est refusée deux fois — par cette permission et
// par le filtre global qui rejette toute écriture.
func (h *SupplierInvoicesHandler) CancelSupplierInvoices(c *gin.Context) {
	var req cancelSupplierInvoicesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "aucune facture sélectionnée"})
		return
	}
	if len(req.IDs) > MaxCancelBatch {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": fmt.Sprintf("%d factures demandées, %d au maximum par lot",
				len(req.IDs), MaxCancelBatch)})
		return
	}

	// Le délai grandit avec le lot : chaque annulation passe une écriture, et
	// un forfait de cinq secondes couperait un lot honnête au milieu.
	delai := time.Duration(10+len(req.IDs)) * time.Second
	ctx, cancel := context.WithTimeout(c.Request.Context(), delai)
	defer cancel()

	userID := ""
	if claims := mw.GetClaims(c); claims != nil {
		userID = claims.UserID
	}

	results := make([]cancelResult, 0, len(req.IDs))
	for _, id := range req.IDs {
		r := h.cancelOne(ctx, id, userID, c.ClientIP(), req.Reason)
		// La trace porte le VERDICT, refus compris. Ne tracer que les succès
		// laisserait sans réponse la question qui se pose après coup : « on a
		// bien essayé de la retirer, pourquoi est-elle encore là ? »
		// Une annulation ne supprime rien — la pièce est conservée
		// (CO art. 958f) — mais elle change son statut. La transition est donc
		// écrite comme telle : ce que la facture était, ce qu'elle devient.
		//
		// Un REFUS est tracé aussi, et son état « après » porte alors le statut
		// inchangé : la question qui se pose après coup est « on a bien essayé
		// de la retirer, pourquoi est-elle encore là ? », et une trace qui ne
		// garderait que les succès y resterait muette.
		trace(c, h.db, h.usePostgres, TableSupplierInvoices,
			ActionSupplierInvoiceCancelled, id, accounting.Modification(
				map[string]any{"status": r.StatutAvant},
				map[string]any{
					"status":            r.StatutApres,
					"outcome":           r.Outcome,
					"detail":            r.Detail,
					"reversal_entry_id": r.EntryID,
					"reason":            req.Reason,
				},
			))
		results = append(results, r)
	}

	traites := 0
	for _, r := range results {
		if r.Outcome == "cancelled" || r.Outcome == "deleted" {
			traites++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"processed": traites,
		"total":     len(results),
		"results":   results,
	})
}

// cancelOne traite une facture et ne rend jamais d'erreur : le lot continue.
//
// Un échec sur la troisième ligne ne doit pas empêcher la quatrième d'être
// traitée ni annuler les deux premières — celles-ci sont déjà justes, et les
// défaire pour une raison qui leur est étrangère serait absurde. Chaque ligne
// porte donc son propre verdict.
func (h *SupplierInvoicesHandler) cancelOne(
	ctx context.Context, id, userID, ip, reason string,
) cancelResult {
	var (
		status    string
		entryID   string
		paid      float64
		reference string
	)
	q := db.Rebind(`SELECT status, COALESCE(journal_entry_id, ''), amount_paid,
	                       supplier_reference
	                FROM supplier_invoices WHERE id = ?`, h.usePostgres)
	switch err := h.db.QueryRowContext(ctx, q, id).Scan(
		&status, &entryID, &paid, &reference); {
	case err == sql.ErrNoRows:
		return cancelResult{ID: id, Outcome: "refused", Detail: "facture introuvable"}
	case err != nil:
		return cancelResult{ID: id, Outcome: "refused", Detail: "lecture impossible"}
	}

	switch status {
	case "cancelled":
		// Idempotent : recocher une ligne déjà annulée n'est pas une erreur.
		return cancelResult{ID: id, Outcome: "skipped", Detail: "déjà annulée",
			StatutAvant: status, StatutApres: status}

	case "draft":
		// Rien n'est entré dans les livres : la pièce peut disparaître.
		delQ := db.Rebind("DELETE FROM supplier_invoices WHERE id = ? AND status = 'draft'",
			h.usePostgres)
		if _, err := h.db.ExecContext(ctx, delQ, id); err != nil {
			// StatutAvant/Apres renseignes, comme dans les six autres branches.
			// Sans eux, la trace enregistrait une transition "" -> "" et la
			// question « dans quel etat etait-elle ? » restait sans reponse --
			// alors qu'ici le statut EST connu : c'est un brouillon.
			return cancelResult{ID: id, Outcome: "refused", Detail: "suppression impossible",
				StatutAvant: status, StatutApres: status}
		}
		return cancelResult{ID: id, Outcome: "deleted",
			Detail:      "brouillon supprimé — rien n'était entré dans les livres",
			StatutAvant: status, StatutApres: "(supprimée)"}

	case "paid":
		return cancelResult{ID: id, Outcome: "refused",
			Detail: "facture déjà payée : l'argent est parti, l'annuler laisserait " +
				"un décaissement sans dette en face. Saisissez le remboursement du fournisseur.",
			StatutAvant: status, StatutApres: status}
	}

	// Reste « booked ». Un règlement partiel compte comme un paiement.
	if round2(paid) != 0 {
		return cancelResult{ID: id, Outcome: "refused",
			Detail:      fmt.Sprintf("un règlement de %.2f a déjà été enregistré", paid),
			StatutAvant: status, StatutApres: status}
	}

	reversal, err := h.reverseSupplierEntry(ctx, entryID, userID, ip, reason, reference)
	if err != nil {
		return cancelResult{ID: id, Outcome: "refused", Detail: err.Error(),
			StatutAvant: status, StatutApres: status}
	}

	// Le statut ne bascule qu'APRÈS l'extourne. Dans l'autre ordre, un échec de
	// l'écriture laisserait une facture annoncée annulée dont la charge court
	// toujours — précisément le défaut qu'on ferme ici.
	updQ := db.Rebind(
		"UPDATE supplier_invoices SET status = 'cancelled', updated_at = ? WHERE id = ?",
		h.usePostgres)
	if _, err := h.db.ExecContext(ctx, updQ, time.Now().UTC(), id); err != nil {
		return cancelResult{ID: id, Outcome: "refused",
			Detail:      "l'extourne " + reversal + " est passée mais le statut n'a pas pu être changé",
			StatutAvant: status, StatutApres: status}
	}
	return cancelResult{ID: id, Outcome: "cancelled", EntryID: reversal,
		Detail:      "extournée — charge et TVA déductible neutralisées",
		StatutAvant: status, StatutApres: "cancelled"}
}

// reverseSupplierEntry passe l'écriture inverse de la comptabilisation.
//
// Elle est construite en RELISANT l'écriture d'origine plutôt qu'en recalculant
// depuis la facture : entre les deux, un taux de TVA ou un compte de charge a
// pu être corrigé, et une extourne qui ne solde pas exactement ce qui a été
// passé laisse un résidu sur des comptes que personne ne va rapprocher.
func (h *SupplierInvoicesHandler) reverseSupplierEntry(
	ctx context.Context, entryID, userID, ip, reason, reference string,
) (string, error) {
	if h.accountingSvc == nil {
		return "", fmt.Errorf("service comptable indisponible")
	}
	if entryID == "" {
		// Comptabilisée sans écriture : le cas existe sur des pièces à zéro.
		// Rien à extourner, l'annulation seule suffit.
		return "", nil
	}

	q := db.Rebind(`SELECT account_id, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0),
	                       COALESCE(description, '')
	                FROM journal_lines WHERE entry_id = ? ORDER BY sequence`, h.usePostgres)
	rows, err := h.db.QueryContext(ctx, q, entryID)
	if err != nil {
		return "", fmt.Errorf("lecture de l'écriture d'origine: %w", err)
	}
	defer rows.Close()

	var lines []accounting.LineInput
	seq := 0
	for rows.Next() {
		var (
			accountID   string
			debit       float64
			credit      float64
			description string
		)
		if err := rows.Scan(&accountID, &debit, &credit, &description); err != nil {
			return "", fmt.Errorf("lecture d'une ligne: %w", err)
		}
		// Débit et crédit sont échangés : c'est toute l'extourne.
		l := accounting.LineInput{
			AccountID: accountID, Description: "Extourne — " + description, Sequence: seq,
		}
		if round2(debit) != 0 {
			v := round2(debit)
			l.CreditAmount = &v
		}
		if round2(credit) != 0 {
			v := round2(credit)
			l.DebitAmount = &v
		}
		lines = append(lines, l)
		seq++
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("parcours des lignes: %w", err)
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("l'écriture d'origine %s ne porte aucune ligne", entryID)
	}

	// L'extourne porte la date du JOUR, pas celle de la facture. La postdater
	// au jour de l'achat rouvrirait un exercice peut-être clos, et ferait
	// disparaître la charge d'une période déjà déclarée — ce que le
	// verrouillage de période refuse, à juste titre.
	motif := reason
	if motif == "" {
		motif = "annulation"
	}
	entry, err := h.accountingSvc.CreateEntry(ctx, userID, accounting.CreateEntryRequest{
		Date: time.Now().UTC(),
		Description: fmt.Sprintf("Extourne facture fournisseur %s — %s",
			reference, motif),
		Lines: lines,
	})
	if err != nil {
		return "", fmt.Errorf("création de l'extourne: %w", err)
	}

	// Marquer l'écriture comme extourne, et la rattacher à celle qu'elle annule.
	//
	// Ces deux colonnes existaient et n'étaient renseignées QUE par le chemin
	// des factures clients (invoicing/service.go). Une extourne fournisseur
	// partait donc dans l'archive légale — celle que le CO art. 958f impose de
	// conserver dix ans et qu'on remet à sa fiduciaire — comme une écriture
	// ordinaire, sans lien vers ce qu'elle annule. Elle s'y décrivait pourtant
	// elle-même comme une extourne, dans son libellé.
	//
	// AVANT la comptabilisation, et pas après : trg_journal_entries_no_update
	// (migration 0001) refuse toute mise à jour d'une écriture dont le statut
	// est déjà « posted ». C'est aussi l'ordre que suit le chemin client.
	flagQ := db.Rebind(
		`UPDATE journal_entries SET is_reversal = 1, reversal_of_id = ? WHERE id = ?`,
		h.usePostgres)
	if _, err := h.db.ExecContext(ctx, flagQ, entryID, entry.ID); err != nil {
		return "", fmt.Errorf("marquage de l'extourne %s: %w", entry.ID, err)
	}

	if err := h.accountingSvc.PostEntry(ctx, userID, entry.ID, ip); err != nil {
		return "", fmt.Errorf("comptabilisation de l'extourne %s: %w", entry.ID, err)
	}
	return entry.ID, nil
}

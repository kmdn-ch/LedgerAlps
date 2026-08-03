package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Anonymisation d'un contact — nLPD (RS 235.1) art. 6 al. 4 et art. 32.
//
// Ce que la loi demande, et ce qu'elle interdit en même temps :
//
//   - nLPD art. 6 al. 4 : détruire ou anonymiser les données personnelles dès
//     qu'elles ne sont plus nécessaires ;
//   - nLPD art. 32 : la personne concernée peut demander l'effacement ;
//   - CO art. 958f : conserver dix ans les livres et les pièces comptables ;
//   - LTVA art. 26 : la facture doit nommer son destinataire.
//
// Ce n'est pas une contradiction, mais une confusion fréquente : ce que la loi
// commerciale protège, c'est la **pièce**, pas la fiche client. Depuis que
// chaque facture porte l'identité de son destinataire telle qu'elle était à
// l'émission (migration 0014), la fiche peut être vidée sans qu'aucune pièce
// comptable ne perde une mention obligatoire.
//
// Deux garde-fous délibérés :
//
//  1. L'opération est **irréversible et le dit**. Un « annuler » laisserait
//     croire que les données sont récupérables ; elles ne le sont pas, et c'est
//     précisément ce qu'on a promis à la personne concernée.
//  2. Elle refuse un contact déjà anonymisé, plutôt que de réécrire la date et
//     de faire croire à un traitement récent.

// anonymisedLabel remplace le nom. Un identifiant lisible reste nécessaire pour
// que les listes et les rapports ne montrent pas une ligne vide, ce qui
// passerait pour un défaut d'affichage plutôt que pour une décision.
func anonymisedLabel(id string) string {
	suffix := id
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return "Contact anonymisé (" + suffix + ")"
}

type anonymiseResult struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	AnonymisedAt  time.Time `json:"anonymised_at"`
	InvoicesKept  int       `json:"invoices_kept"`
	LegalBasis    []string  `json:"legal_basis"`
	WhatWasErased []string  `json:"what_was_erased"`
	WhatWasKept   []string  `json:"what_was_kept"`
}

// AnonymiseContact POST /api/v1/contacts/:id/anonymise
//
// Accès : administrateur uniquement. Effacer les données d'une personne est une
// décision, pas une opération de saisie.
func (h *ContactsHandler) AnonymiseContact(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "seul un administrateur peut anonymiser un contact"})
		return
	}

	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var name string
	var already sql.NullTime
	readQ := db.Rebind(
		`SELECT name, anonymised_at FROM contacts WHERE id = ?`, h.usePostgres)
	switch err := h.db.QueryRowContext(ctx, readQ, id).Scan(&name, &already); {
	case err == sql.ErrNoRows:
		c.JSON(http.StatusNotFound, gin.H{"error": "contact introuvable"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if already.Valid {
		c.JSON(http.StatusConflict, gin.H{
			"error":         "ce contact a déjà été anonymisé le " + already.Time.Format("02.01.2006"),
			"anonymised_at": already.Time,
		})
		return
	}

	// Compte des pièces conservées : c'est le chiffre qui rassure l'utilisateur
	// sur ce que l'opération NE fait pas.
	var invoices int
	if err := h.db.QueryRowContext(ctx, db.Rebind(
		`SELECT COUNT(*) FROM invoices WHERE contact_id = ?`, h.usePostgres), id,
	).Scan(&invoices); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Refus si une facture n'a pas son identité figée : l'anonymiser effacerait
	// le destinataire d'une pièce comptable. En pratique le rattrapage au
	// démarrage l'empêche, mais s'en remettre à cela serait supposer plutôt que
	// vérifier — et l'erreur serait irréversible.
	var unfrozen int
	if err := h.db.QueryRowContext(ctx, db.Rebind(`
		SELECT COUNT(*) FROM invoices
		WHERE contact_id = ? AND (recipient_name IS NULL OR recipient_name = '')`,
		h.usePostgres), id).Scan(&unfrozen); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if unfrozen > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf(
				"%d facture(s) de ce contact ne portent pas l'identité de leur destinataire. "+
					"L'anonymiser effacerait une mention que la LTVA art. 26 rend obligatoire. "+
					"Redémarrez LedgerAlps pour que ces factures soient complétées, puis réessayez.",
				unfrozen),
		})
		return
	}

	now := time.Now().UTC()
	label := anonymisedLabel(id)

	// La ligne est conservée, vidée. La supprimer casserait la clé étrangère que
	// portent les factures, et donc le lien entre la pièce et son écriture.
	updQ := db.Rebind(`
		UPDATE contacts SET
			name = ?, legal_name = NULL, email = NULL, phone = NULL,
			address = NULL, city = NULL, postal_code = NULL,
			iban = NULL, qr_iban = NULL, vat_number = NULL, uid_number = NULL,
			notes = NULL, is_active = 0,
			anonymised_at = ?, updated_at = ?
		WHERE id = ?`, h.usePostgres)
	if _, err := h.db.ExecContext(ctx, updQ, label, now, now, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, anonymiseResult{
		ID:           id,
		Label:        label,
		AnonymisedAt: now,
		InvoicesKept: invoices,
		LegalBasis: []string{
			"nLPD art. 6 al. 4 — anonymisation dès que les données ne sont plus nécessaires",
			"nLPD art. 32 — droit à l'effacement",
			"CO art. 958f — conservation dix ans des pièces comptables",
			"LTVA art. 26 — la facture doit nommer son destinataire",
		},
		WhatWasErased: []string{
			"nom et raison sociale", "adresse postale", "courriel et téléphone",
			"IBAN et QR-IBAN", "numéro de TVA et IDE", "notes",
		},
		WhatWasKept: []string{
			fmt.Sprintf("%d document(s) comptable(s), avec l'identité du destinataire telle qu'elle figurait à l'émission", invoices),
			"les écritures au journal et leur chaîne d'intégrité",
			"la date de l'anonymisation, comme preuve du traitement",
		},
	})
}

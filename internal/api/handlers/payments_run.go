package handlers

// Ordre de paiement des factures fournisseurs (pain.001).
//
// # Pourquoi cette route existe
//
// L'export pain.001 existait déjà, mais il fallait lui décrire chaque virement
// à la main dans un corps JSON : nom du créancier, IBAN, montant, référence.
// L'écran le disait sans détour — « exportez via l'API POST
// /api/v1/payments/export ». Autant dire que la fonction n'existait pas : elle
// était réservée à qui sait forger une requête HTTP, c'est-à-dire à personne
// parmi les gens à qui ce produit s'adresse.
//
// # Les montants ne viennent PAS du navigateur
//
// La sélection porte sur des identifiants de factures ; le serveur relit le
// créancier, l'IBAN, le montant et la référence dans la base. C'est la
// différence entre « payer ces factures » et « virer ces sommes » : dans le
// second cas, un navigateur compromis, une extension bavarde ou une simple
// erreur d'arrondi côté interface suffiraient à changer un montant. Ici, ce qui
// part à la banque est ce qui est dans les livres, et rien d'autre.
//
// # Générer le fichier n'est PAS payer
//
// Aucun statut ne bouge à l'export. La facture reste « comptabilisée » jusqu'à
// ce que le débit apparaisse au relevé bancaire — c'est le rapprochement
// camt.053 qui l'établit. Marquer « payée » à la génération serait une
// affirmation que rien ne soutient : le fichier peut n'être jamais déposé,
// refusé par la banque, ou modifié entre-temps.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/iso20022"
)

// payableItem est une facture fournisseur en attente de règlement.
type payableItem struct {
	ID           string  `json:"id"`
	SupplierID   string  `json:"supplier_id"`
	SupplierName string  `json:"supplier_name"`
	Reference    string  `json:"reference"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	DueDate      string  `json:"due_date"`
	DaysLate     int     `json:"days_late"`

	// IBAN retenu pour ce paiement, et sa nature. Un QR-IBAN se reconnaît à son
	// identifiant d'institution (positions 5 à 9, valeurs 30000 à 31999) et
	// n'accepte QU'une référence QR.
	IBAN     string `json:"iban"`
	IsQRIBAN bool   `json:"is_qr_iban"`
	// Référence de paiement et son type déduit : QRR, SCOR, ou vide.
	PaymentReference string `json:"payment_reference"`
	ReferenceType    string `json:"reference_type"`

	// BlockedReason est vide quand la facture est payable. Sinon il dit ce qui
	// manque ET où le corriger : « IBAN manquant » sans indiquer la fiche du
	// fournisseur laisse chercher.
	BlockedReason string `json:"blocked_reason"`
}

type PaymentRunHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewPaymentRunHandler(database *sql.DB, usePostgres bool) *PaymentRunHandler {
	return &PaymentRunHandler{db: database, usePostgres: usePostgres}
}

// ListPayable GET /api/v1/payments/payable
//
// Les factures fournisseurs comptabilisées et non réglées, avec ce qui les
// empêche éventuellement d'être payées.
func (h *PaymentRunHandler) ListPayable(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	items, err := h.loadPayable(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	debtorName, debtorIBAN := h.debtor(ctx)
	debtorProblem := ""
	switch {
	case strings.TrimSpace(debtorIBAN) == "":
		debtorProblem = "L'IBAN de votre entreprise n'est pas renseigné. " +
			"Paramètres → Entreprise : sans lui, aucun fichier de paiement ne peut être produit."
	case compliance.ValidateIBAN(debtorIBAN) != nil:
		debtorProblem = "L'IBAN de votre entreprise n'est pas valide (Paramètres → Entreprise)."
	case strings.TrimSpace(debtorName) == "":
		debtorProblem = "Le nom de votre entreprise n'est pas renseigné (Paramètres → Entreprise)."
	}

	c.JSON(http.StatusOK, gin.H{
		"debtor": gin.H{
			"name":    debtorName,
			"iban":    compliance.FormatIBAN(debtorIBAN),
			"problem": debtorProblem,
		},
		"items": items,
	})
}

// debtor lit l'identité de l'entreprise qui paie.
func (h *PaymentRunHandler) debtor(ctx context.Context) (name, iban string) {
	q := db.Rebind(
		`SELECT COALESCE(company_name,''), COALESCE(iban,'') FROM company_settings LIMIT 1`,
		h.usePostgres)
	_ = h.db.QueryRowContext(ctx, q).Scan(&name, &iban)
	return name, iban
}

// loadPayable charge les factures payables, éventuellement restreintes à une
// sélection.
//
// Le filtre porte sur le statut « comptabilisée » : un brouillon n'est pas
// encore une charge dans les livres, et le payer avant de l'enregistrer inverse
// l'ordre — la trésorerie bougerait sans que la dette existe. Le refus est donc
// une règle comptable, pas une contrainte technique, et l'écran le dit.
func (h *PaymentRunHandler) loadPayable(ctx context.Context, ids []string) ([]payableItem, error) {
	q := `
		SELECT si.id, si.supplier_id, COALESCE(c.name,''), si.supplier_reference,
		       si.total_amount - si.amount_paid, si.currency,
		       COALESCE(si.due_date, ''), COALESCE(si.payment_reference, ''),
		       COALESCE(c.iban, ''), COALESCE(c.qr_iban, '')
		FROM supplier_invoices si
		LEFT JOIN contacts c ON c.id = si.supplier_id
		WHERE si.status = 'booked' AND si.total_amount - si.amount_paid > 0`

	args := []any{}
	if len(ids) > 0 {
		q += " AND si.id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
		for _, id := range ids {
			args = append(args, id)
		}
	}
	q += " ORDER BY si.due_date, si.supplier_reference"

	rows, err := h.db.QueryContext(ctx, db.Rebind(q, h.usePostgres), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	today := time.Now().UTC().Truncate(24 * time.Hour)
	items := []payableItem{}
	for rows.Next() {
		var it payableItem
		var iban, qrIBAN string
		if err := rows.Scan(&it.ID, &it.SupplierID, &it.SupplierName, &it.Reference,
			&it.Amount, &it.Currency, &it.DueDate, &it.PaymentReference,
			&iban, &qrIBAN); err != nil {
			return nil, err
		}
		if it.Currency == "" {
			it.Currency = "CHF"
		}
		if d, err := time.Parse("2006-01-02", firstTen(it.DueDate)); err == nil {
			if late := int(today.Sub(d).Hours() / 24); late > 0 {
				it.DaysLate = late
			}
		}
		decidePayment(&it, iban, qrIBAN)
		items = append(items, it)
	}
	return items, rows.Err()
}

// decidePayment choisit l'IBAN et le type de référence, ou dit ce qui bloque.
//
// La règle vient des Implementation Guidelines SIX (QR-facture v2.4 §4.2.2,
// champs 28 et 29) et vaut dans les deux sens : une référence QR EXIGE un
// QR-IBAN, un IBAN ordinaire n'accepte PAS de référence QR. L'inadéquation
// entre les deux est la première cause de rejet bancaire — d'où le refus ici
// plutôt qu'un fichier que la banque renverra.
func decidePayment(it *payableItem, iban, qrIBAN string) {
	ref := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(it.PaymentReference)))
	it.PaymentReference = ref

	isQRRef := len(ref) == 27 && allDigits(ref)
	isSCOR := strings.HasPrefix(ref, "RF") && len(ref) >= 5 && len(ref) <= 25

	switch {
	case isQRRef:
		if strings.TrimSpace(qrIBAN) == "" {
			it.BlockedReason = "Cette facture porte une référence QR, qui exige un QR-IBAN. " +
				"Renseignez-le sur la fiche du fournisseur (Contacts → " + it.SupplierName + ")."
			return
		}
		it.IBAN, it.IsQRIBAN, it.ReferenceType = compliance.NormaliseIBAN(qrIBAN), true, "QRR"
	case isSCOR:
		if strings.TrimSpace(iban) == "" {
			it.BlockedReason = "Aucun IBAN sur la fiche du fournisseur (Contacts → " +
				it.SupplierName + ")."
			return
		}
		it.IBAN, it.ReferenceType = compliance.NormaliseIBAN(iban), "SCOR"
	case ref != "":
		it.BlockedReason = "La référence de paiement n'est ni une référence QR " +
			"(27 chiffres) ni une référence créancière ISO 11649 (RF…). Corrigez-la sur la " +
			"facture, ou effacez-la pour payer sans référence structurée."
		return
	default:
		// Sans référence, un IBAN ordinaire suffit : le motif du virement part
		// en texte libre. C'est le cas courant hors QR-facture.
		if strings.TrimSpace(iban) == "" {
			it.BlockedReason = "Aucun IBAN sur la fiche du fournisseur (Contacts → " +
				it.SupplierName + ")."
			return
		}
		it.IBAN = compliance.NormaliseIBAN(iban)
	}

	if err := compliance.ValidateIBAN(it.IBAN); err != nil {
		it.BlockedReason = fmt.Sprintf("L'IBAN du fournisseur n'est pas valide : %v.", err)
		it.IBAN = ""
		return
	}
	if it.Amount <= 0 {
		it.BlockedReason = "Le solde à payer est nul."
	}
}

// buildRunTransactions convertit une sélection de factures en virements.
//
// Renvoie une erreur nommant la facture fautive dès qu'une seule est bloquée :
// produire un fichier amputé silencieusement laisserait croire que tout est
// payé, et le manque ne se découvrirait qu'à la relance du fournisseur.
func (h *PaymentRunHandler) buildRunTransactions(
	ctx context.Context, ids []string,
) ([]iso20022.CreditTransfer, error) {
	items, err := h.loadPayable(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("lecture des factures: %w", err)
	}
	if len(items) != len(ids) {
		return nil, fmt.Errorf(
			"%d facture(s) sur %d ne sont plus payables : elles ont pu être réglées ou "+
				"annulées entre-temps. Rechargez la liste", len(ids)-len(items), len(ids))
	}

	txs := make([]iso20022.CreditTransfer, 0, len(items))
	for _, it := range items {
		if it.BlockedReason != "" {
			return nil, fmt.Errorf("facture %s (%s) : %s",
				it.Reference, it.SupplierName, it.BlockedReason)
		}
		t := iso20022.CreditTransfer{
			// L'identifiant de bout en bout permet de reconnaître le débit au
			// relevé. Il porte la référence du fournisseur, pas un compteur :
			// c'est ce qu'on cherche quand on remonte une ligne bancaire.
			EndToEndID:   endToEndID(it.Reference, it.ID),
			CreditorName: it.SupplierName,
			CreditorIBAN: it.IBAN,
			Amount:       it.Amount,
			Currency:     it.Currency,
		}
		if it.ReferenceType != "" {
			t.Reference = it.PaymentReference
			t.ReferenceType = it.ReferenceType
		} else {
			t.Unstructured = strings.TrimSpace("Facture " + it.Reference)
		}
		txs = append(txs, t)
	}
	return txs, nil
}

// endToEndID construit un identifiant lisible et conforme.
//
// Les caractères admis sont restreints par la norme ; un numéro de facture
// fournisseur contient parfois des espaces, des barres obliques ou des
// accents. On nettoie plutôt que de refuser : le champ sert à retrouver le
// paiement, pas à valider quoi que ce soit.
func endToEndID(reference, id string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return -1
		}
	}, reference)
	if clean == "" {
		clean = id
	}
	if len(clean) > 35 {
		clean = clean[:35]
	}
	return clean
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func firstTen(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

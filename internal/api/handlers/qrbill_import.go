package handlers

// Lire le QR d'une facture fournisseur déposée.
//
// # Ce que cette route fait, et ce qu'elle ne fait pas
//
// Elle LIT. Elle n'enregistre rien, ne crée aucun contact, ne modifie aucune
// facture. Elle rend ce que le QR contient et laisse l'interface le proposer,
// à charge pour l'utilisateur de confirmer.
//
// Ce choix n'est pas de la prudence de principe. Une facture fournisseur entre
// dans les livres ET dans la déclaration de TVA : un champ pré-rempli qu'on
// relit vaut mieux qu'un champ juste qu'on n'a pas vu. Enregistrer d'office
// ferait entrer dans la comptabilité une valeur que personne n'a regardée.
//
// # Rien ne sort de la machine
//
// Le fichier est lu en mémoire, ses images extraites dans un dossier temporaire
// effacé aussitôt, et rien n'est transmis nulle part. Une facture fournisseur
// contient le nom, l'adresse et l'IBAN d'un tiers : l'envoyer à un service
// d'extraction en ligne contredirait la promesse du produit.

import (
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/qrbill"
)

type QRBillHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewQRBillHandler(database *sql.DB, usePostgres bool) *QRBillHandler {
	return &QRBillHandler{db: database, usePostgres: usePostgres}
}

// ReadSupplierBill POST /api/v1/supplier-invoices/read-qr
//
// Reçoit un PDF ou une image en multipart (champ « file ») et rend ce que le
// QR-facture contient, plus le fournisseur déjà connu s'il en existe un.
func (h *QRBillHandler) ReadSupplierBill(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "aucun fichier reçu (champ « file » attendu)"})
		return
	}
	if fh.Size > qrbill.MaxPDFBytes {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "le fichier dépasse 10 Mo"})
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "le fichier n'a pas pu être lu"})
		return
	}
	defer f.Close()

	data, err := qrbill.ReadAllLimited(f)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	var bill *qrbill.Bill
	switch strings.ToLower(filepath.Ext(fh.Filename)) {
	case ".pdf":
		bill, err = qrbill.DecodePDF(data)
	case ".png", ".jpg", ".jpeg":
		bill, err = qrbill.DecodeImage(data)
	default:
		// On tente quand même le PDF : l'extension est un indice, pas une
		// preuve, et un fichier renommé reste lisible.
		bill, err = qrbill.DecodePDF(data)
	}

	if err != nil {
		// « Aucun QR trouvé » n'est pas une panne : beaucoup de factures n'en
		// portent pas, et la saisie à la main reste le chemin normal. Le
		// distinguer d'une erreur de lecture évite de faire soupçonner un
		// fichier corrompu là où il n'y a rien à trouver.
		if errors.Is(err, qrbill.ErrNoQRCode) {
			c.JSON(http.StatusOK, gin.H{
				"found": false,
				"reason": "Aucun QR-facture n'a été trouvé dans ce document. Certaines " +
					"factures n'en portent pas, et sur d'autres le code est dessiné en " +
					"traits plutôt qu'en image — LedgerAlps ne sait alors pas le lire. " +
					"Saisissez les champs à la main.",
			})
			return
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"found": false, "error": err.Error()})
		return
	}

	// Le fournisseur est-il déjà connu ? La comparaison porte sur l'IBAN, qui
	// identifie un compte sans ambiguïté — un nom se saisit de dix façons.
	supplierID, supplierName := h.findSupplierByIBAN(c, bill.CreditorIBAN)

	c.JSON(http.StatusOK, gin.H{
		"found": true,
		"bill":  bill,
		"supplier": gin.H{
			"id":   supplierID,
			"name": supplierName,
		},
		// Dit clairement ce qui reste à faire. Le QR ne porte ni le montant hors
		// taxe, ni le taux de TVA, ni le compte de charge : ce sont des
		// décisions comptables, pas des données du bulletin.
		"note": "Le QR ne contient ni le montant hors taxe, ni le taux de TVA, ni le " +
			"compte de charge : complétez-les et vérifiez le reste avant d'enregistrer.",
	})
}

// findSupplierByIBAN cherche un contact fournisseur par IBAN ou QR-IBAN.
func (h *QRBillHandler) findSupplierByIBAN(c *gin.Context, iban string) (id, name string) {
	if iban == "" {
		return "", ""
	}
	q := db.Rebind(`
		SELECT id, name FROM contacts
		WHERE is_active = 1
		  AND contact_type IN ('supplier', 'both')
		  AND (REPLACE(UPPER(COALESCE(iban, '')), ' ', '') = ?
		    OR REPLACE(UPPER(COALESCE(qr_iban, '')), ' ', '') = ?)
		LIMIT 1`, h.usePostgres)
	_ = h.db.QueryRowContext(c.Request.Context(), q, iban, iban).Scan(&id, &name)
	return id, name
}

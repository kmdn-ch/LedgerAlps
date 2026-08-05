package handlers

// Dossier de validation SIX.
//
// SIX exploite un portail qui contrôle les Swiss Payment Standards — le
// *payload* du code QR ET l'image du bulletin. C'est la seule vérification qui
// fasse autorité, et c'est précisément celle qui manquait : nos tests vérifient
// notre lecture de la spécification, pas la conformité du bulletin produit.
// Ils passeraient tous avec un contresens sur la spécification.
//
// LedgerAlps ne peut pas s'y connecter : le portail demande un compte, et
// créer un compte au nom de quelqu'un d'autre n'est pas une chose qu'un
// logiciel fait à sa place. Ce qui est livré est donc le dossier à déposer —
// le payload exact, le bulletin, et la marche à suivre — pour que la
// vérification tienne en un dépôt de fichier plutôt qu'en une reconstitution à
// la main.
//
// Le payload provient de la MÊME fonction que celle du rendu (pdfsvc.QRPayload).
// Deux constructions séparées divergeraient un jour, et ce jour-là on ferait
// valider autre chose que ce qu'on envoie aux clients — c'est-à-dire qu'on
// validerait pour rien.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	pdfsvc "github.com/kmdn-ch/ledgeralps/internal/services/pdf"
)

// PortalURL est l'adresse du portail de validation des Swiss Payment Standards.
const PortalURL = "https://validation.iso-payments.ch/"

// SixValidationDossier GET /api/v1/invoices/:id/six-validation
//
// Renvoie une archive contenant tout ce que le portail demande.
func (h *InvoicesHandler) SixValidationDossier(c *gin.Context) {
	id := c.Param("id")

	pdfBytes, invoiceNumber, err := h.buildInvoicePDF(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	inv, err := h.invoiceDataFor(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	payload, err := pdfsvc.QRPayload(inv)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "le payload QR n'a pas pu être produit: " + err.Error()})
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	add := func(name string, content []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(content)
		return err
	}

	// Le payload part en .txt, encodé UTF-8 sans BOM et avec des fins de ligne
	// LF : c'est ce que la spécification impose au contenu du QR, et le portail
	// contrôle les octets. Un CRLF ferait échouer la validation pour une raison
	// que personne ne devinerait.
	if err := add(invoiceNumber+"-payload.txt", []byte(payload)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := add(invoiceNumber+"-bulletin.pdf", pdfBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := add("LISEZ-MOI.txt", []byte(dossierReadme(invoiceNumber))); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	name := fmt.Sprintf("validation-six-%s-%s.zip", invoiceNumber, time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func dossierReadme(invoiceNumber string) string {
	return `DOSSIER DE VALIDATION — SWISS PAYMENT STANDARDS
================================================

Facture : ` + invoiceNumber + `
Portail : ` + PortalURL + `

CE QUE CONTIENT CETTE ARCHIVE

  ` + invoiceNumber + `-payload.txt    le contenu exact du code QR
  ` + invoiceNumber + `-bulletin.pdf   le bulletin tel qu'il part au client

Le payload est celui que LedgerAlps encode réellement dans le QR : il sort de la
même fonction que l'impression. Vous validez donc ce que vos clients reçoivent,
et non une reconstitution.

MARCHE À SUIVRE

  1. Ouvrez ` + PortalURL + ` et connectez-vous.
     Le portail demande un compte. LedgerAlps ne peut pas le créer à votre
     place, et ne le fera pas : un compte engage celui à qui il appartient.

  2. Déposez le fichier -payload.txt dans le contrôle du contenu QR.

  3. Déposez le fichier -bulletin.pdf dans le contrôle de l'image.
     Le portail vérifie les deux séparément : un payload correct sur un
     bulletin mal disposé reste refusé par les banques.

  4. Reportez le résultat dans Paramètres > Maintenance > Conformité.

CE QUE CETTE VALIDATION APPORTE, ET CE QU'ELLE N'APPORTE PAS

Elle apporte le seul avis qui fasse autorité sur la conformité du bulletin.
Les tests automatisés de LedgerAlps vérifient notre lecture de la
spécification ; ils passeraient tous avec un contresens sur celle-ci.

Elle n'apporte AUCUNE certification. Il n'existe pas de « certification
ISO 20022 » : le Registration Authority l'écrit lui-même, et les organismes qui
vendent un « audit certifiant » ne s'appuient sur rien. Ce qui a de la valeur en
Suisse est la conformité aux Swiss Payment Standards de SIX, constatée par ce
portail.

À REFAIRE QUAND

  - après une mise à jour de LedgerAlps qui touche au bulletin ;
  - après un changement d'IBAN, de QR-IBAN ou de raison sociale ;
  - à chaque nouvelle version des Swiss Payment Standards.
`
}

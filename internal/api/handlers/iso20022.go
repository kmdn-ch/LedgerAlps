package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/banking"
	"github.com/kmdn-ch/ledgeralps/internal/services/iso20022"
)

// ISO20022Handler handles ISO 20022 payment generation and bank statement import.
type ISO20022Handler struct {
	// reconcile conserve les ecritures analysees. Nil dans les tests qui
	// n'exercent que la lecture du XML.
	reconcile *banking.Service
	// run construit les virements depuis les factures fournisseurs. Nil dans les
	// tests qui n'exercent que la generation du XML.
	run *PaymentRunHandler

	// db et usePostgres servent UNIQUEMENT a la piste d'audit.
	//
	// Un import de releve bancaire et une generation d'ordre de virement sont
	// deux mouvements d'argent : le premier fait entrer des ecritures dans les
	// livres, le second produit le fichier que la banque execute. Ni l'un ni
	// l'autre ne laissait de trace, alors que les constantes existaient
	// (ActionBankStatementImported, ActionPaymentFileGenerated). Nil dans les
	// tests qui n'exercent que l'analyse du XML : `trace` est alors silencieuse.
	db          *sql.DB
	usePostgres bool
}

// WithAudit branche la piste d'audit sur les deux operations qui deplacent de
// l'argent. Sans elle, le handler fonctionne mais ne trace rien.
func (h *ISO20022Handler) WithAudit(database *sql.DB, usePostgres bool) *ISO20022Handler {
	h.db = database
	h.usePostgres = usePostgres
	return h
}

func NewISO20022Handler() *ISO20022Handler { return &ISO20022Handler{} }

// WithPaymentRun branche la selection de factures fournisseurs.
func (h *ISO20022Handler) WithPaymentRun(run *PaymentRunHandler) *ISO20022Handler {
	h.run = run
	return h
}

// NewISO20022HandlerWithReconciliation branche la conservation des écritures.
// Le constructeur sans service reste, pour les tests qui n'exercent que
// l'analyse du XML.
func NewISO20022HandlerWithReconciliation(svc *banking.Service) *ISO20022Handler {
	return &ISO20022Handler{reconcile: svc}
}

// ─── pain.001 — Credit Transfer Export ───────────────────────────────────────

type pain001Transaction struct {
	EndToEndID    string  `json:"end_to_end_id"  binding:"required"`
	CreditorName  string  `json:"creditor_name"  binding:"required"`
	CreditorIBAN  string  `json:"creditor_iban"  binding:"required"`
	Amount        float64 `json:"amount"         binding:"required,gt=0"`
	Currency      string  `json:"currency"`
	Reference     string  `json:"reference"`      // reference structuree
	ReferenceType string  `json:"reference_type"` // QRR ou SCOR
	Unstructured  string  `json:"unstructured"`   // motif en texte libre
}

// pain001Request accepte DEUX formes.
//
// La forme historique decrit chaque virement a la main. Elle reste, pour les
// integrations qui l'utilisent deja.
//
// La forme utile — celle de l'interface — ne transmet que des identifiants de
// factures fournisseurs : le serveur relit lui-meme le creancier, l'IBAN, le
// montant et la reference dans les livres. C'est la difference entre « payer ces
// factures » et « virer ces sommes » : dans le second cas, un navigateur
// compromis ou une erreur d'arrondi cote interface suffirait a changer un
// montant qui part a la banque.
//
// Les champs du debiteur ne sont plus obligatoires : ils viennent de la fiche
// entreprise, que personne ne devrait retaper a chaque paiement.
type pain001Request struct {
	ExecutionDate string `json:"execution_date" binding:"required"` // YYYY-MM-DD
	DebtorName    string `json:"debtor_name"`
	DebtorIBAN    string `json:"debtor_iban"`
	DebtorBIC     string `json:"debtor_bic"`

	SupplierInvoiceIDs []string             `json:"supplier_invoice_ids"`
	Transactions       []pain001Transaction `json:"transactions"`
}

// ExportPain001 godoc
// POST /api/v1/payments/export
// Generates a pain.001.001.09 XML file. Returns application/xml as a download.
func (h *ISO20022Handler) ExportPain001(c *gin.Context) {
	var req pain001Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	execDate, err := time.Parse("2006-01-02", req.ExecutionDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "la date d'exécution doit être au format AAAA-MM-JJ"})
		return
	}

	var txs []iso20022.CreditTransfer

	switch {
	case len(req.SupplierInvoiceIDs) > 0:
		if h.run == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "l'ordre de paiement est indisponible"})
			return
		}
		built, err := h.run.buildRunTransactions(c.Request.Context(), req.SupplierInvoiceIDs)
		if err != nil {
			// Le refus nomme la facture et ce qui manque. Produire un fichier
			// ampute laisserait croire que tout est paye, et le manque ne se
			// decouvrirait qu'a la relance du fournisseur.
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		txs = built
		if req.DebtorName == "" || req.DebtorIBAN == "" {
			name, iban := h.run.debtor(c.Request.Context())
			if req.DebtorName == "" {
				req.DebtorName = name
			}
			if req.DebtorIBAN == "" {
				req.DebtorIBAN = iban
			}
		}

	case len(req.Transactions) > 0:
		txs = make([]iso20022.CreditTransfer, len(req.Transactions))
		for i, t := range req.Transactions {
			cur := t.Currency
			if cur == "" {
				cur = "CHF"
			}
			txs[i] = iso20022.CreditTransfer{
				EndToEndID:    t.EndToEndID,
				CreditorName:  t.CreditorName,
				CreditorIBAN:  t.CreditorIBAN,
				Amount:        t.Amount,
				Currency:      cur,
				Reference:     t.Reference,
				ReferenceType: t.ReferenceType,
				Unstructured:  t.Unstructured,
			}
		}

	default:
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "aucune facture sélectionnée"})
		return
	}

	// L'IBAN de l'entreprise est verifie AVANT la generation : sans lui, le
	// generateur echoue sur un message technique, alors que la correction est
	// dans Parametres -> Entreprise.
	if strings.TrimSpace(req.DebtorIBAN) == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "l'IBAN de votre entreprise n'est pas renseigné (Paramètres → Entreprise) : " +
				"aucun ordre de paiement ne peut etre produit sans compte a debiter"})
		return
	}
	if err := compliance.ValidateIBAN(req.DebtorIBAN); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": fmt.Sprintf("l'IBAN de votre entreprise n'est pas valide (%v) — "+
				"corrigez-le dans Parametres -> Entreprise", err)})
		return
	}

	xmlBytes, err := iso20022.GeneratePain001(iso20022.Pain001Request{
		DebtorName:    req.DebtorName,
		DebtorIBAN:    req.DebtorIBAN,
		DebtorBIC:     req.DebtorBIC,
		ExecutionDate: execDate,
		Transactions:  txs,
	})
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("paiements-%s.xml", req.ExecutionDate)

	// Ce fichier est celui que la banque EXECUTE : c'est le moment où l'argent
	// part. Il n'existe pas d'action du produit dont on veuille davantage
	// savoir qui l'a déclenchée, et elle n'était pas tracée.
	//
	// Ni les IBAN des bénéficiaires ni les montants individuels n'entrent dans
	// l'état : le nombre de virements, le total et la date d'exécution
	// suffisent à rapprocher la trace du fichier, et recopier les
	// coordonnées de tiers dans une table conservée dix ans irait contre la
	// nLPD art. 6. Le nom du débiteur est masqué par `masquerEtat`.
	var total float64
	for _, t := range txs {
		total += t.Amount
	}
	trace(c, h.db, h.usePostgres, accounting.TablePayments,
		ActionPaymentFileGenerated, filename, accounting.Creation(map[string]any{
			"fichier":        filename,
			"format":         "pain.001",
			"virements":      len(txs),
			"total":          total,
			"date_execution": req.ExecutionDate,
		}))

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/xml; charset=UTF-8", xmlBytes)
}

// ─── camt.053 — Bank Statement Import ────────────────────────────────────────

// ImportCamt053 godoc
// POST /api/v1/bank-statements/import
// Accepts a raw camt.053.001.08 XML body (Content-Type: application/xml)
// or a multipart file upload (field name: "file").
// Returns parsed bank entries as JSON.
func (h *ISO20022Handler) ImportCamt053(c *gin.Context) {
	var xmlData []byte

	contentType := c.GetHeader("Content-Type")
	if contentType == "application/xml" || contentType == "text/xml" {
		// Raw XML body
		var err error
		xmlData, err = io.ReadAll(io.LimitReader(c.Request.Body, 10<<20)) // 10 MB max
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "le corps de la requête n'a pas pu être lu"})
			return
		}
	} else {
		// Multipart file upload
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "envoyez le XML dans le corps de la requête (Content-Type: application/xml) ou dans le champ « file » d'un formulaire",
			})
			return
		}
		defer file.Close()
		xmlData, err = io.ReadAll(io.LimitReader(file, 10<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "le fichier déposé n'a pas pu être lu"})
			return
		}
	}

	if len(xmlData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty file"})
		return
	}

	entries, err := iso20022.ParseCamt053(xmlData)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// Conserver ce qui vient d'être analysé. Sans cela, le relevé était lu,
	// renvoyé au navigateur, puis oublié : impossible de savoir ce qui avait
	// déjà été traité, et réimporter le relevé du mois obligeait à tout revoir.
	// Les doublons sont comptés, pas réécrits.
	var imported, duplicate int
	if h.reconcile != nil {
		res, impErr := h.reconcile.Import(c.Request.Context(), entries)
		if impErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": impErr.Error()})
			return
		}
		imported, duplicate = res.Imported, res.Duplicate
	}
	// L'import fait ENTRER des ecritures dans les livres. Le compte du
	// nombre importe / doublons ecartes est ce qu'on veut relire apres coup
	// quand un solde ne concorde pas ; les lignes elles-memes vivent deja
	// dans bank_entries, il n'y a pas a les recopier.
	trace(c, h.db, h.usePostgres, TableBankEntries,
		ActionBankStatementImported, fmt.Sprintf("camt053-%d", time.Now().UTC().Unix()),
		accounting.Creation(map[string]any{
			"format":    "camt.053",
			"lues":      len(entries),
			"importees": imported,
			"doublons":  duplicate,
		}))

	// Convert to API-friendly response
	type entryResponse struct {
		Amount          float64 `json:"amount"`
		Currency        string  `json:"currency"`
		IsCredit        bool    `json:"is_credit"`
		BookingDate     string  `json:"booking_date"`
		ValueDate       string  `json:"value_date"`
		BankRef         string  `json:"bank_ref"`
		EndToEndRef     string  `json:"end_to_end_ref,omitempty"`
		QRReference     string  `json:"qr_reference,omitempty"`
		CounterpartName string  `json:"counterpart_name,omitempty"`
		CounterpartIBAN string  `json:"counterpart_iban,omitempty"`
		Unstructured    string  `json:"unstructured,omitempty"`
	}

	result := make([]entryResponse, 0, len(entries))
	for _, e := range entries {
		r := entryResponse{
			Amount:          e.Amount,
			Currency:        e.Currency,
			IsCredit:        e.IsCredit,
			BankRef:         e.BankRef,
			EndToEndRef:     e.EndToEndRef,
			QRReference:     e.QRReference,
			CounterpartName: e.CounterpartName,
			CounterpartIBAN: e.CounterpartIBAN,
			Unstructured:    e.Unstructured,
		}
		if !e.BookingDate.IsZero() {
			r.BookingDate = e.BookingDate.Format("2006-01-02")
		}
		if !e.ValueDate.IsZero() {
			r.ValueDate = e.ValueDate.Format("2006-01-02")
		}
		result = append(result, r)
	}

	// `imported` et `duplicate` SORTENT dans la réponse.
	//
	// Ils étaient calculés puis jetés — le code portait littéralement
	// `_ = imported` / `_ = duplicate`. L'écran de rapprochement, lui, les lit
	// (`d.imported ?? 0`) : il annonçait donc « 0 écriture(s) ajoutée(s) » à
	// chaque import, quel qu'en soit le résultat. Vérifié sur un serveur réel,
	// deux écritures entrées en base et l'écran disant zéro.
	//
	// La distinction compte : « 12 ajoutées » et « 0 ajoutée, 12 déjà connues »
	// décrivent deux situations opposées — un relevé qu'on vient d'intégrer, et
	// un relevé qu'on réimporte par erreur. Sans ces chiffres, l'utilisateur ne
	// peut pas savoir laquelle des deux il vient de provoquer.
	c.JSON(http.StatusOK, gin.H{
		"entries":   result,
		"count":     len(result),
		"imported":  imported,
		"duplicate": duplicate,
	})
}

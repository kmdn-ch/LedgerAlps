package handlers

// La comptabilité simplifiée — le « carnet du lait » du CO art. 957 al. 2.
//
// # Pourquoi c'est une LECTURE
//
// Ce rapport ne crée rien et ne modifie rien : il relit le journal et le
// présente sous la forme que la loi reconnaît pour une entreprise individuelle
// sous le seuil. Il exige donc `PermRead`, comme le bilan et le compte de
// résultat — et une fiduciaire en lecture seule doit pouvoir l'établir, c'est
// même une des raisons pour lesquelles on lui donne un accès.
//
// # LedgerAlps ne devient pas une comptabilité simplifiée pour autant
//
// Le produit continue de tenir la partie double, qui DÉPASSE le minimum légal.
// Ce document est une présentation extraite de ces livres, pas un mode dégradé.
// La distinction compte : elle est à l'avantage de l'utilisateur devant
// l'administration, et elle explique pourquoi rien n'est à « activer ».

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/pdf"
	"github.com/kmdn-ch/ledgeralps/internal/services/simplified"
)

type SimplifiedAccountingHandler struct {
	svc         *simplified.Service
	db          *sql.DB
	usePostgres bool
}

func NewSimplifiedAccountingHandler(database *sql.DB, usePostgres bool) *SimplifiedAccountingHandler {
	return &SimplifiedAccountingHandler{
		svc:         simplified.New(database, usePostgres),
		db:          database,
		usePostgres: usePostgres,
	}
}

// periode lit et valide les bornes communes aux trois sorties.
func (h *SimplifiedAccountingHandler) periode(c *gin.Context) (du, au string, ok bool) {
	du, au = c.Query("from"), c.Query("to")
	if du == "" || au == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "les paramètres « from » et « to » sont requis (AAAA-MM-JJ)"})
		return "", "", false
	}
	if err := simplified.PeriodeValide(du, au); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return "", "", false
	}
	return du, au, true
}

// Carnet GET /api/v1/reports/simplified-accounting
func (h *SimplifiedAccountingHandler) Carnet(c *gin.Context) {
	du, au, ok := h.periode(c)
	if !ok {
		return
	}
	// Dix secondes : le carnet parcourt le journal d'un exercice entier, ce qui
	// est plus long qu'une lecture de fiche.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	carnet, err := h.svc.Etablir(ctx, du, au)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}
	c.JSON(http.StatusOK, carnet)
}

// CarnetCSV GET /api/v1/exports/simplified-accounting.csv
//
// Les trois parties dans un seul fichier, séparées par une ligne vide : c'est
// un document, pas trois. Les découper obligerait à les recoller pour les
// présenter, et laisserait la porte ouverte à n'en remettre qu'une partie.
func (h *SimplifiedAccountingHandler) CarnetCSV(c *gin.Context) {
	du, au, ok := h.periode(c)
	if !ok {
		return
	}
	// Dix secondes : le carnet parcourt le journal d'un exercice entier, ce qui
	// est plus long qu'une lecture de fiche.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	k, err := h.svc.Etablir(ctx, du, au)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	rows := [][]string{
		{"Comptabilité simplifiée (CO art. 957 al. 2)"},
		{"Période", k.Du, "au", k.Au},
		{},
		{"RECETTES"},
		{"Compte", "Libellé", "Montant"},
	}
	for _, l := range k.Recettes {
		rows = append(rows, []string{l.Code, l.Libelle, money(l.Montant)})
	}
	rows = append(rows,
		[]string{"", "Total des recettes", money(k.TotalRecettes)},
		[]string{},
		[]string{"DÉPENSES"},
		[]string{"Compte", "Libellé", "Montant"})
	for _, l := range k.Depenses {
		rows = append(rows, []string{l.Code, l.Libelle, money(l.Montant)})
	}
	rows = append(rows,
		[]string{"", "Total des dépenses", money(k.TotalDepenses)},
		[]string{},
		[]string{"", "RÉSULTAT", money(k.Resultat)},
		[]string{},
		[]string{"ÉTAT DU PATRIMOINE au " + k.Au},
		[]string{"Compte", "Libellé", "Montant"},
		[]string{"Avoirs"})
	for _, p := range k.Avoirs {
		rows = append(rows, []string{p.Code, p.Libelle, money(p.Montant)})
	}
	rows = append(rows,
		[]string{"", "Total des avoirs", money(k.TotalAvoirs)},
		[]string{"Engagements"})
	for _, p := range k.Engagements {
		rows = append(rows, []string{p.Code, p.Libelle, money(p.Montant)})
	}
	rows = append(rows,
		[]string{"", "Total des engagements", money(k.TotalEngagements)},
		[]string{"", "FORTUNE NETTE", money(k.Fortune)},
		[]string{},
		[]string{"Chiffre d'affaires de la période", money(k.Eligibilite.ChiffreAffaires)},
		[]string{"Comptabilité simplifiée admise (< 500 000)", oui(k.Eligibilite.Eligible)},
		[]string{"Assujettissement TVA (≥ 100 000)", oui(k.Eligibilite.AssujettiTVA)},
	)

	writeCSV(c, fmt.Sprintf("comptabilite-simplifiee_%s_%s.csv", k.Du, k.Au), rows)
}

func oui(v bool) string {
	if v {
		return "oui"
	}
	return "non"
}

// CarnetPDF GET /api/v1/reports/simplified-accounting.pdf
//
// C'est CE document que l'on tend à l'administration. Il porte donc l'identité
// de l'entreprise, la base légale, et le régime applicable — un carnet anonyme
// ne prouve rien de qui l'a établi.
func (h *SimplifiedAccountingHandler) CarnetPDF(c *gin.Context) {
	du, au, ok := h.periode(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	k, err := h.svc.Etablir(ctx, du, au)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erreur de base de données"})
		return
	}

	d := pdf.CarnetData{
		Du: k.Du, Au: k.Au,
		TotalRecettes:    k.TotalRecettes,
		TotalDepenses:    k.TotalDepenses,
		Resultat:         k.Resultat,
		TotalAvoirs:      k.TotalAvoirs,
		TotalEngagements: k.TotalEngagements,
		Fortune:          k.Fortune,
		ChiffreAffaires:  k.Eligibilite.ChiffreAffaires,
		Eligible:         k.Eligibilite.Eligible,
		AssujettiTVA:     k.Eligibilite.AssujettiTVA,
	}
	for _, l := range k.Recettes {
		d.Recettes = append(d.Recettes, pdf.CarnetLigne{Code: l.Code, Libelle: l.Libelle, Montant: l.Montant})
	}
	for _, l := range k.Depenses {
		d.Depenses = append(d.Depenses, pdf.CarnetLigne{Code: l.Code, Libelle: l.Libelle, Montant: l.Montant})
	}
	for _, p := range k.Avoirs {
		d.Avoirs = append(d.Avoirs, pdf.CarnetLigne{Code: p.Code, Libelle: p.Libelle, Montant: p.Montant})
	}
	for _, p := range k.Engagements {
		d.Engagements = append(d.Engagements, pdf.CarnetLigne{Code: p.Code, Libelle: p.Libelle, Montant: p.Montant})
	}

	// L'identité de l'entreprise. Un échec de lecture ne doit pas empêcher de
	// produire le document : mieux vaut un carnet sans en-tête qu'aucun carnet.
	var nom, rue, npa, ville, ide, devise string
	q := db.Rebind(`
		SELECT COALESCE(company_name,''), COALESCE(address_street,''),
		       COALESCE(address_postal_code,''), COALESCE(address_city,''),
		       COALESCE(che_number,''), COALESCE(currency,'CHF')
		  FROM company_settings LIMIT 1`, h.usePostgres)
	if err := h.db.QueryRowContext(ctx, q).Scan(&nom, &rue, &npa, &ville, &ide, &devise); err == nil {
		d.Entreprise, d.IDE, d.Devise = nom, ide, devise
		d.Adresse = strings.TrimSpace(strings.TrimSpace(rue+" ") + " " + strings.TrimSpace(npa+" "+ville))
		d.Adresse = strings.TrimSpace(strings.Trim(d.Adresse, ","))
	}
	if d.Entreprise == "" {
		d.Entreprise = "(raison sociale non renseignée)"
	}

	octets, err := pdf.GenerateCarnet(d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "le PDF n'a pas pu être produit"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="comptabilite-simplifiee_%s_%s.pdf"`, k.Du, k.Au))
	c.Data(http.StatusOK, "application/pdf", octets)
}

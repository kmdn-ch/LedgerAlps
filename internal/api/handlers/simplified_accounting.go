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
	"github.com/kmdn-ch/ledgeralps/internal/i18n"
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

	// Le CSV suit la même langue que le PDF : c'est le même document sous une
	// autre forme, et le remettre à sa fiduciaire alémanique en français
	// n'aurait pas plus de sens ici que là.
	lang := i18n.Langue(c)
	L := pdf.LibellesCarnet(lang)

	rows := [][]string{
		{L.Titre + " (" + L.SousTitre + ")"},
		{L.Periode, k.Du, k.Au},
		{},
		{L.Recettes},
		{L.Compte, L.Libelle, L.Montant},
	}
	for _, l := range k.Recettes {
		rows = append(rows, []string{l.Code, l.Libelle, money(l.Montant)})
	}
	rows = append(rows,
		[]string{"", L.TotalRec, money(k.TotalRecettes)},
		[]string{},
		[]string{L.Depenses},
		[]string{L.Compte, L.Libelle, L.Montant})
	for _, l := range k.Depenses {
		rows = append(rows, []string{l.Code, l.Libelle, money(l.Montant)})
	}
	rows = append(rows,
		[]string{"", L.TotalDep, money(k.TotalDepenses)},
		[]string{},
		[]string{"", L.Resultat, money(k.Resultat)},
		[]string{},
		[]string{L.Patrimoine + k.Au},
		[]string{L.Compte, L.Libelle, L.Montant},
		[]string{L.Avoirs})
	for _, p := range k.Avoirs {
		rows = append(rows, []string{p.Code, p.Libelle, money(p.Montant)})
	}
	rows = append(rows,
		[]string{"", L.TotalAvoirs, money(k.TotalAvoirs)},
		[]string{L.Engagements})
	for _, p := range k.Engagements {
		rows = append(rows, []string{p.Code, p.Libelle, money(p.Montant)})
	}
	rows = append(rows,
		[]string{"", L.TotalEngag, money(k.TotalEngagements)},
		[]string{"", L.Fortune, money(k.Fortune)},
		[]string{},
		[]string{L.CA, money(k.Eligibilite.ChiffreAffaires)},
	)

	// Le CSV dit exactement ce que dit le PDF, y compris quand il ne conclut
	// pas. Répondre « non » à « comptabilité simplifiée admise » sur un
	// trimestre laisserait croire à un refus, et « non » à l'assujettissement
	// TVA serait franchement faux : ni l'un ni l'autre ne se mesure sur une
	// période qui n'est pas un exercice.
	if !k.Eligibilite.SurExerciceComplet &&
		k.Eligibilite.ChiffreAffaires < simplified.SeuilComptabiliteSimplifiee {
		rows = append(rows, []string{L.PeriodePartielle})
	} else {
		rows = append(rows,
			[]string{L.LigneAdmise, oui(k.Eligibilite.Eligible, lang)},
			[]string{L.LigneTVA, oui(k.Eligibilite.AssujettiTVA, lang)},
		)
	}

	writeCSV(c, fmt.Sprintf("%s_%s_%s.csv", pdf.NomFichierCarnet(lang), k.Du, k.Au), rows)
}

// oui rend le « oui / non » de la langue du document.
func oui(v bool, l i18n.Lang) string {
	L := pdf.LibellesCarnet(l)
	if v {
		return L.Oui
	}
	return L.Non
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

	// La langue de l'interface AU MOMENT DU CLIC.
	//
	// Elle voyage dans `Accept-Language`, que le client pose à chaque requête.
	// Un carnet établi depuis un écran allemand doit sortir en allemand : c'est
	// la pièce que l'on tend à une administration cantonale, et elle doit
	// parler la langue de son destinataire — pas celle du code source.
	lang := i18n.Langue(c)

	d := pdf.CarnetData{
		Du: k.Du, Au: k.Au, Langue: lang,
		TotalRecettes:    k.TotalRecettes,
		TotalDepenses:    k.TotalDepenses,
		Resultat:         k.Resultat,
		TotalAvoirs:      k.TotalAvoirs,
		TotalEngagements: k.TotalEngagements,
		Fortune:          k.Fortune,
		ChiffreAffaires:  k.Eligibilite.ChiffreAffaires,
		Eligible:         k.Eligibilite.Eligible,
		AssujettiTVA:     k.Eligibilite.AssujettiTVA,

		SurExerciceComplet: k.Eligibilite.SurExerciceComplet,
		// La comparaison au seuil se fait ICI, avec la constante qui porte sa
		// référence légale — le paquet pdf met en page, il ne connaît pas le
		// droit comptable.
		DepasseSeuilRegime: k.Eligibilite.ChiffreAffaires >= simplified.SeuilComptabiliteSimplifiee,
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
		d.Entreprise = pdf.SansRaisonSociale(lang)
	}

	octets, err := pdf.GenerateCarnet(d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "le PDF n'a pas pu être produit"})
		return
	}
	// Le nom du fichier suit la langue lui aussi : il se retrouve dans un
	// dossier de téléchargements parmi cent autres, et c'est par son nom qu'on
	// le reconnaît.
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s_%s_%s.pdf"`, pdf.NomFichierCarnet(lang), k.Du, k.Au))
	c.Data(http.StatusOK, "application/pdf", octets)
}

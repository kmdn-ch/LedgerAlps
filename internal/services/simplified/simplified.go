// Package simplified produit la comptabilité simplifiée du CO art. 957 al. 2 —
// le « carnet du lait ».
//
// # Ce que la loi demande, et ce qu'elle ne demande pas
//
// L'art. 957 al. 2 ch. 1 dispense l'entreprise individuelle dont le chiffre
// d'affaires du dernier exercice est inférieur à 500 000 francs de tenir une
// comptabilité en partie double. Elle doit alors tenir « une comptabilité des
// recettes et des dépenses ainsi que du patrimoine ».
//
// Trois parties, donc, et la troisième est celle qu'on oublie : un relevé de
// recettes et de dépenses SEUL ne satisfait pas l'article. L'état du patrimoine
// — ce que l'on possède et ce que l'on doit à la date de clôture — en fait
// partie.
//
// # Pourquoi ce n'est PAS le compte de résultat
//
// « Recettes et dépenses » veut dire base CAISSE : on compte l'argent quand il
// entre et quand il sort. LedgerAlps tient une comptabilité d'ENGAGEMENT — une
// facture émise devient un produit le jour de son émission, pas le jour de son
// encaissement.
//
// Dériver ce document du compte de résultat produirait donc un compte de
// résultat portant un autre nom, avec les créances non encaissées comptées
// comme des recettes. Un contrôleur qui le rapproche du relevé bancaire voit
// l'écart immédiatement.
//
// # D'où viennent les mouvements
//
// Du journal, et de nulle part ailleurs. Tout mouvement d'argent touche un
// compte de LIQUIDITÉS — caisse, poste, banque. Un débit sur ces comptes est
// une entrée d'argent, un crédit une sortie, et le compte de contrepartie dit
// de quoi il s'agit. C'est exactement la façon dont un carnet du lait se tient
// à la main, et cela se rapproche du relevé bancaire au centime.
//
// Les factures fournisseurs ne pouvaient pas servir de source : leur colonne
// `amount_paid` est un cumul SANS DATE, inutilisable pour rattacher un
// décaissement à une période.
//
// # Le virement interne, et pourquoi il fausse tout
//
// Un retrait au bancomat touche deux comptes de liquidités : crédit banque,
// débit caisse. Ce n'est ni une recette ni une dépense — c'est le même argent
// qui change de poche. Une implémentation naïve le compterait DEUX fois, une
// fois dans chaque colonne, et gonflerait les deux totaux sans jamais changer
// le résultat. Ces écritures sont donc écartées explicitement.
package simplified

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Seuils légaux, en francs.
const (
	// SeuilComptabiliteSimplifiee — CO art. 957 al. 2 ch. 1. Au-delà, la
	// comptabilité en partie double redevient obligatoire et ce document ne
	// peut plus être présenté seul.
	SeuilComptabiliteSimplifiee = 500_000.0

	// SeuilAssujettissementTVA — LTVA art. 10 al. 2 let. a. Au-delà,
	// l'assujettissement est obligatoire et le carnet doit être accompagné du
	// décompte TVA.
	//
	// Deux lois différentes, deux seuils qui ne font pas la même chose : l'un
	// décide de la FORME des livres, l'autre de l'assujettissement à un impôt.
	SeuilAssujettissementTVA = 100_000.0
)

// estLiquidite dit si un compte porte de l'argent disponible.
//
// Plan comptable suisse PME : 1000 caisse, 1010 poste, 1020 banque. La borne
// s'arrête à 1029 — 1060 « Titres cotés » est un placement, pas de l'argent
// qu'on dépense, et 1100 « Débiteurs » est une créance, c'est-à-dire
// exactement ce que la base caisse refuse de compter.
func estLiquidite(code string) bool {
	if len(code) < 4 {
		return false
	}
	return code >= "1000" && code <= "1029"
}

// Ligne est un poste du carnet, agrégé par compte de contrepartie.
type Ligne struct {
	Code    string  `json:"code"`
	Libelle string  `json:"libelle"`
	Montant float64 `json:"montant"`
}

// PostePatrimoine est un élément de l'état du patrimoine.
type PostePatrimoine struct {
	Code    string  `json:"code"`
	Libelle string  `json:"libelle"`
	Montant float64 `json:"montant"`
}

// Eligibilite dit si l'entreprise a le droit de présenter ce document.
//
// Le produire sans le dire serait pire que ne pas le produire : une entreprise
// au-delà du seuil qui remet un carnet du lait à l'administration remet un
// document que la loi ne reconnaît pas dans son cas.
type Eligibilite struct {
	ChiffreAffaires float64 `json:"chiffre_affaires"`
	Eligible        bool    `json:"eligible"`
	// AssujettiTVA dit si le chiffre d'affaires impose l'assujettissement,
	// indépendamment de ce que la fiche entreprise déclare.
	AssujettiTVA bool `json:"assujetti_tva"`
	// StatutDeclare est ce que la fiche entreprise affirme : « liable »,
	// « exempt », ou vide. L'écart entre le chiffre d'affaires réel et cette
	// déclaration est ce qu'il faut montrer.
	StatutDeclare string `json:"statut_declare"`
}

// Carnet est la comptabilité simplifiée d'une période.
type Carnet struct {
	Du string `json:"du"`
	Au string `json:"au"`

	Recettes      []Ligne `json:"recettes"`
	Depenses      []Ligne `json:"depenses"`
	TotalRecettes float64 `json:"total_recettes"`
	TotalDepenses float64 `json:"total_depenses"`
	Resultat      float64 `json:"resultat"`

	// L'état du patrimoine à la date de clôture — la troisième exigence de
	// l'art. 957 al. 2, celle qu'on oublie.
	Avoirs           []PostePatrimoine `json:"avoirs"`
	Engagements      []PostePatrimoine `json:"engagements"`
	TotalAvoirs      float64           `json:"total_avoirs"`
	TotalEngagements float64           `json:"total_engagements"`
	Fortune          float64           `json:"fortune"`

	Eligibilite Eligibilite `json:"eligibilite"`
}

// Service calcule le carnet depuis le journal.
type Service struct {
	db          *sql.DB
	usePostgres bool
}

func New(database *sql.DB, usePostgres bool) *Service {
	return &Service{db: database, usePostgres: usePostgres}
}

// Etablir produit le carnet pour la période [du, au].
func (s *Service) Etablir(ctx context.Context, du, au string) (Carnet, error) {
	c := Carnet{Du: du, Au: au}

	if err := s.mouvements(ctx, &c); err != nil {
		return Carnet{}, err
	}
	if err := s.patrimoine(ctx, &c); err != nil {
		return Carnet{}, err
	}
	if err := s.eligibilite(ctx, &c); err != nil {
		return Carnet{}, err
	}
	return c, nil
}

// mouvements remplit les recettes et les dépenses depuis les mouvements de
// liquidités.
//
// La requête ramène, pour chaque écriture comptabilisée de la période, ses
// lignes avec le code du compte. Le classement se fait ensuite en Go plutôt
// qu'en SQL : reconnaître un virement interne demande de regarder l'écriture
// ENTIÈRE — ses deux côtés à la fois —, ce qu'une agrégation ligne à ligne ne
// permet pas.
func (s *Service) mouvements(ctx context.Context, c *Carnet) error {
	q := db.Rebind(`
		SELECT je.id, a.code, a.name,
		       COALESCE(jl.debit_amount, 0), COALESCE(jl.credit_amount, 0)
		  FROM journal_entries je
		  JOIN journal_lines  jl ON jl.entry_id = je.id
		  JOIN accounts       a  ON a.id = jl.account_id
		 WHERE je.status = 'posted'
		   AND je.date >= ? AND je.date <= ?
		 ORDER BY je.id, a.code`, s.usePostgres)

	rows, err := s.db.QueryContext(ctx, q, c.Du, c.Au)
	if err != nil {
		return fmt.Errorf("lecture des mouvements: %w", err)
	}
	defer rows.Close()

	type ligneJournal struct {
		code, nom     string
		debit, credit float64
	}
	parEcriture := map[string][]ligneJournal{}
	ordre := []string{}

	for rows.Next() {
		var id string
		var l ligneJournal
		if err := rows.Scan(&id, &l.code, &l.nom, &l.debit, &l.credit); err != nil {
			return fmt.Errorf("lecture d'une ligne: %w", err)
		}
		if _, vu := parEcriture[id]; !vu {
			ordre = append(ordre, id)
		}
		parEcriture[id] = append(parEcriture[id], l)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("parcours des mouvements: %w", err)
	}

	recettes := map[string]*Ligne{}
	depenses := map[string]*Ligne{}

	for _, id := range ordre {
		lignes := parEcriture[id]

		// Le virement interne : toutes les lignes touchent des liquidités.
		// L'argent change de poche sans entrer ni sortir.
		queDesLiquidites := true
		for _, l := range lignes {
			if !estLiquidite(l.code) {
				queDesLiquidites = false
				break
			}
		}
		if queDesLiquidites {
			continue
		}

		// Le sens du mouvement est donné par les comptes de liquidités ; la
		// nature, par les contreparties.
		var entree, sortie float64
		for _, l := range lignes {
			if estLiquidite(l.code) {
				entree += l.debit
				sortie += l.credit
			}
		}
		if entree == 0 && sortie == 0 {
			// Écriture sans mouvement d'argent — un amortissement, une
			// régularisation. Elle n'a pas sa place dans un carnet du lait.
			continue
		}

		cible := depenses
		if entree > sortie {
			cible = recettes
		}

		for _, l := range lignes {
			if estLiquidite(l.code) {
				continue
			}
			// La contrepartie porte le montant du côté opposé au mouvement.
			montant := l.credit
			if entree <= sortie {
				montant = l.debit
			}
			if montant == 0 {
				continue
			}
			if p, vu := cible[l.code]; vu {
				p.Montant += montant
			} else {
				cible[l.code] = &Ligne{Code: l.code, Libelle: l.nom, Montant: montant}
			}
		}
	}

	c.Recettes = trier(recettes)
	c.Depenses = trier(depenses)
	for _, l := range c.Recettes {
		c.TotalRecettes += l.Montant
	}
	for _, l := range c.Depenses {
		c.TotalDepenses += l.Montant
	}
	c.Resultat = arrondi(c.TotalRecettes - c.TotalDepenses)
	c.TotalRecettes = arrondi(c.TotalRecettes)
	c.TotalDepenses = arrondi(c.TotalDepenses)
	return nil
}

// patrimoine remplit l'état du patrimoine à la date de clôture.
//
// C'est la troisième exigence de l'art. 957 al. 2, et la seule des trois qui
// ne se lit pas dans les mouvements de la période : elle porte sur un ÉTAT, à
// une date, cumulé depuis l'origine.
func (s *Service) patrimoine(ctx context.Context, c *Carnet) error {
	q := db.Rebind(`
		SELECT a.code, a.name, a.account_type,
		       COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.debit_amount  END), 0),
		       COALESCE(SUM(CASE WHEN je.id IS NOT NULL THEN jl.credit_amount END), 0)
		  FROM accounts a
		  LEFT JOIN journal_lines jl ON jl.account_id = a.id
		  LEFT JOIN journal_entries je
		         ON je.id = jl.entry_id
		        AND je.status = 'posted'
		        AND je.date <= ?
		 WHERE a.account_type IN ('asset','liability')
		 GROUP BY a.code, a.name, a.account_type
		 ORDER BY a.code`, s.usePostgres)

	rows, err := s.db.QueryContext(ctx, q, c.Au)
	if err != nil {
		return fmt.Errorf("lecture du patrimoine: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var code, nom, typ string
		var debit, credit float64
		if err := rows.Scan(&code, &nom, &typ, &debit, &credit); err != nil {
			return fmt.Errorf("lecture d'un poste: %w", err)
		}
		var solde float64
		if typ == "asset" {
			solde = debit - credit
		} else {
			solde = credit - debit
		}
		solde = arrondi(solde)
		// Un compte à zéro n'est pas un poste du patrimoine : l'y faire
		// figurer allongerait le document de lignes vides, dans un document
		// dont la brièveté est justement la raison d'être.
		if solde == 0 {
			continue
		}
		p := PostePatrimoine{Code: code, Libelle: nom, Montant: solde}
		if typ == "asset" {
			c.Avoirs = append(c.Avoirs, p)
			c.TotalAvoirs += solde
		} else {
			c.Engagements = append(c.Engagements, p)
			c.TotalEngagements += solde
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("parcours du patrimoine: %w", err)
	}

	c.TotalAvoirs = arrondi(c.TotalAvoirs)
	c.TotalEngagements = arrondi(c.TotalEngagements)
	c.Fortune = arrondi(c.TotalAvoirs - c.TotalEngagements)
	return nil
}

// eligibilite mesure le chiffre d'affaires de la période et le confronte aux
// deux seuils.
//
// Le chiffre d'affaires se lit sur les comptes de PRODUITS, en base
// d'engagement — c'est la grandeur que la loi vise (« chiffre d'affaires
// réalisé »), et non le total encaissé. Les deux diffèrent, et confondre l'un
// avec l'autre ferait passer sous le seuil quelqu'un qui l'a dépassé.
func (s *Service) eligibilite(ctx context.Context, c *Carnet) error {
	q := db.Rebind(`
		-- COALESCE sur CHAQUE terme, et non sur la somme : le schéma impose
		-- qu'un seul côté soit renseigné, l'autre NULL. « credit - debit »
		-- vaudrait donc NULL sur toute ligne, et le total zéro — un chiffre
		-- d'affaires nul rendrait tout le monde éligible.
		SELECT COALESCE(SUM(COALESCE(jl.credit_amount, 0) - COALESCE(jl.debit_amount, 0)), 0)
		  FROM journal_entries je
		  JOIN journal_lines  jl ON jl.entry_id = je.id
		  JOIN accounts       a  ON a.id = jl.account_id
		 WHERE je.status = 'posted'
		   AND je.date >= ? AND je.date <= ?
		   AND a.account_type = 'revenue'`, s.usePostgres)

	var ca float64
	if err := s.db.QueryRowContext(ctx, q, c.Du, c.Au).Scan(&ca); err != nil {
		return fmt.Errorf("lecture du chiffre d'affaires: %w", err)
	}
	ca = arrondi(ca)

	statut := ""
	stQ := db.Rebind(`SELECT COALESCE(vat_status, '') FROM company_settings LIMIT 1`, s.usePostgres)
	if err := s.db.QueryRowContext(ctx, stQ).Scan(&statut); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lecture du statut TVA: %w", err)
	}

	c.Eligibilite = Eligibilite{
		ChiffreAffaires: ca,
		Eligible:        ca < SeuilComptabiliteSimplifiee,
		AssujettiTVA:    ca >= SeuilAssujettissementTVA,
		StatutDeclare:   statut,
	}
	return nil
}

// trier rend les lignes ordonnées par code de compte.
//
// L'ordre du plan comptable, et non celui des montants : c'est celui que le
// lecteur suit, et il ne change pas d'une période à l'autre — deux exercices
// se comparent ligne à ligne.
func trier(m map[string]*Ligne) []Ligne {
	out := make([]Ligne, 0, len(m))
	for _, l := range m {
		l.Montant = arrondi(l.Montant)
		out = append(out, *l)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Code < out[j-1].Code; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// arrondi ramène au centime.
//
// Les sommes de flottants dérivent : additionner cent lignes produit des
// 1234.5600000000002 qui s'affichent tels quels et font douter d'un document
// destiné à l'administration.
func arrondi(v float64) float64 {
	return math.Round(v*100) / 100
}

// PeriodeValide contrôle les bornes.
func PeriodeValide(du, au string) error {
	d, err := time.Parse("2006-01-02", du)
	if err != nil {
		return fmt.Errorf("« du » doit être au format AAAA-MM-JJ")
	}
	a, err := time.Parse("2006-01-02", au)
	if err != nil {
		return fmt.Errorf("« au » doit être au format AAAA-MM-JJ")
	}
	if a.Before(d) {
		return fmt.Errorf("la date de fin précède la date de début")
	}
	return nil
}

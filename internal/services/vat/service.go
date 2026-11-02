package vat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// VATDeclaration représente les chiffres d'une déclaration TVA périodique
// selon le formulaire AFC 318 (méthode effective) ou AFC 100 (TDFN).
type VATDeclaration struct {
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
	Method       string    `json:"method"` // "effective" ou "tdfn"
	TotalRevenue float64   `json:"total_revenue"`
	VATCollected struct {
		Standard float64 `json:"standard"` // 8.1%
		Reduced  float64 `json:"reduced"`  // 2.6%
		Special  float64 `json:"special"`  // 3.8%
		Total    float64 `json:"total"`
	} `json:"vat_collected"`
	VATDeductible float64 `json:"vat_deductible"` // Impôt préalable déductible (chiffre 400)
	VATPayable    float64 `json:"vat_payable"`    // Montant dû / à rembourser (302+312+342-400)

	// Méthode TDFN seulement : le taux accordé par l'AFC (en %, saisi dans la fiche entreprise) et la base sur
	// laquelle il s'applique — le chiffre d'affaires TTC des factures à taux positif, avoirs déduits.
	TDFNRate float64 `json:"tdfn_rate,omitempty"`
	TDFNBase float64 `json:"tdfn_base,omitempty"`
}

// ErrTauxTDFNAbsent : la méthode TDFN applique le taux que l'AFC a ACCORDÉ à l'entreprise. Il se saisit, il
// ne se devine pas : sans lui, il n'y a pas de déclaration à produire (audit croisé Gemini, CRIT-07).
var ErrTauxTDFNAbsent = errors.New(
	"le taux TDFN n'est pas renseigné : saisissez celui que l'AFC vous a accordé (Paramètres → Banque, rubrique TVA, statut « assujetti »)")

// Service calculates Swiss VAT declarations for periodic AFC reporting.
type Service struct {
	db          *sql.DB
	usePostgres bool
}

// New creates a new VAT Service.
func New(database *sql.DB, usePostgres bool) *Service {
	return &Service{db: database, usePostgres: usePostgres}
}

// GenerateDeclaration calcule la déclaration TVA pour une période donnée.
//
// Méthode "effective" (AFC 318) : agrège les montants TVA par taux depuis les
// factures émises ou payées dans la période.
//
// Méthode "tdfn" (taux de la dette fiscale nette, AFC 100) : applique le taux que l'AFC a ACCORDÉ à
// l'entreprise (fiche entreprise, `tdfn_rate`) au chiffre d'affaires TTC des factures à taux positif,
// avoirs déduits (art. 37 LTVA). Sans taux saisi, la déclaration est refusée : le taux « normal × 0,8 »
// que ce code appliquait jusqu'ici (6,48 %, sur le chiffre d'affaires hors taxe) n'était celui d'aucune
// branche.
func (s *Service) GenerateDeclaration(ctx context.Context, periodStart, periodEnd time.Time, method string) (*VATDeclaration, error) {
	if method != "effective" && method != "tdfn" {
		return nil, fmt.Errorf("unknown VAT method %q: must be 'effective' or 'tdfn'", method)
	}
	// Le taux se lit AVANT d'agréger : sans lui, inutile de parcourir les factures.
	var tauxTDFN float64
	if method == "tdfn" {
		var t sql.NullFloat64
		if err := s.db.QueryRowContext(ctx, `SELECT tdfn_rate FROM company_settings LIMIT 1`).Scan(&t); err != nil &&
			!errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("read tdfn rate: %w", err)
		}
		if !t.Valid || t.Float64 <= 0 {
			return nil, ErrTauxTDFNAbsent
		}
		tauxTDFN = t.Float64
	}

	decl := &VATDeclaration{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Method:      method,
	}

	// ── Aggregate invoice data for the period ─────────────────────────────────
	// We collect subtotal_amount (HT) and vat_amount with the associated vat_rate.
	// Only sent/paid/archived invoices count for TVA (accrual basis).
	//
	// document_type must be filtered here. The invoices table also holds price
	// offers and credit notes, and both used to land in the declaration:
	//
	//   - A quote creates no tax debt. Under LTVA art. 40 al. 1 let. a the debt
	//     arises "au moment de la facturation", and an offer the prospect may
	//     never accept is not an invoice. Counting it made the business declare
	//     and pay VAT on revenue it had not earned.
	//   - A credit note reduces the debt (LTVA art. 41 — "modification
	//     ultérieure de la dette d'impôt"), so it belongs here, but with the
	//     opposite sign. Amounts are stored unsigned so the document reads
	//     naturally on screen and on paper; the sign is applied at aggregation
	//     instead. Summing it as-is used to *add* VAT where it should subtract.
	aggQ := db.Rebind(`
		SELECT
			COALESCE(SUM(CASE WHEN document_type = 'credit_note'
			                  THEN -subtotal_amount ELSE subtotal_amount END), 0) AS total_ht,
			COALESCE(SUM(CASE WHEN document_type = 'credit_note'
			                  THEN -vat_amount ELSE vat_amount END), 0)           AS total_vat,
			vat_rate
		FROM invoices
		WHERE document_type IN ('invoice', 'credit_note')
		  -- 'archived' compte : on n'archive qu'une facture PAYÉE, et l'archivage ne
		  -- défait pas la dette de TVA née à la facturation (LTVA art. 40). Sans lui,
		  -- archiver ses factures réglées du premier trimestre effaçait leur chiffre
		  -- d'affaires et leur TVA du décompte (audit croisé Gemini, CRIT-07).
		  AND status IN ('sent', 'paid', 'archived')
		  AND issue_date BETWEEN ? AND ?
		GROUP BY vat_rate
	`, s.usePostgres)

	rows, err := s.db.QueryContext(ctx, aggQ,
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("aggregate invoices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var totalHT, totalVAT, vatRate float64
		if err := rows.Scan(&totalHT, &totalVAT, &vatRate); err != nil {
			return nil, fmt.Errorf("scan invoice aggregate: %w", err)
		}
		decl.TotalRevenue += totalHT

		// TDFN : la base est le chiffre d'affaires TTC des factures À TAUX POSITIF. Un chiffre d'affaires à
		// 0 % (exportation, prestation exclue) n'est pas imposé au taux de la dette fiscale nette ; et la
		// TVA facturée en fait partie, le taux étant fixé sur la contre-prestation convenue, taxe comprise.
		if method == "tdfn" && vatRate > 0.0001 {
			decl.TDFNBase += totalHT + totalVAT
		}

		if method == "effective" {
			// Deux notations coexistent, et les confondre vidait les lignes
			// 312 et 342 du formulaire.
			//
			// `invoices.vat_rate` est stocké en POURCENTAGE (8.1) depuis la
			// migration 0005 — « new inserts always use percentages » — ce que
			// confirme le calcul de invoicing/service.go (`base * VATRate/100`).
			// Les constantes de `compliance`, elles, sont en FRACTION (0.081),
			// forme dont la méthode TDFN a besoin plus bas pour multiplier un
			// chiffre d'affaires. Les comparer directement donnait un écart de
			// 8.019 pour toute facture au taux normal : aucun `case` ne pouvait
			// jamais matcher, et TOUT tombait dans le `default`.
			//
			// On ramène donc la valeur lue à la même échelle que les
			// constantes, plutôt que de changer celles-ci : la ligne 137 en
			// dépend sous leur forme fractionnaire.
			rateFraction := vatRate / 100

			// Assign VAT to the correct bucket by rate (tolerance ±0.001,
			// soit ±0.1 point de pourcentage — assez large pour absorber un
			// arrondi de stockage, assez étroite pour ne pas confondre 2.6 %,
			// 3.8 % et 8.1 %, séparés d'au moins 1.2 point).
			switch {
			case absFloat(rateFraction-compliance.VATRateStandard) < 0.001:
				decl.VATCollected.Standard += totalVAT
			case absFloat(rateFraction-compliance.VATRateReduced) < 0.001:
				decl.VATCollected.Reduced += totalVAT
			case absFloat(rateFraction-compliance.VATRateSpecial) < 0.001:
				decl.VATCollected.Special += totalVAT
			default:
				// Taux inconnu (facture antérieure à un changement de taux,
				// taux étranger) : on l'agrège au taux normal plutôt que de le
				// perdre. Mieux vaut une ligne discutable qu'un montant absent
				// de la déclaration.
				decl.VATCollected.Standard += totalVAT
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("invoice aggregate rows: %w", err)
	}

	switch method {
	case "effective":
		// VATCollected already populated per-rate above.
		decl.VATCollected.Total = compliance.RoundTo5Rappen(
			decl.VATCollected.Standard +
				decl.VATCollected.Reduced +
				decl.VATCollected.Special,
		)

	case "tdfn":
		decl.TDFNRate = tauxTDFN
		decl.TDFNBase = compliance.RoundCents(decl.TDFNBase)
		vatDue := compliance.RoundTo5Rappen(decl.TDFNBase * tauxTDFN / 100)
		decl.VATCollected.Standard = vatDue
		decl.VATCollected.Total = vatDue
	}

	// ── Impôt préalable déductible (chiffre 400) ──────────────────────────────
	// Input VAT comes from supplier invoices that have been booked into the
	// accounts. Drafts are excluded (not yet in the books) and so are cancelled
	// ones, matching the accrual basis used for collected VAT above.
	//
	// Under the TDFN method the net-rate already embeds an allowance for input
	// tax, so claiming it again would be double-counting (art. 37 LTVA).
	if method == "effective" {
		dedQ := db.Rebind(`
			SELECT COALESCE(SUM(vat_amount), 0)
			FROM supplier_invoices
			WHERE status IN ('booked', 'paid')
			  AND issue_date BETWEEN ? AND ?
		`, s.usePostgres)

		var deductible float64
		if err := s.db.QueryRowContext(ctx, dedQ,
			periodStart.Format("2006-01-02"),
			periodEnd.Format("2006-01-02"),
		).Scan(&deductible); err != nil {
			return nil, fmt.Errorf("aggregate supplier invoices: %w", err)
		}
		decl.VATDeductible = compliance.RoundTo5Rappen(deductible)
	}

	// ── VATPayable = collected − deductible ───────────────────────────────────
	decl.VATPayable = compliance.RoundTo5Rappen(decl.VATCollected.Total - decl.VATDeductible)

	return decl, nil
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

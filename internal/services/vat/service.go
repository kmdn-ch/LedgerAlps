package vat

import (
	"context"
	"database/sql"
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
}

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
// Méthode "tdfn" (taux de dette fiscale net, AFC 100) : applique un taux
// forfaitaire de compliance.VATRateStandard × 0.8 sur le CA brut HT.
// Cette approximation est valide pour les PME à activité mixte (art. 37 LTVA).
func (s *Service) GenerateDeclaration(ctx context.Context, periodStart, periodEnd time.Time, method string) (*VATDeclaration, error) {
	if method != "effective" && method != "tdfn" {
		return nil, fmt.Errorf("unknown VAT method %q: must be 'effective' or 'tdfn'", method)
	}

	decl := &VATDeclaration{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Method:      method,
	}

	// ── Aggregate invoice data for the period ─────────────────────────────────
	// We collect subtotal_amount (HT) and vat_amount with the associated vat_rate.
	// Only sent/paid invoices count for TVA (accrual basis).
	aggQ := db.Rebind(`
		SELECT
			COALESCE(SUM(subtotal_amount), 0) AS total_ht,
			COALESCE(SUM(vat_amount), 0)      AS total_vat,
			vat_rate
		FROM invoices
		WHERE status IN ('sent', 'paid')
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

		if method == "effective" {
			// Assign VAT to the correct bucket by rate (tolerance ±0.001)
			switch {
			case absFloat(vatRate-compliance.VATRateStandard) < 0.001:
				decl.VATCollected.Standard += totalVAT
			case absFloat(vatRate-compliance.VATRateReduced) < 0.001:
				decl.VATCollected.Reduced += totalVAT
			case absFloat(vatRate-compliance.VATRateSpecial) < 0.001:
				decl.VATCollected.Special += totalVAT
			default:
				// Unknown rate: aggregate into standard bucket
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
		// Taux de dette fiscale net: forfaitaire ≈ VATRateStandard × 0.8
		// sur le chiffre d'affaires HT brut (art. 37 LTVA).
		tdfnRate := compliance.VATRateStandard * 0.8
		vatDue := compliance.RoundTo5Rappen(decl.TotalRevenue * tdfnRate)
		decl.VATCollected.Standard = vatDue
		decl.VATCollected.Total = vatDue
	}

	// ── Impôt préalable déductible (chiffre 400) ──────────────────────────────
	// Aggregate VAT paid on supplier invoices (purchase VAT) — currently
	// represented by journal lines on accounts of type 'asset' linked to VAT.
	// As a pragmatic approximation we query purchase invoices with negative
	// amounts or a dedicated input-tax account. For now this field is left at
	// zero until a dedicated supplier-invoice module is implemented; callers
	// can supply it via an optional update. VATDeductible is intentionally
	// not computed here to avoid over-claiming.
	decl.VATDeductible = 0

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

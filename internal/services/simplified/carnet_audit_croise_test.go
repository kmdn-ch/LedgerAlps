package simplified

// Audit croisé Gemini, CRIT-02 : le carnet du lait remis à l'administration était
// faux de trois façons.
//
//   - après la clôture de l'exercice, l'écriture qui vire le résultat débitait les
//     produits : chiffre d'affaires de 0.00, donc « éligible » et « non assujetti à
//     la TVA » pour quelqu'un à 650 000 francs ;
//   - un emprunt, un apport ou un prélèvement du propriétaire — de l'argent qui
//     entre ou sort sans être une recette ni une dépense — s'ajoutaient aux
//     recettes et aux dépenses ;
//   - une extourne comptait deux fois : sa contrepartie gonflait les recettes ET les
//     dépenses.

import (
	"context"
	"database/sql"
	"testing"
)

// ecritureSpeciale passe une écriture comptabilisée en choisissant sa référence et
// son drapeau d'extourne.
func ecritureSpeciale(t *testing.T, d *sql.DB, date, ref string, extourne bool, codeDebit, codeCredit string, montant float64) {
	t.Helper()
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&n)
	id := "s" + string(rune('a'+n))
	flag := 0
	if extourne {
		flag = 1
	}
	if _, err := d.Exec(`
		INSERT INTO journal_entries (id, date, reference, description, status, is_reversal, created_by_id)
		VALUES (?, ?, ?, 'essai', 'draft', ?, 'u1')`, id, date, ref, flag); err != nil {
		t.Fatalf("écriture: %v", err)
	}
	defer comptabiliser(t, d, id)
	for i, l := range []struct {
		code          string
		debit, credit any
	}{{codeDebit, montant, nil}, {codeCredit, nil, montant}} {
		if _, err := d.Exec(`
			INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount)
			VALUES (?, ?, ?, ?, ?)`, id+"-"+string(rune('0'+i)), id, idCompte(t, d, l.code), l.debit, l.credit); err != nil {
			t.Fatalf("ligne: %v", err)
		}
	}
}

func TestLaClotureNeFaitPasTomberLeChiffreDAffaires(t *testing.T) {
	s, d := baseTest(t)
	ecritureSpeciale(t, d, "2025-06-01", "JN-2025-001", false, "1020", "3200", 650000)
	// L'écriture de clôture : elle débite les produits et crédite le résultat.
	ecritureSpeciale(t, d, "2025-12-31", "CLOTURE-2025", false, "3200", "5900", 650000)

	c, err := s.Etablir(context.Background(), "2025-01-01", "2025-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if c.Eligibilite.ChiffreAffaires != 650000 {
		t.Errorf("chiffre d'affaires = %.2f, attendu 650000.00 : l'écriture de clôture l'a effacé", c.Eligibilite.ChiffreAffaires)
	}
	if c.Eligibilite.Eligible {
		t.Error("« éligible » pour 650 000 francs de chiffre d'affaires (seuil de 500 000)")
	}
	if !c.Eligibilite.AssujettiTVA {
		t.Error("« non assujetti » pour 650 000 francs de chiffre d'affaires (seuil de 100 000)")
	}
	if c.TotalRecettes != 650000 {
		t.Errorf("recettes = %.2f, attendu 650000.00", c.TotalRecettes)
	}
}

func TestUnEmpruntUnApportEtUnPrelevementNeSontNiRecettesNiDepenses(t *testing.T) {
	s, d := baseTest(t)
	ecriture(t, d, "2026-03-10", "Vente encaissée", "1020", "3000", 5000)
	ecriture(t, d, "2026-03-15", "Loyer payé", "6000", "1020", 1200)
	ecriture(t, d, "2026-04-01", "Emprunt bancaire", "1020", "2100", 50000)
	ecriture(t, d, "2026-04-02", "Apport du propriétaire", "1020", "2800", 20000)
	ecriture(t, d, "2026-04-03", "Prélèvement privé", "2800", "1020", 4000)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir (la concordance avec les liquidités doit tenir) : %v", err)
	}
	if c.TotalRecettes != 5000 || c.TotalDepenses != 1200 || c.Resultat != 3800 {
		t.Errorf("recettes %.2f / dépenses %.2f / résultat %.2f, attendu 5000 / 1200 / 3800 : "+
			"le financement a pollué le résultat", c.TotalRecettes, c.TotalDepenses, c.Resultat)
	}
	// Financement : emprunt +50 000 ; capital +20 000 − 4 000.
	if c.TotalFinancement != 66000 {
		t.Errorf("financement = %.2f, attendu 66000.00 (%+v)", c.TotalFinancement, c.Financement)
	}
	if len(c.Financement) != 2 {
		t.Errorf("%d postes de financement, attendu 2 : %+v", len(c.Financement), c.Financement)
	}
}

// Régler un fournisseur ou l'impôt reste une dépense de caisse : seules les dettes
// FINANCIÈRES sont du financement.
func TestRegler2000OuLImpotResteUneDepense(t *testing.T) {
	s, d := baseTest(t)
	ecriture(t, d, "2026-03-10", "Règlement fournisseur", "2000", "1020", 800)
	ecriture(t, d, "2026-03-11", "Impôt", "2270", "1020", 300)
	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if c.TotalDepenses != 1100 || c.TotalFinancement != 0 {
		t.Errorf("dépenses %.2f / financement %.2f, attendu 1100 / 0", c.TotalDepenses, c.TotalFinancement)
	}
}

func TestUneExtourneAnnuleAuLieuDeGonflerLesDeuxCotes(t *testing.T) {
	s, d := baseTest(t)
	ecritureSpeciale(t, d, "2026-03-10", "JN-2026-001", false, "1020", "3000", 1000)
	ecritureSpeciale(t, d, "2026-03-11", "JN-2026-002", true, "3000", "1020", 1000) // l'extourne

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if c.TotalRecettes != 0 || c.TotalDepenses != 0 || c.Resultat != 0 {
		t.Errorf("recettes %.2f / dépenses %.2f / résultat %.2f après une écriture annulée, attendu 0 / 0 / 0",
			c.TotalRecettes, c.TotalDepenses, c.Resultat)
	}
	if len(c.Recettes) != 0 || len(c.Depenses) != 0 {
		t.Errorf("des postes à zéro sont affichés : %+v %+v", c.Recettes, c.Depenses)
	}
}

// Une extourne dont l'original est d'une autre période vient en DÉDUCTION des
// recettes de celle-ci : le résultat reste égal au mouvement de liquidités.
func TestUneExtourneDUnePeriodeAnterieureVientEnDeduction(t *testing.T) {
	s, d := baseTest(t)
	ecritureSpeciale(t, d, "2025-11-10", "JN-2025-001", false, "1020", "3000", 1000)
	ecritureSpeciale(t, d, "2026-01-05", "JN-2026-001", true, "3000", "1020", 1000)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir : %v", err)
	}
	if c.TotalRecettes != -1000 || c.Resultat != -1000 || c.TotalDepenses != 0 {
		t.Errorf("recettes %.2f / dépenses %.2f / résultat %.2f, attendu -1000 / 0 / -1000",
			c.TotalRecettes, c.TotalDepenses, c.Resultat)
	}
}

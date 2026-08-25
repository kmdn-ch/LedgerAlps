package simplified

// Le carnet du lait, vérifié sur des écritures réelles.
//
// Ces tests n'inventent pas de lignes d'audit : ils écrivent au journal par le
// schéma du produit, puis relisent. Un test qui fabriquerait le résultat
// attendu des deux côtés ne prouverait rien — c'est la leçon retenue du défaut
// d'empreinte de la v1.4.6.

import (
	"context"
	"database/sql"
	"math"
	"os"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func baseTest(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-carnet-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"})
	if err != nil {
		t.Fatalf("ouverture: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id, email, name, password_hash, is_admin) VALUES ('u1','a@b.ch','A','x',1)`); err != nil {
		t.Fatal(err)
	}
	return New(database, false), database
}

// idCompte rend l'identifiant d'un compte du plan semé.
func idCompte(t *testing.T, d *sql.DB, code string) string {
	t.Helper()
	var id string
	if err := d.QueryRow(`SELECT id FROM accounts WHERE code = ?`, code).Scan(&id); err != nil {
		t.Fatalf("compte %s introuvable: %v", code, err)
	}
	return id
}

// ecriture passe une écriture comptabilisée à deux lignes.
func ecriture(t *testing.T, d *sql.DB, date, libelle, codeDebit, codeCredit string, montant float64) {
	t.Helper()
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&n)
	id := "e" + string(rune('a'+n))

	if _, err := d.Exec(`
		INSERT INTO journal_entries (id, date, reference, description, status, created_by_id)
		VALUES (?, ?, ?, ?, 'posted', 'u1')`, id, date, "JN-TEST-"+id, libelle); err != nil {
		t.Fatalf("écriture: %v", err)
	}
	// Le schéma impose qu'un SEUL côté soit renseigné, l'autre NULL — et non
	// zéro. C'est ce qui empêche une ligne d'être à la fois au débit et au
	// crédit, donc de compter deux fois.
	for i, l := range []struct {
		code          string
		debit, credit any
	}{
		{codeDebit, montant, nil},
		{codeCredit, nil, montant},
	} {
		if _, err := d.Exec(`
			INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount)
			VALUES (?, ?, ?, ?, ?)`,
			id+"-"+string(rune('0'+i)), id, idCompte(t, d, l.code), l.debit, l.credit); err != nil {
			t.Fatalf("ligne: %v", err)
		}
	}
}

// Une vente encaissée est une recette ; un achat payé est une dépense.
func TestLesEncaissementsEtLesDecaissements(t *testing.T) {
	s, d := baseTest(t)

	// Encaissement d'une vente : banque au débit, produit au crédit.
	ecriture(t, d, "2026-03-10", "Vente encaissée", "1020", "3000", 5000)
	// Paiement d'une charge : charge au débit, banque au crédit.
	ecriture(t, d, "2026-03-15", "Loyer payé", "6000", "1020", 1200)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}

	if c.TotalRecettes != 5000 {
		t.Errorf("recettes = %.2f, attendu 5000", c.TotalRecettes)
	}
	if c.TotalDepenses != 1200 {
		t.Errorf("dépenses = %.2f, attendu 1200", c.TotalDepenses)
	}
	if c.Resultat != 3800 {
		t.Errorf("résultat = %.2f, attendu 3800", c.Resultat)
	}
	if len(c.Recettes) != 1 || c.Recettes[0].Code != "3000" {
		t.Errorf("la recette n'est pas classée sur son compte de produit : %+v", c.Recettes)
	}
	if len(c.Depenses) != 1 || c.Depenses[0].Code != "6000" {
		t.Errorf("la dépense n'est pas classée sur son compte de charge : %+v", c.Depenses)
	}
}

// LE test qui protège la justesse : un virement interne n'est ni une recette
// ni une dépense.
//
// Un retrait au bancomat crédite la banque et débite la caisse. Une lecture
// ligne à ligne le compterait DEUX fois — une entrée et une sortie — et
// gonflerait les deux totaux sans changer le résultat. Le document présenté à
// l'administration annoncerait alors un chiffre d'affaires qui n'existe pas.
func TestUnVirementInterneNeCompteNiEnRecetteNiEnDepense(t *testing.T) {
	s, d := baseTest(t)

	ecriture(t, d, "2026-03-10", "Vente encaissée", "1020", "3000", 5000)
	// Retrait au bancomat : deux comptes de liquidités.
	ecriture(t, d, "2026-03-12", "Retrait bancomat", "1000", "1020", 800)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}

	if c.TotalRecettes != 5000 {
		t.Errorf("recettes = %.2f, attendu 5000 — le virement interne a été compté", c.TotalRecettes)
	}
	if c.TotalDepenses != 0 {
		t.Errorf("dépenses = %.2f, attendu 0 — le virement interne a été compté", c.TotalDepenses)
	}
}

// Une facture émise mais NON encaissée ne figure pas au carnet.
//
// C'est toute la différence entre la base caisse et la comptabilité
// d'engagement, et la raison pour laquelle ce document ne peut pas être dérivé
// du compte de résultat.
func TestUneFactureNonEncaisseeNEstPasUneRecette(t *testing.T) {
	s, d := baseTest(t)

	// Facture émise : créance au débit, produit au crédit. Aucun argent reçu.
	ecriture(t, d, "2026-03-10", "Facture émise", "1100", "3000", 9000)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}

	if c.TotalRecettes != 0 {
		t.Errorf("recettes = %.2f, attendu 0 — une créance a été comptée comme un encaissement",
			c.TotalRecettes)
	}
	// Mais elle compte pour le chiffre d'affaires : c'est la grandeur que la
	// loi vise pour le seuil, et elle se mesure en base d'engagement.
	if c.Eligibilite.ChiffreAffaires != 9000 {
		t.Errorf("chiffre d'affaires = %.2f, attendu 9000", c.Eligibilite.ChiffreAffaires)
	}
}

// Une écriture sans mouvement d'argent — un amortissement — n'a pas sa place
// dans un carnet du lait.
func TestUnAmortissementNEntrePasAuCarnet(t *testing.T) {
	s, d := baseTest(t)

	ecriture(t, d, "2026-12-31", "Amortissement véhicule", "6800", "1510", 2000)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}
	if c.TotalRecettes != 0 || c.TotalDepenses != 0 {
		t.Errorf("un amortissement est entré au carnet : recettes %.2f, dépenses %.2f",
			c.TotalRecettes, c.TotalDepenses)
	}
}

// La période borne le carnet.
func TestLaPeriodeEstRespectee(t *testing.T) {
	s, d := baseTest(t)

	ecriture(t, d, "2025-12-31", "Vente exercice précédent", "1020", "3000", 4000)
	ecriture(t, d, "2026-06-01", "Vente de l'exercice", "1020", "3000", 6000)
	ecriture(t, d, "2027-01-02", "Vente exercice suivant", "1020", "3000", 7000)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}
	if c.TotalRecettes != 6000 {
		t.Errorf("recettes = %.2f, attendu 6000 — les bornes de période ne tiennent pas",
			c.TotalRecettes)
	}
}

// L'état du patrimoine : ce qu'on possède moins ce qu'on doit, à la clôture.
//
// C'est la troisième exigence de l'art. 957 al. 2. Un relevé de recettes et de
// dépenses seul ne satisfait pas l'article.
func TestLEtatDuPatrimoine(t *testing.T) {
	s, d := baseTest(t)

	ecriture(t, d, "2026-03-10", "Vente encaissée", "1020", "3000", 5000)
	// Une dette fournisseur ouverte.
	ecriture(t, d, "2026-04-01", "Achat à crédit", "6000", "2000", 1500)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}

	if c.TotalAvoirs != 5000 {
		t.Errorf("avoirs = %.2f, attendu 5000", c.TotalAvoirs)
	}
	if c.TotalEngagements != 1500 {
		t.Errorf("engagements = %.2f, attendu 1500", c.TotalEngagements)
	}
	if c.Fortune != 3500 {
		t.Errorf("fortune = %.2f, attendu 3500", c.Fortune)
	}
	// Un compte à zéro n'encombre pas le document.
	for _, p := range append(c.Avoirs, c.Engagements...) {
		if p.Montant == 0 {
			t.Errorf("un poste à zéro figure au patrimoine : %+v", p)
		}
	}
}

// Les deux seuils, et ce qu'ils décident.
func TestLesSeuilsLegaux(t *testing.T) {
	cas := []struct {
		nom              string
		ca               float64
		eligible         bool
		assujettiTVA     bool
		descriptionSeuil string
	}{
		{"petit indépendant", 80_000, true, false, "sous les deux seuils"},
		{"juste sous la TVA", 99_999, true, false, "LTVA art. 10 al. 2 let. a"},
		{"assujetti à la TVA", 100_000, true, true, "au seuil TVA"},
		{"carnet + décompte TVA", 250_000, true, true, "entre les deux seuils"},
		{"juste sous le CO", 499_999, true, true, "CO art. 957 al. 2"},
		{"partie double obligatoire", 500_000, false, true, "au seuil du CO"},
		{"largement au-dessus", 900_000, false, true, "au-delà"},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			eligible := c.ca < SeuilComptabiliteSimplifiee
			assujetti := c.ca >= SeuilAssujettissementTVA
			if eligible != c.eligible {
				t.Errorf("CA %.0f : éligible = %v, attendu %v (%s)",
					c.ca, eligible, c.eligible, c.descriptionSeuil)
			}
			if assujetti != c.assujettiTVA {
				t.Errorf("CA %.0f : assujetti TVA = %v, attendu %v (%s)",
					c.ca, assujetti, c.assujettiTVA, c.descriptionSeuil)
			}
		})
	}
}

// L'éligibilité se mesure sur des écritures réelles, pas sur une constante.
func TestLEligibiliteSeMesureSurLesLivres(t *testing.T) {
	s, d := baseTest(t)

	ecriture(t, d, "2026-05-01", "Grosse vente", "1020", "3000", 520_000)

	c, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}
	if c.Eligibilite.Eligible {
		t.Errorf("CA de %.0f déclaré éligible à la comptabilité simplifiée",
			c.Eligibilite.ChiffreAffaires)
	}
	if !c.Eligibilite.AssujettiTVA {
		t.Error("CA de 520 000 déclaré non assujetti à la TVA")
	}
}

// Un compte de liquidités se reconnaît, un compte de créances non.
func TestCeQuiCompteCommeLiquidite(t *testing.T) {
	// 1090/1091 sont les comptes de VIREMENT INTERNE du plan PME : les
	// laisser hors de la fenetre faisait compter chaque virement comme une
	// recette ou une depense. 10200 et 10290 sont des sous-comptes : ils
	// suivent leur racine, ce que la comparaison lexicographique ne faisait
	// pas -- elle rendait 10200 liquide et 10290 non liquide.
	liquides := []string{"1000", "1010", "1020", "1029", "1030", "1050",
		"1090", "1091", "1099", "10200", "10290", "1020.1"}
	// 1060 « Titres cotes » est un placement ; 1100 « Debiteurs » une creance.
	// Les codes mal formes sont refuses : POST /accounts ne valide rien.
	autres := []string{"1060", "1100", "1101", "1200", "2000", "3000", "6000",
		"", "99", "1000A", "1020 ", "abcd", "10a0"}

	for _, c := range liquides {
		if !estLiquidite(c) {
			t.Errorf("le compte %s devrait être une liquidité", c)
		}
	}
	for _, c := range autres {
		if estLiquidite(c) {
			t.Errorf("le compte %s ne devrait PAS être une liquidité", c)
		}
	}
}

// Les bornes de période sont contrôlées avant tout calcul.
func TestLesBornesSontControlees(t *testing.T) {
	if err := PeriodeValide("2026-01-01", "2026-12-31"); err != nil {
		t.Errorf("une période valide est refusée: %v", err)
	}
	if PeriodeValide("01.01.2026", "2026-12-31") == nil {
		t.Error("un format de date invalide est accepté")
	}
	if PeriodeValide("2026-12-31", "2026-01-01") == nil {
		t.Error("une fin antérieure au début est acceptée")
	}
}

// ligneComposee est une ligne d'écriture pour ecritureComposee.
type ligneComposee struct {
	code          string
	debit, credit float64
}

// ecritureComposee passe une écriture comptabilisée à N lignes.
//
// Le fabricant `ecriture` ci-dessus n'en produit que de deux lignes, et c'est
// ce qui a laissé passer le classement en bloc : une écriture à deux lignes a
// toujours sa contrepartie du côté opposé à la liquidité, donc le défaut ne
// pouvait pas s'y manifester. Un encaissement diminué de frais, un règlement
// avec escompte, une caisse de fin de journée — la forme normale de ces trois
// opérations est à trois lignes.
func ecritureComposee(t *testing.T, d *sql.DB, date, libelle string, lignes []ligneComposee) {
	t.Helper()
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&n)
	id := "e" + string(rune('a'+n))

	var totalDebit, totalCredit float64
	for _, l := range lignes {
		totalDebit += l.debit
		totalCredit += l.credit
	}
	if math.Abs(totalDebit-totalCredit) > 0.005 {
		t.Fatalf("écriture de test déséquilibrée : débits %.2f, crédits %.2f", totalDebit, totalCredit)
	}

	if _, err := d.Exec(`
		INSERT INTO journal_entries (id, date, reference, description, status, created_by_id)
		VALUES (?, ?, ?, ?, 'posted', 'u1')`, id, date, "JN-TEST-"+id, libelle); err != nil {
		t.Fatalf("écriture: %v", err)
	}
	for i, l := range lignes {
		// Un seul côté renseigné, l'autre NULL — comme le schéma l'impose.
		var debit, credit any
		if l.debit != 0 {
			debit = l.debit
		}
		if l.credit != 0 {
			credit = l.credit
		}
		if _, err := d.Exec(`
			INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount)
			VALUES (?, ?, ?, ?, ?)`,
			id+"-"+string(rune('0'+i)), id, idCompte(t, d, l.code), debit, credit); err != nil {
			t.Fatalf("ligne: %v", err)
		}
	}
}

// poste rend le montant d'un compte dans une liste, ou -1 s'il est absent.
func poste(lignes []Ligne, code string) float64 {
	for _, l := range lignes {
		if l.Code == code {
			return l.Montant
		}
	}
	return -1
}

// Une écriture composée porte ses DEUX sens à la fois, et le carnet doit les
// montrer tous les deux.
//
// C'est le défaut É-1 du second audit : le classement se faisait par écriture
// entière, puis chaque contrepartie n'était lue que du côté opposé au
// mouvement. Celle qui se trouvait du même côté que la liquidité portait zéro
// et disparaissait en silence — les frais bancaires n'apparaissaient nulle
// part, et l'escompte obtenu SOUS-DÉCLARAIT le bénéfice.
func TestUneEcritureComposeePorteSesDeuxSens(t *testing.T) {
	cas := []struct {
		nom      string
		lignes   []ligneComposee
		recettes map[string]float64
		depenses map[string]float64
		resultat float64
	}{
		{
			nom: "encaissement de 1000 diminué de 50 de frais bancaires",
			lignes: []ligneComposee{
				{code: "1020", debit: 950},
				{code: "6500", debit: 50},
				{code: "3000", credit: 1000},
			},
			recettes: map[string]float64{"3000": 1000},
			depenses: map[string]float64{"6500": 50},
			resultat: 950,
		},
		{
			nom: "règlement fournisseur de 1000 avec 20 d'escompte obtenu",
			lignes: []ligneComposee{
				{code: "6100", debit: 1000},
				{code: "1020", credit: 980},
				{code: "3400", credit: 20},
			},
			recettes: map[string]float64{"3400": 20},
			depenses: map[string]float64{"6100": 1000},
			resultat: -980,
		},
		{
			nom: "caisse du jour : vente de 800, salaire de 300 payé du tiroir",
			lignes: []ligneComposee{
				{code: "1000", debit: 500},
				{code: "5000", debit: 300},
				{code: "3000", credit: 800},
			},
			recettes: map[string]float64{"3000": 800},
			depenses: map[string]float64{"5000": 300},
			resultat: 500,
		},
	}

	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			s, d := baseTest(t)
			ecritureComposee(t, d, "2026-04-10", c.nom, c.lignes)

			k, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
			if err != nil {
				t.Fatalf("Etablir: %v", err)
			}
			for code, attendu := range c.recettes {
				if got := poste(k.Recettes, code); math.Abs(got-attendu) > 0.005 {
					t.Errorf("recette %s = %.2f, attendu %.2f", code, got, attendu)
				}
			}
			for code, attendu := range c.depenses {
				if got := poste(k.Depenses, code); math.Abs(got-attendu) > 0.005 {
					t.Errorf("dépense %s = %.2f, attendu %.2f", code, got, attendu)
				}
			}
			if math.Abs(k.Resultat-c.resultat) > 0.005 {
				t.Errorf("résultat = %.2f, attendu %.2f", k.Resultat, c.resultat)
			}
			// L'identité qui définit le document : le résultat est le
			// mouvement net de trésorerie, au centime.
			if math.Abs(k.TotalRecettes-k.TotalDepenses-c.resultat) > 0.005 {
				t.Errorf("recettes %.2f - dépenses %.2f ne fait pas %.2f",
					k.TotalRecettes, k.TotalDepenses, c.resultat)
			}
		})
	}
}

// Les seuils légaux ne s'apprécient QUE sur un exercice.
//
// C'est le défaut É-3 du second audit : le chiffre d'affaires était mesuré sur
// la période demandée, quelle qu'elle soit, puis comparé à des seuils annuels.
// Une entreprise à 1,6 million qui demandait un carnet trimestriel recevait un
// PDF affirmant en vert, sous la référence légale, que la comptabilité
// simplifiée lui était admise.
func TestLesSeuilsNeSApprecientQueSurUnExercice(t *testing.T) {
	s, d := baseTest(t)

	// 400 000 francs encaissés par trimestre — 1,6 million sur l'année, donc
	// très au-delà des deux seuils.
	for _, date := range []string{"2026-02-10", "2026-05-10", "2026-08-10", "2026-11-10"} {
		ecriture(t, d, date, "Vente encaissée", "1020", "3000", 400_000)
	}

	t.Run("le trimestre ne conclut pas en faveur de l'entreprise", func(t *testing.T) {
		k, err := s.Etablir(context.Background(), "2026-01-01", "2026-03-31")
		if err != nil {
			t.Fatalf("Etablir: %v", err)
		}
		if k.Eligibilite.SurExerciceComplet {
			t.Error("un trimestre est tenu pour un exercice complet")
		}
		if k.Eligibilite.Eligible {
			t.Errorf("une entreprise à 1,6 million est déclarée éligible sur un trimestre "+
				"(chiffre d'affaires mesuré : %.2f)", k.Eligibilite.ChiffreAffaires)
		}
	})

	t.Run("l'exercice complet conclut, et défavorablement", func(t *testing.T) {
		k, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
		if err != nil {
			t.Fatalf("Etablir: %v", err)
		}
		if !k.Eligibilite.SurExerciceComplet {
			t.Error("un exercice civil n'est pas tenu pour complet")
		}
		if k.Eligibilite.Eligible {
			t.Error("1,6 million est déclaré sous le seuil de 500 000")
		}
		if !k.Eligibilite.AssujettiTVA {
			t.Error("1,6 million est déclaré non assujetti à la TVA")
		}
	})
}

// Une entreprise réellement sous le seuil reste éligible sur son exercice.
//
// Le garde-fou d'É-3 ne doit pas refuser le document à ceux à qui la loi
// l'accorde : c'est la moitié du correctif qu'il serait facile d'oublier.
func TestUnExerciceSousLeSeuilResteEligible(t *testing.T) {
	s, d := baseTest(t)
	ecriture(t, d, "2026-06-10", "Vente encaissée", "1020", "3000", 80_000)

	k, err := s.Etablir(context.Background(), "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("Etablir: %v", err)
	}
	if !k.Eligibilite.SurExerciceComplet {
		t.Fatal("l'exercice civil n'est pas reconnu")
	}
	if !k.Eligibilite.Eligible {
		t.Error("80 000 francs sur l'exercice ne donne pas droit au carnet")
	}
	if k.Eligibilite.AssujettiTVA {
		t.Error("80 000 francs déclenche l'assujettissement TVA")
	}
}

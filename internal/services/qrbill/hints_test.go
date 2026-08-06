package qrbill

// Ce que la couche texte doit apprendre, et ce qu'elle ne doit pas inventer.
//
// Les cas ci-dessous viennent d'une facture réelle — un rappel d'ePost Service
// AG — dont la mise en page a produit les deux erreurs qui comptent : la date
// du rappel prise pour celle de la facture, et un décalage d'une colonne qui
// mettait l'échéance sous « Montant ». Les deux passaient inaperçues à l'œil.

import "testing"

// mk fabrique une ligne à partir de couples (texte, abscisse).
func mk(y float64, cells ...any) line {
	var l line
	l.y = y
	for i := 0; i < len(cells); i += 2 {
		l.items = append(l.items, textItem{
			s: cells[i].(string), x: cells[i+1].(float64), y: y,
		})
	}
	return l
}

// La disposition exacte du rappel : sept colonnes, un en-tête sur deux mots,
// et des nombres alignés à droite.
func lignesDuRappel() []line {
	return []line{
		mk(300,
			"Numéro de facture", 58.0, "Date", 69.0, "Devise", 77.0,
			"Montant", 91.0, "Échu", 102.0, "Retard", 108.0,
			"Niveau de", 112.0, "relance", 113.0),
		mk(288,
			"538690", 58.0, "01.12.2025", 66.0, "CHF", 72.0,
			"53.30", 83.0, "31.12.2025", 87.0, "50", 96.0, "1", 101.0),
	}
}

// LE test : chaque valeur sous SA colonne, malgré l'alignement à droite.
func TestUneLigneDeTableauSeLitMalgreLAlignementADroite(t *testing.T) {
	row := findInvoiceRow(lignesDuRappel())
	if row == nil {
		t.Fatal("aucune ligne de facture reconnue")
	}
	attendu := map[string]string{
		"numéro de facture": "538690",
		"date":              "01.12.2025",
		"devise":            "CHF",
		"montant":           "53.30",
		"échu":              "31.12.2025",
	}
	for col, val := range attendu {
		if row[col] != val {
			t.Errorf("colonne %q = %q, attendu %q", col, row[col], val)
		}
	}
}

// Le décalage d'un cran laisse chaque valeur juste et chaque étiquette fausse :
// rien ne cloche à l'œil, et l'échéance lue est en réalité un montant. C'est
// exactement ce que le contrôle de cohérence doit refuser.
func TestUnAppariementDecaleEstRefuse(t *testing.T) {
	decale := tableRow{
		"numéro de facture": "538690",
		"date":              "01.12.2025",
		"devise":            "53.30",     // un montant sous « Devise »
		"montant":           "31.12.2025", // une date sous « Montant »
	}
	if plausible(decale) {
		t.Fatal("un appariement décalé d'un cran a été accepté")
	}
	correct := tableRow{
		"numéro de facture": "538690",
		"devise":            "CHF",
		"montant":           "53.30",
		"échu":              "31.12.2025",
	}
	if !plausible(correct) {
		t.Fatal("un appariement correct a été refusé")
	}
}

// Un en-tête qui tient sur deux mots partage le bord gauche de sa colonne.
// Sans la fusion, les comptes ne concordent plus et l'appariement par rang est
// abandonné à tort.
func TestUnEnTeteSurDeuxMotsNeCompteQuUneColonne(t *testing.T) {
	l := mk(300, "Échu", 102.0, "Niveau de", 112.0, "relance", 113.0)
	merged := mergeWrappedCells(l)
	if len(merged) != 2 {
		t.Fatalf("%d colonnes après fusion, attendu 2 : %+v", len(merged), merged)
	}
	if merged[1].s != "Niveau de relance" {
		t.Errorf("en-tête fusionné = %q", merged[1].s)
	}
	// Une colonne voisine ne doit PAS être avalée : dix points d'écart séparent
	// deux colonnes distinctes.
	if merged[0].s != "Échu" {
		t.Errorf("la colonne voisine a été fusionnée : %q", merged[0].s)
	}
}

// Sur un rappel, deux dates portent l'étiquette « Date » : celle du rappel en
// tête de page, et celle de la facture rappelée dans le tableau. Prendre la
// première donne une pièce dont le numéro et la date ne se rapportent pas au
// même document — un défaut qui ne se voit qu'au rapprochement.
func TestLaDateVientDeLaLIGNEDeLaFactureEtPasDeLEnTeteDePage(t *testing.T) {
	lignes := append([]line{
		mk(560, "Date", 98.0, "19.02.2026", 122.0), // date du rappel, en tête
	}, lignesDuRappel()...)

	row := findInvoiceRow(lignes)
	if row == nil {
		t.Fatal("aucune ligne reconnue")
	}
	if row["date"] != "01.12.2025" {
		t.Fatalf("date lue %q — c'est celle du rappel, pas celle de la facture", row["date"])
	}
}

// ─── La TVA ──────────────────────────────────────────────────────────────────

// Le piège : le numéro d'assujetti d'un fournisseur contient « MWST » ou
// « TVA ». Chercher ces mots ferait croire à de la TVA sur une facture qui n'en
// porte aucune — et gonflerait l'impôt préalable déclaré.
func TestUnNumeroDIdentificationNEstPasUneMentionDeTVA(t *testing.T) {
	if reUID.MatchString("IDE CHE-103.727.240 MWST") == false {
		t.Fatal("le numéro IDE n'est pas reconnu")
	}
	// Aucun taux dans cette ligne : rien ne doit être retenu.
	if reRate.MatchString("IDE CHE-103.727.240 MWST") {
		t.Fatal("un taux a été trouvé là où il n'y en a pas")
	}
}

func TestUnTauxDeTVASeReconnaitDansSesEcrituresCourantes(t *testing.T) {
	cas := map[string]bool{
		"TVA 8.1 %":            true,
		"MWST 8,1%":            true,
		"dont TVA 2.6 % CHF 5": true,
		"TVA":                  false,
		"CHE-103.727.240 MWST": false,
	}
	for txt, attendu := range cas {
		if got := reRate.MatchString(txt); got != attendu {
			t.Errorf("%q : taux trouvé = %v, attendu %v", txt, got, attendu)
		}
	}
}

// ─── Les dates ───────────────────────────────────────────────────────────────

func TestLesDatesSeNormalisentVersLeFormatDeLInterface(t *testing.T) {
	cas := map[string]string{
		"31.12.2025": "2025-12-31",
		"01/12/2025": "2025-12-01",
		"2025-12-31": "2025-12-31",
		"pas une date": "",
	}
	for in, attendu := range cas {
		if got := normaliseDate(in); got != attendu {
			t.Errorf("%q → %q, attendu %q", in, got, attendu)
		}
	}
}

// Une date n'est pas un numéro de facture, et elle en a la forme.
func TestUneDateNEstPasPriseePourUnNumeroDeFacture(t *testing.T) {
	if isInvoiceNumber("01.12.2025") {
		t.Fatal("une date acceptée comme numéro de facture")
	}
	for _, n := range []string{"538690", "FA-2026-118", "2026/441"} {
		if !isInvoiceNumber(n) {
			t.Errorf("%q refusé comme numéro de facture", n)
		}
	}
}

// ─── L'encodage ──────────────────────────────────────────────────────────────

// Les accents arrivent en octal dans l'encodage WinAnsi. Sans conversion,
// « Échéance » ne se reconnaîtrait jamais — or c'est l'étiquette qui qualifie
// la valeur.
func TestLesAccentsEtLApostropheCourbeSeDecodent(t *testing.T) {
	cas := map[string]string{
		`(Num\351ro de facture)`:  "Numéro de facture",
		`(\311chu)`:               "Échu",
		`(jusqu\222au 18.02.2026)`: "jusqu’au 18.02.2026",
		`(Wilhelmsh\366he 1)`:     "Wilhelmshöhe 1",
	}
	for in, attendu := range cas {
		if got := decodePDFText(in); got != attendu {
			t.Errorf("%s → %q, attendu %q", in, got, attendu)
		}
	}
}

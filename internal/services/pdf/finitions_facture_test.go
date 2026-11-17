package pdf

// Deux défauts vus sur les factures de la 1.6.6-rc1 :
//
//   - « Main-d'œuvre » s'imprimait « Main-d'?uvre » : « œ » n'est pas dans Latin-1, et
//     latin1() le remplaçait par « ? » alors que la police du PDF (Windows-1252) a le glyphe ;
//   - le logo de l'entreprise était étiré dans une boîte fixe de 22 × 16 mm, donc déformé.

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// ── Les lettres ─────────────────────────────────────────────────────────────────

func TestLesLettresHorsLatin1SortentEnWindows1252OuSansAccent(t *testing.T) {
	for _, cas := range []struct {
		entrée, attendu string
	}{
		// Windows-1252 : le glyphe est dans la police, il faut seulement le bon octet.
		{"œ", "\x9c"}, {"Œ", "\x8c"}, {"š", "\x9a"}, {"Š", "\x8a"}, {"ž", "\x9e"}, {"Ž", "\x8e"}, {"Ÿ", "\x9f"},
		{"main-d’œuvre", "main-d'\x9cuvre"},
		{"cœur sœur nœud bœuf", "c\x9cur s\x9cur n\x9cud b\x9cuf"},
		// Hors Windows-1252 : la lettre de base plutôt qu'un « ? ».
		{"Dvořák", "Dvor\xe1k"}, // ř → r, á est dans Latin-1
		{"Čolić", "Colic"},
		{"Łukasiewicz", "Lukasiewicz"},
		{"Đurđević", "Durdevic"},
		{"Erdős", "Erdos"},
		{"İstanbul", "Istanbul"},
		// Ce qui était déjà juste le reste.
		{"é è à ç ô î ü ö ä ß ø æ", "\xe9 \xe8 \xe0 \xe7 \xf4 \xee \xfc \xf6 \xe4 \xdf \xf8 \xe6"},
		{"« guillemets » — tiret", "\xab guillemets \xbb - tiret"},
		// Une écriture que la police n'a pas : « ? », honnêtement.
		{"αβγ", "???"},
		{"日本", "??"},
		// Les contrôles C1 n'ont pas de glyphe et ne doivent pas se lire comme « œ ».
		{"a\u009cb", "ab"},
	} {
		if got := latin1(cas.entrée); got != cas.attendu {
			t.Errorf("latin1(%q) = %q, attendu %q", cas.entrée, got, cas.attendu)
		}
	}
}

// Le cas de la facture : un texte français courant, tous champs confondus, ne perd AUCUN caractère
// sur le papier. Ce test cherche des « ? » dans tout ce que le PDF imprime — un champ qui oublierait
// latin1() ou une lettre qui n'a pas de représentation y apparaîtrait.
func TestUneFactureAccentueeNeContientAucunPointDInterrogation(t *testing.T) {
	texte := "Main-d’œuvre incluse — « garantie » 2 ans ; Œuvres d’art, cœur ; Škoda, Dvořák, Łódź ; naïve café Zürich"
	notes := texte
	sortie, err := Generate(InvoiceData{
		InvoiceNumber: "FA-2026-0001", DocumentType: "invoice",
		IssueDate: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF", TotalAmount: 100, SubtotalAmount: 92.5, VATAmount: 7.5, VATRate: 8.1,
		Notes: &notes, Terms: &notes,
		Lines: []InvoiceLine{{Description: texte, Quantity: 1, UnitPrice: 100, LineTotal: 100, VATRate: 8.1}},
		Company: CompanyInfo{
			Name: "Œuvres Étoile Sàrl", Address: "Rue de l’Hôpital 12", City: "1700 Fribourg",
			Phone: "+41 26 000 00 00", Email: "contact@etoile.ch", UIDNumber: "CHE-123.456.789",
			IBAN: "CH9300762011623852957", BankName: "Banque cantonale de Genève", BankAddress: "Quai de l’Île 17",
		},
		Customer: CustomerInfo{Name: "Müller & Frères SA", Address: "Chemin des Écoliers 3", City: "1204 Genève"},
	})
	if err != nil {
		t.Fatalf("génération: %v", err)
	}
	contenu := fluxDécompressés(sortie)

	textes := regexp.MustCompile(`\(((?:[^()\\]|\\.)*)\)\s*Tj`).FindAllSubmatch(contenu, -1)
	if len(textes) < 30 {
		t.Fatalf("%d textes lus seulement : le test ne voit pas le document", len(textes))
	}
	for _, m := range textes {
		if bytes.Contains(m[1], []byte("?")) {
			t.Errorf("le PDF imprime un « ? » : %q", m[1])
		}
	}
	for _, attendu := range []string{"Main-d'\x9cuvre", "\x8cuvres", "c\x9cur", "\x8akoda", "Dvor\xe1k", "Lodz"} {
		// « Łódź » → « L\xf3dz » : le « ó » reste dans Latin-1.
		if attendu == "Lodz" {
			attendu = "L\xf3dz"
		}
		if !bytes.Contains(contenu, []byte(attendu)) {
			t.Errorf("le PDF ne porte pas %q", attendu)
		}
	}
}

// ── La boîte du logo ────────────────────────────────────────────────────────────

func TestLeLogoEstInscritDansLaBoiteSansDeformation(t *testing.T) {
	for _, cas := range []struct {
		nom        string
		pxL, pxH   int
		attenduL   float64
		attenduH   float64
		contrainte string
	}{
		{"carré", 300, 300, 24, 24, "hauteur"},
		{"horizontal 3:1", 300, 100, 34, 34.0 / 3, "largeur"},
		{"vertical 1:3", 100, 300, 8, 24, "hauteur"},
		{"4:3", 300, 225, 32, 24, "hauteur"},
		{"exactement la boîte", 340, 240, 34, 24, "les deux"},
		{"illisible", 0, 0, 34, 24, "boîte"},
	} {
		l, h := inscrireDansLaBoite(cas.pxL, cas.pxH, logoMaxL, logoMaxH)
		if math.Abs(l-cas.attenduL) > 0.01 || math.Abs(h-cas.attenduH) > 0.01 {
			t.Errorf("%s : %.2f × %.2f mm, attendu %.2f × %.2f (%s contraint)", cas.nom, l, h, cas.attenduL, cas.attenduH, cas.contrainte)
		}
		if l > logoMaxL+1e-9 || h > logoMaxH+1e-9 {
			t.Errorf("%s : %.2f × %.2f sort de la boîte %.0f × %.0f", cas.nom, l, h, logoMaxL, logoMaxH)
		}
	}
}

func logoPNG(t *testing.T, l, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, l, h))
	for x := 0; x < l; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 30, G: 90, B: 160, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func factureAvecLogo(t *testing.T, logo string) []byte {
	t.Helper()
	sortie, err := Generate(InvoiceData{
		InvoiceNumber: "FA-1", DocumentType: "invoice",
		IssueDate: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF", TotalAmount: 100,
		Lines:    []InvoiceLine{{Description: "Prestation", Quantity: 1, UnitPrice: 100, LineTotal: 100}},
		Company:  CompanyInfo{Name: "Etoile Sarl", Address: "Rue 1", City: "1700 Fribourg", LogoData: logo},
		Customer: CustomerInfo{Name: "Client SA"},
	})
	if err != nil {
		t.Fatalf("génération: %v", err)
	}
	return fluxDécompressés(sortie)
}

const mmParPoint = 25.4 / 72

// logoImprimé lit, dans le flux de la page, où et à quelle taille l'image a été posée (en mm).
func logoImprimé(t *testing.T, contenu []byte) (x, l, h float64, ok bool) {
	t.Helper()
	m := regexp.MustCompile(`q ([\d.]+) 0 0 ([\d.]+) ([\d.]+) ([\d.]+) cm\s*/I\S+ Do Q`).FindSubmatch(contenu)
	if m == nil {
		return 0, 0, 0, false
	}
	f := func(b []byte) float64 {
		v, err := strconv.ParseFloat(string(b), 64)
		if err != nil {
			t.Fatal(err)
		}
		return v * mmParPoint
	}
	return f(m[3]), f(m[1]), f(m[2]), true
}

// xDuNom donne l'abscisse (mm) du premier texte imprimé qui commence par « Etoile ».
func xDuNom(t *testing.T, contenu []byte) float64 {
	t.Helper()
	m := regexp.MustCompile(`BT ([\d.]+) [\d.]+ Td \(Etoile`).FindSubmatch(contenu)
	if m == nil {
		t.Fatal("le nom de l'émetteur n'est pas dans le PDF")
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return v * mmParPoint
}

// Le PDF, pas seulement la fonction : la taille posée sur la page garde le rapport de l'image, tient
// dans la boîte, et le texte de l'émetteur démarre juste après — sans recouvrir le logo.
func TestLeLogoImprimeGardeLesProportionsDeLImage(t *testing.T) {
	for _, cas := range []struct {
		nom      string
		pxL, pxH int
	}{
		{"carré", 300, 300}, {"horizontal 3:1", 300, 100}, {"vertical 1:3", 100, 300},
		{"4:3", 300, 225}, {"petit carré", 80, 80},
	} {
		t.Run(cas.nom, func(t *testing.T) {
			contenu := factureAvecLogo(t, logoPNG(t, cas.pxL, cas.pxH))
			x, l, h, ok := logoImprimé(t, contenu)
			if !ok {
				t.Fatal("aucune image posée sur la page")
			}
			attendu := float64(cas.pxL) / float64(cas.pxH)
			if got := l / h; math.Abs(got-attendu)/attendu > 0.01 {
				t.Errorf("logo %.2f × %.2f mm : rapport %.3f, celui de l'image est %.3f — il est déformé", l, h, got, attendu)
			}
			if l > logoMaxL+0.05 || h > logoMaxH+0.05 {
				t.Errorf("logo %.2f × %.2f mm hors de la boîte %.0f × %.0f", l, h, logoMaxL, logoMaxH)
			}
			if math.Abs(x-15) > 0.05 {
				t.Errorf("logo à x = %.2f mm, attendu 15 (marge de la page)", x)
			}
			// Le texte part à droite du logo, jamais dessus.
			if nom := xDuNom(t, contenu); nom < x+l {
				t.Errorf("le nom de l'émetteur (x = %.1f mm) recouvre le logo (jusqu'à %.1f mm)", nom, x+l)
			}
		})
	}
}

// Avant la correction, un logo plus gros qu'un timbre-poste n'existait pas : la boîte était de 22 × 16.
func TestLeLogoEstPlusGrandQueL_AncienneBoite(t *testing.T) {
	contenu := factureAvecLogo(t, logoPNG(t, 300, 300))
	_, l, h, ok := logoImprimé(t, contenu)
	if !ok {
		t.Fatal("aucune image posée sur la page")
	}
	if h <= 16 {
		t.Errorf("logo carré de %.1f mm de haut : pas plus grand que l'ancienne boîte (16 mm)", h)
	}
	if l <= 16 {
		t.Errorf("logo carré de %.1f mm de large : pas plus grand que l'ancienne boîte", l)
	}
}

// Sans logo, rien ne bouge : le texte de l'émetteur reste à la marge.
func TestSansLogoLeTexteResteALaMarge(t *testing.T) {
	contenu := factureAvecLogo(t, "")
	if _, _, _, ok := logoImprimé(t, contenu); ok {
		t.Error("une image est posée alors qu'il n'y a pas de logo")
	}
	if nom := xDuNom(t, contenu); nom > 17 {
		t.Errorf("le nom de l'émetteur est à x = %.1f mm sans logo, attendu la marge (15 mm + 1 de cellule)", nom)
	}
}

// Un logo illisible ne doit pas empêcher d'éditer la facture : le générateur le recevait tel quel et
// mettait tout le document en erreur.
func TestUnLogoIllisibleNeBloquePasLaFacture(t *testing.T) {
	illisible := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("ceci n'est pas une image"))
	contenu := factureAvecLogo(t, illisible) // factureAvecLogo échoue si Generate rend une erreur
	if _, _, _, ok := logoImprimé(t, contenu); ok {
		t.Error("une image illisible a été posée sur la page")
	}
	if nom := xDuNom(t, contenu); nom > 17 {
		t.Errorf("sans logo exploitable le texte devrait reprendre la marge, il est à x = %.1f mm", nom)
	}
}

// ── Les titres du bulletin de paiement ──────────────────────────────────────────

// « Récépissé » et « Référence » étaient écrits en dur, en français : une facture allemande,
// italienne ou anglaise les imprimait en français alors que le catalogue les traduit (« Empfangsschein »,
// « Ricevuta »…) et que les autres titres du bulletin le sont. Le titre du bulletin suit la langue du
// document, comme le reste.
// qrIBANDeTest (qriban_test.go) a la bonne plage mais une clé de contrôle fausse : le bulletin est alors
// abandonné en silence (le PDF sort sans lui). Celui-ci a une clé valide, donc un bulletin complet.
const qrIBANValide = "CH7330000012345678901"

func TestLesTitresDuBulletinSuiventLaLangueDuDocument(t *testing.T) {
	for _, cas := range []struct {
		lang, recu, reference string
	}{
		{"fr", "R\xe9c\xe9piss\xe9", "R\xe9f\xe9rence"},
		{"de", "Empfangsschein", "Referenz"},
		{"it", "Ricevuta", "Riferimento"},
		{"en", "Receipt", "Reference"},
	} {
		t.Run(cas.lang, func(t *testing.T) {
			doc := docAvecComptes(qrIBANValide, "") // un QR-IBAN : le bulletin porte une référence
			doc.Lang = cas.lang
			sortie, err := Generate(doc)
			if err != nil {
				t.Fatalf("génération: %v", err)
			}
			contenu := fluxDécompressés(sortie)
			if !bytes.Contains(contenu, []byte("("+cas.recu+")")) {
				t.Errorf("le bulletin ne titre pas son récépissé %q", cas.recu)
			}
			// Une fois sur le récépissé, une fois sur la partie paiement.
			if n := bytes.Count(contenu, []byte("("+cas.reference+")")); n != 2 {
				t.Errorf("%q imprimé %d fois, attendu 2 (récépissé et partie paiement)", cas.reference, n)
			}
			if cas.lang != "fr" {
				for _, francais := range []string{"R\xe9c\xe9piss\xe9", "R\xe9f\xe9rence"} {
					if bytes.Contains(contenu, []byte("("+francais+")")) {
						t.Errorf("le bulletin d'un document %q porte encore %q, en français", cas.lang, francais)
					}
				}
			}
		})
	}
}

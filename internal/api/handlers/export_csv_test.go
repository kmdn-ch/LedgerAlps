package handlers

import (
	"bytes"
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/i18n"
)

func parseCSV(t *testing.T, data []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(data, []byte("\ufeff")) {
		t.Fatal("BOM UTF-8 absent : Excel sous Windows afficherait « Genève » comme « GenÃ¨ve »")
	}
	r := csv.NewReader(strings.NewReader(string(bytes.TrimPrefix(data, []byte("\ufeff")))))
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV illisible: %v", err)
	}
	return rows
}

func TestCSVColumnsAreTheUnionOfKeysAndStable(t *testing.T) {
	records := []map[string]any{
		{"id": "a", "name": "Genève"},
		{"id": "b", "iban": "CH…"}, // clé absente du premier enregistrement
	}
	data, err := csvFromRecords(records, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := parseCSV(t, data)

	want := []string{"iban", "id", "name"}
	if len(rows[0]) != len(want) {
		t.Fatalf("en-tête = %v, attendu %v", rows[0], want)
	}
	for i, c := range want {
		if rows[0][i] != c {
			t.Fatalf("en-tête = %v, attendu %v (ordre trié, pour que deux exports se comparent)", rows[0], want)
		}
	}

	// Une cellule vide vaut « absent », et doit rester distincte d'un zéro.
	if rows[1][0] != "" {
		t.Fatalf("iban de la première ligne = %q, attendu vide", rows[1][0])
	}
	if rows[1][2] != "Genève" {
		t.Fatalf("accent perdu : %q", rows[1][2])
	}
}

// Les montants ne doivent pas partir en notation scientifique ni traîner des
// décimales inventées : un CSV comptable se relit à l'œil.
func TestCSVRendersAmountsPlainly(t *testing.T) {
	records := []map[string]any{
		{"a": float64(1000), "b": 1000.55, "c": float64(0), "d": 1234567890.0, "e": nil, "f": true},
	}
	data, err := csvFromRecords(records, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := parseCSV(t, data)

	want := map[string]string{"a": "1000", "b": "1000.55", "c": "0", "d": "1234567890", "e": "", "f": "1"}
	for i, col := range rows[0] {
		if got := rows[1][i]; got != want[col] {
			t.Errorf("colonne %s = %q, attendu %q", col, got, want[col])
		}
	}
}

// Les lignes imbriquées doivent sortir dans leur propre fichier avec la clé
// étrangère : les aplatir dans une cellule aurait produit du JSON dans du CSV.
func TestNestedLinesBecomeTheirOwnTable(t *testing.T) {
	parents := []map[string]any{
		{"id": "e1", "reference": "JN-001", "lines": []any{
			map[string]any{"id": "l1", "debit_amount": float64(500)},
			map[string]any{"id": "l2", "credit_amount": float64(500)},
		}},
		{"id": "e2", "reference": "JN-002", "lines": []any{
			map[string]any{"id": "l3", "debit_amount": float64(100)},
		}},
	}

	lines := extractNested(parents, "lines", "entry_id")
	if len(lines) != 3 {
		t.Fatalf("%d ligne(s) extraite(s), attendu 3", len(lines))
	}
	for _, l := range lines {
		if l["entry_id"] == nil || l["entry_id"] == "" {
			t.Fatalf("ligne sans clé étrangère : %v — elle serait irrattachable", l)
		}
	}
	if lines[0]["entry_id"] != "e1" || lines[2]["entry_id"] != "e2" {
		t.Fatalf("clés étrangères mal réparties : %v", lines)
	}

	// Et le parent ne doit plus porter la colonne `lines`.
	data, err := csvFromRecords(parents, map[string]bool{"lines": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range parseCSV(t, data)[0] {
		if col == "lines" {
			t.Fatal("la colonne `lines` subsiste dans le fichier parent : du JSON dans du CSV")
		}
	}
}

// L'archive doit produire un jeu complet et nommé de façon prévisible.
func TestBuildCSVFilesProducesTheExpectedSet(t *testing.T) {
	files, err := buildCSVFiles(map[string]any{
		"accounts":        []map[string]any{{"id": "a1", "code": "1020"}},
		"journal_entries": []map[string]any{{"id": "e1", "lines": []any{map[string]any{"id": "l1"}}}},
		"invoices":        []map[string]any{{"id": "i1", "lines": []any{map[string]any{"id": "il1"}}}},
		"contacts":        []map[string]any{},
		"fiscal_years":    []map[string]any{{"id": "f1", "name": "2026"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, f := range files {
		got[f.name] = true
	}
	for _, want := range []string{
		"accounts.csv", "journal_entries.csv", "journal_lines.csv",
		"invoices.csv", "invoice_lines.csv", "contacts.csv", "fiscal_years.csv",
	} {
		if !got[want] {
			t.Errorf("%s manquant dans l'archive", want)
		}
	}

	readme := string(csvReadme(i18n.Défaut, files))
	if !strings.Contains(readme, "journal_lines.entry_id") {
		t.Error("le LISEZ-MOI ne documente pas les relations : un CSV sans clé de lecture se prête aux contresens")
	}
}

// CWE-1236 : une cellule qui commence par = + - @ (ou tabulation, retour
// chariot) est exécutée comme formule par Excel et Calc. Les textes saisis par
// des tiers — description, nom de contact — doivent sortir neutralisés, sans
// que les montants négatifs cessent d'être des nombres.
func TestCSVNeutraliseLesFormules(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`=HYPERLINK("http://x","clic")`, `'=HYPERLINK("http://x","clic")`},
		{`+cmd|' /C calc'!A0`, `'+cmd|' /C calc'!A0`},
		{`-2+3`, `'-2+3`},
		{`@SUM(A1:A9)`, `'@SUM(A1:A9)`},
		{"	=1+1", "'	=1+1"},
		{"=1+1", "'=1+1"},
		{`+41 79 123 45 67`, `'+41 79 123 45 67`},
		// Intacts : texte ordinaire, vide, montants.
		{`Menuiserie Dupont`, `Menuiserie Dupont`},
		{``, ``},
		{`-1234.50`, `-1234.50`},
		{`+12`, `+12`},
		{`2026-03-04`, `2026-03-04`},
		{`a=b`, `a=b`},
	} {
		if got := neutraliserFormule(tc.in); got != tc.want {
			t.Errorf("neutraliserFormule(%q) = %q, attendu %q", tc.in, got, tc.want)
		}
	}
}

func TestCSVFromRecordsNeutraliseLesCellulesTexte(t *testing.T) {
	data, err := csvFromRecords([]map[string]any{
		{"description": `=1+1`, "montant": -250.5},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := parseCSV(t, data)
	if rows[1][0] != `'=1+1` {
		t.Errorf("description = %q, attendu la formule neutralisée", rows[1][0])
	}
	if rows[1][1] != "-250.5" {
		t.Errorf("montant = %q, un montant négatif doit rester un nombre", rows[1][1])
	}
}

// Le second producteur de CSV (journal, grand livre, balance, carnet) passe par
// writeCSV : il doit neutraliser lui aussi.
func TestWriteCSVNeutraliseLesFormules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeCSV(c, "x.csv", [][]string{
		{"Libellé", "Montant"},
		{`=cmd|' /C calc'!A0`, "-80.00"},
	})
	rows := parseCSV(t, w.Body.Bytes())
	if rows[1][0] != `'=cmd|' /C calc'!A0` {
		t.Errorf("libellé = %q, attendu la formule neutralisée", rows[1][0])
	}
	if rows[1][1] != "-80.00" {
		t.Errorf("montant = %q, un montant négatif doit rester un nombre", rows[1][1])
	}
}

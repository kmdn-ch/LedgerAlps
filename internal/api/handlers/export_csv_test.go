package handlers

import (
	"bytes"
	"encoding/csv"
	"github.com/kmdn-ch/ledgeralps/internal/i18n"
	"strings"
	"testing"
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

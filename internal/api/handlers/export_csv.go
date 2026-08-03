package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Export de réversibilité — le pendant CSV de l'archive légale.
//
// L'archive contient déjà du JSON, exact et complet, mais qui suppose d'écrire
// du code pour être exploité. Le CSV s'ouvre dans un tableur et s'importe dans
// n'importe quel logiciel comptable. Sans lui, « vos données vous appartiennent »
// reste une affirmation sans moyen de l'exercer : le verrouillage fournisseur
// ne tient pas au refus d'exporter, il tient au format de l'export.
//
// Les lignes imbriquées (écritures, factures) sortent dans leurs propres
// fichiers, reliées par la clé étrangère. Les aplatir dans une cellule aurait
// produit du JSON dans du CSV — le pire des deux.

// csvFromRecords sérialise une liste d'objets JSON en CSV.
//
// Les colonnes sont l'union des clés rencontrées, triées : deux exports
// successifs produisent le même en-tête, et un champ ajouté au modèle apparaît
// sans qu'il faille toucher ici. Une valeur absente donne une cellule vide,
// distincte d'un zéro.
func csvFromRecords(records []map[string]any, skip map[string]bool) ([]byte, error) {
	cols := map[string]bool{}
	for _, r := range records {
		for k := range r {
			if !skip[k] {
				cols[k] = true
			}
		}
	}
	header := make([]string, 0, len(cols))
	for k := range cols {
		header = append(header, k)
	}
	sort.Strings(header)

	var buf bytes.Buffer
	// BOM UTF-8 : sans lui, Excel sous Windows lit le CSV en ANSI et affiche
	// « Genève » comme « GenÃ¨ve ». Le public visé ouvrira ce fichier dans Excel.
	buf.WriteString("\ufeff")

	w := csv.NewWriter(&buf)
	// Point-virgule : séparateur attendu par Excel dans les locales suisses et
	// européennes, où la virgule est le séparateur décimal.
	w.Comma = ';'

	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, r := range records {
		row := make([]string, len(header))
		for i, k := range header {
			row[i] = csvCell(r[k])
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// csvCell rend une valeur JSON en texte.
func csvCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "1"
		}
		return "0"
	case float64:
		// Les identifiants et les montants passent tous par float64 après
		// décodage JSON. FormatFloat avec -1 rend 1000 comme "1000" et 1000.55
		// comme "1000.55", sans notation scientifique ni zéros parasites.
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(raw)
	}
}

// toRecords convertit une valeur quelconque en liste d'objets, via JSON.
// Passer par JSON garantit que le CSV montre exactement les mêmes champs que le
// fichier JSON voisin, y compris les règles `omitempty` et le masquage IBAN.
func toRecords(v any) ([]map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// extractNested sort les lignes imbriquées sous `field` dans leur propre liste,
// en y ajoutant la clé étrangère `fkName` pointant sur l'identifiant du parent.
func extractNested(parents []map[string]any, field, fkName string) []map[string]any {
	var out []map[string]any
	for _, p := range parents {
		nested, ok := p[field].([]any)
		if !ok {
			continue
		}
		parentID, _ := p["id"].(string)
		for _, n := range nested {
			line, ok := n.(map[string]any)
			if !ok {
				continue
			}
			row := make(map[string]any, len(line)+1)
			for k, v := range line {
				row[k] = v
			}
			row[fkName] = parentID
			out = append(out, row)
		}
	}
	return out
}

// csvSet est un fichier CSV prêt à être ajouté à l'archive.
type csvSet struct {
	name string
	data []byte
}

// buildCSVFiles produit le jeu CSV correspondant aux données déjà exportées.
func buildCSVFiles(sources map[string]any) ([]csvSet, error) {
	var files []csvSet

	add := func(name string, records []map[string]any, skip map[string]bool) error {
		data, err := csvFromRecords(records, skip)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		files = append(files, csvSet{name: name, data: data})
		return nil
	}

	// Les tables dont les lignes sont imbriquées : le parent perd sa colonne
	// `lines`, qui part dans un fichier dédié relié par clé étrangère.
	nested := map[string]struct{ field, fk, child string }{
		"journal_entries": {"lines", "entry_id", "journal_lines"},
		"invoices":        {"lines", "invoice_id", "invoice_lines"},
	}

	names := make([]string, 0, len(sources))
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		records, err := toRecords(sources[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if n, ok := nested[name]; ok {
			if err := add(n.child+".csv", extractNested(records, n.field, n.fk), nil); err != nil {
				return nil, err
			}
			if err := add(name+".csv", records, map[string]bool{n.field: true}); err != nil {
				return nil, err
			}
			continue
		}
		if err := add(name+".csv", records, nil); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// csvReadme accompagne les fichiers : un CSV sans clé de lecture se prête aux
// contresens, et celui-ci porte des données comptables.
func csvReadme(files []csvSet) []byte {
	var b strings.Builder
	b.WriteString("Export de réversibilité — LedgerAlps\n")
	b.WriteString("====================================\n\n")
	b.WriteString("Ces fichiers CSV contiennent les mêmes données que les fichiers JSON\n")
	b.WriteString("de cette archive, dans un format ouvrable par un tableur ou importable\n")
	b.WriteString("dans un autre logiciel comptable. Le JSON reste la référence : il porte\n")
	b.WriteString("les types exacts, là où le CSV est du texte.\n\n")
	b.WriteString("Format\n")
	b.WriteString("------\n")
	b.WriteString("  - Séparateur : point-virgule (;) — attendu par Excel en Suisse.\n")
	b.WriteString("  - Encodage   : UTF-8 avec BOM, pour qu'Excel affiche les accents.\n")
	b.WriteString("  - Décimales  : point (.), comme dans le JSON.\n")
	b.WriteString("  - Cellule vide = valeur absente, à distinguer d'un zéro.\n\n")
	b.WriteString("Relations\n")
	b.WriteString("---------\n")
	b.WriteString("  journal_lines.entry_id    → journal_entries.id\n")
	b.WriteString("  invoice_lines.invoice_id  → invoices.id\n")
	b.WriteString("  journal_lines.account_id  → accounts.id\n")
	b.WriteString("  invoices.contact_id       → contacts.id\n")
	b.WriteString("  journal_entries.fiscal_year_id → fiscal_years.id\n\n")
	b.WriteString("Fichiers\n")
	b.WriteString("--------\n")
	for _, f := range files {
		b.WriteString(fmt.Sprintf("  %-24s %7d octets\n", f.name, len(f.data)))
	}
	b.WriteString("\nNote sur les données personnelles (nLPD) : les IBAN des contacts sont\n")
	b.WriteString("masqués dans cet export, comme dans le JSON. L'export complet d'un IBAN\n")
	b.WriteString("se fait depuis la fiche du contact concerné.\n")
	return []byte(b.String())
}

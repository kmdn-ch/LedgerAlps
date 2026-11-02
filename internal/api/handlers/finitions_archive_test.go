package handlers

// Audit croisé Gemini, finitions (P3) : archive légale (LOW-03).
//
// L'archive est assemblée table par table, puis recopiée entrée compressée par entrée compressée
// (`zip.Writer.Copy`) pour que le manifeste soit la première. Ce test fige ce que l'on promet à
// quelqu'un qui rouvre l'archive dans dix ans : la disposition, des empreintes qui disent vrai,
// des CSV complets et un LISEZ-MOI qui annonce les bonnes tailles.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func lireEntree(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// deuxComptes rend l'identifiant des deux premiers comptes du plan. Le curseur se ferme par defer, dans
// une fonction à part : c'est la forme que le Lint de la CI (sqlclosecheck) accepte.
func deuxComptes(t *testing.T, database *sql.DB) [2]string {
	t.Helper()
	rows, err := database.Query(`SELECT id FROM accounts ORDER BY code LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids [2]string
	for i := 0; i < 2 && rows.Next(); i++ {
		if err := rows.Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil || ids[1] == "" {
		t.Fatalf("plan comptable trop court : %v", err)
	}
	return ids
}

func TestLArchiveLegaleGardeSaDispositionEtSesEmpreintes(t *testing.T) {
	database, _, _ := cancelEnv(t)

	// De quoi remplir chaque table : deux écritures comptabilisées, deux factures à plusieurs lignes.
	comptes := deuxComptes(t, database)
	a1, a2 := comptes[0], comptes[1]
	for _, e := range []string{"e1", "e2"} {
		if _, err := database.Exec(`INSERT INTO journal_entries (id, reference, date, description, status, created_by_id)
			VALUES (?, ?, '2026-03-15', 'Écriture « d''essai »', 'draft', 'u1')`, e, "JN-"+e); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, credit_amount, sequence)
			VALUES (?, ?, ?, 100, NULL, 1), (?, ?, ?, NULL, 100, 2)`,
			e+"a", e, a1, e+"b", e, a2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`UPDATE journal_entries SET status = 'posted'`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"f1", "f2"} {
		if _, err := database.Exec(`INSERT INTO invoices (id, invoice_number, document_type, contact_id, status, issue_date,
			due_date, currency, subtotal_amount, vat_amount, total_amount, vat_rate, created_by_id)
			VALUES (?, ?, 'invoice', 'c1', 'sent', '2026-03-01', '2026-03-31', 'CHF', 200, 16.2, 216.2, 8.1, 'u1')`,
			f, "FA-"+f); err != nil {
			t.Fatal(err)
		}
		for s := 0; s < 2; s++ {
			if _, err := database.Exec(`INSERT INTO invoice_lines (id, invoice_id, description, quantity, unit_price,
				vat_rate, line_total, sequence) VALUES (?, ?, 'Conseil', 1, 100, 8.1, 100, ?)`,
				fmt.Sprintf("%sl%d", f, s), f, s); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Les pièces et mouvements ajoutés sur décision du propriétaire (CRIT-11) : une facture fournisseur à deux
	// lignes, un paiement, une écriture de relevé bancaire, deux maillons du journal d'audit.
	brouillonFournisseur(t, database, "s1", "F-1", 200, 16.2)
	for s := 0; s < 2; s++ {
		if _, err := database.Exec(`INSERT INTO supplier_invoice_lines (id, supplier_invoice_id, description, quantity,
			unit_price, vat_rate, line_total, sequence) VALUES (?, 's1', 'Papier', 1, 100, 0.081, 100, ?)`,
			fmt.Sprintf("sl%d", s), s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`INSERT INTO payments (id, invoice_id, amount, payment_date, method, created_by_id)
		VALUES ('p1', 'f1', 216.2, '2026-03-20', 'bank_transfer', 'u1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO bank_entries (id, fingerprint, amount, currency, is_credit, booking_date, counterparty, invoice_id)
		VALUES ('b1', 'fp1', 216.2, 'CHF', 1, '2026-03-20', 'Client SA', 'f1')`); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 2; n++ {
		if _, err := database.Exec(`INSERT INTO audit_logs (id, user_id, action, table_name, record_id, sequence_number, entry_hash, prev_hash)
			VALUES (?, 'u1', 'essai', 'invoices', 'f1', ?, ?, ?)`,
			fmt.Sprintf("al%d", n), n, fmt.Sprintf("hash%d", n), fmt.Sprintf("hash%d", n-1)); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/archive", NewExportHandler(database, false).LegalArchive)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/archive", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("archive : %d %s", w.Code, w.Body.String())
	}

	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("archive illisible : %v", err)
	}

	// 1. La disposition : le manifeste d'abord, le JSON, les CSV, le LISEZ-MOI en dernier.
	var noms []string
	parNom := map[string]*zip.File{}
	for _, f := range zr.File {
		court := f.Name[strings.Index(f.Name, "/")+1:]
		noms = append(noms, court)
		parNom[court] = f
	}
	attendu := []string{
		"manifest.json",
		"accounts.json", "journal_entries.json", "invoices.json", "contacts.json", "fiscal_years.json",
		"supplier_invoices.json", "payments.json", "bank_entries.json", "audit_logs.json",
		"csv/accounts.csv", "csv/audit_logs.csv", "csv/bank_entries.csv", "csv/contacts.csv", "csv/fiscal_years.csv",
		"csv/invoice_lines.csv", "csv/invoices.csv",
		"csv/journal_lines.csv", "csv/journal_entries.csv",
		"csv/payments.csv", "csv/supplier_invoice_lines.csv", "csv/supplier_invoices.csv",
		"csv/LISEZ-MOI.txt",
	}
	if strings.Join(noms, "\n") != strings.Join(attendu, "\n") {
		t.Fatalf("disposition de l'archive :\n  obtenu  %v\n  attendu %v", noms, attendu)
	}

	// 2. Chaque empreinte du manifeste dit vrai, et le manifeste n'en oublie aucune.
	var manifeste struct {
		Version string            `json:"version"`
		Hashes  map[string]string `json:"sha256_hashes"`
	}
	if err := json.Unmarshal(lireEntree(t, parNom["manifest.json"]), &manifeste); err != nil {
		t.Fatal(err)
	}
	if len(manifeste.Hashes) != len(attendu)-1 {
		t.Errorf("%d empreintes pour %d fichiers", len(manifeste.Hashes), len(attendu)-1)
	}
	for nom, f := range parNom {
		if nom == "manifest.json" {
			continue
		}
		somme := sha256.Sum256(lireEntree(t, f))
		if got := manifeste.Hashes[nom]; got != fmt.Sprintf("%x", somme) {
			t.Errorf("empreinte de %s : manifeste %q, contenu %x", nom, got, somme)
		}
	}

	// 3. Les données y sont : lignes d'écriture et de facture reliées à leur parent par clé étrangère.
	compter := func(nom string) [][]string {
		r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(lireEntree(t, parNom[nom])), "\ufeff")))
		r.Comma = ';'
		enregistrements, err := r.ReadAll()
		if err != nil {
			t.Fatalf("%s : %v", nom, err)
		}
		return enregistrements
	}
	if n := len(compter("csv/journal_lines.csv")) - 1; n != 4 {
		t.Errorf("journal_lines.csv : %d ligne(s), attendu 4", n)
	}
	if n := len(compter("csv/invoice_lines.csv")) - 1; n != 4 {
		t.Errorf("invoice_lines.csv : %d ligne(s), attendu 4", n)
	}
	if entete := compter("csv/journal_entries.csv")[0]; strings.Contains(strings.Join(entete, ","), "lines") {
		t.Errorf("journal_entries.csv garde une colonne de lignes imbriquées : %v", entete)
	}
	if entete := compter("csv/journal_lines.csv")[0]; !strings.Contains(strings.Join(entete, ","), "entry_id") {
		t.Errorf("journal_lines.csv sans clé étrangère entry_id : %v", entete)
	}

	// 3 bis. Les pièces et mouvements ajoutés : la facture fournisseur et ses lignes reliées par clé étrangère, le
	// paiement, l'écriture de relevé, et le journal d'audit avec ses empreintes.
	if n := len(compter("csv/supplier_invoices.csv")) - 1; n != 1 {
		t.Errorf("supplier_invoices.csv : %d facture(s), attendu 1", n)
	}
	lignesFournisseur := compter("csv/supplier_invoice_lines.csv")
	if n := len(lignesFournisseur) - 1; n != 2 {
		t.Errorf("supplier_invoice_lines.csv : %d ligne(s), attendu 2", n)
	}
	if !strings.Contains(strings.Join(lignesFournisseur[0], ","), "supplier_invoice_id") {
		t.Errorf("supplier_invoice_lines.csv sans clé étrangère supplier_invoice_id : %v", lignesFournisseur[0])
	}
	if n := len(compter("csv/payments.csv")) - 1; n != 1 {
		t.Errorf("payments.csv : %d paiement(s), attendu 1", n)
	}
	if n := len(compter("csv/bank_entries.csv")) - 1; n != 1 {
		t.Errorf("bank_entries.csv : %d écriture(s) de relevé, attendu 1", n)
	}
	journalAudit := compter("csv/audit_logs.csv")
	if n := len(journalAudit) - 1; n != 2 {
		t.Errorf("audit_logs.csv : %d maillon(s), attendu 2", n)
	}
	if !strings.Contains(strings.Join(journalAudit[0], ","), "entry_hash") || !strings.Contains(strings.Join(journalAudit[0], ","), "prev_hash") {
		t.Errorf("audit_logs.csv sans ses empreintes : %v", journalAudit[0])
	}

	// 4. Le LISEZ-MOI annonce la taille réelle de chaque CSV.
	readme := string(lireEntree(t, parNom["csv/LISEZ-MOI.txt"]))
	annonce := regexp.MustCompile(`(?m)^  (\S+\.csv)\s+(\d+) octets$`).FindAllStringSubmatch(readme, -1)
	if len(annonce) != 12 {
		t.Fatalf("le LISEZ-MOI annonce %d fichier(s) CSV, attendu 12 :\n%s", len(annonce), readme)
	}
	if !strings.Contains(readme, "supplier_invoice_lines.supplier_invoice_id") || !strings.Contains(readme, "payments.invoice_id") {
		t.Errorf("le LISEZ-MOI ne documente pas les relations des pièces ajoutées :\n%s", readme)
	}
	for _, m := range annonce {
		reel := len(lireEntree(t, parNom["csv/"+m[1]]))
		if taille, _ := strconv.Atoi(m[2]); taille != reel {
			t.Errorf("LISEZ-MOI : %s annoncé à %d octets, %d dans l'archive", m[1], taille, reel)
		}
	}
}

// Le journal d'audit est exporté EN ENTIER, quels que soient les filtres : sa chaîne d'empreintes ne se
// vérifie que continue. Les pièces et mouvements, eux, suivent la période demandée.
func TestLesFiltresDeLArchiveNeCoupentPasLaChaineDAudit(t *testing.T) {
	database, _, _ := cancelEnv(t)
	brouillonFournisseur(t, database, "s1", "F-1", 200, 16.2)
	for n := 1; n <= 3; n++ {
		if _, err := database.Exec(`INSERT INTO audit_logs (id, user_id, action, table_name, record_id, sequence_number, entry_hash, prev_hash)
			VALUES (?, 'u1', 'essai', 'invoices', 'f1', ?, ?, ?)`,
			fmt.Sprintf("al%d", n), n, fmt.Sprintf("hash%d", n), fmt.Sprintf("hash%d", n-1)); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/archive", NewExportHandler(database, false).LegalArchive)
	w := httptest.NewRecorder()
	// Une période où il n'y a aucune pièce.
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/archive?from=2030-01-01&to=2030-12-31", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("archive : %d %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var audit, fournisseurs []map[string]any
	for _, f := range zr.File {
		switch {
		case strings.HasSuffix(f.Name, "/audit_logs.json"):
			_ = json.Unmarshal(lireEntree(t, f), &audit)
		case strings.HasSuffix(f.Name, "/supplier_invoices.json"):
			_ = json.Unmarshal(lireEntree(t, f), &fournisseurs)
		}
	}
	if len(audit) != 3 {
		t.Errorf("%d maillon(s) d'audit exporté(s) sous un filtre de période, attendu les 3 de la chaîne", len(audit))
	}
	if len(fournisseurs) != 0 {
		t.Errorf("%d facture(s) fournisseur sur une période qui n'en a aucune", len(fournisseurs))
	}
}

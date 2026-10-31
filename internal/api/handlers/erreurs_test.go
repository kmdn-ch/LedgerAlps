package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// CWE-209 : une réponse d'erreur ne doit pas laisser lire le schéma de la base,
// ni un chemin du poste. La cause va au journal, le client reçoit une phrase.

func contexteDeTest() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	return c, w
}

func TestErreurInterneNeRendPasLaCause(t *testing.T) {
	c, w := contexteDeTest()
	erreurInterne(c, "les comptes n'ont pas pu être lus",
		errors.New(`open C:\Users\Paul\AppData\Roaming\LedgerAlps\ledgeralps.db: no such table: accounts`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("statut = %d, attendu 500", w.Code)
	}
	body := w.Body.String()
	for _, fuite := range []string{"no such table", "accounts", `C:\Users`, "AppData", "ledgeralps.db"} {
		if strings.Contains(body, fuite) {
			t.Errorf("la réponse laisse voir %q : %s", fuite, body)
		}
	}
	if !strings.Contains(body, "les comptes n'ont pas pu être lus") {
		t.Errorf("la phrase destinée à l'utilisateur est absente : %s", body)
	}
}

// Un refus métier garde son message et son statut : c'est ce que l'interface
// affiche. Seule une erreur qui laisse voir le moteur de base devient un 500.
func TestRefusGardeLesMessagesMetierEtMasqueLesDetailsTechniques(t *testing.T) {
	c, w := contexteDeTest()
	refus(c, http.StatusUnprocessableEntity, errors.New("le total des notes de crédit dépasserait le montant de la facture"))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "dépasserait") {
		t.Errorf("refus métier altéré : %d %s", w.Code, w.Body.String())
	}

	for _, technique := range []string{
		"insert invoice: sqlite3: constraint failed: UNIQUE constraint failed: invoices.invoice_number",
		"load invoice: SQL logic error: no such column: x (1)",
		"commit: database is locked",
	} {
		c, w := contexteDeTest()
		refus(c, http.StatusUnprocessableEntity, errors.New(technique))
		if w.Code != http.StatusInternalServerError {
			t.Errorf("%q : statut %d, attendu 500", technique, w.Code)
		}
		if strings.Contains(w.Body.String(), "invoice_number") || strings.Contains(w.Body.String(), "sqlite") ||
			strings.Contains(w.Body.String(), "column") || strings.Contains(w.Body.String(), "locked") {
			t.Errorf("%q : la réponse laisse voir le détail : %s", technique, w.Body.String())
		}
	}
}

// Garde-fou de source : aucun gestionnaire ne doit refabriquer la fuite en
// écrivant `StatusInternalServerError … err.Error()` ou en collant la cause au
// message.
func TestAucuneReponse500NeRevelleSaCause(t *testing.T) {
	// (?i) : la variable s'appelle parfois `impErr` — l'ancienne expression ne voyait
	// que `err` en minuscules, et une fuite de la base est passée dessous (audit 9, M-02).
	re := regexp.MustCompile(`(?i)StatusInternalServerError[^\n]*(\w*err\w*\.Error\(\)|\+\s*\w*err\b|%v"\s*,\s*\w*err)`)
	fichiers, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fichiers {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, ligne := range strings.Split(string(b), "\n") {
			if re.MatchString(ligne) {
				t.Errorf("%s:%d renvoie la cause d'une erreur au client : %s — passer par erreurInterne(c, message, err)",
					f, i+1, strings.TrimSpace(ligne))
			}
		}
	}
}

// Audit 9, M-02 : « syntax error » figurait parmi les marques d'une erreur de base
// de données. Un relevé camt.053 malformé — « XML syntax error on line 1 » —
// répondait donc 500 au lieu de 422, régression de la 1.6.5.
func TestUnReleveXMLMalformeResteUnRefus(t *testing.T) {
	c, w := contexteDeTest()
	refus(c, http.StatusUnprocessableEntity,
		errors.New("camt.053: parse XML: XML syntax error on line 1: unexpected EOF"))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("statut = %d, attendu 422 : un fichier mal formé est un refus, pas une panne", w.Code)
	}
	if !strings.Contains(w.Body.String(), "XML syntax error") {
		t.Errorf("le motif n'est plus rendu à l'utilisateur : %s", w.Body.String())
	}

	// Le moteur SQL, lui, reste masqué sous toutes ses formes.
	for _, sql := range []string{
		`SQL logic error: near "FROM": syntax error`,
		"sqlite3: SQL logic error: incomplete input",
	} {
		c, w := contexteDeTest()
		refus(c, http.StatusUnprocessableEntity, errors.New(sql))
		if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "FROM") {
			t.Errorf("%q : statut %d, corps %s", sql, w.Code, w.Body.String())
		}
	}
}

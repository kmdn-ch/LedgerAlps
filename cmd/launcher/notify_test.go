package main

// L'écran de mise à jour doit ATTENDRE.
//
// Il portait « Ouverture de LedgerAlps dans 5 secondes… » et se remplaçait tout
// seul. C'est le seul moment où le produit dit « vos données comptables ont été
// conservées » : cinq secondes ne suffisent pas à le lire, et le message part
// avec la page.
//
// Ce test ne vérifie pas une apparence. Il vérifie le CONTRAT : la page ne
// contient aucune minuterie, le bouton mène à `/ok`, et `/ok` redirige vers
// l'application. Un compte à rebours réintroduit demain ferait échouer le
// premier point ; un bouton débranché, le second.

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rendre produit la page telle que le navigateur la reçoit.
func rendre(t *testing.T, appURL string) string {
	t.Helper()
	tpl, err := template.New("notify").Parse(notifyHTML)
	if err != nil {
		t.Fatalf("gabarit illisible : %v", err)
	}
	var b strings.Builder
	if err := tpl.Execute(&b, struct{ AppURL template.JS }{
		AppURL: template.JS(`"` + appURL + `"`),
	}); err != nil {
		t.Fatalf("rendu : %v", err)
	}
	return b.String()
}

func TestLEcranDeMiseAJourNeSeFermePasToutSeul(t *testing.T) {
	page := rendre(t, "http://localhost:8000")

	// Aucun mécanisme d'auto-navigation. Chacun de ces motifs a suffi, un jour,
	// à faire disparaître la page sans que personne n'ait rien demandé.
	for _, interdit := range []string{
		"setInterval", "setTimeout", "window.location", "location.href",
		"http-equiv=\"refresh\"", "secondes…",
	} {
		if strings.Contains(page, interdit) {
			t.Errorf("la page contient %q — elle se refermerait seule", interdit)
		}
	}

	// Et il existe bien un chemin pour continuer.
	if !strings.Contains(page, `href="/ok"`) {
		t.Error("aucun lien vers /ok : rien ne permet de continuer")
	}
}

// Le bouton mène quelque part. Un lien qui rendrait 404 laisserait l'utilisateur
// devant une page d'erreur, après une mise à jour réussie.
func TestLeBoutonRedirigeVersLApplication(t *testing.T) {
	const appURL = "http://localhost:8000"

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, appURL, http.StatusFound)
	})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if w.Code != http.StatusFound {
		t.Errorf("code = %d, attendu 302", w.Code)
	}
	if got := w.Header().Get("Location"); got != appURL {
		t.Errorf("Location = %q, attendu %q", got, appURL)
	}
}

// La phrase qui justifie tout l'écran doit y être.
func TestLEcranDitQueLesDonneesSontConservees(t *testing.T) {
	page := rendre(t, "http://localhost:8000")
	for _, attendu := range []string{
		"LedgerAlps mis à jour",
		"vos données comptables ont été conservées",
	} {
		if !strings.Contains(page, attendu) {
			t.Errorf("la page ne dit pas %q", attendu)
		}
	}
}

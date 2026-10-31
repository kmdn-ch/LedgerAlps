package main

// Audit 9, M-04 : l'assistant d'installation écoute sur 127.0.0.1 et `/setup`
// fabrique l'administrateur. Une page web ouverte pendant l'installation pouvait
// balayer les ports et lui poster une configuration — le navigateur ne bloque
// pas une requête simple. Le jeton, l'en-tête Host et l'origine ferment la porte.

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	hotePort = "127.0.0.1:43211"
	jetonOK  = "0123456789abcdef0123456789abcdef"
)

func appelAssistant(t *testing.T, page bool, methode, cible, host string, entetes map[string]string) (code int, atteint bool) {
	t.Helper()
	h := gardeAssistant(hotePort, jetonOK, page, func(w http.ResponseWriter, r *http.Request) {
		atteint = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(methode, cible, nil)
	req.Host = host
	for k, v := range entetes {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w.Code, atteint
}

func TestLaPageDeLAssistantExigeLeJeton(t *testing.T) {
	if code, atteint := appelAssistant(t, true, http.MethodGet, "/?t="+jetonOK, hotePort, nil); code != 200 || !atteint {
		t.Fatalf("avec le bon jeton : %d (atteint=%v)", code, atteint)
	}
	for nom, cible := range map[string]string{
		"sans jeton":       "/",
		"mauvais jeton":    "/?t=nope",
		"jeton tronqué":    "/?t=" + jetonOK[:10],
		"jeton en en-tête": "/",
	} {
		if code, atteint := appelAssistant(t, true, http.MethodGet, cible, hotePort,
			map[string]string{"X-Setup-Token": jetonOK}); code != 403 || atteint {
			t.Errorf("%s : %d (atteint=%v), attendu 403", nom, code, atteint)
		}
	}
	// Une page autre que la racine n'existe pas.
	if code, _ := appelAssistant(t, true, http.MethodGet, "/autre?t="+jetonOK, hotePort, nil); code != 404 {
		t.Errorf("chemin inconnu : %d, attendu 404", code)
	}
}

func TestLesRoutesDeLAssistantExigentJetonHostEtOrigine(t *testing.T) {
	bon := map[string]string{"X-Setup-Token": jetonOK, "Origin": "http://" + hotePort}
	if code, atteint := appelAssistant(t, false, http.MethodPost, "/setup", hotePort, bon); code != 200 || !atteint {
		t.Fatalf("appel légitime : %d (atteint=%v)", code, atteint)
	}
	// Le navigateur n'envoie pas toujours Origin sur un GET de même origine.
	if code, _ := appelAssistant(t, false, http.MethodGet, "/uid-lookup?che=x", hotePort,
		map[string]string{"X-Setup-Token": jetonOK}); code != 200 {
		t.Errorf("sans Origin : %d, attendu 200", code)
	}

	cas := map[string]struct {
		host    string
		entetes map[string]string
	}{
		"sans jeton":                {hotePort, map[string]string{"Origin": "http://" + hotePort}},
		"mauvais jeton":             {hotePort, map[string]string{"X-Setup-Token": "faux"}},
		"jeton vide":                {hotePort, map[string]string{"X-Setup-Token": ""}},
		"Host d'un site tiers":      {"evil.example:43211", map[string]string{"X-Setup-Token": jetonOK}},
		"Host localhost":            {"localhost:43211", map[string]string{"X-Setup-Token": jetonOK}},
		"autre port":                {"127.0.0.1:43212", map[string]string{"X-Setup-Token": jetonOK}},
		"Origin d'un site tiers":    {hotePort, map[string]string{"X-Setup-Token": jetonOK, "Origin": "http://evil.example"}},
		"Origin null (page isolée)": {hotePort, map[string]string{"X-Setup-Token": jetonOK, "Origin": "null"}},
	}
	for nom, c := range cas {
		if code, atteint := appelAssistant(t, false, http.MethodPost, "/setup", c.host, c.entetes); code != 403 || atteint {
			t.Errorf("%s : %d (atteint=%v), attendu 403 sans exécuter le gestionnaire", nom, code, atteint)
		}
	}
}

// La page et la garde vont ensemble : un jeton exigé que la page n'envoie pas
// rendrait l'installation impossible, et un jeton affiché hors d'un contexte
// JavaScript serait mal échappé.
func TestLaPageDeLAssistantPorteLeJetonSurChaqueAppel(t *testing.T) {
	tpl, err := template.New("setup").Parse(setupHTML)
	if err != nil {
		t.Fatalf("gabarit illisible : %v", err)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, jetonOK); err != nil {
		t.Fatal(err)
	}
	page := sb.String()
	if !strings.Contains(page, `const SETUP_TOKEN = "`+jetonOK+`";`) {
		t.Errorf("le jeton n'est pas posé comme chaîne JavaScript dans la page")
	}
	if n := strings.Count(page, "'X-Setup-Token': SETUP_TOKEN"); n != 2 {
		t.Errorf("%d appel(s) portent le jeton, attendu 2 (/uid-lookup et /setup)", n)
	}
}

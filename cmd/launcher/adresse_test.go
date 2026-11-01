package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// Audit croisé Gemini, série 4, HIGH-25 : le lanceur doit trouver le serveur là où il répond.
//
// Rejoué sur un serveur de bac à sable à l'écoute sur une adresse non-boucle : http://…/health
// répondait 400 (« client sent an HTTP request to an HTTPS server »), https://…/health répondait 200.

func serveurDeTest(t *testing.T, tls bool) (port string, fermer func()) {
	t.Helper()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	var s *httptest.Server
	if tls {
		s = httptest.NewTLSServer(h)
	} else {
		s = httptest.NewServer(h)
	}
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port(), s.Close
}

func TestLeLanceurTrouveUnServeurEnClair(t *testing.T) {
	port, fermer := serveurDeTest(t, false)
	defer fermer()
	base, ok := serveurJoignable(&config{Port: port})
	if !ok || base != "http://localhost:"+port {
		t.Fatalf("serveur en clair : %q, %v — attendu http://localhost:%s", base, ok, port)
	}
}

// Le défaut : un serveur en TLS (certificat auto-signé) n'était pas reconnu — le lanceur en lançait un
// second et abandonnait au bout de vingt secondes.
func TestLeLanceurTrouveUnServeurEnTLSAutoSigne(t *testing.T) {
	port, fermer := serveurDeTest(t, true)
	defer fermer()
	for nom, cfg := range map[string]*config{
		"sans rien d'autre":     {Port: port},
		"avec un certificat":    {Port: port, TLSCert: "cert.pem", TLSKey: "key.pem"},
		"hôte en boucle locale": {Port: port, Host: "127.0.0.1"},
	} {
		t.Run(nom, func(t *testing.T) {
			base, ok := serveurJoignable(cfg)
			if !ok {
				t.Fatalf("serveur en TLS non trouvé (%+v)", cfg)
			}
			if base[:8] != "https://" {
				t.Errorf("adresse %q : attendu https://", base)
			}
		})
	}
}

// Un serveur qui n'est pas là n'est pas inventé.
func TestLeLanceurNeTrouveRienSiLeServeurNEstPasLa(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, p, _ := net.SplitHostPort(l.Addr().String())
	l.Close() // le port est libre, plus personne n'y écoute
	if base, ok := serveurJoignable(&config{Port: p}); ok {
		t.Fatalf("un serveur absent a été « trouvé » : %q", base)
	}
}

func TestLesCandidatsSuiventLaConfiguration(t *testing.T) {
	cas := []struct {
		nom   string
		cfg   config
		debut string
		n     int
	}{
		{"défaut", config{Port: "8000"}, "http://localhost:8000", 2},
		{"TLS configuré : https d'abord", config{Port: "8000", TLSCert: "c", TLSKey: "k"}, "https://localhost:8000", 2},
		{"hôte précis : lui d'abord", config{Port: "8000", Host: "192.168.1.5"}, "http://192.168.1.5:8000", 4},
		{"toutes interfaces : localhost", config{Port: "8000", Host: "0.0.0.0"}, "http://localhost:8000", 2},
		{"IPv6 précise : crochets", config{Port: "8000", Host: "fe80::1"}, "http://[fe80::1]:8000", 4},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			got := candidatsDuServeur(&c.cfg)
			if len(got) != c.n || got[0] != c.debut {
				t.Errorf("candidats %v, attendu %d adresses commençant par %s", got, c.n, c.debut)
			}
		})
	}
}

func TestJoindreServeurAttendLeServeur(t *testing.T) {
	port, fermer := serveurDeTest(t, true)
	defer fermer()
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	base, err := joindreServeur(ctx, &config{Port: port})
	if err != nil || base == "" {
		t.Fatalf("joindreServeur : %q, %v", base, err)
	}
}

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

// Le serveur n'écoute que sur la boucle locale, en HTTP (1.6.6) : le lanceur n'a qu'une adresse à
// sonder, et ne reconnaît pas un serveur TLS — il n'y en a plus.

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

func TestLAdresseDuServeurEstLocaleEtEnClair(t *testing.T) {
	if got := adresseDuServeur(&config{Port: "8000"}); got != "http://localhost:8000" {
		t.Errorf("adresse = %q, attendu http://localhost:8000", got)
	}
}

func TestLeLanceurTrouveLeServeurLocal(t *testing.T) {
	port, fermer := serveurDeTest(t, false)
	defer fermer()
	base, ok := serveurJoignable(&config{Port: port})
	if !ok || base != "http://localhost:"+port {
		t.Fatalf("serveur local : %q, %v — attendu http://localhost:%s", base, ok, port)
	}
}

// Il n'y a plus de serveur TLS : un serveur qui répond en TLS n'est pas le nôtre, il n'est pas
// « trouvé » en clair.
func TestLeLanceurNeReconnaitPasUnServeurTLS(t *testing.T) {
	port, fermer := serveurDeTest(t, true)
	defer fermer()
	if base, ok := serveurJoignable(&config{Port: port}); ok {
		t.Fatalf("un serveur TLS a été « trouvé » : %q", base)
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

func TestJoindreServeurAttendLeServeur(t *testing.T) {
	port, fermer := serveurDeTest(t, false)
	defer fermer()
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	base, err := joindreServeur(ctx, &config{Port: port})
	if err != nil || base == "" {
		t.Fatalf("joindreServeur : %q, %v", base, err)
	}
}

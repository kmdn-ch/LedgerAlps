package db

import (
	"testing"
	"time"
)

// La table security_events annonçait « retention-limited » dans son schéma sans
// que rien ne l'applique. Une adresse IP est une donnée personnelle : la nLPD
// art. 6 al. 4 impose de la détruire ou de l'anonymiser dès qu'elle n'est plus
// nécessaire. Un commentaire décrivant une garantie inexistante est pire que
// l'absence de garantie — il empêche de voir le manque.

func TestRetentionAnonymisesThenDeletes(t *testing.T) {
	database := newBackfillDB(t)
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	seed := func(id string, age time.Duration) {
		t.Helper()
		if _, err := database.Exec(`
			INSERT INTO security_events (id, event_type, ip_address, detail, created_at)
			VALUES (?, 'login_lockout', '203.0.113.7', 'test', ?)`,
			id, now.Add(-age)); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	seed("recent", 10*24*time.Hour)   // conservé tel quel
	seed("mid", 120*24*time.Hour)     // adresse anonymisée, événement gardé
	seed("ancient", 400*24*time.Hour) // supprimé

	rep, err := ApplyRetention(database, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if rep.IPsAnonymised != 2 || rep.EventsDeleted != 1 {
		// « mid » et « ancient » dépassent tous deux la rétention d'adresse ;
		// « ancient » est ensuite supprimé.
		t.Fatalf("rapport = %+v, attendu 2 anonymisées et 1 supprimé", rep)
	}

	var ipRecent, ipMid string
	if err := database.QueryRow(
		`SELECT ip_address FROM security_events WHERE id = 'recent'`).Scan(&ipRecent); err != nil {
		t.Fatal(err)
	}
	if ipRecent != "203.0.113.7" {
		t.Fatalf("un événement récent a perdu son adresse (%q) : le signal de sécurité disparaît sans nécessité", ipRecent)
	}

	if err := database.QueryRow(
		`SELECT ip_address FROM security_events WHERE id = 'mid'`).Scan(&ipMid); err != nil {
		t.Fatal(err)
	}
	if ipMid != "(anonymisée)" {
		t.Fatalf("adresse de « mid » = %q, attendu le marqueur d'anonymisation", ipMid)
	}

	var remaining int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM security_events WHERE id = 'ancient'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("un événement au-delà d'un an subsiste")
	}
}

// Le marqueur explicite distingue « purgée » de « jamais enregistrée ». Une
// seconde passe ne doit pas le recompter, sinon le rapport affiché à
// l'utilisateur annoncerait un travail qui n'a pas eu lieu.
func TestRetentionIsIdempotent(t *testing.T) {
	database := newBackfillDB(t)
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	if _, err := database.Exec(`
		INSERT INTO security_events (id, event_type, ip_address, created_at)
		VALUES ('e', 'login_lockout', '198.51.100.4', ?)`, now.Add(-200*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	first, err := ApplyRetention(database, false, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ApplyRetention(database, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.IPsAnonymised != 1 {
		t.Fatalf("première passe = %+v, attendu 1 anonymisation", first)
	}
	if second.IPsAnonymised != 0 || second.EventsDeleted != 0 {
		t.Fatalf("seconde passe = %+v, attendu aucun effet", second)
	}
}

// Une base sans événement ne doit rien produire, et surtout pas une erreur au
// démarrage.
func TestRetentionOnEmptyTable(t *testing.T) {
	database := newBackfillDB(t)
	rep, err := ApplyRetention(database, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if rep.IPsAnonymised != 0 || rep.EventsDeleted != 0 {
		t.Fatalf("rapport = %+v sur une base vide", rep)
	}
}

package invoicing

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// « Un compte désactivé ne peut plus rien faire, et les documents portent le nom
// de leur auteur » — la phrase affichée dans l'interface. Elle doit se vérifier,
// pas se croire.
//
// Elle ne se vérifiait qu'à moitié : la chaîne d'empreintes du CO art. 957a ne
// couvrait que le journal. Une facture portait « qui l'a créée » et rien sur
// qui l'avait envoyée, corrigée ou annulée — c'est-à-dire tout ce qui lui
// arrive après sa naissance.

func auditService(t *testing.T) (*Service, *sql.DB, string) {
	t.Helper()
	s, database, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	s.accountingSvc = accounting.New(database, false)
	// audit_logs.user_id porte une clé étrangère vers users : une trace ne peut
	// pas désigner un compte inexistant, et c'est voulu — une trace attribuée à
	// un identifiant qui n'existe nulle part ne prouve rien.
	if _, err := database.Exec(
		`INSERT INTO users (id,email,name,password_hash,is_admin) VALUES ('u2','b@t.ch','B','x',0)`); err != nil {
		t.Fatal(err)
	}
	return s, database, contactID
}

// auditRowsPour lit les traces d'UNE action.
//
// Filtrer par action, et non compter toutes les lignes de la table : sinon
// chaque nouvelle action cablee cassera mecaniquement des tests qui ne la
// concernent pas -- c'est ce qui est arrive quand ActionDocumentCreated a ete
// branchee, et un test qui casse pour une bonne nouvelle finit par etre
// desactive.
func auditRowsPour(t *testing.T, database *sql.DB, table, action string) []struct {
	Action, UserID, RecordID string
} {
	t.Helper()
	var out []struct{ Action, UserID, RecordID string }
	for _, r := range auditRows(t, database, table) {
		if r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

func auditRows(t *testing.T, database *sql.DB, table string) []struct {
	Action, UserID, RecordID string
} {
	t.Helper()
	rows, err := database.Query(
		`SELECT action, user_id, record_id FROM audit_logs WHERE table_name = ? ORDER BY sequence_number`,
		table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []struct{ Action, UserID, RecordID string }
	for rows.Next() {
		var r struct{ Action, UserID, RecordID string }
		if err := rows.Scan(&r.Action, &r.UserID, &r.RecordID); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// L'envoi d'une facture doit laisser une trace nommant son auteur.
func TestLEnvoiDUneFactureEstTraceAvecSonAuteur(t *testing.T) {
	s, database, contactID := auditService(t)
	invID := makeInvoice(t, s, contactID, 500, 0)

	err := s.TransitionBy(context.Background(), invID, models.InvoiceStatusSent,
		Actor{UserID: "u1", IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}

	rows := auditRowsPour(t, database, accounting.TableInvoices,
		accounting.ActionDocumentTransition)
	if len(rows) != 1 {
		t.Fatalf("%d trace(s) de transition pour un envoi, attendu 1", len(rows))
	}
	if rows[0].UserID != "u1" {
		t.Errorf("auteur = %q, attendu u1", rows[0].UserID)
	}
	if rows[0].RecordID != invID {
		t.Errorf("document = %q, attendu %q", rows[0].RecordID, invID)
	}
	if rows[0].Action != accounting.ActionDocumentTransition {
		t.Errorf("action = %q", rows[0].Action)
	}
}

// L'annulation aussi — c'est même l'action qu'on cherche le plus souvent à
// attribuer après coup.
func TestLAnnulationEstTracee(t *testing.T) {
	s, database, contactID := auditService(t)
	invID := makeInvoice(t, s, contactID, 500, 0)

	if err := s.TransitionBy(context.Background(), invID, models.InvoiceStatusSent,
		Actor{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionBy(context.Background(), invID, models.InvoiceStatusCancelled,
		Actor{UserID: "u2"}); err != nil {
		t.Fatal(err)
	}

	rows := auditRowsPour(t, database, accounting.TableInvoices,
		accounting.ActionDocumentTransition)
	if len(rows) != 2 {
		t.Fatalf("%d traces de transition, attendu 2", len(rows))
	}
	if rows[1].UserID != "u2" {
		t.Fatalf("l'annulation est attribuée à %q, attendu u2", rows[1].UserID)
	}
}

// LE point qui donne sa valeur à la trace : elle entre dans la MÊME chaîne
// d'empreintes que les écritures, et se vérifie donc de la même façon.
//
// Un second registre non chaîné aurait la force d'une table qu'on peut
// modifier — c'est-à-dire aucune.
func TestLaTraceDUnDocumentEstDansLaChaineEtSeVerifie(t *testing.T) {
	s, database, contactID := auditService(t)
	invID := makeInvoice(t, s, contactID, 500, 0)
	if err := s.TransitionBy(context.Background(), invID, models.InvoiceStatusSent,
		Actor{UserID: "u1", IP: "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}

	var userID, action, tableName, recordID, before, after, ip, storedHash string
	var createdAt any
	var seq int64
	err := database.QueryRow(`
		SELECT user_id, action, table_name, record_id,
		       COALESCE(before_state,''), COALESCE(after_state,''),
		       COALESCE(ip_address,''), entry_hash, sequence_number, created_at
		FROM audit_logs WHERE table_name = ?`, accounting.TableInvoices).
		Scan(&userID, &action, &tableName, &recordID, &before, &after, &ip, &storedHash, &seq, &createdAt)
	if err != nil {
		t.Fatal(err)
	}

	// La séquence est commune : la trace du document s'intercale dans la même
	// numérotation que les écritures, ce qui rend un retrait détectable.
	if seq < 1 {
		t.Fatalf("numéro de séquence = %d", seq)
	}

	_ = createdAt
	// Recalcul avec l'algorithme de vérification du produit.
	ts := mustTime(t, database, storedHash)
	recomputed := security.ComputeEntryHash(userID, action, tableName, recordID, before, after, ip, ts)
	if recomputed != storedHash {
		t.Fatalf("l'empreinte ne se recalcule pas :\n  stockée   %s\n  recalculée %s", storedHash, recomputed)
	}
}

// Altérer une trace doit casser son empreinte — sinon la chaîne ne prouve rien.
func TestAltererUneTraceCasseSonEmpreinte(t *testing.T) {
	s, database, contactID := auditService(t)
	invID := makeInvoice(t, s, contactID, 500, 0)
	if err := s.TransitionBy(context.Background(), invID, models.InvoiceStatusSent,
		Actor{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}

	var storedHash string
	database.QueryRow(`SELECT entry_hash FROM audit_logs WHERE table_name = ?`,
		accounting.TableInvoices).Scan(&storedHash)

	// Quelqu'un réattribue l'action à un autre compte RÉEL — l'attaque
	// plausible. Un identifiant inventé serait de toute façon refusé par la
	// clé étrangère de audit_logs.user_id.
	if _, err := database.Exec(
		`UPDATE audit_logs SET user_id = 'u2' WHERE table_name = ?`,
		accounting.TableInvoices); err != nil {
		t.Fatal(err)
	}

	var userID, action, tableName, recordID, before, after, ip string
	database.QueryRow(`
		SELECT user_id, action, table_name, record_id,
		       COALESCE(before_state,''), COALESCE(after_state,''), COALESCE(ip_address,'')
		FROM audit_logs WHERE table_name = ?`, accounting.TableInvoices).
		Scan(&userID, &action, &tableName, &recordID, &before, &after, &ip)

	ts := mustTime(t, database, storedHash)
	recomputed := security.ComputeEntryHash(userID, action, tableName, recordID, before, after, ip, ts)
	if recomputed == storedHash {
		t.Fatal("réattribuer l'action à un autre compte n'a pas cassé l'empreinte")
	}
}

// Sans auteur, aucune trace n'est écrite — plutôt qu'une trace anonyme, qui
// ferait croire à une couverture inexistante.
func TestSansAuteurAucuneTraceAnonyme(t *testing.T) {
	s, database, contactID := auditService(t)
	invID := makeInvoice(t, s, contactID, 500, 0)

	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}
	// La CREATION, elle, a bien un auteur (makeInvoice le fournit) : on ne
	// compte donc que les transitions, qui sont ce que ce test eprouve.
	rows := auditRowsPour(t, database, accounting.TableInvoices,
		accounting.ActionDocumentTransition)
	if len(rows) != 0 {
		t.Fatalf("%d trace(s) écrite(s) sans auteur : %+v", len(rows), rows)
	}
}

// mustTime relit l'horodatage tel qu'il est STOCKÉ. Le recalculer à partir
// d'une valeur reconstruite ferait échouer la vérification pour une raison qui
// n'a rien à voir avec l'intégrité — c'est le défaut corrigé en v1.4.6.
func mustTime(t *testing.T, database *sql.DB, hash string) time.Time {
	t.Helper()
	var ts time.Time
	if err := database.QueryRow(
		`SELECT created_at FROM audit_logs WHERE entry_hash = ?`, hash).Scan(&ts); err != nil {
		t.Fatal(err)
	}
	return ts
}

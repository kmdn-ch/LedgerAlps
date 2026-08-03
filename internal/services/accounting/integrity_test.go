package accounting

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Ce fichier existe à cause d'un défaut précis.
//
// Jusqu'à la v1.4.6, la comptabilisation calculait l'empreinte d'intégrité sur
// un JSON et un horodatage QUI N'ÉTAIENT PAS ceux enregistrés dans la ligne :
// l'after_state haché contenait un champ posted_at absent du JSON inséré, et le
// created_at haché venait de Go tandis que la colonne était remplie par le
// DEFAULT CURRENT_TIMESTAMP de SQLite, à la seconde près. Conséquence : aucune
// écriture comptabilisée ne pouvait se revérifier, jamais.
//
// Le défaut a survécu à toute la suite de tests parce que chaque test qui
// touchait aux empreintes fabriquait lui-même ses lignes, avec les mêmes
// valeurs des deux côtés. Il n'est apparu qu'au premier appel réel, en écrivant
// l'écran de la piste d'audit.
//
// La leçon, et la raison d'être des tests ci-dessous : une empreinte ne se
// vérifie qu'en écrivant par le vrai chemin, puis en relisant depuis la base.
// Recalculer en mémoire ce qu'on vient de calculer en mémoire ne prouve rien.

func newAccountingDB(t *testing.T) (*Service, *sql.DB, string, string) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-acct-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := database.Exec(
		`INSERT INTO users (id, email, name, password_hash, is_admin) VALUES (?, ?, ?, ?, 1)`,
		"u1", "acct@example.test", "Comptable de test", "x",
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Le plan comptable est posé par la migration 0002 : on emprunte deux
	// comptes réels plutôt que d'en inventer.
	var bank, sales string
	q := `SELECT id FROM accounts WHERE code = ? LIMIT 1`
	if err := database.QueryRow(q, "1020").Scan(&bank); err != nil {
		t.Fatalf("compte 1020 introuvable: %v", err)
	}
	if err := database.QueryRow(q, "3000").Scan(&sales); err != nil {
		t.Fatalf("compte 3000 introuvable: %v", err)
	}
	return New(database, false), database, bank, sales
}

func postOne(t *testing.T, s *Service, bank, sales string, day int) string {
	t.Helper()
	ctx := context.Background()
	debit, credit := 1000.0, 1000.0

	entry, err := s.CreateEntry(ctx, "u1", CreateEntryRequest{
		Date:        time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC),
		Description: "Vente",
		Lines: []LineInput{
			{AccountID: bank, DebitAmount: &debit, Sequence: 1},
			{AccountID: sales, CreditAmount: &credit, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatalf("créer l'écriture: %v", err)
	}
	if err := s.PostEntry(ctx, "u1", entry.ID, "127.0.0.1"); err != nil {
		t.Fatalf("comptabiliser: %v", err)
	}
	return entry.ID
}

// Le test qui manquait : comptabiliser par le vrai chemin, puis recalculer
// l'empreinte à partir de ce que la BASE contient réellement.
func TestPostedEntryHashRecomputesFromWhatIsStored(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 4)

	var (
		userID, action, tableName, recordID string
		beforeState, afterState, ipAddress  string
		storedHash                          string
		createdAt                           time.Time
		hashVersion                         int
	)
	err := database.QueryRow(`
		SELECT COALESCE(user_id,''), action, table_name, record_id,
		       COALESCE(before_state,''), COALESCE(after_state,''), COALESCE(ip_address,''),
		       entry_hash, created_at, hash_version
		FROM audit_logs WHERE record_id = ?`, entryID).Scan(
		&userID, &action, &tableName, &recordID,
		&beforeState, &afterState, &ipAddress,
		&storedHash, &createdAt, &hashVersion,
	)
	if err != nil {
		t.Fatalf("relire l'entrée d'audit: %v", err)
	}

	if hashVersion != 2 {
		t.Fatalf("hash_version = %d, attendu 2 : les nouvelles écritures doivent utiliser le calcul corrigé", hashVersion)
	}

	recomputed := security.ComputeEntryHash(
		userID, action, tableName, recordID,
		beforeState, afterState, ipAddress, createdAt,
	)
	if recomputed != storedHash {
		t.Fatalf(
			"l'empreinte ne se recalcule pas depuis la base :\n  stockée   = %s\n  recalculée = %s\n"+
				"L'empreinte doit couvrir EXACTEMENT les valeurs enregistrées, sinon elle ne prouve rien.",
			storedHash, recomputed)
	}
}

// Même vérification, par la porte publique du service.
func TestVerifyEntryIntegrityAcceptsAFreshlyPostedEntry(t *testing.T) {
	s, _, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 5)

	if err := s.VerifyEntryIntegrity(context.Background(), entryID); err != nil {
		t.Fatalf("une écriture qui vient d'être comptabilisée est déclarée non intègre : %v", err)
	}
}

// Une altération réelle doit toujours être refusée : la tolérance accordée aux
// entrées anciennes ne doit pas avoir désarmé le contrôle.
func TestVerifyEntryIntegrityRejectsATamperedEntry(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 6)

	if _, err := database.Exec(
		`UPDATE audit_logs SET after_state = ? WHERE record_id = ?`,
		`{"credit":1,"debit":1,"entry_id":"x","status":"posted"}`, entryID,
	); err != nil {
		t.Fatal(err)
	}

	err := s.VerifyEntryIntegrity(context.Background(), entryID)
	if err == nil {
		t.Fatal("un contenu modifié est accepté")
	}
	if _, ok := err.(ErrIntegrityViolation); !ok {
		t.Fatalf("erreur = %T (%v), attendu ErrIntegrityViolation", err, err)
	}
}

// Une entrée en ancien format n'est pas une entrée corrompue. La distinction
// doit remonter dans le type d'erreur, sans quoi l'appelant ne peut pas dire
// « je ne sais pas » au lieu de « vos livres ont été altérés ».
func TestVerifyEntryIntegrityDistinguishesLegacyFromTampered(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 7)

	if _, err := database.Exec(
		`UPDATE audit_logs SET hash_version = 1 WHERE record_id = ?`, entryID,
	); err != nil {
		t.Fatal(err)
	}

	err := s.VerifyEntryIntegrity(context.Background(), entryID)
	if _, ok := err.(ErrIntegrityNotVerifiable); !ok {
		t.Fatalf("erreur = %T (%v), attendu ErrIntegrityNotVerifiable", err, err)
	}
}

// La chaîne doit se construire correctement sur plusieurs écritures : chaque
// prev_hash pointe sur l'empreinte de la précédente, les numéros se suivent.
func TestPostedEntriesFormAContinuousChain(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	for day := 10; day <= 13; day++ {
		postOne(t, s, bank, sales, day)
	}

	rows, err := database.Query(
		`SELECT sequence_number, entry_hash, COALESCE(prev_hash,'') FROM audit_logs ORDER BY sequence_number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var (
		expectedSeq  int64 = 1
		expectedPrev string
		seen         int
	)
	for rows.Next() {
		var seq int64
		var hash, prev string
		if err := rows.Scan(&seq, &hash, &prev); err != nil {
			t.Fatal(err)
		}
		if seq != expectedSeq {
			t.Fatalf("numéro de séquence = %d, attendu %d", seq, expectedSeq)
		}
		if prev != expectedPrev {
			t.Fatalf("prev_hash de l'entrée %d = %q, attendu %q", seq, prev, expectedPrev)
		}
		expectedSeq++
		expectedPrev = hash
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 4 {
		t.Fatalf("%d entrées dans la chaîne, attendu 4", seen)
	}
}

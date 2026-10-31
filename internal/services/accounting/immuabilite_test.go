package accounting

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db/dbtest"
)

// Audit croisé Gemini, CRIT-10 : « l'illusion d'immuabilité ».
//
// Seule `journal_entries` avait ses déclencheurs. Sur une écriture comptabilisée, un UPDATE de
// journal_lines (compte 6000 → 6200, montant 999 999) était accepté, et la vérification de la
// chaîne répondait ensuite « intègre » : le maillon d'audit ne scellait que les TOTAUX.
//
// Trois défenses, dont aucune ne suffit seule :
//   - les déclencheurs ferment la porte à tout ce qui passe par la base ;
//   - le condensé des lignes, scellé dans le maillon chaîné, fait VOIR une réécriture faite en
//     retirant les déclencheurs — ce que fait quiconque écrit dans le fichier ;
//   - l'empreinte portée par l'écriture est comparée à celle du journal d'audit.

func TestLesLignesDUneEcritureComptabiliseeSontImmuables(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 4)

	for nom, requete := range map[string]string{
		"changer le compte":   `UPDATE journal_lines SET account_id = (SELECT id FROM accounts WHERE code = '6000') WHERE entry_id = '` + entryID + `' AND credit_amount IS NOT NULL`,
		"changer le montant":  `UPDATE journal_lines SET debit_amount = 999999 WHERE entry_id = '` + entryID + `' AND debit_amount IS NOT NULL`,
		"supprimer une ligne": `DELETE FROM journal_lines WHERE entry_id = '` + entryID + `' AND credit_amount IS NOT NULL`,
		"ajouter une ligne": `INSERT INTO journal_lines (id, entry_id, account_id, debit_amount, sequence)
		                      VALUES ('intruse', '` + entryID + `', '` + bank + `', 1, 9)`,
	} {
		if _, err := database.Exec(requete); err == nil {
			t.Errorf("%s : accepté sur une écriture comptabilisée", nom)
		} else if !strings.Contains(err.Error(), "comptabilisée") {
			t.Errorf("%s : refusé, mais pas par le déclencheur d'immuabilité : %v", nom, err)
		}
	}
}

// Les lignes d'un BROUILLON, elles, se modifient : on les édite avant de comptabiliser.
func TestLesLignesDUnBrouillonSeModifient(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	debit, credit := 50.0, 50.0
	entry, err := s.CreateEntry(context.Background(), "u1", CreateEntryRequest{
		Date: dateDe(2026, 3, 9), Description: "brouillon",
		Lines: []LineInput{
			{AccountID: bank, DebitAmount: &debit, Sequence: 1},
			{AccountID: sales, CreditAmount: &credit, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE journal_lines SET description = 'corrigé' WHERE entry_id = ?`, entry.ID); err != nil {
		t.Fatalf("modifier un brouillon : %v", err)
	}
	if _, err := database.Exec(`DELETE FROM journal_lines WHERE entry_id = ? AND credit_amount IS NOT NULL`, entry.ID); err != nil {
		t.Fatalf("supprimer une ligne d'un brouillon : %v", err)
	}
}

func TestLeJournalDAuditNeSeReecritPas(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	postOne(t, s, bank, sales, 4)
	if _, err := database.Exec(`UPDATE audit_logs SET user_id = 'u1'`); err == nil {
		t.Error("un maillon d'audit a été modifié")
	}
	if _, err := database.Exec(`DELETE FROM audit_logs`); err == nil {
		t.Error("un maillon d'audit a été supprimé")
	}
}

// LE scénario de l'audit : le compte d'une ligne change, le montant non. Les totaux, donc
// l'ancien maillon, ne bougeaient pas — la vérification répondait « intègre ». L'attaquant a
// retiré les déclencheurs, comme tout qui écrit dans le fichier.
func TestUnCompteChangeSansToucherAuMontantSeVoit(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 4)
	ctx := context.Background()
	if err := s.VerifyEntryIntegrity(ctx, entryID); err != nil {
		t.Fatalf("une écriture intacte est refusée : %v", err)
	}

	dbtest.Desarmer(t, database)
	if _, err := database.Exec(
		`UPDATE journal_lines SET account_id = (SELECT id FROM accounts WHERE code = '6000')
		 WHERE entry_id = ? AND credit_amount IS NOT NULL`, entryID); err != nil {
		t.Fatal(err)
	}
	// Le montant n'a pas changé : l'équilibre, lui, ne voit rien.
	var d, c float64
	if err := database.QueryRow(
		`SELECT SUM(COALESCE(debit_amount,0)), SUM(COALESCE(credit_amount,0)) FROM journal_lines WHERE entry_id = ?`, entryID,
	).Scan(&d, &c); err != nil || d != c {
		t.Fatalf("préambule : l'écriture doit rester équilibrée (%.2f / %.2f, %v)", d, c, err)
	}

	err := s.VerifyEntryIntegrity(ctx, entryID)
	var alteree ErrLinesAltered
	if !errors.As(err, &alteree) {
		t.Fatalf("la réécriture d'un compte n'est pas détectée : %v", err)
	}
}

// Un condensé n'a de valeur que si le maillon qui le porte est lui-même scellé : retoucher le
// condensé dans le maillon pour le faire correspondre aux lignes rompt la chaîne.
func TestRetoucherLeCondenseRompLaChaine(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 4)
	dbtest.Desarmer(t, database)

	if _, err := database.Exec(
		`UPDATE journal_lines SET account_id = (SELECT id FROM accounts WHERE code = '6000')
		 WHERE entry_id = ? AND credit_amount IS NOT NULL`, entryID); err != nil {
		t.Fatal(err)
	}
	nouveau, err := LinesDigest(context.Background(), database, false, entryID)
	if err != nil {
		t.Fatal(err)
	}
	var etat string
	if err := database.QueryRow(`SELECT after_state FROM audit_logs WHERE record_id = ? AND action = 'post'`, entryID).Scan(&etat); err != nil {
		t.Fatal(err)
	}
	var ancien string
	for _, part := range strings.Split(etat, ",") {
		if strings.Contains(part, "lines_digest") {
			ancien = strings.Trim(strings.Split(part, ":")[1], `"}`)
		}
	}
	if ancien == "" || ancien == nouveau {
		t.Fatalf("préambule : le condensé aurait dû changer (%q → %q)", ancien, nouveau)
	}
	if _, err := database.Exec(`UPDATE audit_logs SET after_state = REPLACE(after_state, ?, ?) WHERE record_id = ?`, ancien, nouveau, entryID); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyEntryIntegrity(context.Background(), entryID); err == nil {
		t.Fatal("un condensé réécrit dans le maillon, sans refaire la chaîne, est accepté")
	} else if _, ok := err.(ErrIntegrityViolation); !ok {
		t.Fatalf("erreur = %T (%v), attendu ErrIntegrityViolation", err, err)
	}
}

// L'empreinte portée par l'écriture était lue et jamais comparée (CRIT-10 c).
func TestLEmpreinteStockeeSurLEcritureEstVerifiee(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 4)
	// Le déclencheur de journal_entries protège aussi cette colonne : il faut le retirer.
	if _, err := database.Exec(`DROP TRIGGER IF EXISTS trg_journal_entries_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE journal_entries SET integrity_hash = 'falsifiee' WHERE id = ?`, entryID); err != nil {
		t.Fatal(err)
	}
	err := s.VerifyEntryIntegrity(context.Background(), entryID)
	if _, ok := err.(ErrIntegrityViolation); !ok {
		t.Fatalf("erreur = %T (%v), attendu ErrIntegrityViolation : l'empreinte de l'écriture a été falsifiée", err, err)
	}
}

// Une écriture comptabilisée AVANT cette version ne porte pas de condensé : ses lignes ne sont
// pas vérifiables, et on ne l'accuse de rien.
func TestUneEcritureSansCondenseNEstPasAccusee(t *testing.T) {
	s, database, bank, sales := newAccountingDB(t)
	entryID := postOne(t, s, bank, sales, 4)
	dbtest.Desarmer(t, database)
	// Retirer le condensé ET refaire l'empreinte : l'état d'une écriture d'avant cette version.
	if err := reecrireMaillonSansCondense(t, database, entryID); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyEntryIntegrity(context.Background(), entryID); err != nil {
		t.Fatalf("une écriture d'avant le condensé est refusée : %v", err)
	}
}

func dateDe(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

var reCondense = regexp.MustCompile(`"lines_digest":"[0-9a-f]+",`)

// reecrireMaillonSansCondense reproduit l'état d'une écriture comptabilisée avant le condensé :
// un maillon sans `lines_digest`, avec son empreinte et celle de l'écriture refaites — ce que
// la version précédente aurait écrit.
func reecrireMaillonSansCondense(t *testing.T, database *sql.DB, entryID string) error {
	t.Helper()
	var userID, action, table, recordID, avant, apres, ip, prev string
	var created time.Time
	if err := database.QueryRow(`
		SELECT COALESCE(user_id,''), action, table_name, record_id, COALESCE(before_state,''), COALESCE(after_state,''),
		       COALESCE(ip_address,''), created_at, COALESCE(prev_hash,'')
		FROM audit_logs WHERE record_id = ? AND action = 'post'`, entryID).
		Scan(&userID, &action, &table, &recordID, &avant, &apres, &ip, &created, &prev); err != nil {
		return err
	}
	sans := reCondense.ReplaceAllString(apres, "")
	if sans == apres {
		t.Fatalf("préambule : le maillon ne portait pas de condensé : %s", apres)
	}
	nouveau := security.ComputeEntryHash(userID, action, table, recordID, avant, sans, ip, created)
	if _, err := database.Exec(`UPDATE audit_logs SET after_state = ?, entry_hash = ? WHERE record_id = ? AND action = 'post'`, sans, nouveau, entryID); err != nil {
		return err
	}
	if _, err := database.Exec(`DROP TRIGGER IF EXISTS trg_journal_entries_no_update`); err != nil {
		return err
	}
	_, err := database.Exec(`UPDATE journal_entries SET integrity_hash = ? WHERE id = ?`, security.ChainHash(prev, nouveau), entryID)
	return err
}

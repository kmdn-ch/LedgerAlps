package db

// La migration 0026 pose un index UNIQUE sur le numéro de séquence de la chaîne
// d'audit. Une base DÉJÀ fourchue — parce qu'elle a tourné avant que l'écriture
// passe en transaction — doit pouvoir migrer quand même : échouer ici
// laisserait l'utilisateur sans ses livres au démarrage, pour un défaut qui ne
// l'empêchait pas de travailler.

import (
	"database/sql"
	"os"
	"testing"
)

func TestUneBaseFourchueMigreQuandMeme(t *testing.T) {
	f, err := os.CreateTemp("", "fourche-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	d, err := sql.Open("sqlite3", "file:"+f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	if err := Migrate(d, false); err != nil {
		t.Fatalf("migration initiale: %v", err)
	}

	// Reproduire l'état d'avant : index non unique, et deux maillons qui
	// partagent le numéro 2 — exactement ce que produisait la course.
	if _, err := d.Exec(`DROP INDEX IF EXISTS idx_audit_logs_sequence`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO users(id,email,name,password_hash,role,is_admin,is_active,created_at,updated_at)
		VALUES('u1','a@b.ch','A','x','admin',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for i, seq := range []int{1, 2, 2, 3} {
		if _, err := d.Exec(`INSERT INTO audit_logs(id,user_id,action,table_name,record_id,entry_hash,prev_hash,sequence_number,created_at)
			VALUES(?,'u1','create','invoices','r','h','p',?,CURRENT_TIMESTAMP)`,
			string(rune('a'+i)), seq); err != nil {
			t.Fatal(err)
		}
	}

	// Rejouer 0026 sur cette base fourchue.
	if _, err := d.Exec(
		`DELETE FROM schema_migrations WHERE version='0026_audit_sequence_unique'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d, false); err != nil {
		t.Fatalf("la migration échoue sur une base fourchue — l'utilisateur "+
			"resterait sans ses livres au démarrage: %v", err)
	}

	var distincts, total int
	if err := d.QueryRow(
		`SELECT COUNT(DISTINCT sequence_number), COUNT(*) FROM audit_logs`).
		Scan(&distincts, &total); err != nil {
		t.Fatal(err)
	}
	if distincts != total {
		t.Errorf("après migration : %d numéros distincts pour %d maillons — la fourche subsiste",
			distincts, total)
	}

	// Et la contrainte tient désormais : une insertion en double est refusée.
	if _, err := d.Exec(`INSERT INTO audit_logs(id,user_id,action,table_name,record_id,entry_hash,prev_hash,sequence_number,created_at)
		VALUES('z','u1','create','invoices','r','h','p',1,CURRENT_TIMESTAMP)`); err == nil {
		t.Error("un numéro de séquence en double est encore accepté — l'index unique ne tient pas")
	}
}

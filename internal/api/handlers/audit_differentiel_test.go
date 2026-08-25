package handlers

// L'audit différentiel, vérifié de bout en bout.
//
// Les tests unitaires du paquet `accounting` couvrent le calcul des champs
// modifiés et le masquage. Ceux-ci couvrent ce qui compte pour la conformité :
// qu'un état antérieur réellement écrit en base n'empêche PAS la chaîne
// d'empreintes du CO art. 957a de se vérifier, et que les maillons écrits avant
// la fonctionnalité continuent de se vérifier à côté des nouveaux.
//
// C'est la question qui décide de tout : une piste plus riche qui casserait la
// vérification serait un recul, pas un progrès.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// ecrireMaillon ajoute un maillon d'audit dans une transaction, comme le
// produit le fait.
func ecrireMaillon(t *testing.T, database *sql.DB, action, recordID string, tr accounting.Transition) {
	t.Helper()
	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("ouverture de transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := accounting.RecordDocumentAction(context.Background(), tx, false,
		accounting.TableInvoices, "u1", action, recordID, "192.0.2.7", tr); err != nil {
		t.Fatalf("écriture du maillon: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// lireMaillon rend l'état antérieur et l'état suivant tels qu'ils sont STOCKÉS.
func lireMaillon(t *testing.T, database *sql.DB, recordID string) (avant, apres string) {
	t.Helper()
	err := database.QueryRow(`
		SELECT COALESCE(before_state, ''), COALESCE(after_state, '')
		  FROM audit_logs WHERE record_id = ? ORDER BY sequence_number DESC LIMIT 1`,
		recordID).Scan(&avant, &apres)
	if err != nil {
		t.Fatalf("lecture du maillon %s: %v", recordID, err)
	}
	return avant, apres
}

// LE test de conformité : une chaîne qui mélange créations et modifications,
// avec de vrais états antérieurs, doit se vérifier intégralement.
func TestLaChaineSeVerifieAvecUnEtatAnterieur(t *testing.T) {
	h, database := newAuditDB(t)

	// Une création : rien ne précède.
	ecrireMaillon(t, database, "document_created", "f1",
		accounting.Creation(map[string]any{"status": "draft", "total": 1000.0}))

	// Deux modifications successives, avec l'état qu'elles remplacent.
	ecrireMaillon(t, database, "document_transition", "f1",
		accounting.Modification(
			map[string]any{"status": "draft", "total": 1000.0},
			map[string]any{"status": "sent", "total": 1000.0}))

	ecrireMaillon(t, database, "document_transition", "f1",
		accounting.Modification(
			map[string]any{"status": "sent", "total": 1000.0},
			map[string]any{"status": "paid", "total": 1000.0}))

	// Une suppression : il ne reste que ce qui précédait.
	ecrireMaillon(t, database, "document_deleted", "f2",
		accounting.Suppression(map[string]any{"status": "draft", "total": 250.0}))

	code, rep := runChainVerify(t, h)
	if code != http.StatusOK || !rep.Verified {
		t.Fatalf("la chaîne est déclarée rompue alors que rien n'a été altéré : "+
			"code=%d verified=%v breaks=%+v", code, rep.Verified, rep.Breaks)
	}
	if rep.Entries != 4 {
		t.Fatalf("comptage = %d maillons, attendu 4", rep.Entries)
	}
}

// Une création écrit NULL, pas la chaîne vide : « rien ne précédait » et
// « l'état antérieur était vide » ne sont pas la même chose, et c'est ce qui
// rend les maillons antérieurs à la fonctionnalité recalculables à l'identique.
func TestUneCreationLaisseLEtatAnterieurNul(t *testing.T) {
	_, database := newAuditDB(t)
	ecrireMaillon(t, database, "document_created", "f1",
		accounting.Creation(map[string]any{"status": "draft"}))

	var brut sql.NullString
	if err := database.QueryRow(
		`SELECT before_state FROM audit_logs WHERE record_id = 'f1'`).Scan(&brut); err != nil {
		t.Fatalf("lecture: %v", err)
	}
	if brut.Valid {
		t.Errorf("before_state vaut %q sur une création, attendu NULL", brut.String)
	}
}

// Une modification écrit réellement ce qu'elle remplace.
func TestUneModificationEcritCeQuElleRemplace(t *testing.T) {
	_, database := newAuditDB(t)
	ecrireMaillon(t, database, "document_transition", "f1",
		accounting.Modification(
			map[string]any{"status": "draft"},
			map[string]any{"status": "sent"}))

	avant, apres := lireMaillon(t, database, "f1")
	if !strings.Contains(avant, `"status":"draft"`) {
		t.Errorf("l'état antérieur ne porte pas le statut remplacé : %s", avant)
	}
	if !strings.Contains(apres, `"status":"sent"`) {
		t.Errorf("l'état suivant ne porte pas le nouveau statut : %s", apres)
	}
	if !strings.Contains(apres, `"champs_modifies":["status"]`) {
		t.Errorf("le champ modifié n'est pas nommé : %s", apres)
	}
}

// Le cas qui motive la fonctionnalité : l'IBAN de l'entreprise.
//
// Les deux valeurs sont masquées (nLPD art. 6) — c'est le compte bancaire d'un
// indépendant. Ce qui doit subsister, c'est QUE l'IBAN a changé et qui l'a
// changé : de quoi déclencher une vérification, sans conserver aucun des deux
// numéros.
func TestUnChangementDIBANEstTraceSansConserverLesIBAN(t *testing.T) {
	_, database := newAuditDB(t)
	ecrireMaillon(t, database, "company_settings_updated", "company",
		accounting.Modification(
			map[string]any{"iban": "CH1100000000000000000", "currency": "CHF"},
			map[string]any{"iban": "CH9300762011623852957", "currency": "CHF"}))

	avant, apres := lireMaillon(t, database, "company")

	for _, secret := range []string{"CH1100000000000000000", "CH9300762011623852957"} {
		if strings.Contains(avant, secret) || strings.Contains(apres, secret) {
			t.Errorf("un IBAN subsiste en clair dans la piste :\n  avant %s\n  après %s", avant, apres)
		}
	}
	if !strings.Contains(apres, `"champs_modifies":["iban"]`) {
		t.Errorf("le changement d'IBAN n'est pas signalé : %s", apres)
	}
	// Et l'auteur du changement est bien celui qu'on croit.
	var auteur string
	if err := database.QueryRow(
		`SELECT user_id FROM audit_logs WHERE record_id = 'company'`).Scan(&auteur); err != nil {
		t.Fatalf("lecture de l'auteur: %v", err)
	}
	if auteur != "u1" {
		t.Errorf("auteur = %q, attendu u1", auteur)
	}
}

// Les maillons écrits AVANT la fonctionnalité — donc sans état antérieur — se
// vérifient toujours, et à côté des nouveaux.
//
// Sans cette garantie, livrer l'audit différentiel ferait basculer en « livres
// altérés » toutes les installations existantes au premier démarrage.
func TestLesAnciensMaillonsSeVerifientEncore(t *testing.T) {
	h, database := newAuditDB(t)

	// Trois maillons à l'ancienne : la chaîne telle qu'elle existait.
	seedChain(t, database, 3)
	// Puis un maillon neuf, qui porte un état antérieur.
	ecrireMaillon(t, database, "document_transition", "f9",
		accounting.Modification(
			map[string]any{"status": "draft"},
			map[string]any{"status": "sent"}))

	code, rep := runChainVerify(t, h)
	if code != http.StatusOK || !rep.Verified {
		t.Fatalf("mélange ancien/nouveau déclaré rompu : code=%d verified=%v breaks=%+v",
			code, rep.Verified, rep.Breaks)
	}
	if rep.Entries != 4 {
		t.Fatalf("comptage = %d maillons, attendu 4", rep.Entries)
	}
}

// Altérer l'état antérieur doit casser la chaîne.
//
// C'est la contrepartie indispensable : si `before_state` entrait dans la base
// sans entrer dans l'empreinte, on pourrait réécrire l'historique des
// modifications sans que rien ne le signale — une piste d'audit décorative.
func TestAltererLEtatAnterieurCasseLaChaine(t *testing.T) {
	h, database := newAuditDB(t)
	ecrireMaillon(t, database, "document_transition", "f1",
		accounting.Modification(
			map[string]any{"status": "draft"},
			map[string]any{"status": "sent"}))

	if code, rep := runChainVerify(t, h); code != http.StatusOK || !rep.Verified {
		t.Fatalf("la chaîne est rompue avant même l'altération : %+v", rep)
	}

	// Réécrire discrètement ce que la modification prétend avoir remplacé.
	if _, err := database.Exec(
		`UPDATE audit_logs SET before_state = ? WHERE record_id = 'f1'`,
		`{"status":"paid"}`); err != nil {
		t.Fatalf("altération: %v", err)
	}

	code, rep := runChainVerify(t, h)
	if rep.Verified {
		t.Fatalf("un état antérieur réécrit passe inaperçu — la piste ne prouve rien "+
			"(code=%d, %+v)", code, rep)
	}
}

// La liste des champs modifiés entre elle aussi dans l'empreinte : la réécrire
// pour masquer qu'un IBAN a bougé doit casser la chaîne.
func TestAltererLaListeDesChampsCasseLaChaine(t *testing.T) {
	h, database := newAuditDB(t)
	ecrireMaillon(t, database, "company_settings_updated", "company",
		accounting.Modification(
			map[string]any{"iban": "CH11", "currency": "CHF"},
			map[string]any{"iban": "CH93", "currency": "CHF"}))

	var apres string
	if err := database.QueryRow(
		`SELECT after_state FROM audit_logs WHERE record_id = 'company'`).Scan(&apres); err != nil {
		t.Fatalf("lecture: %v", err)
	}
	var etat map[string]any
	if err := json.Unmarshal([]byte(apres), &etat); err != nil {
		t.Fatalf("décodage: %v", err)
	}
	// Effacer la trace du changement d'IBAN.
	etat[accounting.CleChampsModifies] = []string{"currency"}
	falsifie, _ := json.Marshal(etat)
	if _, err := database.Exec(
		`UPDATE audit_logs SET after_state = ? WHERE record_id = 'company'`,
		string(falsifie)); err != nil {
		t.Fatalf("altération: %v", err)
	}

	if _, rep := runChainVerify(t, h); rep.Verified {
		t.Fatal("effacer « iban » de la liste des champs modifiés passe inaperçu")
	}
}

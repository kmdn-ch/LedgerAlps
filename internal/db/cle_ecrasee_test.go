package db

import (
	"bytes"
	"errors"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/core/secretstore"
)

// Audit croisé Gemini, CRIT-17 : `DatabaseKeys.Create` ÉCRASAIT une clé existante.
//
// La clé de la base est scellée sur le compte Windows ; la remplacer rend illisible la base
// qu'elle chiffrait, pour toujours — sans la phrase de récupération, il n'y a plus rien à
// faire. L'assistant d'installation, lancé par erreur sur une configuration illisible, le
// faisait en un clic.
func TestCreerUneCleNEcrasePasUneCleExistante(t *testing.T) {
	isoler(t)
	dir := t.TempDir()
	keys := NewDatabaseKeys(dir)

	original, err := keys.Create("colline-fromage-tunnel-95-Valais")
	if err != nil {
		t.Fatalf("première création : %v", err)
	}

	if _, err := keys.Create("autre-phrase-de-recuperation-77-Jura"); !errors.Is(err, ErrDatabaseKeyExists) {
		t.Fatalf("seconde création : %v, attendu ErrDatabaseKeyExists", err)
	}

	// Rien n'a bougé : la clé d'origine ouvre toujours, et la phrase d'origine la récupère.
	apres, err := keys.Key()
	if err != nil {
		t.Fatalf("la clé d'origine n'est plus lisible : %v", err)
	}
	if !bytes.Equal(original, apres) {
		t.Fatal("la clé a été remplacée malgré le refus")
	}
	if recuperee, err := keys.Recover("colline-fromage-tunnel-95-Valais"); err != nil || !bytes.Equal(recuperee, original) {
		t.Fatalf("la phrase de récupération d'origine ne rend plus la clé d'origine : %v", err)
	}
}

// Même constat quand seule l'enveloppe de récupération subsiste (le coffre du compte est vide :
// autre session, autre profil) : créer une clé par-dessus détruirait la seule voie de retour.
func TestCreerUneCleNEcrasePasLEnveloppeDeRecuperation(t *testing.T) {
	isoler(t)
	dir := t.TempDir()
	keys := NewDatabaseKeys(dir)
	if _, err := keys.Create("colline-fromage-tunnel-95-Valais"); err != nil {
		t.Fatal(err)
	}
	// Le coffre du compte est vidé ; le fichier de récupération reste.
	if err := keys.store.Delete(secretstore.NameDatabaseKey); err != nil && keys.Configured() {
		t.Skipf("impossible de vider le coffre dans ce contexte : %v", err)
	}
	if keys.Configured() {
		t.Skip("le coffre n'a pas pu être vidé : cas non testable ici")
	}
	if _, err := keys.Create("autre-phrase-de-recuperation-77-Jura"); !errors.Is(err, ErrDatabaseKeyExists) {
		t.Fatalf("création avec une enveloppe de récupération présente : %v, attendu ErrDatabaseKeyExists", err)
	}
}

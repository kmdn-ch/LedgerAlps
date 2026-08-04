package secretstore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetGetRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	secret := []byte("phrase de passe de sauvegarde — accentuée")

	if err := s.Set(NameBackupPassphrase, secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(NameBackupPassphrase)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("relu %q, attendu %q", got, secret)
	}
}

func TestGetAbsent(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Get(NameDatabaseKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("erreur = %v, attendu ErrNotFound", err)
	}
	if s.Has(NameDatabaseKey) {
		t.Fatal("Has annonce un secret qui n'existe pas")
	}
}

// Le secret ne doit apparaître nulle part en clair dans le fichier. C'est tout
// l'objet du paquet ; le vérifier sur le fichier réel, pas sur l'intention.
func TestSecretPasEnClairDansLeFichierSousWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("hors Windows, le secret est délibérément en base64 dans un fichier 0600")
	}
	dir := t.TempDir()
	s := New(dir)
	secret := []byte("MotDePasseTresReconnaissable42")
	if err := s.Set(NameBackupPassphrase, secret); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, secret) {
		t.Fatal("le secret est en clair dans secrets.json")
	}
	// Et il ne doit pas non plus être simplement encodé en base64.
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Entries[NameBackupPassphrase].Seal != sealDPAPI {
		t.Fatalf("scellement = %q, attendu %q sous Windows",
			f.Entries[NameBackupPassphrase].Seal, sealDPAPI)
	}
}

// Un secret scellé sur un autre compte doit produire une erreur explicite, et
// non un secret silencieusement vide : la suite du code en dépend pour savoir
// s'il faut demander la phrase de récupération.
func TestSecretDUnAutreCompteEchoueExplicitement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI seulement")
	}
	dir := t.TempDir()
	s := New(dir)
	if err := s.Set(NameDatabaseKey, []byte("clef")); err != nil {
		t.Fatal(err)
	}
	// Abîmer le blob scellé simule un scellement fait ailleurs : DPAPI refuse
	// dans les deux cas.
	//
	// Il faut toucher la charge utile, pas l'en-tête : les premiers octets d'un
	// blob DPAPI sont une constante identique pour tout le monde, et les
	// remplacer par eux-mêmes ne corrompt rien — première version de ce test,
	// qui passait donc pour la mauvaise raison.
	p := filepath.Join(dir, "secrets.json")
	raw, _ := os.ReadFile(p)
	var f file
	json.Unmarshal(raw, &f)
	e := f.Entries[NameDatabaseKey]
	blob, err := base64.StdEncoding.DecodeString(e.Data)
	if err != nil {
		t.Fatal(err)
	}
	blob[len(blob)/2] ^= 0xFF
	e.Data = base64.StdEncoding.EncodeToString(blob)
	f.Entries[NameDatabaseKey] = e
	out, _ := json.Marshal(f)
	os.WriteFile(p, out, 0o600)

	got, err := s.Get(NameDatabaseKey)
	if err == nil {
		t.Fatalf("secret descellé alors qu'il ne devait pas l'être: %q", got)
	}
	if got != nil {
		t.Fatal("un secret est renvoyé en même temps que l'erreur")
	}
}

func TestDelete(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set(NameBackupPassphrase, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(NameBackupPassphrase); err != nil {
		t.Fatal(err)
	}
	if s.Has(NameBackupPassphrase) {
		t.Fatal("secret encore présent après suppression")
	}
	// Supprimer deux fois n'est pas une erreur.
	if err := s.Delete(NameBackupPassphrase); err != nil {
		t.Fatalf("seconde suppression: %v", err)
	}
}

func TestPlusieursSecretsCoexistent(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set(NameBackupPassphrase, []byte("phrase")); err != nil {
		t.Fatal(err)
	}
	key, err := NewKey(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(NameDatabaseKey, key); err != nil {
		t.Fatal(err)
	}
	back, err := s.Get(NameBackupPassphrase)
	if err != nil || string(back) != "phrase" {
		t.Fatalf("phrase perdue en écrivant la clé: %q %v", back, err)
	}
	got, err := s.Get(NameDatabaseKey)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("clé relue %x, attendu %x (%v)", got, key, err)
	}
}

func TestFichierEn0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("les droits POSIX ne s'appliquent pas")
	}
	dir := t.TempDir()
	s := New(dir)
	if err := s.Set(NameBackupPassphrase, []byte("x")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("droits %v, attendu 0600 — c'est toute la protection ici", fi.Mode().Perm())
	}
}

func TestSecretVideRefuse(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set(NameBackupPassphrase, nil); err == nil {
		t.Fatal("un secret vide a été accepté")
	}
}

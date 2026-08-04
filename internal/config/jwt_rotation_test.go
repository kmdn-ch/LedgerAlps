package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// La rotation existait derrière un bouton, donc elle n'arrivait jamais. Ces
// tests portent sur ce qui décide à sa place.

func withConfigFile(t *testing.T, contents map[string]any) *Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	if contents != nil {
		if err := os.MkdirAll(AppDataDir(), 0o700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(contents)
		if err := os.WriteFile(ConfigFilePath(), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &Config{}
}

func readConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Une clé sans horodatage vient d'une installation antérieure à la rotation
// automatique : on ignore son âge, et sur une installation qui date la réponse
// est « longtemps ». Elle doit donc tourner.
func TestUneCleSansHorodatageTourne(t *testing.T) {
	cfg := withConfigFile(t, map[string]any{"jwt_secret": "ancienne-cle", "port": "8000"})
	cfg.JWTSecret = "ancienne-cle"
	cfg.JWTSecretMaxAgeDays = 1

	rotated, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatalf("MaybeRotateJWTSecret: %v", err)
	}
	if !rotated {
		t.Fatal("une clé d'âge inconnu n'a pas tourné")
	}
	if cfg.JWTSecret == "ancienne-cle" {
		t.Fatal("cfg porte encore l'ancienne clé : le serveur qui démarre l'utiliserait")
	}
	if len(cfg.JWTSecret) != 64 {
		t.Fatalf("clé de %d caractères, attendu 64 (32 octets en hexadécimal)", len(cfg.JWTSecret))
	}
	m := readConfig(t)
	if m["jwt_secret"] != cfg.JWTSecret {
		t.Fatal("le fichier ne porte pas la clé rendue dans cfg")
	}
	if m["jwt_secret_rotated_at"] == nil {
		t.Fatal("aucune date de rotation écrite : la clé retournerait au prochain démarrage")
	}
}

func TestUneCleRecenteNeTournePas(t *testing.T) {
	cfg := withConfigFile(t, map[string]any{"jwt_secret": "cle-recente", "port": "8000"})
	cfg.JWTSecret = "cle-recente"
	cfg.JWTSecretMaxAgeDays = 1
	cfg.JWTSecretRotatedAt = time.Now().Add(-2 * time.Hour)

	rotated, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Fatal("une clé de deux heures a tourné alors que la périodicité est d'un jour")
	}
	if cfg.JWTSecret != "cle-recente" {
		t.Fatal("la clé a changé sans rotation annoncée")
	}
}

func TestUneCleTropVieilleTourne(t *testing.T) {
	cfg := withConfigFile(t, map[string]any{"jwt_secret": "vieille", "port": "8000"})
	cfg.JWTSecret = "vieille"
	cfg.JWTSecretMaxAgeDays = 1
	cfg.JWTSecretRotatedAt = time.Now().Add(-25 * time.Hour)

	rotated, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Fatal("une clé de 25 heures n'a pas tourné")
	}
}

// Zéro veut dire « désactivé », et doit être respecté — pas relu comme
// « absent, donc valeur par défaut ».
func TestPeriodiciteZeroDesactiveLaRotation(t *testing.T) {
	cfg := withConfigFile(t, map[string]any{"jwt_secret": "immuable", "port": "8000"})
	cfg.JWTSecret = "immuable"
	cfg.JWTSecretMaxAgeDays = 0

	rotated, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Fatal("rotation effectuée alors qu'elle est désactivée")
	}
}

// Le piège du pointeur : « 0 » enregistré doit se relire « 0 », et non
// retomber sur la valeur par défaut au démarrage suivant — ce qui
// réactiverait en silence ce que l'utilisateur vient de couper.
func TestZeroEnregistreSeRelitZero(t *testing.T) {
	withConfigFile(t, map[string]any{"jwt_secret": "x", "port": "8000", "sqlite_path": "x.db"})
	if err := SetJWTSecretMaxAge(0); err != nil {
		t.Fatal(err)
	}
	fc, err := loadFromFile(ConfigFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if fc.JWTSecretMaxAgeDays == nil {
		t.Fatal("la clé n'a pas été écrite")
	}
	if *fc.JWTSecretMaxAgeDays != 0 {
		t.Fatalf("relu %d, attendu 0", *fc.JWTSecretMaxAgeDays)
	}
}

// Une rotation manuelle doit horodater elle aussi : sans cela, elle serait
// suivie d'une rotation automatique au démarrage suivant, la clé passant pour
// « jamais tournée » — deux déconnexions pour un seul geste.
func TestLaRotationManuelleHorodateAussi(t *testing.T) {
	withConfigFile(t, map[string]any{"jwt_secret": "avant", "port": "8000"})
	if _, err := RotateJWTSecret(); err != nil {
		t.Fatal(err)
	}
	m := readConfig(t)
	stamp, ok := m["jwt_secret_rotated_at"].(string)
	if !ok || stamp == "" {
		t.Fatal("rotation manuelle sans horodatage")
	}
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		t.Fatalf("horodatage illisible %q: %v", stamp, err)
	}
}

// La rotation ne doit pas emporter les autres clés du fichier — jwt_secret
// compris pour les réglages voisins. Le fichier est relu-modifié-réécrit sur
// une map générique précisément pour ça.
func TestLaRotationPreserveLesAutresReglages(t *testing.T) {
	withConfigFile(t, map[string]any{
		"jwt_secret": "avant", "port": "8123", "sqlite_path": "C:/data/l.db",
		"tls_cert": "cert.pem", "un_reglage_inconnu": "à préserver",
	})
	if _, err := RotateJWTSecret(); err != nil {
		t.Fatal(err)
	}
	m := readConfig(t)
	for k, want := range map[string]any{
		"port": "8123", "sqlite_path": "C:/data/l.db",
		"tls_cert": "cert.pem", "un_reglage_inconnu": "à préserver",
	} {
		if m[k] != want {
			t.Errorf("%s = %v, attendu %v", k, m[k], want)
		}
	}
}

// Deux rotations de suite ne doivent pas rendre la même clé.
func TestChaqueRotationDonneUneCleDifferente(t *testing.T) {
	withConfigFile(t, map[string]any{"jwt_secret": "x", "port": "8000"})
	if _, err := RotateJWTSecret(); err != nil {
		t.Fatal(err)
	}
	first := readConfig(t)["jwt_secret"]
	if _, err := RotateJWTSecret(); err != nil {
		t.Fatal(err)
	}
	if readConfig(t)["jwt_secret"] == first {
		t.Fatal("deux rotations ont produit la même clé")
	}
}

func TestPeriodiciteHorsBornesRefusee(t *testing.T) {
	withConfigFile(t, map[string]any{"jwt_secret": "x", "port": "8000"})
	if err := SetJWTSecretMaxAge(-1); err == nil {
		t.Error("une périodicité négative a été acceptée")
	}
	if err := SetJWTSecretMaxAge(400); err == nil {
		t.Error("une périodicité de 400 jours a été acceptée")
	}
}

// L'état montré à l'interface doit être cohérent : pas de prochaine échéance
// annoncée quand la rotation est désactivée.
func TestLEtatNAnnoncePasDEcheanceQuandCestDesactive(t *testing.T) {
	cfg := &Config{JWTSecretMaxAgeDays: 0, JWTSecretRotatedAt: time.Now()}
	if st := RotationStatus(cfg); st.NextAt != nil {
		t.Fatalf("échéance annoncée (%v) alors que la rotation est désactivée", st.NextAt)
	}
	cfg = &Config{JWTSecretMaxAgeDays: 1, JWTSecretRotatedAt: time.Now()}
	st := RotationStatus(cfg)
	if st.NextAt == nil {
		t.Fatal("aucune échéance annoncée alors que la rotation est active")
	}
	if delta := st.NextAt.Sub(*st.RotatedAt); delta != 24*time.Hour {
		t.Fatalf("échéance à +%v, attendu +24h", delta)
	}
}

func TestLeCheminDuFichierEstBienIsole(t *testing.T) {
	withConfigFile(t, map[string]any{"jwt_secret": "x", "port": "8000"})
	if got := ConfigFilePath(); filepath.Base(got) != "config.json" {
		t.Fatalf("chemin inattendu: %s", got)
	}
}

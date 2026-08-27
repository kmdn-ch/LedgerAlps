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
	cfg.JWTSecretRotatedAt = time.Now().Add(-25 * time.Hour)

	rotated, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Fatal("une clé de 25 heures n'a pas tourné")
	}
}

// Une installation où la périodicité avait été coupée — `jwt_secret_max_age_days`
// à zéro — ne doit plus rien couper du tout. C'est le cas qui compte de cette
// suppression : le réglage a existé, il est resté dans des fichiers, et s'il
// continuait d'être lu, la rotation quotidienne annoncée à l'écran serait un
// mensonge sur exactement les installations qu'elle vient corriger.
func TestUnAncienReglageNeDesactivePlusLaRotation(t *testing.T) {
	cfg := withConfigFile(t, map[string]any{
		"jwt_secret": "immuable", "port": "8000", "jwt_secret_max_age_days": 0,
	})
	cfg.JWTSecret = "immuable"

	rotated, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Fatal("un ancien « jamais » enregistré empêche encore la rotation")
	}

	// Et la clé morte quitte le fichier : la laisser ferait croire à qui
	// l'ouvre que la valeur qu'il y lit s'applique encore.
	if _, present := readConfig(t)["jwt_secret_max_age_days"]; present {
		t.Fatal("jwt_secret_max_age_days survit dans le fichier alors que plus rien ne le lit")
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

// L'état montré à l'interface doit être cohérent : une échéance annoncée dès
// qu'une date de rotation existe, et aucune tant qu'il n'y en a pas — une
// installation antérieure à la rotation automatique, où annoncer « prochaine
// le … » à partir de rien inventerait une date.
func TestLEtatAnnonceLEcheanceADemain(t *testing.T) {
	st := RotationStatus(&Config{JWTSecretRotatedAt: time.Now()})
	if st.NextAt == nil {
		t.Fatal("aucune échéance annoncée alors que la clé a une date de rotation")
	}
	if delta := st.NextAt.Sub(*st.RotatedAt); delta != 24*time.Hour {
		t.Fatalf("échéance à +%v, attendu +24h", delta)
	}

	vide := RotationStatus(&Config{})
	if vide.RotatedAt != nil || vide.NextAt != nil {
		t.Fatalf("dates inventées pour une clé jamais tournée: %+v", vide)
	}
}

func TestLeCheminDuFichierEstBienIsole(t *testing.T) {
	withConfigFile(t, map[string]any{"jwt_secret": "x", "port": "8000"})
	if got := ConfigFilePath(); filepath.Base(got) != "config.json" {
		t.Fatalf("chemin inattendu: %s", got)
	}
}

// ─── Rotation et JWT_SECRET d'environnement (audit 4, É-2) ───────────────────
//
// Deux des trois chemins d'installation livrés posent JWT_SECRET dans
// l'environnement du service : Linux/systemd via `EnvironmentFile=`, et
// Windows Service via la valeur REG_MULTI_SZ `Environment` posée par
// `scripts/install.ps1`. Sur ces deux chemins, `applyEnvOverrides` réimpose la
// valeur statique par-dessus toute clé tournée relue du fichier — et cette
// préséance est VOULUE, documentée dans config.go après un incident réel
// (SQLITE_PATH d'une restauration écrasé par le fichier).
//
// La rotation écrivait donc un secret dans config.json, faisait avancer
// `jwt_secret_rotated_at`, et l'écran affichait une date crédible — pendant que
// la clé RÉELLEMENT utilisée pour signer restait figée depuis l'installation.
// Une promesse que le mode de déploiement empêche de tenir est pire qu'une
// promesse absente : elle décourage de chercher ailleurs.

// TestLaRotationNeMentPasQuandLeSecretVientDeLEnvironnement reproduit le cycle
// complet de DEUX démarrages, qui est le seul endroit où le défaut se voit.
func TestLaRotationNeMentPasQuandLeSecretVientDeLEnvironnement(t *testing.T) {
	const secretStatique = "secret-statique-de-l-environnement-assez-long-pour-passer"
	writeConfigFile(t, map[string]any{
		"jwt_secret": "une-cle-de-fichier-suffisamment-longue-pour-la-validation",
	})
	t.Setenv("JWT_SECRET", secretStatique)

	// Premier démarrage : la clé n'a jamais tourné.
	cfg := Load()
	if cfg.JWTSecret != secretStatique {
		t.Fatalf("préséance changée : la clé devrait venir de l'environnement, pas du fichier")
	}
	tourne, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if tourne {
		t.Error("la rotation s'est déclarée effectuée alors que l'environnement " +
			"réimposera la clé statique au prochain démarrage")
	}
	if cfg.JWTSecret != secretStatique {
		t.Error("la clé en mémoire a été remplacée par une clé qui ne survivra pas au redémarrage")
	}

	// Second démarrage : c'est ici que le mensonge se voyait.
	cfg2 := Load()
	if cfg2.JWTSecret != secretStatique {
		t.Errorf("clé au second démarrage = %q, attendu la valeur statique — "+
			"c'est bien elle qui signe, quoi qu'affiche l'écran", cfg2.JWTSecret)
	}
	st := RotationStatus(cfg2)
	if st.RotatedAt != nil || st.NextAt != nil {
		t.Errorf("l'écran annonce encore une rotation (rotated=%v next=%v) alors "+
			"qu'aucune ne peut avoir lieu sur ce déploiement", st.RotatedAt, st.NextAt)
	}
	if !st.BloqueeParEnvironnement {
		t.Error("l'état ne dit pas que l'environnement bloque la rotation : " +
			"l'interface ne peut donc pas l'expliquer")
	}
}

// Le chemin NSIS/lanceur (config.json pur, aucune variable d'environnement)
// doit continuer de tourner normalement — c'est le seul des trois qui tenait
// réellement la promesse, et il ne doit pas être puni par le correctif.
func TestLaRotationFonctionneToujoursSansVariableDEnvironnement(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret": "une-cle-de-fichier-suffisamment-longue-pour-la-validation",
	})
	os.Unsetenv("JWT_SECRET")

	cfg := Load()
	ancienne := cfg.JWTSecret
	tourne, err := MaybeRotateJWTSecret(cfg, time.Now())
	if err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if !tourne {
		t.Fatal("la rotation aurait dû avoir lieu : aucune variable d'environnement ne la bloque")
	}
	if cfg.JWTSecret == ancienne {
		t.Error("la clé n'a pas changé")
	}
	st := RotationStatus(cfg)
	if st.BloqueeParEnvironnement {
		t.Error("l'état signale un blocage alors qu'aucune variable n'est posée")
	}
	if st.RotatedAt == nil || st.NextAt == nil {
		t.Error("après une vraie rotation, l'écran doit pouvoir afficher les deux dates")
	}

	// Et la nouvelle clé doit survivre au redémarrage.
	cfg2 := Load()
	if cfg2.JWTSecret != cfg.JWTSecret {
		t.Error("la clé tournée n'a pas été relue au démarrage suivant")
	}
}

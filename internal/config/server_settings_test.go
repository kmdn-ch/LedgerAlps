package config

import (
	"encoding/json"
	"os"
	"testing"
)

// Le défaut que ceci garde : config.json est écrit une fois par l'assistant puis réécrit par
// l'application (la rotation de la clé de signature). Une installation mise à jour porte les clés
// de sa naissance — et parfois celles d'une version plus ancienne ou plus récente. Réécrire le
// fichier doit ajouter ce qui change sans perdre le reste.
//
// Perdre jwt_secret verrouillerait chacun hors de sa comptabilité, ce pour quoi l'écriture passe
// par une carte générique et non par une structure.
func TestLaRotationPreserveLesCleInconnues(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":  "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path": "ledgeralps.db",
		"port":        "8000",
		"debug":       false,
		// Une clé d'une version que ce binaire ne connaît pas.
		"une_option_future": "à préserver",
		// Un réglage réseau d'avant la 1.6.6 : il n'est plus lu, il n'est pas pour autant effacé.
		"host": "0.0.0.0",
	})

	if _, err := RotateJWTSecret(); err != nil {
		t.Fatalf("RotateJWTSecret: %v", err)
	}

	raw, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("le fichier écrit n'est pas du JSON valide: %v", err)
	}

	if got["sqlite_path"] != "ledgeralps.db" {
		t.Error("sqlite_path a été perdu — l'application ouvrirait une autre base")
	}
	if got["une_option_future"] != "à préserver" {
		t.Error("une clé inconnue a été supprimée ; ce sont les données d'une autre version, pas du bruit")
	}
	if got["host"] != "0.0.0.0" {
		t.Error("une clé réseau d'une version antérieure a été réécrite ou perdue")
	}
	if got["jwt_secret"] == "un-secret-suffisamment-long-pour-passer-la-validation" {
		t.Error("la clé de signature n'a pas changé")
	}
	if got["jwt_secret_rotated_at"] == nil {
		t.Error("la date de rotation n'a pas été écrite")
	}
}

// Un config.json illisible doit interrompre l'écriture au lieu d'être écrasé : le fichier porte
// encore le secret de signature et le chemin de la base.
func TestCorruptConfigIsNotOverwritten(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret": "un-secret-suffisamment-long-pour-passer-la-validation",
	})
	if err := os.WriteFile(ConfigFilePath(), []byte("{ ceci n'est pas du JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := RotateJWTSecret(); err == nil {
		t.Fatal("la rotation a réussi sur un fichier illisible")
	}
	raw, _ := os.ReadFile(ConfigFilePath())
	if string(raw) != "{ ceci n'est pas du JSON" {
		t.Error("le fichier illisible a été écrasé ; il pouvait encore être réparé à la main")
	}
}

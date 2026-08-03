package config

// Writing the network settings back to config.json.
//
// config.json is created by the setup wizard on first run and never touched
// again. An upgrade therefore never adds new keys: an installation from v1.1
// still carries the five keys it was born with. Anything added since — host,
// TLS, the insecure opt-in — is simply absent, reads as its zero value, and no
// amount of editing the environment helps on Windows because the launcher
// starts the server itself.
//
// So the settings have to be writable from the application. Hand-editing JSON
// in %APPDATA% is not an answer for accounting software.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerSettings is the network-facing subset a user can change at runtime.
// Everything here takes effect at the next start, which is why saving one
// returns a restart request rather than pretending it applied.
type ServerSettings struct {
	Host              string `json:"host"`
	TLSCert           string `json:"tls_cert"`
	TLSKey            string `json:"tls_key"`
	AllowInsecureHTTP bool   `json:"allow_insecure_http"`
}

// CurrentServerSettings reports what is in effect now.
func CurrentServerSettings(cfg *Config) ServerSettings {
	return ServerSettings{
		Host:              cfg.Host,
		TLSCert:           cfg.TLSCert,
		TLSKey:            cfg.TLSKey,
		AllowInsecureHTTP: cfg.AllowInsecureHTTP,
	}
}

// RotateJWTSecret remplace le secret de signature par un nouveau tirage
// aléatoire de 32 octets et retourne sa longueur en caractères hexadécimaux.
//
// Le secret ne sert qu'à signer et relire les jetons d'accès et de
// rafraîchissement — vérifié dans le code, pas supposé. Le régénérer déconnecte
// donc toutes les sessions ouvertes, et rien d'autre : les mots de passe sont
// hachés avec bcrypt et ne dépendent pas de lui, aucune donnée comptable n'est
// touchée, et les sauvegardes restent utilisables — elles ne contiennent pas
// `config.json`, et leur chiffrement dérive d'une phrase de passe indépendante.
//
// À utiliser en cas de suspicion de fuite du fichier de configuration : joint à
// un ticket de support, copié sur une clé, poussé par erreur dans un dépôt. Qui
// détient ce secret forge un jeton valide pour n'importe quel compte,
// administrateur compris, sans connaître aucun mot de passe.
//
// Ne remplace pas le chiffrement du disque : qui lit `config.json` lit aussi
// `ledgeralps.db`, posé dans le même dossier — il n'a alors nul besoin de
// forger quoi que ce soit.
//
// L'écriture réutilise la lecture-modification-écriture sur une map générique :
// sérialiser une structure supprimerait les clés inconnues du fichier.
func RotateJWTSecret() (int, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return 0, fmt.Errorf("génération du secret: %w", err)
	}
	secret := hex.EncodeToString(b)

	if err := updateConfigFile(func(existing map[string]any) {
		existing["jwt_secret"] = secret
	}); err != nil {
		return 0, err
	}
	return len(secret), nil
}

// SaveServerSettings fusionne s dans config.json en préservant toutes les
// autres clés. Les réglages prennent effet au prochain démarrage, ce pour quoi
// l'enregistrement demande un redémarrage au lieu de prétendre avoir appliqué.
func SaveServerSettings(s ServerSettings) error {
	return updateConfigFile(func(existing map[string]any) {
		existing["host"] = s.Host
		// force_tls a existé le temps d'une pré-version puis a été retiré : servir
		// en TLS sur localhost n'apportait rien qu'un avertissement de certificat
		// répété. La clé est supprimée plutôt qu'ignorée, pour qu'un fichier de
		// cette période ne conserve pas un réglage sans effet.
		delete(existing, "force_tls")
		existing["allow_insecure_http"] = s.AllowInsecureHTTP
		// Empty means "not configured": storing "" would be indistinguishable from
		// a path the user cleared on purpose, and both should read as absent.
		setOrDelete(existing, "tls_cert", s.TLSCert)
		setOrDelete(existing, "tls_key", s.TLSKey)
	})
}

// updateConfigFile applique `mutate` au contenu de config.json et le réécrit.
//
// La lecture-modification-écriture porte sur une map générique et non sur une
// structure : sérialiser une structure supprimerait toute clé qu'elle ne
// connaît pas — `jwt_secret` au premier chef, ce qui déconnecterait tout le
// monde et rendrait les sauvegardes inaccessibles jusqu'à réinstallation.
func updateConfigFile(mutate func(existing map[string]any)) error {
	path := ConfigFilePath()

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("le fichier de configuration est illisible, rien n'a été modifié: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lecture de la configuration: %w", err)
	}

	mutate(existing)

	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("encodage de la configuration: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("création du dossier de configuration: %w", err)
	}

	// Écriture dans un fichier voisin puis renommage : une coupure en cours
	// d'écriture laisserait sinon un config.json tronqué, et l'application ne
	// redémarrerait plus. Le renommage dans le même dossier est atomique sur
	// Windows comme sur POSIX.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("écriture de la configuration: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remplacement de la configuration: %w", err)
	}
	return nil
}

func setOrDelete(m map[string]any, key, value string) {
	if value == "" {
		delete(m, key)
		return
	}
	m[key] = value
}

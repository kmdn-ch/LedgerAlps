package config

// Écrire dans config.json.
//
// config.json est créé par l'assistant au premier lancement. Ce fichier porte ce que l'application
// y réécrit ensuite : la rotation de la clé de signature.
//
// Les réglages d'accès réseau (adresse d'écoute, TLS) n'existent plus : LedgerAlps n'écoute que sur
// la boucle locale (voir reseau.go).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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
	secret, err := newSecret()
	if err != nil {
		return 0, err
	}
	// L'horodatage part avec la clé : sans lui, une rotation manuelle serait
	// suivie d'une rotation automatique au démarrage suivant, la clé passant
	// pour « jamais tournée ».
	if err := updateConfigFile(func(existing map[string]any) {
		existing["jwt_secret"] = secret
		existing["jwt_secret_rotated_at"] = time.Now().UTC().Format(time.RFC3339)
	}); err != nil {
		return 0, err
	}
	return len(secret), nil
}

// newSecret tire 32 octets et les rend en hexadécimal.
func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("génération du secret: %w", err)
	}
	return hex.EncodeToString(b), nil
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

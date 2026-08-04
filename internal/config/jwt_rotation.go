package config

// Rotation automatique de la clé de signature.
//
// La rotation existait, derrière un bouton. Un bouton de sécurité qu'il faut
// penser à cliquer n'est pas une mesure de sécurité : c'est une intention. La
// clé tournait donc, dans les faits, jamais.
//
// # Ce que la rotation protège, et ce qu'elle ne protège pas
//
// La clé ne sert qu'à signer les jetons de session. Qui la détient forge un
// jeton valide pour n'importe quel compte, administrateur compris, sans
// connaître aucun mot de passe. La faire tourner borne la durée de vie d'une
// fuite : un `config.json` joint à un ticket de support il y a une semaine ne
// vaut plus rien aujourd'hui.
//
// Elle ne protège pas contre quelqu'un qui a accès au fichier maintenant — il
// lira simplement la nouvelle clé. Et qui lit `config.json` lit aussi
// `ledgeralps.db`, posé dans le même dossier : il n'a alors nul besoin de forger
// quoi que ce soit. La rotation vise la fuite *passée*, pas l'accès *présent*.
//
// # Pourquoi au démarrage et jamais en cours de session
//
// Régénérer la clé invalide toutes les sessions. Le faire sur une minuterie
// pendant que quelqu'un saisit une facture le déconnecterait au milieu — et
// LedgerAlps n'enregistre pas de brouillon automatique : la saisie serait
// perdue. Une mesure de sécurité qui fait perdre du travail est une mesure
// qu'on finit par désactiver.
//
// Au démarrage, la seule conséquence est une reconnexion, au moment où
// l'utilisateur ouvre l'application de toute façon.

import (
	"fmt"
	"time"
)

// DefaultJWTSecretMaxAgeDays est l'âge au-delà duquel la clé est régénérée au
// démarrage suivant. Un jour : le coût est une reconnexion quotidienne, sur une
// application qu'on ouvre le matin et ferme le soir.
const DefaultJWTSecretMaxAgeDays = 1

// JWTRotationStatus décrit l'état de la rotation, pour que l'interface le
// montre au lieu de laisser deviner.
type JWTRotationStatus struct {
	// MaxAgeDays vaut 0 quand la rotation automatique est désactivée.
	MaxAgeDays int        `json:"max_age_days"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	// NextAt est nul quand la rotation automatique est désactivée.
	NextAt *time.Time `json:"next_at,omitempty"`
}

// RotationStatus reports the current rotation state.
func RotationStatus(cfg *Config) JWTRotationStatus {
	st := JWTRotationStatus{MaxAgeDays: cfg.JWTSecretMaxAgeDays}
	if !cfg.JWTSecretRotatedAt.IsZero() {
		t := cfg.JWTSecretRotatedAt
		st.RotatedAt = &t
		if cfg.JWTSecretMaxAgeDays > 0 {
			n := t.Add(time.Duration(cfg.JWTSecretMaxAgeDays) * 24 * time.Hour)
			st.NextAt = &n
		}
	}
	return st
}

// MaybeRotateJWTSecret régénère la clé si elle a dépassé son âge, et met cfg à
// jour pour que le serveur qui démarre utilise la nouvelle.
//
// Une horodatation absente — toute installation antérieure à cette version —
// compte comme « jamais tournée » et déclenche une rotation. C'est le
// comportement voulu : on ne sait pas depuis quand cette clé existe, et sur une
// installation qui date, la réponse est « longtemps ».
//
// Un échec n'est pas fatal et le dit : refuser de démarrer parce qu'on n'a pas
// pu écrire un fichier de configuration laisserait l'utilisateur sans ses
// livres, ce qui est pire que de garder la clé un jour de plus.
func MaybeRotateJWTSecret(cfg *Config, now time.Time) (bool, error) {
	if cfg.JWTSecretMaxAgeDays <= 0 {
		return false, nil // rotation automatique désactivée
	}
	maxAge := time.Duration(cfg.JWTSecretMaxAgeDays) * 24 * time.Hour
	if !cfg.JWTSecretRotatedAt.IsZero() && now.Sub(cfg.JWTSecretRotatedAt) < maxAge {
		return false, nil
	}

	// Une horodatation dans le futur signale une horloge reculée ou un fichier
	// bricolé. Tourner quand même : le cas contraire — ne jamais tourner parce
	// qu'une date aberrante est toujours « récente » — est le plus dangereux.
	secret, err := newSecret()
	if err != nil {
		return false, err
	}
	if err := updateConfigFile(func(existing map[string]any) {
		existing["jwt_secret"] = secret
		existing["jwt_secret_rotated_at"] = now.UTC().Format(time.RFC3339)
	}); err != nil {
		return false, err
	}
	cfg.JWTSecret = secret
	cfg.JWTSecretRotatedAt = now.UTC()
	return true, nil
}

// SetJWTSecretMaxAge enregistre la périodicité de la rotation. Zéro la
// désactive.
func SetJWTSecretMaxAge(days int) error {
	if days < 0 || days > 365 {
		return fmt.Errorf("périodicité hors bornes: %d jours (0 pour désactiver, 365 au plus)", days)
	}
	return updateConfigFile(func(existing map[string]any) {
		existing["jwt_secret_max_age_days"] = days
	})
}

// DefaultIdleLogoutMinutes déconnecte après ce délai sans activité.
//
// Dix minutes, pas cinq : LedgerAlps n'enregistre aucun brouillon automatique,
// et lire une facture fournisseur de deux pages avant de la saisir prend plus
// de cinq minutes sans qu'une touche soit frappée. Un délai qui coupe pendant
// la lecture est un délai qu'on désactive.
const DefaultIdleLogoutMinutes = 10

// SetIdleLogoutMinutes enregistre le délai d'inactivité. Zéro le désactive.
//
// La borne basse n'est pas cosmétique. En dessous de deux minutes, la
// déconnexion tombe pendant qu'on lit une facture fournisseur avant de la
// saisir — et comme aucun brouillon n'est enregistré, la saisie est perdue. Un
// réglage qui fait perdre du travail finit désactivé, ce qui laisse la session
// ouverte indéfiniment : le durcir trop revient à ne rien durcir.
func SetIdleLogoutMinutes(minutes int) error {
	if minutes < 0 || minutes > 24*60 {
		return fmt.Errorf("délai hors bornes: %d minutes (0 pour désactiver, 1440 au plus)", minutes)
	}
	if minutes > 0 && minutes < 2 {
		return fmt.Errorf("délai trop court: %d minute — en dessous de deux minutes, "+
			"la déconnexion tombe pendant la lecture d'un document et la saisie en cours est perdue", minutes)
	}
	return updateConfigFile(func(existing map[string]any) {
		existing["idle_logout_minutes"] = minutes
	})
}

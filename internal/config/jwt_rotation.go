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
//
// # Pourquoi la périodicité n'est plus un réglage
//
// Elle l'a été : jamais / chaque jour / chaque semaine / chaque mois. Trois de
// ces quatre choix n'existaient que pour affaiblir le quatrième, et « jamais »
// rendait à l'identique la situation que la rotation automatique venait de
// corriger — une clé qui ne tourne pas, derrière une case à cocher cette fois.
//
// Un réglage dont toutes les valeurs sauf une sont pires que le défaut n'est
// pas un réglage, c'est un piège. La périodicité est donc une constante, et
// l'écran ne propose plus qu'une seule commande : la régénération immédiate,
// pour le cas que la périodicité ne couvre pas — on vient de s'apercevoir
// d'une fuite et attendre demain serait trop long.

import (
	"fmt"
	"time"
)

// JWTSecretRotationDays est l'âge au-delà duquel la clé est régénérée au
// démarrage suivant. Un jour : le coût est une reconnexion quotidienne, sur une
// application qu'on ouvre le matin et ferme le soir.
//
// Ce n'est pas une valeur par défaut — rien ne la surcharge.
const JWTSecretRotationDays = 1

// JWTRotationStatus décrit l'état de la rotation, pour que l'interface le
// montre au lieu de laisser deviner.
//
// La périodicité n'y figure pas : elle est constante, et l'interface l'énonce
// en toutes lettres plutôt que de rendre un nombre que rien ne fait varier.
type JWTRotationStatus struct {
	RotatedAt *time.Time `json:"rotated_at,omitempty"`
	NextAt    *time.Time `json:"next_at,omitempty"`
	// BloqueeParEnvironnement dit que la clé vient de la variable
	// JWT_SECRET, donc qu'aucune rotation ne peut aboutir sur ce déploiement.
	//
	// L'écran affichait auparavant une date de rotation qui avançait
	// normalement, alors que la clé réellement en service était figée depuis
	// l'installation. Mieux vaut dire « la rotation est désactivée par votre
	// mode d'installation » que laisser croire à une protection absente.
	BloqueeParEnvironnement bool `json:"bloquee_par_environnement,omitempty"`
}

// RotationStatus reports the current rotation state.
//
// Les deux dates restent nulles tant que la clé n'a jamais tourné — une
// installation antérieure à la rotation automatique. Le cas ne dure qu'un
// démarrage : MaybeRotateJWTSecret fait précisément tourner celle-là.
func RotationStatus(cfg *Config) JWTRotationStatus {
	var st JWTRotationStatus
	// Quand l'environnement impose la clé, les deux dates n'ont aucun sens :
	// celle du fichier décrit une rotation qui n'a jamais pris effet. On rend
	// donc l'état nu, plus le motif — l'interface a de quoi l'expliquer.
	if cfg.JWTSecretDepuisEnv {
		st.BloqueeParEnvironnement = true
		return st
	}
	if !cfg.JWTSecretRotatedAt.IsZero() {
		t := cfg.JWTSecretRotatedAt
		st.RotatedAt = &t
		n := t.Add(JWTSecretRotationDays * 24 * time.Hour)
		st.NextAt = &n
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
	// Une clé imposée par l'environnement ne peut pas tourner : `applyEnvOverrides`
	// la réimposera au démarrage suivant, par-dessus tout ce qu'on écrirait ici.
	//
	// Tourner quand même avait un coût réel, pas seulement cosmétique : on
	// écrivait un secret inutile dans config.json, on faisait avancer
	// `jwt_secret_rotated_at`, et l'écran affichait une date crédible pendant
	// que la clé en service restait celle de l'installation. Ne rien faire et
	// le DIRE (voir RotationStatus) vaut mieux qu'une promesse intenable.
	//
	// Deux des trois chemins d'installation livrés sont dans ce cas :
	// Linux/systemd et Windows Service. Le chemin NSIS/lanceur, lui, n'utilise
	// aucune variable d'environnement et continue de tourner normalement.
	if cfg.JWTSecretDepuisEnv {
		return false, nil
	}

	const maxAge = JWTSecretRotationDays * 24 * time.Hour
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
		// Le réglage de périodicité a existé et a pu être écrit. Plus rien ne
		// le lit : le laisser dans le fichier ferait croire à qui l'ouvre que
		// la valeur qu'il y voit s'applique encore.
		delete(existing, "jwt_secret_max_age_days")
	}); err != nil {
		return false, err
	}
	cfg.JWTSecret = secret
	cfg.JWTSecretRotatedAt = now.UTC()
	return true, nil
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

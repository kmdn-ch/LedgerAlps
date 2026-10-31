package main

// Quand l'assistant d'installation a-t-il le droit de tourner ?
//
// # Ce qu'il faisait (audit croisé Gemini, CRIT-17)
//
// `main` lançait l'assistant sur TOUTE erreur de lecture de la configuration — fichier
// absent, mais aussi JSON abîmé, port illisible, droits refusés. L'assistant écrit alors une
// configuration neuve (nouveau secret, chemin par défaut), et, si l'on coche « chiffrer la
// base », crée une clé de base NEUVE : celle qui existait, scellée sur ce compte, était écrasée.
// Une base chiffrée avec elle devenait illisible, pour de bon. Il suffisait d'une virgule
// oubliée dans config.json.
//
// # La règle
//
// L'assistant ne tourne que sur une machine VIERGE : aucun fichier de configuration (pas même
// un illisible), aucune base, aucune clé. Dans tous les autres cas, on s'arrête en disant
// pourquoi — et on ne touche à rien.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// tracesDInstallation liste ce qui, dans `dataDir`, prouve qu'une installation existe déjà.
func tracesDInstallation(dataDir string) []string {
	var traces []string
	for _, f := range []string{"ledgeralps.db", "ledgeralps.db-wal"} {
		if _, err := os.Stat(filepath.Join(dataDir, f)); err == nil {
			traces = append(traces, f)
		}
	}
	keys := db.NewDatabaseKeys(dataDir)
	if keys.Configured() {
		traces = append(traces, "une clé de base de données")
	} else if keys.HasRecovery() {
		traces = append(traces, "le fichier de récupération de la clé de base")
	}
	return traces
}

// peutLancerAssistant dit si l'assistant d'installation peut tourner. `cfgErr` est l'erreur
// de lecture de la configuration qui l'a fait envisager. Rend la raison du refus, en français,
// destinée à l'utilisateur.
func peutLancerAssistant(dataDir string, cfgErr error) (bool, string) {
	config := filepath.Join(dataDir, "config.json")
	if _, err := os.Stat(config); err == nil {
		return false, fmt.Sprintf(
			"Le fichier de configuration %s existe mais n'a pas pu être lu : %v. "+
				"L'assistant d'installation n'est PAS lancé : il écraserait votre configuration, "+
				"et la clé de votre base chiffrée avec elle. Corrigez le fichier, ou déplacez-le "+
				"ailleurs pour repartir de zéro après avoir sauvegardé le dossier %s.",
			config, cfgErr, dataDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Sprintf(
			"Le fichier de configuration %s n'est pas accessible : %v. "+
				"L'assistant d'installation n'est PAS lancé : il pourrait écraser une installation existante.",
			config, err)
	}
	if traces := tracesDInstallation(dataDir); len(traces) > 0 {
		return false, fmt.Sprintf(
			"Aucune configuration dans %s, mais des données d'une installation existante y figurent (%s). "+
				"L'assistant d'installation n'est PAS lancé : il créerait une base et une clé neuves "+
				"par-dessus. Restaurez le fichier config.json d'origine, ou faites examiner ce dossier avant "+
				"tout nouveau départ.",
			dataDir, strings.Join(traces, ", "))
	}
	return true, ""
}

// refuserLAssistant consigne la raison, la dépose dans un fichier que le système ouvre (le
// lanceur n'a pas de console, un refus muet ressemblerait à un lanceur qui ne fait rien), et
// sort sans rien avoir touché.
func refuserLAssistant(raison string) {
	if logger != nil {
		logger.Printf("[FATAL] %s", raison)
	}
	note := filepath.Join(appDataDir(), "LedgerAlps-ERREUR.txt")
	if err := os.WriteFile(note, []byte("LedgerAlps ne démarre pas.\r\n\r\n"+raison+"\r\n"), 0o600); err == nil {
		openBrowser(note)
	}
	os.Exit(1)
}

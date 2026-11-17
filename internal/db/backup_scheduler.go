package db

// La sauvegarde automatique, au fil du temps et pas seulement au démarrage.
//
// # Le défaut
//
// `MaybeAutoBackup` n'était appelée qu'UNE fois, au démarrage du serveur. Sur un poste qu'on éteint le
// soir, c'est suffisant : chaque matin le serveur repart, voit que la dernière copie a plus d'un jour et
// en prend une. Mais le serveur n'est pas arrêté à la fermeture du navigateur — le lanceur le démarre,
// puis se termine, et il survit — et sur une machine qui reste allumée (ou en veille prolongée), plus
// aucune copie n'était prise jusqu'au redémarrage suivant. La donnée la plus précieuse du produit, les
// pièces que le CO art. 958f oblige à conserver dix ans, n'avait alors qu'une copie vieille de semaines.
//
// La purge de rétention et l'attestation d'intégrité, elles, tournaient déjà chaque jour.
//
// # Ce que fait ce fichier
//
// Il rappelle la sauvegarde automatique à intervalle régulier. La question « une copie est-elle due ? »
// reste celle de `MaybeAutoBackup` — la plus récente a-t-elle plus de `DefaultInterval` ? — : on la pose
// souvent (toutes les heures) pour qu'une copie due soit prise dans l'heure, plutôt que de poser un
// minuteur de vingt-quatre heures exactes, qui sauterait un tour dès que la copie précédente a quelques
// secondes de moins qu'un jour.

import (
	"context"
	"log"
	"time"
)

// AutoBackupCheckEvery est la fréquence à laquelle le serveur regarde si une sauvegarde automatique est
// due. Regarder ne coûte qu'une lecture de dossier ; la copie, elle, n'est prise que si la plus récente
// a plus de `DefaultInterval`.
const AutoBackupCheckEvery = time.Hour

// StartAutoBackupScheduler appelle `job` toutes les `every`, jusqu'à l'annulation de `ctx`. Il rend la
// main aussitôt : la boucle tourne dans sa propre goroutine, et ne fait rien au démarrage (la première
// copie, si elle est due, est prise par l'appelant avant de servir).
//
// Une panique de `job` est interceptée : la sauvegarde est une mesure de sécurité, et une goroutine qui
// panique arrête TOUT le processus — c'est-à-dire l'accès de l'utilisateur à sa comptabilité. Elle est
// journalisée et le tour suivant a lieu.
func StartAutoBackupScheduler(ctx context.Context, every time.Duration, job func(context.Context)) {
	if every <= 0 {
		every = AutoBackupCheckEvery
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				executerSansPanique(ctx, job)
			}
		}
	}()
}

func executerSansPanique(ctx context.Context, job func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WARNING: la sauvegarde automatique a paniqué (%v) ; le prochain passage aura lieu comme prévu", r)
		}
	}()
	job(ctx)
}

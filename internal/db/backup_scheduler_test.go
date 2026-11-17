package db

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// attendre rend vrai dès que cond l'est, faux au bout de limite. Les délais sont courts : ces tests
// n'attendent que ce que la goroutine met à tourner.
func attendre(limite time.Duration, cond func() bool) bool {
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// Le défaut : la sauvegarde automatique n'était appelée qu'au démarrage. Elle doit revenir, encore et
// encore, tant que le serveur tourne — et s'arrêter avec lui.
func TestLaSauvegardeAutomatiqueRevientTantQueLeServeurTourne(t *testing.T) {
	ctx, annuler := context.WithCancel(context.Background())
	var appels atomic.Int32
	StartAutoBackupScheduler(ctx, 10*time.Millisecond, func(context.Context) { appels.Add(1) })

	if !attendre(2*time.Second, func() bool { return appels.Load() >= 3 }) {
		t.Fatalf("%d appel(s) seulement : la sauvegarde ne revient pas", appels.Load())
	}

	annuler()
	time.Sleep(50 * time.Millisecond) // le temps qu'un tour en cours se termine
	figé := appels.Load()
	time.Sleep(100 * time.Millisecond)
	if appels.Load() != figé {
		t.Errorf("la sauvegarde continue après l'arrêt du serveur : %d → %d appels", figé, appels.Load())
	}
}

// Le planificateur ne fait rien au démarrage : la première copie, si elle est due, est prise par
// l'appelant avant de servir. Sans cela, deux copies partiraient à la même seconde.
func TestLePlanificateurNeFaitRienAvantLePremierTour(t *testing.T) {
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	var appels atomic.Int32
	StartAutoBackupScheduler(ctx, time.Hour, func(context.Context) { appels.Add(1) })
	time.Sleep(100 * time.Millisecond)
	if n := appels.Load(); n != 0 {
		t.Errorf("%d appel(s) avant le premier tour", n)
	}
}

// Une panique dans une sauvegarde ne doit pas arrêter le serveur — ni les sauvegardes suivantes.
func TestUnePaniqueDeLaSauvegardeNArretePasLeServeur(t *testing.T) {
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	var appels atomic.Int32
	StartAutoBackupScheduler(ctx, 10*time.Millisecond, func(context.Context) {
		if appels.Add(1) == 1 {
			panic("défaut simulé")
		}
	})
	if !attendre(2*time.Second, func() bool { return appels.Load() >= 3 }) {
		t.Fatalf("%d appel(s) : la panique du premier tour a arrêté la boucle", appels.Load())
	}
}

// Une fréquence nulle ou négative retombe sur la valeur par défaut au lieu de tourner à vide.
func TestUneFrequenceNonPositiveRetombeSurLaValeurParDefaut(t *testing.T) {
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	var appels atomic.Int32
	StartAutoBackupScheduler(ctx, 0, func(context.Context) { appels.Add(1) })
	time.Sleep(100 * time.Millisecond)
	if n := appels.Load(); n != 0 {
		t.Errorf("fréquence nulle : %d appel(s) en 100 ms, la valeur par défaut est d'une heure", n)
	}
}

// De bout en bout, avec la vraie sauvegarde : une copie est reprise EN COURS DE ROUTE quand la plus
// récente est devenue plus vieille que l'intervalle — ce que le seul appel au démarrage ne faisait pas.
func TestUneCopieEstReprisePendantQueLeServeurTourne(t *testing.T) {
	database, cfg := newTestDB(t)
	dir := t.TempDir()
	ctx, annuler := context.WithCancel(context.Background())

	// Le test ne doit rendre la main qu'une fois la dernière sauvegarde terminée : sinon elle écrit
	// encore dans le dossier temporaire pendant que Go le supprime (« directory not empty », vu en CI).
	// On compte la tâche AVANT de regarder l'annulation : une tâche qui démarre juste après `annuler`
	// est soit comptée (on l'attend), soit voit l'annulation et ne touche à rien.
	var enCours atomic.Int32
	defer func() {
		annuler()
		if !attendre(5*time.Second, func() bool { return enCours.Load() == 0 }) {
			t.Error("une sauvegarde est encore en cours après l'arrêt")
		}
	}()

	job := func(ctx context.Context) {
		enCours.Add(1)
		defer enCours.Add(-1)
		if ctx.Err() != nil {
			return
		}
		// Intervalle de 30 ms : « plus d'un jour » à l'échelle du test.
		_, _ = MaybeAutoBackup(ctx, database, cfg, dir, 30*time.Millisecond, 100, "")
	}

	// Démarrage : la première copie, comme dans cmd/server/main.go.
	job(ctx)
	if copies, _ := ListBackups(dir); len(copies) != 1 {
		t.Fatalf("après le démarrage : %d copie(s), attendu 1", len(copies))
	}

	// Puis le serveur tourne, sans redémarrer.
	StartAutoBackupScheduler(ctx, 20*time.Millisecond, job)
	if !attendre(5*time.Second, func() bool {
		copies, _ := ListBackups(dir)
		return len(copies) >= 3
	}) {
		copies, _ := ListBackups(dir)
		t.Fatalf("%d copie(s) seulement : aucune copie n'est reprise tant que le serveur tourne", len(copies))
	}
}

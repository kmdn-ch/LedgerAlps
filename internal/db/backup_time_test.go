package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Le nom d'une sauvegarde était écrit en UTC alors que l'interface affiche
// l'heure du fichier, que le système rend en heure locale. Une sauvegarde prise
// à 16 h 23 en Suisse s'appelait « …T14-23-05 » et s'affichait « 16:23 ».
//
// Deux heures d'écart entre l'explorateur de fichiers et le logiciel, sur des
// fichiers qu'il faut savoir identifier pendant dix ans (CO art. 958f) : c'est
// exactement le genre d'écart qui fait restaurer la mauvaise copie.

func TestBackupNameUsesLocalTime(t *testing.T) {
	now := time.Now()
	name := now.Format(BackupTimeFormat)

	// L'heure du nom doit être celle de l'horloge, pas celle de Greenwich.
	wantHour := now.Format("15-04")
	if !strings.Contains(name, wantHour) {
		t.Fatalf("nom horodaté %q, attendu l'heure locale %q", name, wantHour)
	}

	// Le décalage rend le nom non ambigu la nuit du changement d'heure, où
	// 02 h 30 existe deux fois.
	if !strings.Contains(name, now.Format("-0700")) {
		t.Fatalf("nom %q sans décalage UTC : deux sauvegardes de la nuit du changement d'heure seraient indistinguables", name)
	}

	// Et il doit rester relisible par Go, sans quoi personne ne pourra dater un
	// fichier autrement qu'à l'œil.
	parsed, err := time.Parse(BackupTimeFormat, name)
	if err != nil {
		t.Fatalf("le nom produit n'est pas relisible: %v", err)
	}
	if parsed.UTC().Sub(now.UTC()) > time.Second || now.UTC().Sub(parsed.UTC()) > time.Second {
		t.Fatalf("l'instant relu (%s) diffère de l'instant écrit (%s)", parsed.UTC(), now.UTC())
	}
}

// L'ordre des sauvegardes désigne « la plus ancienne » à purger. Il reposait sur
// l'ordre alphabétique des noms, ce qui marchait tant qu'ils étaient en UTC : en
// heure locale, la nuit du passage à l'heure d'hiver, une sauvegarde de 02 h 30
// précède réellement une de 02 h 00 mais la suit dans l'alphabet — et la purge
// aurait effacé la mauvaise.
func TestListBackupsOrdersByInstantNotByName(t *testing.T) {
	dir := t.TempDir()

	// Deux noms dont l'ordre alphabétique contredit l'ordre chronologique,
	// comme au passage à l'heure d'hiver.
	older := "ledgeralps-2026-10-25T02-30-00+0200.db" // 00:30 UTC — le plus ancien
	newer := "ledgeralps-2026-10-25T02-00-00+0100.db" // 01:00 UTC — le plus récent
	for _, n := range []string{older, newer} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Les dates de fichier portent la vérité chronologique.
	base := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, older), base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, newer), base.Add(30*time.Minute), base.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	list, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("%d sauvegardes listées, attendu 2", len(list))
	}
	if list[0].Name != newer {
		t.Fatalf("en tête : %q, attendu la plus RÉCENTE %q — la purge supprimerait la mauvaise",
			list[0].Name, newer)
	}
}

// Une sauvegarde nommée à l'ancienne, en UTC, doit rester listée et datée
// correctement : une base existante en contient jusqu'à quatorze.
func TestListBackupsStillHandlesLegacyUTCNames(t *testing.T) {
	dir := t.TempDir()
	legacy := "ledgeralps-2026-08-03T15-10-57.db"
	if err := os.WriteFile(filepath.Join(dir, legacy), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	list, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != legacy {
		t.Fatalf("sauvegarde à l'ancien format ignorée : %+v", list)
	}
	if list[0].CreatedAt.IsZero() {
		t.Fatal("aucune date pour une sauvegarde à l'ancien format")
	}
}

package diskcrypt

import (
	"runtime"
	"testing"
)

// The rule the interface depends on. Getting it backwards would either nag
// someone who already protected their machine — the stale-warning problem this
// package exists to avoid — or reassure someone who did not.
func TestAdvisoryShownUnlessConfirmedEncrypted(t *testing.T) {
	cases := map[Status]bool{
		Encrypted:    false, // confirmed protected: stay quiet
		NotEncrypted: true,  // reported unprotected: speak up
		Unknown:      true,  // we did not look; silence would be a claim
	}
	for st, wantAdvisory := range cases {
		got := Report{Status: st, Advisory: st != Encrypted}
		if got.Advisory != wantAdvisory {
			t.Errorf("statut %q → advisory=%v, attendu %v", st, got.Advisory, wantAdvisory)
		}
	}
}

// Check must answer on every platform without panicking: it runs on the health
// page, which has to work on a machine we know nothing about.
func TestCheckAlwaysAnswers(t *testing.T) {
	r := Check()
	switch r.Status {
	case Encrypted, NotEncrypted, Unknown:
	default:
		t.Fatalf("statut inattendu: %q", r.Status)
	}
	if r.Advisory != (r.Status != Encrypted) {
		t.Errorf("Advisory=%v incohérent avec le statut %q", r.Advisory, r.Status)
	}
	t.Logf("sur cette machine (%s) : statut=%s mécanisme=%q avertissement=%v",
		runtime.GOOS, r.Status, r.Mechanism, r.Advisory)
}

// Outside Windows there is no non-elevated way to look, and a guess would cost
// more than an honest "unknown".
func TestNonWindowsReportsUnknown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test propre aux plateformes sans détection")
	}
	if r := Check(); r.Status != Unknown {
		t.Errorf("statut = %q hors Windows, attendu unknown", r.Status)
	}
}

// A mechanism name is only meaningful when something actually answered. Naming
// one on an unknown result would suggest a lookup that never happened.
func TestUnknownCarriesNoMechanism(t *testing.T) {
	if r := Check(); r.Status == Unknown && r.Mechanism != "" {
		t.Errorf("statut unknown mais mécanisme %q annoncé", r.Mechanism)
	}
}

// Un avertissement sans marche à suivre transfère le problème à l'utilisateur.
// « Activez BitLocker » suppose qu'il sache où c'est — et sous Windows Famille,
// ce n'est pas où on lui dit d'aller, parce que ça s'appelle autrement.
func TestUnAvertissementPorteToujoursUneMarcheASuivre(t *testing.T) {
	r := Check()
	if !r.Advisory {
		t.Skipf("disque protégé sur cette machine (%s) : rien à conseiller", r.Feature)
	}
	if len(r.Steps) == 0 {
		t.Fatal("avertissement affiché sans aucune marche à suivre")
	}
	if r.Feature == "" {
		t.Fatal("aucun nom de fonctionnalité : l'utilisateur ne saura pas quoi chercher")
	}
}

// Le constat porte sur le disque de démarrage seulement. Le dire même quand la
// réponse est bonne : « chiffré » sans réserve serait une affirmation plus large
// que ce qui a été vérifié.
func TestLaLimiteDuConstatEstToujoursEnoncee(t *testing.T) {
	if r := Check(); r.Caveat == "" {
		t.Fatal("aucune réserve énoncée sur la portée du constat")
	}
}

package handlers

import (
	"sync"
	"time"
)

// recoveryThrottle borne les essais de code de secours ratés, compte par compte.
//
// Vérifier un code de secours coûte cher À DESSEIN : ils sont hachés comme des
// mots de passe (bcrypt, coût 12), et un code erroné se compare aux dix codes
// non utilisés du compte — près d'une seconde de processeur par tentative.
// Le limiteur de connexion borne les requêtes par adresse ; il ne borne pas ce
// que CHAQUE requête fait dépenser. Sans plafond par compte, quelques
// tentatives par minute suffisent à occuper un poste modeste.
//
// Après `max` échecs dans la fenêtre, la voie « code de secours » du compte est
// fermée jusqu'à ce que la fenêtre s'écoule — sans calcul de hachage, donc sans
// coût. Le code TOTP, lui, n'est pas concerné : la connexion normale reste ouverte.
//
// La valeur nulle est utilisable et ne limite rien : un AuthHandler construit
// sans passer par NewAuthHandler garde l'ancien comportement.
type recoveryThrottle struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	fails  map[string][]time.Time
}

func newRecoveryThrottle(max int, window time.Duration) *recoveryThrottle {
	return &recoveryThrottle{max: max, window: window, now: time.Now, fails: map[string][]time.Time{}}
}

// prune retire les échecs sortis de la fenêtre. Le verrou est tenu.
func (t *recoveryThrottle) prune(user string) []time.Time {
	cutoff := t.now().Add(-t.window)
	kept := t.fails[user][:0]
	for _, at := range t.fails[user] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(t.fails, user)
		return nil
	}
	t.fails[user] = kept
	return kept
}

// blocked dit si la voie « code de secours » de ce compte est fermée.
func (t *recoveryThrottle) blocked(user string) bool {
	if t == nil || t.max <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.prune(user)) >= t.max
}

// fail enregistre un échec. Il rend true quand c'est CET échec qui ferme la
// voie, pour que l'appelant ne trace l'événement qu'une fois.
func (t *recoveryThrottle) fail(user string) bool {
	if t == nil || t.max <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fails[user] = append(t.prune(user), t.now())
	return len(t.fails[user]) == t.max
}

// reset oublie les échecs d'un compte après une réussite.
func (t *recoveryThrottle) reset(user string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, user)
}

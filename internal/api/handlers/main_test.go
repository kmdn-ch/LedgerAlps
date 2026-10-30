package handlers

import (
	"os"
	"testing"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"golang.org/x/crypto/bcrypt"
)

// Les tests de ce paquet se connectent des dizaines de fois. Au coût de
// production, chaque hachage pèse un quart de seconde, davantage encore sous le
// détecteur de concurrence de la CI : la suite approchait la limite de dix
// minutes. On abaisse le coût ici ; le coût de production est verrouillé par un
// test du paquet security.
func TestMain(m *testing.M) {
	security.SetBcryptCostForTests(bcrypt.MinCost)
	os.Exit(m.Run())
}

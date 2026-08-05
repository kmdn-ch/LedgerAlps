package mfa

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// Un TOTP écrit à la main se vérifie contre la norme, pas contre soi-même.
//
// La RFC 6238 publie des vecteurs de test : un secret connu, des instants
// connus, des codes connus. Les reproduire est la seule preuve que
// l'implémentation est juste — un test qui compare le code à ce que le même
// code produit passerait avec n'importe quelle erreur d'algorithme.

// Le secret des vecteurs officiels : la chaîne ASCII « 12345678901234567890 »,
// que la RFC donne en hexadécimal. Nos fonctions attendent du base32.
func rfcSecret() string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))
}

// Vecteurs de la RFC 6238, annexe B, pour HMAC-SHA1. La RFC les donne sur huit
// chiffres ; nous en produisons six, donc les six derniers.
func TestVecteursOfficielsDeLaRFC6238(t *testing.T) {
	cases := []struct {
		unix int64
		want string // les 6 derniers chiffres du vecteur à 8 chiffres
	}{
		{59, "287082"},          // RFC : 94287082
		{1111111109, "081804"},  // RFC : 07081804
		{1111111111, "050471"},  // RFC : 14050471
		{1234567890, "005924"},  // RFC : 89005924
		{2000000000, "279037"},  // RFC : 69279037
		{20000000000, "353130"}, // RFC : 65353130
	}
	secret := rfcSecret()
	for _, c := range cases {
		got, err := Code(secret, time.Unix(c.unix, 0).UTC())
		if err != nil {
			t.Fatalf("t=%d: %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("t=%d → %s, attendu %s", c.unix, got, c.want)
		}
	}
}

func TestUnCodeJusteEstAccepte(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, err := Code(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := Verify(secret, code, now, 0); !ok {
		t.Fatal("le code de l'instant courant est refusé")
	}
}

// LE test qui donne sa valeur au second facteur : un code ne sert qu'une fois.
//
// Sans cela, un code regardé par-dessus l'épaule ou lu dans un journal reste
// utilisable pendant toute sa minute de validité.
func TestUnCodeNeSertQuUneFois(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()
	code, _ := Code(secret, now)

	window, ok := Verify(secret, code, now, 0)
	if !ok {
		t.Fatal("premier usage refusé")
	}
	if _, ok := Verify(secret, code, now, window); ok {
		t.Fatal("le même code a été accepté deux fois")
	}
}

// La tolérance d'une fenêtre absorbe la dérive d'horloge d'un téléphone. Sans
// elle, un code saisi à la vingt-neuvième seconde arriverait après son
// expiration et le produit serait inutilisable.
func TestUneFenetreDeToleranceDeChaqueCote(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()

	precedent, _ := Code(secret, now.Add(-Period*time.Second))
	if _, ok := Verify(secret, precedent, now, 0); !ok {
		t.Error("le code de la fenêtre précédente est refusé")
	}
	suivant, _ := Code(secret, now.Add(Period*time.Second))
	if _, ok := Verify(secret, suivant, now, 0); !ok {
		t.Error("le code de la fenêtre suivante est refusé")
	}
	// Deux fenêtres : trop loin. Chaque fenêtre acceptée multiplie les codes
	// valides à un instant donné.
	tropVieux, _ := Code(secret, now.Add(-3*Period*time.Second))
	if _, ok := Verify(secret, tropVieux, now, 0); ok {
		t.Error("un code de trois fenêtres est accepté")
	}
}

func TestUnMauvaisCodeEstRefuse(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()
	for _, bad := range []string{"000000", "123456", "abcdef", "", "12345", "1234567"} {
		if _, ok := Verify(secret, bad, now, 0); ok {
			// 000000 et 123456 peuvent coïncider une fois sur un million ;
			// le test le signale plutôt que de l'ignorer.
			t.Errorf("code %q accepté", bad)
		}
	}
}

// Les espaces que les applications insèrent pour la lisibilité ne doivent pas
// faire échouer la saisie — « 123 456 » est ce que l'utilisateur lit.
func TestLesEspacesDeSaisieSontTolerees(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()
	code, _ := Code(secret, now)
	spaced := code[:3] + " " + code[3:]
	if _, ok := Verify(secret, spaced, now, 0); !ok {
		t.Fatalf("le code %q est refusé alors que %q est accepté", spaced, code)
	}
}

// Un secret ne doit jamais ressembler à un autre.
func TestDeuxSecretsDifferent(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := NewSecret()
		if err != nil {
			t.Fatal(err)
		}
		if seen[s] {
			t.Fatal("deux secrets identiques")
		}
		seen[s] = true
	}
}

// L'URI doit être lisible par une application d'authentification : le secret,
// l'émetteur en deux endroits, et les paramètres explicites.
func TestLURIPorteToutCeQuUneApplicationAttend(t *testing.T) {
	uri := ProvisioningURI("LedgerAlps", "admin@exemple.ch", "ABCDEFGH")
	for _, want := range []string{
		"otpauth://totp/",
		"secret=ABCDEFGH",
		"issuer=LedgerAlps",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Errorf("l'URI ne contient pas %q : %s", want, uri)
		}
	}
	// L'émetteur figure aussi dans le chemin, avant le deux-points : certaines
	// applications ne lisent que celui-là, et le compte apparaîtrait alors sans
	// nom d'application dans la liste.
	//
	// Le deux-points et l'arobase ne sont PAS échappés : ils sont licites dans
	// un segment de chemin, et le format de clé otpauth les attend tels quels.
	// Première version de ce test, qui exigeait « %3A » et échouait sur une URI
	// pourtant correcte.
	if !strings.Contains(uri, "totp/LedgerAlps:admin@exemple.ch?") {
		t.Errorf("l'émetteur manque dans le chemin : %s", uri)
	}
}

func TestUnSecretIllisibleEstRefuseSansPanique(t *testing.T) {
	if _, err := Code("pas du base32 !!!", time.Now()); err == nil {
		t.Fatal("un secret illisible a été accepté")
	}
	if _, ok := Verify("pas du base32 !!!", "123456", time.Now(), 0); ok {
		t.Fatal("vérification acceptée sur un secret illisible")
	}
}

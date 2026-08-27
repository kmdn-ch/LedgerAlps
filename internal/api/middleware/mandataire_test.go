package middleware

// L'invariant : aucun en-tete X-Forwarded-* ne pese sur une decision tant que
// le pair ne figure pas parmi les mandataires declares.
//
// Le troisieme audit a releve que X-Forwarded-For etait bien garde par
// SetTrustedProxies alors que X-Forwarded-Proto ne l'etait pas. Ces tests
// tiennent la symetrie : sans eux, rien n'empeche le trou de revenir a la
// premiere lecture d'en-tete ajoutee ailleurs.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// demander monte un routeur avec la liste de mandataires donnee et rend ce que
// ProtocoleAnnonceHTTPS a repondu.
func demander(t *testing.T, mandataires []string, pair string, entete string) bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ConfianceMandataire(mandataires))

	var verdict bool
	r.GET("/", func(c *gin.Context) {
		verdict = ProtocoleAnnonceHTTPS(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = pair
	if entete != "" {
		req.Header.Set("X-Forwarded-Proto", entete)
	}
	r.ServeHTTP(httptest.NewRecorder(), req)
	return verdict
}

// Le defaut, et le cas de loin le plus courant : LedgerAlps ecoute en direct,
// aucun mandataire declare. L'en-tete ne doit alors rien valoir.
func TestSansMandataireDeclareLEnteteNeVautRien(t *testing.T) {
	if demander(t, nil, "127.0.0.1:5000", "https") {
		t.Error("X-Forwarded-Proto cru sans aucun mandataire declare")
	}
	if demander(t, []string{}, "10.0.0.9:5000", "https") {
		t.Error("X-Forwarded-Proto cru sur une liste vide")
	}
}

func TestUnMandataireDeclareEstCru(t *testing.T) {
	if !demander(t, []string{"10.0.0.1"}, "10.0.0.1:5000", "https") {
		t.Error("un mandataire declare par adresse n'est pas cru")
	}
	if !demander(t, []string{"10.0.0.0/24"}, "10.0.0.7:5000", "https") {
		t.Error("un mandataire declare par CIDR n'est pas cru")
	}
}

// Declarer UN mandataire ne rend pas les autres pairs dignes de confiance.
func TestUnPairHorsListeNEstPasCru(t *testing.T) {
	if demander(t, []string{"10.0.0.1"}, "10.0.0.2:5000", "https") {
		t.Error("un pair voisin du mandataire declare est cru")
	}
	if demander(t, []string{"10.0.0.0/24"}, "10.0.1.5:5000", "https") {
		t.Error("un pair hors du CIDR declare est cru")
	}
}

// Une IPv4 arrivant sous forme mappee IPv6 doit se comparer a un CIDR IPv4
// comme l'adresse qu'elle est -- sinon un mandataire legitime cesse d'etre
// reconnu selon la pile reseau du systeme, ce qui est le pire des defauts :
// intermittent.
func TestUneIPv4MappeeEnIPv6EstReconnue(t *testing.T) {
	if !demander(t, []string{"10.0.0.0/24"}, "[::ffff:10.0.0.7]:5000", "https") {
		t.Error("IPv4 mappee IPv6 non reconnue dans un CIDR IPv4")
	}
}

// Un mandataire de confiance qui annonce autre chose que https ne rend pas la
// requete securisee pour autant.
func TestUnMandataireDeConfianceQuiAnnonceHTTPNeMentPas(t *testing.T) {
	if demander(t, []string{"10.0.0.1"}, "10.0.0.1:5000", "http") {
		t.Error("X-Forwarded-Proto: http compte comme https")
	}
	if demander(t, []string{"10.0.0.1"}, "10.0.0.1:5000", "") {
		t.Error("en-tete absent compte comme https")
	}
}

// Sans l'intergiciel -- un routeur de test monte a la main, par exemple --, le
// contexte ne porte aucun verdict. Le defaut doit etre « pas de confiance ».
func TestSansLIntergiciereLeDefautEstLeRefus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var verdict bool
	r.GET("/", func(c *gin.Context) {
		verdict = ProtocoleAnnonceHTTPS(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if verdict {
		t.Error("en-tete cru alors qu'aucun verdict n'a ete pose")
	}
}

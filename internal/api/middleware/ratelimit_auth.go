package middleware

// Les limiteurs des routes d'authentification, et ce sur quoi chacun compte.
//
// # Le défaut (audit croisé Gemini, CRIT-13)
//
// Un seul limiteur, compté par ADRESSE et remis à zéro sur tout succès, servait
// `/auth/login` ET `/auth/mfa/verify`. Quelqu'un qui connaît le mot de passe — ou
// qui possède n'importe quel compte — intercalait un `login` réussi (200) tous les
// neuf essais : le compteur retombait à zéro, et le code à six chiffres se devinait
// sans limite (120 codes faux traités sur 120, aucun 429).
//
// C'était le reste de l'audit 9, M-01 : j'avais séparé le rafraîchissement et la
// recherche au registre, pas le second facteur.
//
// # Ce que chaque limiteur compte désormais
//
//   - Login (adresse) : inchangé. Il arrête l'arrosage d'un mot de passe sur
//     plusieurs comptes depuis une même adresse.
//   - LoginAccount (adresse + compte visé) : un succès sur UN compte n'efface
//     jamais les échecs portés contre UN AUTRE. Sans lui, un utilisateur ordinaire
//     remettait à zéro le compteur dont il se servait pour deviner le mot de passe
//     de l'administrateur.
//   - MFA (compte) : une instance à part, que `login` ne touche pas, comptée par
//     COMPTE et non par adresse. Obtenir un nouveau jeton d'attente — il suffit du
//     mot de passe — ne donne donc aucun nouvel essai. Cinq échecs, comme les codes
//     de secours : six chiffres se devinent en un million d'essais, et personne
//     n'en tape cinq faux de bonne foi.
//   - Refresh : son propre compteur (audit 9, M-01).
//
// Le compte du second facteur est celui du jeton d'attente, lu AVANT le limiteur :
// une requête sans jeton valide est refusée plus tôt et ne peut de toute façon
// tenter aucun code.

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// KeyFunc dit sur quoi un limiteur compte. `ok` faux : cette requête n'est pas
// comptée (le gestionnaire la refusera de toute façon).
type KeyFunc func(c *gin.Context) (key string, ok bool)

// KeyedBy fait compter le limiteur sur autre chose que l'adresse.
func (l *LoginRateLimiter) KeyedBy(fn KeyFunc) *LoginRateLimiter {
	l.keyFn = fn
	return l
}

// Nature d'un verrouillage, pour la trace de sécurité.
const (
	LockoutLogin        = "login"
	LockoutLoginAccount = "login_account"
	LockoutMFA          = "mfa"
	LockoutRefresh      = "refresh"
)

const (
	// MFAMaxAttempts : échecs de code par compte avant verrouillage.
	MFAMaxAttempts = 5

	// corpsLisibleMax borne ce que le limiteur lit pour trouver le compte visé.
	corpsLisibleMax = 64 << 10
	// emailMax : longueur maximale d'une adresse (RFC 5321). Au-delà on tronque —
	// ne pas compter une adresse démesurée en ferait un moyen d'échapper au plafond.
	emailMax = 254
)

// KeyByMFAAccount compte par compte : celui que nomme le jeton d'attente.
// À placer APRÈS RequireMFAChallenge.
func KeyByMFAAccount(c *gin.Context) (string, bool) {
	claims := GetClaims(c)
	if claims == nil || claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
}

// KeyByIPAndEmail compte par couple (adresse du client, compte visé par la
// tentative). Le compte se lit dans le corps JSON ; le corps est rendu intact au
// gestionnaire.
func KeyByIPAndEmail(c *gin.Context) (string, bool) {
	if c.Request == nil || c.Request.Body == nil {
		return "", false
	}
	reste := c.Request.Body
	brut, _ := io.ReadAll(io.LimitReader(reste, corpsLisibleMax))
	// Le gestionnaire relira le corps tel qu'il est arrivé, y compris ce qui
	// dépasserait la borne.
	c.Request.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(brut), reste), reste}

	var corps struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(brut, &corps) != nil {
		return "", false
	}
	email := strings.ToLower(strings.TrimSpace(corps.Email))
	if email == "" {
		return "", false
	}
	if len(email) > emailMax {
		email = email[:emailMax]
	}
	return c.ClientIP() + "|" + email, true
}

// AuthLimiters regroupe les limiteurs d'authentification : le serveur et les tests
// montent ainsi la MÊME chaîne, et un test qui passe prouve la configuration livrée.
type AuthLimiters struct {
	Login        *LoginRateLimiter
	LoginAccount *LoginRateLimiter
	Refresh      *LoginRateLimiter
	MFA          *LoginRateLimiter
}

// NewAuthLimiters construit les limiteurs. `onLockout` reçoit la nature du
// verrouillage (LockoutLogin…), sa clé — une adresse, « adresse|compte » ou un
// identifiant de compte — et l'heure de fin ; il peut être nil.
func NewAuthLimiters(onLockout func(kind, key string, until time.Time)) *AuthLimiters {
	faire := func(kind string, max int, fn KeyFunc) *LoginRateLimiter {
		l := NewLoginRateLimiter(max, DefaultLoginWindow, DefaultLoginPaliers...)
		if fn != nil {
			l.KeyedBy(fn)
		}
		if onLockout != nil {
			l.OnLockout(func(key string, until time.Time) { onLockout(kind, key, until) })
		}
		return l
	}
	return &AuthLimiters{
		Login:        faire(LockoutLogin, DefaultLoginMaxAttempts, nil),
		LoginAccount: faire(LockoutLoginAccount, DefaultLoginMaxAttempts, KeyByIPAndEmail),
		Refresh:      faire(LockoutRefresh, DefaultLoginMaxAttempts, nil),
		MFA:          faire(LockoutMFA, MFAMaxAttempts, KeyByMFAAccount),
	}
}

// ForLogin rend les filtres de POST /auth/login, à placer devant le gestionnaire.
func (a *AuthLimiters) ForLogin() []gin.HandlerFunc {
	return []gin.HandlerFunc{a.Login.Middleware(), a.LoginAccount.Middleware()}
}

// ForMFAVerify rend les filtres de POST /auth/mfa/verify : le jeton d'attente
// d'abord (il fournit le compte), puis le limiteur du second facteur.
func (a *AuthLimiters) ForMFAVerify(jwtSecret string) []gin.HandlerFunc {
	return []gin.HandlerFunc{RequireMFAChallenge(jwtSecret), a.MFA.Middleware()}
}

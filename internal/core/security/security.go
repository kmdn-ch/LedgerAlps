package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost est le coût de production. Il n'est une variable que pour que les
// tests des autres paquets puissent l'abaisser : voir SetBcryptCostForTests.
var bcryptCost = 12

// SetBcryptCostForTests abaisse le coût de bcrypt pour une suite de tests.
//
// Une connexion coûte près d'un quart de seconde au coût de production — voulu —
// et la CI joue les tests sous le détecteur de concurrence, plusieurs fois plus
// lent : la suite des gestionnaires approchait déjà la limite de dix minutes par
// paquet, et un test de plus la faisait tomber (« test timed out »). Les tests
// ne vérifient pas la lenteur du hachage mais ce qui l'entoure.
//
// Refuse de s'exécuter hors d'un binaire de test : un coût abaissé dans un
// serveur de production serait un affaiblissement silencieux de tous les mots
// de passe. Le coût de production est verrouillé par un test du paquet.
func SetBcryptCostForTests(cost int) {
	if !testing.Testing() {
		panic("security.SetBcryptCostForTests : réservé aux binaires de test")
	}
	bcryptCost = cost
}

// prehash applies SHA-256 to the password before bcrypt.
// bcrypt silently truncates input at 72 bytes; pre-hashing avoids this so that
// passwords of any length produce distinct hashes (e.g. "a"*72 ≠ "a"*73).
func prehash(password string) []byte {
	h := sha256.Sum256([]byte(password))
	return h[:]
}

// HashPassword returns a bcrypt hash of the pre-hashed password.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword(prehash(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword compares a bcrypt hash with a candidate password.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), prehash(password)) == nil
}

// MinPasswordLength est le plancher d'un mot de passe de connexion choisi par son
// titulaire : douze caractères, avec minuscule, majuscule et chiffre.
const MinPasswordLength = 12

// ValidatePasswordPolicy dit pourquoi un mot de passe est trop faible, ou rien.
//
// Le message nomme ce qui manque. « Mot de passe trop faible » oblige à
// deviner, et la plupart des gens devinent en ajoutant un chiffre à la fin.
//
// La règle vaut pour TOUT mot de passe qui entre dans le système : celui qu'on
// choisit à l'installation, celui qu'un administrateur donne à un nouveau compte,
// celui qu'on change. Elle ne valait que pour le dernier, si bien que `12345678`
// ouvrait un compte créé par un administrateur et le premier administrateur
// lui-même (audit 9, F-02).
func ValidatePasswordPolicy(p string) error {
	var lower, upper, digit bool
	for _, r := range p {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	var missing []string
	if len([]rune(p)) < MinPasswordLength {
		missing = append(missing, fmt.Sprintf("%d caractères au minimum", MinPasswordLength))
	}
	if !lower {
		missing = append(missing, "une minuscule")
	}
	if !upper {
		missing = append(missing, "une majuscule")
	}
	if !digit {
		missing = append(missing, "un chiffre")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("il manque : %s", strings.Join(missing, ", "))
}

// ─── JWT ──────────────────────────────────────────────────────────────────────

type Claims struct {
	jwt.RegisteredClaims
	UserID  string `json:"sub"`
	IsAdmin bool   `json:"is_admin"`
	JTI     string `json:"jti"`
	// MFAPending marque un jeton d'ATTENTE : le mot de passe a été accepté, le
	// second facteur pas encore. Il ne vaut que pour /auth/mfa/verify, et le
	// filtre d'authentification le refuse partout ailleurs.
	//
	// Le refus est écrit dans authenticate() plutôt que route par route : ainsi
	// une route ajoutée demain le rejette sans que personne y pense. Un drapeau
	// qu'il faudrait penser à vérifier serait oublié, et un jeton de moitié de
	// connexion vaudrait alors une session complète.
	MFAPending bool `json:"mfa_pending,omitempty"`
	// Typ dit À QUOI le jeton sert : accès, rafraîchissement ou attente de second
	// facteur. Chaque porte n'accepte que le sien.
	//
	// Les trois jetons partageaient la même structure, le même secret et le même
	// algorithme, sans rien qui les distingue (audit 9, A-02). Un jeton de
	// rafraîchissement — trente jours — présenté en `Authorization: Bearer`
	// ouvrait donc toute l'API, survivait à la déconnexion et au changement de mot
	// de passe (la révocation en base ne le touche pas : l'authentification des
	// routes est sans état) et contournait le second facteur. Un jeton sans `typ`
	// n'est accepté nulle part : les sessions ouvertes avant cette version se
	// reconnectent une fois.
	Typ string `json:"typ,omitempty"`
	// NOTE: email is intentionally excluded from the JWT payload (nLPD data minimisation)
}

// Les trois usages d'un jeton : voir Claims.Typ.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
	TokenTypeMFA     = "mfa"
)

// newJTI generates a cryptographically random hex-encoded 16-byte token ID.
func newJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating jti: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateAccessToken creates a short-lived access JWT (default 60 min).
// A unique jti (JWT ID) is embedded to allow per-token revocation tracking.
func GenerateAccessToken(secret, userID string, isAdmin bool, ttl time.Duration) (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", err
	}
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID,
		},
		UserID:  userID,
		IsAdmin: isAdmin,
		JTI:     jti,
		Typ:     TokenTypeAccess,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateMFAChallengeToken crée le jeton d'attente entre le mot de passe et le
// second facteur.
//
// Il vit cinq minutes : le temps de sortir son téléphone, de le déverrouiller et
// de recopier six chiffres, pas davantage. Une heure comme un jeton d'accès
// ordinaire laisserait à quelqu'un qui a volé le mot de passe une heure pour
// trouver aussi le code.
//
// Il ne porte AUCUN droit. Le filtre d'authentification le refuse sur toutes les
// routes sauf celle qui l'échange contre une vraie session.
func GenerateMFAChallengeToken(secret, userID string, isAdmin bool) (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   userID,
		},
		UserID:     userID,
		IsAdmin:    isAdmin,
		JTI:        jti,
		MFAPending: true,
		Typ:        TokenTypeMFA,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// GenerateRefreshToken creates a long-lived refresh JWT.
// Returns the signed token string and the jti so the caller can store it in DB
// for revocation tracking (see refresh_tokens table).
func GenerateRefreshToken(secret, userID string, isAdmin bool, ttl time.Duration) (tokenString, jti string, err error) {
	jti, err = newJTI()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   userID,
		},
		UserID:  userID,
		IsAdmin: isAdmin,
		JTI:     jti,
		Typ:     TokenTypeRefresh,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err = token.SignedString([]byte(secret))
	if err != nil {
		return "", "", fmt.Errorf("signing refresh token: %w", err)
	}
	return tokenString, jti, nil
}

// ParseToken validates a JWT and returns its claims.
func ParseToken(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// ─── Hash chain (CO art. 957a) ────────────────────────────────────────────────

// ComputeEntryHash computes the SHA-256 hash for an audit log entry.
// Fields are JSON-encoded to prevent separator-injection collisions.
// Covers: userID, action, tableName, recordID, beforeState, afterState, ipAddress, createdAt.
func ComputeEntryHash(userID, action, tableName, recordID, beforeState, afterState, ipAddress string, createdAt time.Time) string {
	// JSON array encoding prevents field-boundary collisions (e.g. "a|b" vs "a" + "|b").
	fields := fmt.Sprintf(`[%q,%q,%q,%q,%q,%q,%q,%q]`,
		userID, action, tableName, recordID, beforeState, afterState, ipAddress,
		createdAt.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(fields))
	return hex.EncodeToString(sum[:])
}

// ChainHash returns the SHA-256 of prevHash concatenated with entryHash (CO art. 957a).
// For the first entry (prevHash == ""), returns SHA-256(entryHash) without a leading empty string,
// ensuring the chain anchor is deterministic and collision-free.
func ChainHash(prevHash, entryHash string) string {
	var sum [32]byte
	if prevHash == "" {
		sum = sha256.Sum256([]byte(entryHash))
	} else {
		sum = sha256.Sum256([]byte(prevHash + entryHash))
	}
	return hex.EncodeToString(sum[:])
}

package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

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

// ─── JWT ──────────────────────────────────────────────────────────────────────

type Claims struct {
	jwt.RegisteredClaims
	UserID  string `json:"sub"`
	IsAdmin bool   `json:"is_admin"`
	JTI     string `json:"jti"`
	// NOTE: email is intentionally excluded from the JWT payload (nLPD data minimisation)
}

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
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
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

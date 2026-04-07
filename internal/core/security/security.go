package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// HashPassword returns a bcrypt hash of the plain-text password.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword compares a bcrypt hash with a candidate password.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ─── JWT ──────────────────────────────────────────────────────────────────────

type Claims struct {
	jwt.RegisteredClaims
	UserID  string `json:"sub"`
	IsAdmin bool   `json:"is_admin"`
	// NOTE: email is intentionally excluded from the JWT payload (nLPD data minimisation)
}

// GenerateAccessToken creates a short-lived access JWT (default 60 min).
func GenerateAccessToken(secret, userID string, isAdmin bool, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID,
		},
		UserID:  userID,
		IsAdmin: isAdmin,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
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
// The hash covers: userID + action + table + recordID + afterState + createdAt.
func ComputeEntryHash(userID, action, tableName, recordID, afterState string, createdAt time.Time) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		userID, action, tableName, recordID, afterState, createdAt.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// ChainHash returns SHA-256(prevHash + entryHash), forming the audit chain.
// If prevHash is empty (first entry), SHA-256(entryHash) is returned.
func ChainHash(prevHash, entryHash string) string {
	sum := sha256.Sum256([]byte(prevHash + entryHash))
	return hex.EncodeToString(sum[:])
}

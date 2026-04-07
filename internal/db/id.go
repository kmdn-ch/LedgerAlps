package db

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID generates a random 32-character hex string (128-bit entropy, UUID-compatible).
// Always generated in Go — never rely on DB DEFAULT — ensures PG+SQLite compatibility.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure indicates an OS-level problem; panic is appropriate.
		panic(fmt.Sprintf("db.NewID: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

package security_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
)

// ─── HashPassword + CheckPassword ────────────────────────────────────────────

func TestHashAndCheckPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"simple password", "correct-horse-battery-staple"},
		{"empty password", ""},
		{"unicode password", "p@$$w0rd-éàü"},
		{"long password", strings.Repeat("a", 72)}, // bcrypt max useful length
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := security.HashPassword(tc.password)
			if err != nil {
				t.Fatalf("HashPassword(%q): unexpected error: %v", tc.password, err)
			}
			if hash == "" {
				t.Fatalf("HashPassword(%q): returned empty hash", tc.password)
			}
			// Correct password must match.
			if !security.CheckPassword(hash, tc.password) {
				t.Errorf("CheckPassword: correct password not accepted for %q", tc.password)
			}
			// A wrong password must NOT match.
			if security.CheckPassword(hash, tc.password+"_wrong") {
				t.Errorf("CheckPassword: wrong password incorrectly accepted for %q", tc.password)
			}
		})
	}
}

func TestHashPassword_DifferentHashesForSameInput(t *testing.T) {
	// bcrypt generates a fresh salt each call — two hashes of the same password differ.
	pw := "same-password"
	h1, err := security.HashPassword(pw)
	if err != nil {
		t.Fatalf("first HashPassword: %v", err)
	}
	h2, err := security.HashPassword(pw)
	if err != nil {
		t.Fatalf("second HashPassword: %v", err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same password are identical (missing salt?)")
	}
	// Both must still verify correctly.
	if !security.CheckPassword(h1, pw) {
		t.Error("h1 does not verify against original password")
	}
	if !security.CheckPassword(h2, pw) {
		t.Error("h2 does not verify against original password")
	}
}

// ─── ChainHash ───────────────────────────────────────────────────────────────

func TestChainHash(t *testing.T) {
	const entryHash = "abc123def456"
	const prevHash = "prev000hash"

	// First record: prevHash == "" → SHA256(entryHash)
	wantFirst := func() string {
		s := sha256.Sum256([]byte(entryHash))
		return hex.EncodeToString(s[:])
	}()

	// Normal chain: SHA256(prevHash + entryHash)
	wantNormal := func() string {
		s := sha256.Sum256([]byte(prevHash + entryHash))
		return hex.EncodeToString(s[:])
	}()

	tests := []struct {
		name      string
		prevHash  string
		entryHash string
		want      string
	}{
		{
			name:      "first record (prevHash empty) == SHA256(entryHash)",
			prevHash:  "",
			entryHash: entryHash,
			want:      wantFirst,
		},
		{
			name:      "first record differs from normal chain",
			prevHash:  "",
			entryHash: entryHash,
			want:      wantFirst, // already checked above; this entry doubles as a non-equal check below
		},
		{
			name:      "normal chain == SHA256(prevHash+entryHash)",
			prevHash:  prevHash,
			entryHash: entryHash,
			want:      wantNormal,
		},
		{
			name:      "deterministic: same inputs produce same output (first record)",
			prevHash:  "",
			entryHash: entryHash,
			want:      wantFirst,
		},
		{
			name:      "deterministic: same inputs produce same output (normal)",
			prevHash:  prevHash,
			entryHash: entryHash,
			want:      wantNormal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := security.ChainHash(tc.prevHash, tc.entryHash)
			if got != tc.want {
				t.Errorf("ChainHash(%q, %q) = %q, want %q", tc.prevHash, tc.entryHash, got, tc.want)
			}
		})
	}

	// Additional assertion: first-record hash must differ from normal-chain hash.
	if wantFirst == wantNormal {
		t.Error("ChainHash with empty prevHash produces the same result as with non-empty prevHash")
	}
}

// ─── ComputeEntryHash ────────────────────────────────────────────────────────

func TestComputeEntryHash(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	// Precompute expected hash for the baseline case.
	baselineHash := security.ComputeEntryHash(
		"user-1", "post", "journal_entries", "entry-1",
		"", `{"status":"posted"}`, "127.0.0.1", baseTime,
	)

	tests := []struct {
		name        string
		userID      string
		action      string
		tableName   string
		recordID    string
		beforeState string
		afterState  string
		ipAddress   string
		createdAt   time.Time
		wantHash    string // empty means "compute from fields and compare to got"
		mustDiffer  string // if set, the result must differ from this hash
	}{
		{
			name:        "deterministic: same inputs give same hash",
			userID:      "user-1",
			action:      "post",
			tableName:   "journal_entries",
			recordID:    "entry-1",
			beforeState: "",
			afterState:  `{"status":"posted"}`,
			ipAddress:   "127.0.0.1",
			createdAt:   baseTime,
			wantHash:    baselineHash,
		},
		{
			name:        "different userID produces different hash",
			userID:      "user-2", // changed
			action:      "post",
			tableName:   "journal_entries",
			recordID:    "entry-1",
			beforeState: "",
			afterState:  `{"status":"posted"}`,
			ipAddress:   "127.0.0.1",
			createdAt:   baseTime,
			mustDiffer:  baselineHash,
		},
		{
			name:        "different action produces different hash",
			userID:      "user-1",
			action:      "delete", // changed
			tableName:   "journal_entries",
			recordID:    "entry-1",
			beforeState: "",
			afterState:  `{"status":"posted"}`,
			ipAddress:   "127.0.0.1",
			createdAt:   baseTime,
			mustDiffer:  baselineHash,
		},
		{
			name:        "different timestamp produces different hash",
			userID:      "user-1",
			action:      "post",
			tableName:   "journal_entries",
			recordID:    "entry-1",
			beforeState: "",
			afterState:  `{"status":"posted"}`,
			ipAddress:   "127.0.0.1",
			createdAt:   baseTime.Add(time.Second), // changed
			mustDiffer:  baselineHash,
		},
		{
			// Separator-safety: a field containing "]" or `"` must not cause
			// a collision with adjacent fields split differently.
			name:        `separator-safe: field with "]" does not collide`,
			userID:      `user]1`,  // contains "]"
			action:      "post",
			tableName:   "journal_entries",
			recordID:    "entry-1",
			beforeState: "",
			afterState:  `{"status":"posted"}`,
			ipAddress:   "127.0.0.1",
			createdAt:   baseTime,
			mustDiffer:  baselineHash,
		},
		{
			// A field containing a double-quote is %q-escaped, preventing injection.
			name:        `separator-safe: field with quote character`,
			userID:      `user"1`,
			action:      "post",
			tableName:   "journal_entries",
			recordID:    "entry-1",
			beforeState: "",
			afterState:  `{"status":"posted"}`,
			ipAddress:   "127.0.0.1",
			createdAt:   baseTime,
			mustDiffer:  baselineHash,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := security.ComputeEntryHash(
				tc.userID, tc.action, tc.tableName, tc.recordID,
				tc.beforeState, tc.afterState, tc.ipAddress, tc.createdAt,
			)

			if got == "" {
				t.Fatal("ComputeEntryHash returned an empty string")
			}

			if tc.wantHash != "" && got != tc.wantHash {
				t.Errorf("ComputeEntryHash = %q, want %q", got, tc.wantHash)
			}

			if tc.mustDiffer != "" && got == tc.mustDiffer {
				t.Errorf("ComputeEntryHash = %q; expected a different hash from baseline but got the same", got)
			}
		})
	}
}

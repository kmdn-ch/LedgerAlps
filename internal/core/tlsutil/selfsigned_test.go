package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// The distinction the whole feature rests on. An empty host is not loopback:
// it binds every interface, which is what LedgerAlps used to do by default —
// serving the accounts of a laptop to whatever network it was on.
func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true,
		"::1":       true,
		"localhost": true,
		"127.0.0.5": true,

		"":             false, // every interface
		"0.0.0.0":      false,
		"::":           false,
		"192.168.1.4":  false,
		"10.0.0.1":     false,
		"compta.local": false,
	}
	for host, want := range cases {
		if got := IsLoopback(host); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestGeneratesAUsableCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, err := EnsureSelfSigned(dir, []string{"localhost", "127.0.0.1", "192.168.1.4"})
	if err != nil {
		t.Fatalf("EnsureSelfSigned: %v", err)
	}

	// The real test of a certificate is whether a TLS stack accepts the pair.
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("the generated pair is not usable by crypto/tls: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Covering the address a colleague actually types is the point; a
	// certificate valid only for "localhost" would warn on every LAN visit.
	if err := leaf.VerifyHostname("192.168.1.4"); err != nil {
		t.Errorf("the certificate does not cover the LAN address: %v", err)
	}
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Errorf("the certificate does not cover localhost: %v", err)
	}

	if leaf.NotAfter.Before(time.Now().Add(365 * 24 * time.Hour)) {
		t.Error("validity under a year would train users to click through a fresh warning")
	}
	// A clock a few minutes behind must not make the certificate "not yet valid".
	if !leaf.NotBefore.Before(time.Now()) {
		t.Error("NotBefore leaves no tolerance for a skewed clock")
	}
}

// Regenerating on every start would invalidate the browser exception the user
// granted, and teach them to dismiss the warning without reading it.
func TestExistingPairIsReused(t *testing.T) {
	dir := t.TempDir()
	certPath, _, err := EnsureSelfSigned(dir, []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := EnsureSelfSigned(dir, []string{"localhost"}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("the certificate was regenerated; the browser exception would break at every start")
	}
}

// The private key is the whole protection, so where file modes mean something
// it must be readable by its owner alone.
//
// Windows ignores Unix permission bits — Go reports 0666 whatever we ask for —
// and the real mechanism there is ACLs. What protects the key on Windows is its
// location: %APPDATA% is per-user and already closed to other accounts. Hence
// the skip rather than a weaker assertion that would pass everywhere and prove
// nothing; CI runs on Linux, where this bites.
func TestPrivateKeyIsNotWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows n'applique pas les bits de permission Unix ; la clé est protégée par %APPDATA%")
	}
	dir := t.TempDir()
	_, keyPath, err := EnsureSelfSigned(dir, []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode := st.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("key mode = %04o, want no access for group or others", mode)
	}
}

func TestLocalHostnamesAlwaysCoversLoopback(t *testing.T) {
	hosts := LocalHostnames()
	need := map[string]bool{"localhost": false, "127.0.0.1": false}
	for _, h := range hosts {
		if _, ok := need[h]; ok {
			need[h] = true
		}
	}
	for h, found := range need {
		if !found {
			t.Errorf("%q missing from LocalHostnames() — a certificate must at least cover this machine", h)
		}
	}
}

func TestGenerationFailsCleanlyOnAnUnwritablePath(t *testing.T) {
	// A file where the directory should be: MkdirAll must fail, and nothing
	// half-written may be left claiming to be a certificate.
	base := t.TempDir()
	blocker := filepath.Join(base, "tls")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureSelfSigned(blocker, []string{"localhost"}); err == nil {
		t.Error("generation reported success with nowhere to write")
	}
}

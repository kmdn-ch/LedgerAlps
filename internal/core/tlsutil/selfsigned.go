// Package tlsutil generates the self-signed certificate LedgerAlps uses when it
// is reached from another machine and no certificate was supplied.
//
// A self-signed certificate makes browsers complain, and that is a real cost.
// It is still the right default: the alternative is serving the login password,
// the session token and the backup passphrase in clear on the office network,
// where anyone with a packet capture reads them. A warning the user clicks
// through once beats credentials on the wire every day (LPD art. 8, OPDo art. 3
// al. 1 let. c — protecting against unauthorised use through transmission).
//
// Anyone wanting a certificate browsers accept supplies TLS_CERT and TLS_KEY,
// from an internal CA or a reverse proxy. This is the floor, not the ceiling.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Validity is deliberately long. This certificate protects a local network and
// is not part of any trust chain; forcing a yearly regeneration would only
// teach people to click through a second warning.
const Validity = 10 * 365 * 24 * time.Hour

// EnsureSelfSigned returns paths to a certificate and key in dir, generating
// them when absent. An existing pair is reused so the browser exception a user
// granted survives restarts — regenerating on every start would train them to
// dismiss the warning without reading it.
func EnsureSelfSigned(dir string, hosts []string) (certPath, keyPath string, err error) {
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	if fileExists(certPath) && fileExists(keyPath) {
		return certPath, keyPath, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("création du dossier TLS: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("génération de la clé: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("numéro de série: %w", err)
	}

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"LedgerAlps"},
			CommonName:   "LedgerAlps (certificat auto-signé)",
		},
		NotBefore: time.Now().Add(-time.Hour), // tolerate a skewed clock
		NotAfter:  time.Now().Add(Validity),
		// PAS de KeyUsageCertSign, PAS de IsCA.
		//
		// Ce certificat sert UNE machine. Le rendre capable de signer en
		// faisait une autorité de certification sans contrainte de nom : le
		// geste évident pour faire taire l'avertissement du navigateur est de
		// l'importer dans « Autorités de certification racines de confiance »,
		// et à partir de là, la clé posée dans %APPDATA% — en 0600, droit
		// consultatif sur Windows, donc lisible par tout processus du même
		// compte — permettait de fabriquer un certificat valable pour
		// n'importe quel domaine. Le rayon d'action passait d'un fichier local
		// à l'interception de toute la navigation.
		//
		// Une feuille signée par elle-même produit exactement le même
		// avertissement et la même exception à poser, sans ce pouvoir-là.
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("création du certificat: %w", err)
	}

	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("encodage de la clé: %w", err)
	}
	// 0600: the private key is the whole protection. On Windows this is
	// advisory, which is why it never leaves the application data directory.
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		_ = os.Remove(certPath)
		return "", "", err
	}
	return certPath, keyPath, nil
}

// LocalHostnames lists the names and addresses this machine answers to, so the
// certificate covers however a colleague reaches it — by IP or by hostname.
func LocalHostnames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}

	add("localhost")
	add("127.0.0.1")
	add("::1")
	if name, err := os.Hostname(); err == nil {
		add(name)
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			add(ipnet.IP.String())
		}
	}
	return out
}

// IsLoopback reports whether a host reaches only this machine. An empty host
// means "every interface", which is emphatically not loopback — that default
// is what used to expose LedgerAlps to the whole network without anyone asking.
func IsLoopback(host string) bool {
	switch host {
	case "localhost":
		return true
	case "", "0.0.0.0", "::":
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func writePEM(path, blockType string, der []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("écriture de %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		return fmt.Errorf("encodage PEM de %s: %w", filepath.Base(path), err)
	}
	return f.Close()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

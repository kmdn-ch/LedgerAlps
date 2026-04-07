#!/bin/bash
# LedgerAlps — Génération du certificat TLS auto-signé pour développement local
# En production : remplacer par Let's Encrypt (certbot) ou un certificat commercial

set -e

CERTS_DIR="$(dirname "$0")/../docker/nginx/certs"
mkdir -p "$CERTS_DIR"

echo "Génération du certificat TLS pour ledgeralps.local…"

openssl req -x509 -nodes -days 365 \
  -newkey rsa:2048 \
  -keyout "$CERTS_DIR/ledgeralps.key" \
  -out    "$CERTS_DIR/ledgeralps.crt" \
  -subj   "/C=CH/ST=Vaud/L=Lausanne/O=LedgerAlps/CN=ledgeralps.local" \
  -addext "subjectAltName=DNS:ledgeralps.local,DNS:localhost,IP:127.0.0.1"

echo "Certificat généré :"
echo "  → $CERTS_DIR/ledgeralps.crt"
echo "  → $CERTS_DIR/ledgeralps.key"
echo ""
echo "Pour faire confiance au certificat (macOS) :"
echo "  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain $CERTS_DIR/ledgeralps.crt"
echo ""
echo "Ajoutez 127.0.0.1 ledgeralps.local dans /etc/hosts"

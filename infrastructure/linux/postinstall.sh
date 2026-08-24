#!/usr/bin/env bash
# Post-install: write env template if missing, enable and start service
set -euo pipefail

ENV_EXAMPLE="/etc/ledgeralps/ledgeralps.env.example"
if [ ! -f "$ENV_EXAMPLE" ]; then
    # umask 027 : le gabarit devient le fichier de configuration reel par une
    # simple copie, et une copie reproduit les droits de la source. Le laisser
    # naitre en 0644 revenait a publier JWT_SECRET a tout compte local.
    ( umask 027
      cat > "$ENV_EXAMPLE" <<'EOF'
# LedgerAlps environment configuration
# Copy this file to /etc/ledgeralps/ledgeralps.env

# REQUIRED: Generate with: openssl rand -hex 32
JWT_SECRET=CHANGE_ME

PORT=8000
SQLITE_PATH=/var/lib/ledgeralps/ledgeralps.db
ALLOWED_ORIGINS=http://localhost:5173
LOG_LEVEL=INFO
DEBUG=false
EOF
    )
    chown root:ledgeralps "$ENV_EXAMPLE"
    chmod 640 "$ENV_EXAMPLE"
    echo "Created $ENV_EXAMPLE — copy to ledgeralps.env and set JWT_SECRET"
    echo "  install -o root -g ledgeralps -m 640 $ENV_EXAMPLE /etc/ledgeralps/ledgeralps.env"
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable ledgeralps
    echo "LedgerAlps service enabled. Start with: systemctl start ledgeralps"
fi

#!/usr/bin/env bash
# Post-install: write env template if missing, enable and start service
set -e

ENV_EXAMPLE="/etc/ledgeralps/ledgeralps.env.example"
if [ ! -f "$ENV_EXAMPLE" ]; then
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
    echo "Created $ENV_EXAMPLE — copy to ledgeralps.env and set JWT_SECRET"
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable ledgeralps
    echo "LedgerAlps service enabled. Start with: systemctl start ledgeralps"
fi

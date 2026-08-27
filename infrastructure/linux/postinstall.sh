#!/usr/bin/env bash
# ATTENTION : FICHIER NON BRANCHE - conserve comme reference, jamais execute.
#
# Ce fichier appartient à l'empaquetage .deb/.rpm, ABANDONNÉ depuis la v1.4.5.
# Rien ne l'exécute : ni .goreleaser.yaml, ni un workflow GitHub, ni la
# documentation. Le chemin d'installation Linux vivant est `scripts/install.sh`,
# qui embarque sa propre unité systemd et sa propre gestion des permissions.
#
# Il reste ici parce qu'il documente une arborescence FHS qui fonctionnait, et
# il a déjà servi de référence — le correctif de permissions 0755→0750 a été
# recopié depuis `preinstall.sh` vers `scripts/install.sh`. Mais rien ne
# garantit qu'une correction future faite là-bas soit reportée ici.
#
# AVANT DE LE RÉUTILISER : comparez-le à `scripts/install.sh`, qui fait foi.
#
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

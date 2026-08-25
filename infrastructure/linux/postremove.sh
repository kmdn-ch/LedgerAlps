#!/usr/bin/env bash
# Post-remove: reload systemd
# -euo pipefail, comme preinstall.sh et postinstall.sh. Ces deux scripts
# n'emploient aucune variable et terminent tout par « || true », donc
# l'exposition est nulle : c'est la coherence qui est en jeu, pour que
# personne n'ait a se demander pourquoi deux scripts voisins different.
set -euo pipefail

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload 2>/dev/null || true
fi

echo "LedgerAlps uninstalled. Data preserved in /var/lib/ledgeralps and /etc/ledgeralps."

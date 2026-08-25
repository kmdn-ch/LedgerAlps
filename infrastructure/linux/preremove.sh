#!/usr/bin/env bash
# Pre-remove: stop and disable the service before uninstalling
# -euo pipefail, comme preinstall.sh et postinstall.sh. Ces deux scripts
# n'emploient aucune variable et terminent tout par « || true », donc
# l'exposition est nulle : c'est la coherence qui est en jeu, pour que
# personne n'ait a se demander pourquoi deux scripts voisins different.
set -euo pipefail

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop ledgeralps 2>/dev/null || true
    systemctl disable ledgeralps 2>/dev/null || true
fi

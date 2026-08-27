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

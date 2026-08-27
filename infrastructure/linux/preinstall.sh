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
# Pre-install: create system user and directories
set -euo pipefail

if ! id ledgeralps >/dev/null 2>&1; then
    useradd --system --no-create-home \
            --shell /usr/sbin/nologin \
            --home-dir /var/lib/ledgeralps \
            ledgeralps
fi

mkdir -p /var/lib/ledgeralps /var/log/ledgeralps /etc/ledgeralps
chown ledgeralps:ledgeralps /var/lib/ledgeralps /var/log/ledgeralps
chmod 750 /var/lib/ledgeralps /var/log/ledgeralps
# 750, pas 755 : ce repertoire porte ledgeralps.env, donc JWT_SECRET, la cle
# qui signe les jetons de session. En 755, tout compte local de la machine la
# lisait et pouvait forger un jeton administrateur — le durcissement de l'unite
# systemd protegeait alors tout sauf la cle qui ouvre l'application.
chown root:ledgeralps /etc/ledgeralps
chmod 750 /etc/ledgeralps

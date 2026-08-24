#!/usr/bin/env bash
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

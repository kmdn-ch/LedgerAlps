#!/usr/bin/env bash
# Pre-install: create system user and directories
set -e

if ! id ledgeralps >/dev/null 2>&1; then
    useradd --system --no-create-home \
            --shell /usr/sbin/nologin \
            --home-dir /var/lib/ledgeralps \
            ledgeralps
fi

mkdir -p /var/lib/ledgeralps /var/log/ledgeralps /etc/ledgeralps
chown ledgeralps:ledgeralps /var/lib/ledgeralps /var/log/ledgeralps
chmod 750 /var/lib/ledgeralps /var/log/ledgeralps
chmod 755 /etc/ledgeralps

#!/usr/bin/env bash
# Pre-remove: stop and disable the service before uninstalling
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop ledgeralps 2>/dev/null || true
    systemctl disable ledgeralps 2>/dev/null || true
fi

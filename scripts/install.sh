#!/usr/bin/env bash
# =============================================================================
# LedgerAlps — Linux / macOS installer
#
# Usage (one-liner):
#   curl -fsSL https://raw.githubusercontent.com/kmdn-ch/ledgeralps/main/scripts/install.sh | bash
#
# Override version:
#   LEDGERALPS_VERSION=v1.2.3 bash install.sh
#
# Override install dir:
#   INSTALL_DIR=/opt/ledgeralps bash install.sh
# =============================================================================
set -euo pipefail

REPO="kmdn-ch/ledgeralps"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DATA_DIR:-/etc/ledgeralps}"
SYSTEMD_DIR="/etc/systemd/system"
SERVICE_FILE="$SYSTEMD_DIR/ledgeralps.service"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${BLUE}[ledgeralps]${NC} $*"; }
success() { echo -e "${GREEN}[ledgeralps]${NC} $*"; }
warn()    { echo -e "${YELLOW}[ledgeralps] WARN:${NC} $*"; }
error()   { echo -e "${RED}[ledgeralps] ERROR:${NC} $*" >&2; exit 1; }

# --------------------------------------------------------------------------- #
# Detect OS and architecture                                                  #
# --------------------------------------------------------------------------- #
detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      error "Unsupported OS: $OS. Use scripts/install.ps1 on Windows." ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             error "Unsupported architecture: $ARCH" ;;
  esac

  info "Detected platform: $OS/$ARCH"
}

# --------------------------------------------------------------------------- #
# Resolve latest version from GitHub API                                     #
# --------------------------------------------------------------------------- #
resolve_version() {
  if [ -n "${LEDGERALPS_VERSION:-}" ]; then
    VERSION="$LEDGERALPS_VERSION"
    info "Using specified version: $VERSION"
    return
  fi

  info "Fetching latest release version from GitHub…"
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"

  if command -v curl >/dev/null 2>&1; then
    RESPONSE="$(curl -fsSL "$api_url" 2>/dev/null)"
  elif command -v wget >/dev/null 2>&1; then
    RESPONSE="$(wget -qO- "$api_url" 2>/dev/null)"
  else
    error "Neither curl nor wget is available. Install one and retry."
  fi

  # Parse tag_name — try jq first, fall back to grep/sed
  if command -v jq >/dev/null 2>&1; then
    VERSION="$(echo "$RESPONSE" | jq -r '.tag_name')"
  else
    VERSION="$(echo "$RESPONSE" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  fi

  [ -n "$VERSION" ] || error "Could not determine latest version. Set LEDGERALPS_VERSION manually."
  info "Latest version: $VERSION"
}

# --------------------------------------------------------------------------- #
# Download and install binaries                                               #
# --------------------------------------------------------------------------- #
install_binaries() {
  local archive="ledgeralps_${VERSION}_${OS}_${ARCH}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"
  local tmp_dir
  tmp_dir="$(mktemp -d)"

  info "Downloading $archive…"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$tmp_dir/$archive"
  else
    wget -qO "$tmp_dir/$archive" "$url"
  fi

  info "Extracting…"
  tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"

  info "Installing binaries to $INSTALL_DIR…"
  install -m 0755 "$tmp_dir/ledgeralps-server" "$INSTALL_DIR/ledgeralps-server"
  install -m 0755 "$tmp_dir/ledgeralps-cli"    "$INSTALL_DIR/ledgeralps-cli"

  rm -rf "$tmp_dir"
  success "Installed ledgeralps-server and ledgeralps-cli to $INSTALL_DIR"
}

# --------------------------------------------------------------------------- #
# Write env file template                                                     #
# --------------------------------------------------------------------------- #
write_env_template() {
  mkdir -p "$DATA_DIR"
  if [ ! -f "$DATA_DIR/ledgeralps.env" ]; then
    cat > "$DATA_DIR/ledgeralps.env.example" <<'EOF'
# LedgerAlps environment configuration
# Copy this file to ledgeralps.env and fill in the values.

# REQUIRED: Generate with: openssl rand -hex 32
JWT_SECRET=CHANGE_ME_TO_A_32_CHAR_MINIMUM_SECRET

# HTTP port (default: 8000)
PORT=8000

# SQLite database path (default — good for single-server deployments)
SQLITE_PATH=/var/lib/ledgeralps/ledgeralps.db

# OR use PostgreSQL (comment out SQLITE_PATH and uncomment below)
# POSTGRES_DSN=postgres://user:password@localhost:5432/ledgeralps?sslmode=disable

# CORS — allowed frontend origins (comma-separated)
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000

# Logging
LOG_LEVEL=INFO
DEBUG=false
EOF
    info "Created env template at $DATA_DIR/ledgeralps.env.example"
  fi
}

# --------------------------------------------------------------------------- #
# Create systemd service (Linux only)                                        #
# --------------------------------------------------------------------------- #
install_systemd() {
  [ "$OS" = "linux" ] || return 0
  command -v systemctl >/dev/null 2>&1 || { warn "systemd not found, skipping service install."; return 0; }

  # Create system user
  if ! id ledgeralps >/dev/null 2>&1; then
    info "Creating system user 'ledgeralps'…"
    useradd --system --no-create-home --shell /usr/sbin/nologin --home-dir /var/lib/ledgeralps ledgeralps
  fi

  mkdir -p /var/lib/ledgeralps /var/log/ledgeralps
  chown ledgeralps:ledgeralps /var/lib/ledgeralps /var/log/ledgeralps

  info "Installing systemd service…"
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=LedgerAlps Swiss Accounting Server
Documentation=https://github.com/kmdn-ch/ledgeralps
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ledgeralps
Group=ledgeralps
WorkingDirectory=/var/lib/ledgeralps
EnvironmentFile=-/etc/ledgeralps/ledgeralps.env
ExecStart=$INSTALL_DIR/ledgeralps-server
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5s
KillMode=mixed
TimeoutStopSec=30
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=ledgeralps
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/ledgeralps /var/log/ledgeralps

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable ledgeralps
  success "systemd service installed and enabled"
}

# --------------------------------------------------------------------------- #
# Print next steps                                                            #
# --------------------------------------------------------------------------- #
print_next_steps() {
  echo ""
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}  LedgerAlps ${VERSION} installed successfully!${NC}"
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo "  NEXT STEPS:"
  echo ""
  echo "  1. Generate a strong JWT secret:"
  echo "       export JWT_SECRET=\$(openssl rand -hex 32)"
  echo ""
  echo "  2. Create your config file:"
  echo "       cp $DATA_DIR/ledgeralps.env.example $DATA_DIR/ledgeralps.env"
  echo "       # Edit $DATA_DIR/ledgeralps.env and set JWT_SECRET"
  echo ""
  if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
    echo "  3. Start the service:"
    echo "       sudo systemctl start ledgeralps"
    echo ""
    echo "  4. Check status:"
    echo "       sudo systemctl status ledgeralps"
    echo "       sudo journalctl -u ledgeralps -f"
  else
    echo "  3. Start the server:"
    echo "       export JWT_SECRET=<your-secret>"
    echo "       ledgeralps-server"
  fi
  echo ""
  echo "  5. Create your admin user:"
  echo "       ledgeralps-cli bootstrap --email=admin@example.com --password=yourpassword"
  echo ""
  echo "  6. Open http://localhost:8000"
  echo ""
}

# --------------------------------------------------------------------------- #
# Main                                                                        #
# --------------------------------------------------------------------------- #
main() {
  echo ""
  info "LedgerAlps installer"
  echo ""

  [ "$(id -u)" -eq 0 ] || error "This installer must be run as root (or with sudo)."

  detect_platform
  resolve_version
  install_binaries
  write_env_template
  install_systemd
  print_next_steps
}

main "$@"

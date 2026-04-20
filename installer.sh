#!/usr/bin/env bash
set -euo pipefail

REPO="DiyRex/Neurix"
BINARY="ollama_exporter"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="ollama-exporter"
OLLAMA_HOST="${OLLAMA_HOST:-http://localhost:11434}"
LISTEN_ADDRESS="${LISTEN_ADDRESS:-}"  # empty = auto (9101-9160)

# ── colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()     { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ── detect OS ────────────────────────────────────────────────────────────────
detect_os() {
  case "$(uname -s)" in
    Linux)  echo "linux"   ;;
    Darwin) echo "darwin"  ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) die "Unsupported OS: $(uname -s)" ;;
  esac
}

# ── detect arch ──────────────────────────────────────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "amd64" ;;
    aarch64|arm64)  echo "arm64" ;;
    *) die "Unsupported architecture: $(uname -m)" ;;
  esac
}

# ── fetch latest release tag ─────────────────────────────────────────────────
latest_version() {
  local tag
  tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
  [[ -z "$tag" ]] && die "Could not fetch latest release from GitHub."
  echo "$tag"
}

# ── check dependencies ───────────────────────────────────────────────────────
check_deps() {
  for cmd in curl tar; do
    command -v "$cmd" &>/dev/null || die "'$cmd' is required but not installed."
  done
}

# ── install binary ───────────────────────────────────────────────────────────
install_binary() {
  local os="$1" arch="$2" version="$3"
  local bare_version="${version#v}"   # strip leading v if present
  local archive ext url tmpdir

  if [[ "$os" == "windows" ]]; then
    ext="zip"
  else
    ext="tar.gz"
  fi

  archive="${BINARY}_${bare_version}_${os}_${arch}.${ext}"
  url="https://github.com/${REPO}/releases/download/${version}/${archive}"

  info "Downloading ${archive} ..."
  tmpdir=$(mktemp -d)

  curl -fsSL "$url" -o "${tmpdir}/${archive}" || {
    rm -rf "$tmpdir"
    die "Download failed. Check that release ${version} exists for ${os}/${arch}."
  }

  info "Extracting ..."
  if [[ "$ext" == "tar.gz" ]]; then
    tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
  else
    command -v unzip &>/dev/null || { rm -rf "$tmpdir"; die "'unzip' is required for Windows archives."; }
    unzip -q "${tmpdir}/${archive}" -d "$tmpdir"
  fi

  local bin_src="${tmpdir}/${BINARY}"
  [[ -f "$bin_src" ]] || bin_src=$(find "$tmpdir" -name "$BINARY" -type f | head -1)
  if [[ ! -f "$bin_src" ]]; then
    rm -rf "$tmpdir"
    die "Binary not found in archive."
  fi

  if [[ "$os" == "linux" || "$os" == "darwin" ]]; then
    if [[ -w "$INSTALL_DIR" ]]; then
      mv "$bin_src" "${INSTALL_DIR}/${BINARY}"
    else
      sudo mv "$bin_src" "${INSTALL_DIR}/${BINARY}"
    fi
    sudo chmod +x "${INSTALL_DIR}/${BINARY}"
  else
    INSTALL_DIR="."
    mv "$bin_src" "./${BINARY}.exe"
    warn "Windows detected: binary placed in current directory as ${BINARY}.exe"
  fi

  rm -rf "$tmpdir"
  success "Binary installed to ${INSTALL_DIR}/${BINARY}"
}

# ── systemd service (Linux only) ─────────────────────────────────────────────
install_systemd() {
  [[ "$(detect_os)" != "linux" ]] && return
  command -v systemctl &>/dev/null || { warn "systemd not found, skipping service setup."; return; }

  local listen_arg=""
  [[ -n "$LISTEN_ADDRESS" ]] && listen_arg="--web.listen-address=${LISTEN_ADDRESS}"

  info "Installing systemd service: ${SERVICE_NAME} ..."

  sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null << EOF
[Unit]
Description=Neurix Ollama NVIDIA GPU & Temperature Stats Exporter
Documentation=https://github.com/${REPO}
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY} --ollama.host=${OLLAMA_HOST} ${listen_arg}
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable "${SERVICE_NAME}"
  sudo systemctl restart "${SERVICE_NAME}"

  sleep 2
  if systemctl is-active --quiet "${SERVICE_NAME}"; then
    success "Service ${SERVICE_NAME} is running."
  else
    warn "Service may not have started. Check: sudo journalctl -u ${SERVICE_NAME} -n 20"
  fi
}

# ── launchd plist (macOS only) ───────────────────────────────────────────────
install_launchd() {
  [[ "$(detect_os)" != "darwin" ]] && return

  local plist_dir="$HOME/Library/LaunchAgents"
  local plist="${plist_dir}/com.neurix.ollama-exporter.plist"
  local listen_arg=""
  [[ -n "$LISTEN_ADDRESS" ]] && listen_arg="<string>--web.listen-address=${LISTEN_ADDRESS}</string>"

  mkdir -p "$plist_dir"
  info "Installing launchd agent ..."

  cat > "$plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.neurix.ollama-exporter</string>
  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/${BINARY}</string>
    <string>--ollama.host=${OLLAMA_HOST}</string>
    ${listen_arg}
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/ollama-exporter.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/ollama-exporter.log</string>
</dict>
</plist>
EOF

  launchctl unload "$plist" 2>/dev/null || true
  launchctl load "$plist"
  success "launchd agent loaded. Logs: /tmp/ollama-exporter.log"
}

# ── verify ───────────────────────────────────────────────────────────────────
verify_install() {
  local port="${LISTEN_ADDRESS#:}"
  [[ -z "$port" ]] && port="9101"

  info "Waiting for exporter to come up on port ${port} ..."
  local retries=10
  while (( retries-- > 0 )); do
    if curl -sf "http://localhost:${port}/metrics" | grep -q "ollama_up"; then
      success "Exporter is UP → http://localhost:${port}/metrics"
      echo ""
      echo "  Sample metrics:"
      curl -sf "http://localhost:${port}/metrics" | grep -E "^(ollama_up|ollama_version_info|nvidia_smi_temperature_gpu)" | head -10
      return
    fi
    sleep 1
  done
  warn "Could not verify metrics endpoint. The exporter may still be starting."
  warn "Try: curl http://localhost:${port}/metrics"
}

# ── uninstall ────────────────────────────────────────────────────────────────
uninstall() {
  info "Uninstalling ${BINARY} ..."
  local os
  os=$(detect_os)

  if [[ "$os" == "linux" ]] && command -v systemctl &>/dev/null; then
    sudo systemctl stop "${SERVICE_NAME}"   2>/dev/null || true
    sudo systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
    sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    sudo systemctl daemon-reload
  elif [[ "$os" == "darwin" ]]; then
    local plist="$HOME/Library/LaunchAgents/com.neurix.ollama-exporter.plist"
    launchctl unload "$plist" 2>/dev/null || true
    rm -f "$plist"
  fi

  sudo rm -f "${INSTALL_DIR}/${BINARY}"
  success "Uninstalled."
}

# ── main ─────────────────────────────────────────────────────────────────────
usage() {
  echo "Usage: $0 [install|uninstall|upgrade]"
  echo ""
  echo "Environment variables:"
  echo "  OLLAMA_HOST       Ollama API URL  (default: http://localhost:11434)"
  echo "  LISTEN_ADDRESS    Metrics port    (default: auto-select 9101-9160)"
  echo ""
  echo "Examples:"
  echo "  sudo bash installer.sh"
  echo "  LISTEN_ADDRESS=:9200 sudo bash installer.sh"
  echo "  sudo bash installer.sh uninstall"
}

main() {
  local cmd="${1:-install}"

  case "$cmd" in
    install|upgrade)
      check_deps
      OS=$(detect_os)
      ARCH=$(detect_arch)
      VERSION=$(latest_version)

      echo ""
      echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
      echo -e "${BLUE}  Neurix Ollama Exporter — Installer${NC}"
      echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
      info "OS:       $OS"
      info "Arch:     $ARCH"
      info "Version:  $VERSION"
      info "Host:     $OLLAMA_HOST"
      if [[ -n "$LISTEN_ADDRESS" ]]; then info "Listen:   $LISTEN_ADDRESS"; else info "Listen:   auto (9101-9160)"; fi
      echo ""

      install_binary "$OS" "$ARCH" "$VERSION"
      install_systemd
      install_launchd
      verify_install
      ;;
    uninstall)
      uninstall
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      die "Unknown command: $cmd. Use install, uninstall, or upgrade."
      ;;
  esac
}

main "$@"

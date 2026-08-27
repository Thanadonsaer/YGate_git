#!/bin/sh
set -u

SERVICE=chpp-middleware
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

run_priv() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    printf "This action needs root. Run the menu as root; sudo is not installed on RutOS.\n" >&2
    return 1
  fi
}

has_systemd() { command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; }
if has_systemd; then INSTALL_DIR=/opt/chpp-middleware; else INSTALL_DIR="$ROOT/runtime"; fi
INSTALL_BIN="$INSTALL_DIR/middleware"
INSTALL_LICENSE="$INSTALL_DIR/license.json"

service_action() {
  action="$1"
  if has_systemd; then
    run_priv systemctl "$action" "$SERVICE"
  elif [ -x "/etc/init.d/$SERVICE" ]; then
    run_priv "/etc/init.d/$SERVICE" "$action"
  else
    printf "Service is not installed yet.\n"
    return 1
  fi
}

service_status() {
  if has_systemd; then
    run_priv systemctl status "$SERVICE" --no-pager
  elif [ -x "/etc/init.d/$SERVICE" ]; then
    status_output="$(run_priv "/etc/init.d/$SERVICE" status 2>&1 || true)"
    printf "%s\n" "$status_output"
    case "$status_output" in
      *running*|*Running*) printf "Service is running.\n" ;;
      *)
        if command -v pidof >/dev/null 2>&1 && pidof middleware >/dev/null 2>&1; then
          printf "Service is running.\n"
        else
          printf "Service is stopped.\n"
        fi
        ;;
    esac
  else
    printf "Service is not installed yet.\n"
  fi
}

pause() {
  printf "\nPress Enter to continue..."
  read -r _
}

binary_path() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) bin="$ROOT/build/middleware-linux-amd64" ;;
    armv7l|armv6l|armhf) bin="$ROOT/build/middleware-linux-armv7" ;;
    aarch64|arm64) bin="$ROOT/build/middleware-linux-arm64" ;;
    mips|mipsel|mipsle) bin="$ROOT/build/middleware-linux-mipsle" ;;
    mips64|mips64el) bin="$ROOT/build/middleware-linux-mips64el" ;;
    *)
      printf "Unsupported Linux architecture: %s (supported: amd64, armv7, arm64, mips)\n" "$arch" >&2
      return 1
      ;;
  esac
  if [ -x "$INSTALL_BIN" ]; then
    bin="$INSTALL_BIN"
  fi
  printf "%s" "$bin"
}

license_file() {
  if [ -f "$INSTALL_LICENSE" ]; then
    printf "%s" "$INSTALL_LICENSE"
  else
    printf "%s" "$ROOT/license.json"
  fi
}

show_machine_id() {
  bin="$(binary_path)" || return 1
  if [ ! -f "$bin" ]; then
    printf "Binary file not found: %s\n" "$bin"
    return 1
  fi
  if [ ! -x "$bin" ]; then
    chmod +x "$bin" 2>/dev/null || true
  fi
  if [ ! -x "$bin" ]; then
    printf "Binary exists but is not executable: %s\n" "$bin"
    printf "Run: chmod +x '%s' (also check that the drive is not mounted with noexec)\n" "$bin"
    return 1
  fi
  "$bin" -machine-id
}

install_service() {
  if [ ! -f "$ROOT/deploy/install-systemd.sh" ]; then
    printf "Missing installer: %s\n" "$ROOT/deploy/install-systemd.sh"
    return
  fi
  sh "$ROOT/deploy/install-systemd.sh"
}

uninstall_service() {
  if has_systemd; then
    run_priv systemctl stop "$SERVICE" 2>/dev/null || true
    run_priv systemctl disable "$SERVICE" 2>/dev/null || true
    run_priv rm -f "/etc/systemd/system/$SERVICE.service"
    run_priv systemctl daemon-reload
    run_priv systemctl reset-failed
  elif [ -x "/etc/init.d/$SERVICE" ]; then
    run_priv "/etc/init.d/$SERVICE" stop 2>/dev/null || true
    run_priv "/etc/init.d/$SERVICE" disable 2>/dev/null || true
    run_priv rm -f "/etc/init.d/$SERVICE"
  fi
  printf "Service removed. Database, license and /opt data were kept.\n"
}

activate_license() {
  bin="$(binary_path)" || return 1
  license="$(license_file)"
  if [ ! -f "$bin" ]; then
    printf "Binary file not found: %s\n" "$bin"
    return 1
  fi
  if [ ! -x "$bin" ]; then
    chmod +x "$bin" 2>/dev/null || true
  fi
  if [ ! -x "$bin" ]; then
    printf "Binary exists but is not executable: %s\n" "$bin"
    printf "Run: chmod +x '%s' (also check that the drive is not mounted with noexec)\n" "$bin"
    return 1
  fi
  printf "License token: "
  read -r token
  if [ -z "$token" ]; then
    printf "License token is required.\n"
    return 1
  fi
  "$bin" -activate-license "$token" -license-file "$license"
}

while true; do
  clear
  printf "CHPP Middleware Service Menu\n"
  printf "============================\n"
  printf "1) Install / Update service\n"
  printf "2) Service Status\n"
  printf "3) Start Service\n"
  printf "4) Stop Service\n"
  printf "5) Restart Service\n"
  printf "6) Uninstall Service\n"
  printf "7) Open Web UI\n"
  printf "8) Show Machine ID\n"
  printf "9) Activate License\n"
  printf "0) Exit\n"
  printf "Select: "
  read -r choice

  case "$choice" in
    1) install_service; pause ;;
    2) service_status; pause ;;
    3) service_action start; service_status; pause ;;
    4) service_action stop; service_status; pause ;;
    5) service_action restart; service_status; pause ;;
    6) uninstall_service; pause ;;
    7) xdg-open "http://127.0.0.1:8081/" >/dev/null 2>&1 || printf "Open http://127.0.0.1:8081/ in a browser.\n"; pause ;;
    8) show_machine_id; pause ;;
    9) activate_license; pause ;;
    0) exit 0 ;;
    *) printf "Invalid choice\n"; pause ;;
  esac
done

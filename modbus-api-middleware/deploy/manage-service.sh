#!/bin/sh
set -u

SERVICE=chpp-middleware
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pause() {
  printf "\nPress Enter to continue..."
  read -r _
}

binary_path() {
  arch="$(uname -m)"
  bin="$ROOT/build/middleware-linux-armv7"
  case "$arch" in
    x86_64|amd64) bin="$ROOT/build/middleware-linux-amd64" ;;
  esac
  if [ -x /opt/chpp-middleware/middleware ]; then
    bin=/opt/chpp-middleware/middleware
  fi
  printf "%s" "$bin"
}

show_info() {
  bin="$(binary_path)"
  printf "Middleware info\n"
  printf "===============\n"
  printf "Service: %s\n" "$SERVICE"
  printf "Root: %s\n" "$ROOT"
  printf "Arch: %s\n" "$(uname -m)"
  printf "Binary: %s\n" "$bin"
  if [ -x "$bin" ]; then
    "$bin" -version 2>/dev/null || true
  else
    printf "Version: binary not found or not executable\n"
  fi
  printf "Installed DB: %s\n" "/opt/chpp-middleware/middleware.db"
  [ -f /opt/chpp-middleware/middleware.db ] && ls -lh /opt/chpp-middleware/middleware.db
  printf "Env config: %s\n" "/etc/chpp-middleware.env"
  [ -f /etc/chpp-middleware.env ] && sudo sed -e 's/API_KEY=.*/API_KEY=***MASKED***/' /etc/chpp-middleware.env
  printf "Status: "
  systemctl is-active "$SERVICE" 2>/dev/null || true
  printf "Auto start: "
  systemctl is-enabled "$SERVICE" 2>/dev/null || true
}

install_service() {
  if [ ! -f "$ROOT/deploy/install-systemd.sh" ]; then
    printf "Missing installer: %s\n" "$ROOT/deploy/install-systemd.sh"
    return
  fi
  sh "$ROOT/deploy/install-systemd.sh"
}

uninstall_service() {
  printf "Remove service only, or remove service + /opt/chpp-middleware DB/data?\n"
  printf "1) Service only\n"
  printf "2) Service and data\n"
  printf "0) Cancel\n"
  printf "Select: "
  read -r sub
  case "$sub" in
    1)
      sudo systemctl stop "$SERVICE" 2>/dev/null || true
      sudo systemctl disable "$SERVICE" 2>/dev/null || true
      sudo rm -f "/etc/systemd/system/$SERVICE.service"
      sudo systemctl daemon-reload
      sudo systemctl reset-failed
      ;;
    2)
      sudo systemctl stop "$SERVICE" 2>/dev/null || true
      sudo systemctl disable "$SERVICE" 2>/dev/null || true
      sudo rm -f "/etc/systemd/system/$SERVICE.service" /etc/chpp-middleware.env
      sudo rm -rf /opt/chpp-middleware
      sudo systemctl daemon-reload
      sudo systemctl reset-failed
      ;;
    *) return ;;
  esac
}

while true; do
  clear
  printf "CHPP Middleware Service Menu\n"
  printf "============================\n"
  printf "1) Install / Update service\n"
  printf "2) Info\n"
  printf "3) Status\n"
  printf "4) Start\n"
  printf "5) Stop\n"
  printf "6) Restart\n"
  printf "7) Logs live\n"
  printf "8) Logs latest 100 lines\n"
  printf "9) Enable auto start\n"
  printf "10) Disable auto start\n"
  printf "11) Edit env config\n"
  printf "12) Uninstall service\n"
  printf "0) Exit\n"
  printf "Select: "
  read -r choice

  case "$choice" in
    1) install_service; pause ;;
    2) show_info; pause ;;
    3) sudo systemctl status "$SERVICE" --no-pager; pause ;;
    4) sudo systemctl start "$SERVICE"; sudo systemctl status "$SERVICE" --no-pager; pause ;;
    5) sudo systemctl stop "$SERVICE"; sudo systemctl status "$SERVICE" --no-pager; pause ;;
    6) sudo systemctl restart "$SERVICE"; sudo systemctl status "$SERVICE" --no-pager; pause ;;
    7) sudo journalctl -u "$SERVICE" -f ;;
    8) sudo journalctl -u "$SERVICE" -n 100 --no-pager; pause ;;
    9) sudo systemctl enable "$SERVICE"; pause ;;
    10) sudo systemctl disable "$SERVICE"; pause ;;
    11) sudo ${EDITOR:-nano} /etc/chpp-middleware.env; sudo systemctl restart "$SERVICE"; pause ;;
    12) uninstall_service; pause ;;
    0) exit 0 ;;
    *) printf "Invalid choice\n"; pause ;;
  esac
done

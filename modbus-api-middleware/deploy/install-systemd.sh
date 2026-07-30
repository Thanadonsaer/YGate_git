#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVICE="chpp-middleware"
DEST_DIR="/opt/chpp-middleware"
DEST="$DEST_DIR/middleware"
TMP_DEST="$DEST_DIR/middleware.new"
ARCH="$(uname -m)"
BIN="$ROOT/build/middleware-linux-armv7"
case "$ARCH" in
  x86_64|amd64) BIN="$ROOT/build/middleware-linux-amd64" ;;
esac

if [ ! -f "$BIN" ]; then
  echo "Missing binary: $BIN"
  exit 1
fi

sudo mkdir -p "$DEST_DIR"
sudo systemctl stop "$SERVICE" 2>/dev/null || true

i=0
while sudo systemctl is-active --quiet "$SERVICE" 2>/dev/null; do
  i=$((i + 1))
  if [ "$i" -ge 15 ]; then
    echo "Service did not stop. Check it with: sudo systemctl status $SERVICE --no-pager"
    exit 1
  fi
  sleep 1
done

sudo install -m 0755 "$BIN" "$TMP_DEST"
sudo mv "$TMP_DEST" "$DEST"
if [ ! -f "$DEST_DIR/middleware.db" ] && [ -f "$ROOT/middleware.db" ]; then
  sudo cp "$ROOT/middleware.db" "$DEST_DIR/middleware.db"
fi
sudo cp "$ROOT/deploy/$SERVICE.service" "/etc/systemd/system/$SERVICE.service"
if [ ! -f /etc/chpp-middleware.env ]; then
  sudo tee /etc/chpp-middleware.env >/dev/null <<'ENV'
GATEWAY_ID=MOXA-VT1-01
DB_PATH=/opt/chpp-middleware/middleware.db
LISTEN_ADDR=0.0.0.0:8081
CLEANUP_RETENTION_DAYS=30
ENV
fi
sudo systemctl daemon-reload
sudo systemctl enable --now "$SERVICE"
sudo systemctl status "$SERVICE" --no-pager


#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVICE="chpp-middleware"

run_priv() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "This installer needs root. Run it as root; sudo is not installed on RutOS."
    exit 1
  fi
}

has_systemd() { command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; }
copy_mode() {
  source="$1"
  target="$2"
  mode="$3"
  run_priv cp "$source" "$target"
  run_priv chmod "$mode" "$target"
}
if has_systemd; then
  INSTALL_DIR="/opt/chpp-middleware"
else
  INSTALL_DIR="$ROOT/runtime"
fi
DEST_DIR="$INSTALL_DIR"
DEST="$DEST_DIR/middleware"
TMP_DEST="$DEST_DIR/middleware.new"
LICENSE="$DEST_DIR/license.json"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) BIN="$ROOT/build/middleware-linux-amd64" ;;
  armv7l|armv6l|armhf) BIN="$ROOT/build/middleware-linux-armv7" ;;
  aarch64|arm64) BIN="$ROOT/build/middleware-linux-arm64" ;;
  mips|mipsel|mipsle) BIN="$ROOT/build/middleware-linux-mipsle" ;;
  mips64|mips64el) BIN="$ROOT/build/middleware-linux-mips64el" ;;
  *)
    echo "Unsupported Linux architecture: $ARCH (supported: amd64, armv7, arm64, mips)"
    exit 1
    ;;
esac

if [ ! -f "$BIN" ]; then
  echo "Missing binary: $BIN"
  exit 1
fi
if [ ! -f "$ROOT/license.json" ] && [ ! -f "$LICENSE" ]; then
  echo "Missing license.json. Activate the license from the service menu first."
  exit 1
fi

run_priv mkdir -p "$DEST_DIR"
if has_systemd; then run_priv systemctl stop "$SERVICE" 2>/dev/null || true; else run_priv /etc/init.d/$SERVICE stop 2>/dev/null || true; fi

i=0
while has_systemd && run_priv systemctl is-active --quiet "$SERVICE" 2>/dev/null; do
  i=$((i + 1))
  if [ "$i" -ge 15 ]; then
    echo "Service did not stop. Check the service status."
    exit 1
  fi
  sleep 1
done

copy_mode "$BIN" "$TMP_DEST" 0755
run_priv mv "$TMP_DEST" "$DEST"
if [ -f "$ROOT/license.json" ]; then
  copy_mode "$ROOT/license.json" "$LICENSE" 0600
fi
if [ "$ARCH" = "mips" ] || [ "$ARCH" = "mipsel" ] || [ "$ARCH" = "mipsle" ] || [ "$ARCH" = "mips64" ] || [ "$ARCH" = "mips64el" ]; then
  STORE_NAME="middleware.store"
else
  STORE_NAME="middleware.db"
fi
if [ ! -f "$DEST_DIR/$STORE_NAME" ] && [ -f "$ROOT/$STORE_NAME" ]; then
  run_priv cp "$ROOT/$STORE_NAME" "$DEST_DIR/$STORE_NAME"
fi
if [ "$STORE_NAME" = "middleware.db" ] && [ ! -f "$DEST_DIR/middleware.db" ] && [ -f "$ROOT/middleware.db" ]; then
  run_priv cp "$ROOT/middleware.db" "$DEST_DIR/middleware.db"
fi
if [ ! -f /etc/chpp-middleware.env ]; then
  run_priv tee /etc/chpp-middleware.env >/dev/null <<ENV
GATEWAY_ID=MOXA-VT1-01
DB_PATH=$DEST_DIR/$STORE_NAME
LISTEN_ADDR=0.0.0.0:8081
CLEANUP_RETENTION_DAYS=30
LICENSE_FILE=$LICENSE
ENV
fi
if has_systemd; then
  run_priv mkdir -p /etc/systemd/system
  run_priv cp "$ROOT/deploy/$SERVICE.service" "/etc/systemd/system/$SERVICE.service"
  run_priv systemctl daemon-reload
  run_priv systemctl enable --now "$SERVICE"
  run_priv systemctl status "$SERVICE" --no-pager
else
  run_priv tee "/etc/init.d/$SERVICE" >/dev/null <<INIT
#!/bin/sh /etc/rc.common
START=99
USE_PROCD=1

start_service() {
  . /etc/chpp-middleware.env 2>/dev/null || true
  : "\${GATEWAY_ID:=MOXA-VT1-01}"
  : "\${DB_PATH:=$DEST_DIR/middleware.store}"
  : "\${LISTEN_ADDR:=0.0.0.0:8081}"
  : "\${CLEANUP_RETENTION_DAYS:=30}"
  : "\${LICENSE_FILE:=$LICENSE}"
  procd_open_instance
  procd_set_param command $DEST_DIR/middleware -run -gateway-id "\$GATEWAY_ID" -db "\$DB_PATH" -listen "\$LISTEN_ADDR" -cleanup-retention-days "\$CLEANUP_RETENTION_DAYS" -require-license -license-file "\$LICENSE_FILE"
  procd_set_param stdout 1
  procd_set_param stderr 1
  procd_set_param respawn
  procd_close_instance
}
INIT
  run_priv chmod 0755 "/etc/init.d/$SERVICE"
  run_priv "/etc/init.d/$SERVICE" enable
  run_priv "/etc/init.d/$SERVICE" restart
  run_priv "/etc/init.d/$SERVICE" status 2>/dev/null || true
fi

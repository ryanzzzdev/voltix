#!/usr/bin/env bash
# Install the voltix USER service (systemd --user; no root, no host-wide
# changes). Idempotent: reinstalling refreshes unit/config/binary but
# keeps an existing env file. Requires bin/voltix to exist (make build).
#
# Isolation overrides (used by scripts/native-test.sh so the harness can
# never clobber a developer install):
#   VOLTIX_UNIT_NAME    unit file name        (default voltix.service)
#   VOLTIX_BIN_PATH     installed binary path (default ~/.local/libexec/voltix)
#   VOLTIX_CONFIG_DIR   config + env dir      (default ~/.config/voltix)
#   VOLTIX_STATE_NAME   StateDirectory name   (default voltix)
#   VOLTIX_REFUSE_EXISTING=1  abort if any target path already exists
#   VOLTIX_OWNER_TAG          stamp created resources; remove.sh then
#                             refuses to delete resources it cannot prove
#                             this installation created
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

UNIT_NAME="${VOLTIX_UNIT_NAME:-voltix.service}"
OWNER_TAG="${VOLTIX_OWNER_TAG:-}"
BIN_PATH="${VOLTIX_BIN_PATH:-$HOME/.local/libexec/voltix}"
CONFIG_DIR="${VOLTIX_CONFIG_DIR:-$HOME/.config/voltix}"
STATE_NAME="${VOLTIX_STATE_NAME:-voltix}"
UNIT_PATH="$HOME/.config/systemd/user/$UNIT_NAME"
STATE_DIR="$HOME/.local/state/$STATE_NAME"

[ -x "$ROOT/bin/voltix" ] || { echo "build first: make build"; exit 1; }
command -v systemctl >/dev/null || { echo "systemctl required"; exit 1; }

if [ "${VOLTIX_REFUSE_EXISTING:-0}" = "1" ]; then
  for p in "$UNIT_PATH" "$BIN_PATH" "$CONFIG_DIR" "$STATE_DIR"; do
    if [ -e "$p" ] || [ -L "$p" ]; then
      echo "refusing: $p already exists" >&2
      exit 1
    fi
  done
fi

mkdir -p "$(dirname "$BIN_PATH")" "$CONFIG_DIR" "$HOME/.config/systemd/user"
install -m 0755 "$ROOT/bin/voltix" "$BIN_PATH"
sed "s|@STATE_DIR@|$STATE_DIR|" \
    "$ROOT/deploy/systemd/native.yaml.tpl" > "$CONFIG_DIR/native.yaml"
sed -e "s|@BIN_PATH@|$BIN_PATH|" \
    -e "s|@CONFIG_PATH@|$CONFIG_DIR/native.yaml|" \
    -e "s|@ENV_PATH@|$CONFIG_DIR/env|" \
    -e "s|@STATE_NAME@|$STATE_NAME|" \
    -e "s|@OWNER_TAG@|$OWNER_TAG|" \
    "$ROOT/deploy/systemd/voltix.service.tpl" > "$UNIT_PATH"
chmod 0644 "$UNIT_PATH"
[ -n "$OWNER_TAG" ] && printf '%s\n' "$OWNER_TAG" > "$CONFIG_DIR/.voltix-owner"

# Demo-only tokens (600); override later if wiring a real mock.
if [ ! -f "$CONFIG_DIR/env" ]; then
  umask 077
  printf 'VOLTIX_DISPATCH_TOKEN=%s\nVOLTIX_TELEMETRY_TOKEN=%s\n' \
      "${VOLTIX_DISPATCH_TOKEN:-voltix-native-dispatch}" \
      "${VOLTIX_TELEMETRY_TOKEN:-voltix-native-telemetry}" \
      > "$CONFIG_DIR/env"
fi

systemctl --user daemon-reload
systemctl --user enable "$UNIT_NAME"
echo "installed. start with: systemctl --user start ${UNIT_NAME%.service}"

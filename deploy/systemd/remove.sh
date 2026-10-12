#!/usr/bin/env bash
# Remove exactly what install.sh created under the same overrides
# (service, unit, config, env, binary, and state). Touches nothing else:
# with the default names this is the canonical install; native-test passes
# unique names so cleanup removes only test-created resources.
set -euo pipefail

UNIT_NAME="${VOLTIX_UNIT_NAME:-voltix.service}"
BIN_PATH="${VOLTIX_BIN_PATH:-$HOME/.local/libexec/voltix}"
CONFIG_DIR="${VOLTIX_CONFIG_DIR:-$HOME/.config/voltix}"
STATE_NAME="${VOLTIX_STATE_NAME:-voltix}"
UNIT_PATH="$HOME/.config/systemd/user/$UNIT_NAME"
STATE_DIR="$HOME/.local/state/$STATE_NAME"

systemctl --user stop "$UNIT_NAME" 2>/dev/null || true
systemctl --user disable "$UNIT_NAME" 2>/dev/null || true
rm -f "$UNIT_PATH" "$BIN_PATH" "$CONFIG_DIR/native.yaml" "$CONFIG_DIR/env"

# The user manager may implement StateDirectory as a symlink
# (~/.local/state/<name> -> a real dir elsewhere under $HOME). Resolve
# the link and remove the real directory only when it lives in the
# per-user config/state trees we manage.
REAL_STATE=$(readlink -f "$STATE_DIR" 2>/dev/null || true)
rm -f "$STATE_DIR"
case "$REAL_STATE" in
  "$HOME/.local/state/"* | "$HOME/.config/"*) rm -rf "$REAL_STATE" ;;
esac
rmdir "$CONFIG_DIR" 2>/dev/null || true # only when already empty

systemctl --user daemon-reload
echo "voltix user service removed"

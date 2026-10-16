#!/usr/bin/env bash
# Remove exactly what install.sh created under the same overrides
# (service, unit, config, env, binary, and state). Touches nothing else:
# with the default names this is the canonical install; native-test passes
# unique names so cleanup removes only test-created resources.
#
# When VOLTIX_OWNER_TAG is set, the unit file and the config dir's
# .voltix-owner marker must both carry the tag — otherwise the resources
# were not created by this installation and NOTHING is removed (a stale
# same-name path after PID reuse is not ours to delete).
set -euo pipefail

UNIT_NAME="${VOLTIX_UNIT_NAME:-voltix.service}"
BIN_PATH="${VOLTIX_BIN_PATH:-$HOME/.local/libexec/voltix}"
CONFIG_DIR="${VOLTIX_CONFIG_DIR:-$HOME/.config/voltix}"
STATE_NAME="${VOLTIX_STATE_NAME:-voltix}"
UNIT_PATH="$HOME/.config/systemd/user/$UNIT_NAME"
STATE_DIR="$HOME/.local/state/$STATE_NAME"
OWNER_TAG="${VOLTIX_OWNER_TAG:-}"

if [ -n "$OWNER_TAG" ]; then
  if ! grep -q "voltix-owner: $OWNER_TAG" "$UNIT_PATH" 2>/dev/null \
     || ! grep -qx "$OWNER_TAG" "$CONFIG_DIR/.voltix-owner" 2>/dev/null; then
    echo "refusing: resources not owned by $OWNER_TAG" >&2
    exit 1
  fi
fi

systemctl --user stop "$UNIT_NAME" 2>/dev/null || true
systemctl --user disable "$UNIT_NAME" 2>/dev/null || true
rm -f "$UNIT_PATH" "$BIN_PATH" "$CONFIG_DIR/native.yaml" "$CONFIG_DIR/env" \
      "$CONFIG_DIR/.voltix-owner"

# The user manager may implement StateDirectory as a symlink
# (~/.local/state/<name> -> a real dir elsewhere under $HOME) or a plain
# directory. Resolve first; remove the link or dir, then the resolved
# target only when it lives in the per-user trees we manage.
REAL_STATE=$(readlink -f "$STATE_DIR" 2>/dev/null || true)
if [ -d "$STATE_DIR" ] && [ ! -L "$STATE_DIR" ]; then
  rm -rf -- "$STATE_DIR"
else
  rm -f "$STATE_DIR"
fi
case "$REAL_STATE" in
  "$HOME/.local/state/"* | "$HOME/.config/"*) rm -rf -- "$REAL_STATE" ;;
esac
rmdir "$CONFIG_DIR" 2>/dev/null || true # only when already empty

systemctl --user daemon-reload
echo "voltix user service removed"

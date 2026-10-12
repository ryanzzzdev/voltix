#!/usr/bin/env bash
# Install the voltix USER service (systemd --user; no root, no host-wide
# changes). Idempotent. Requires bin/voltix to exist (make build).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

[ -x "$ROOT/bin/voltix" ] || { echo "build first: make build"; exit 1; }
command -v systemctl >/dev/null || { echo "systemctl required"; exit 1; }

mkdir -p "$HOME/.local/libexec" "$HOME/.config/voltix" "$HOME/.config/systemd/user"
install -m 0755 "$ROOT/bin/voltix" "$HOME/.local/libexec/voltix"
sed "s|@STATE_DIR@|$HOME/.local/state/voltix|" \
    "$ROOT/deploy/systemd/native.yaml.tpl" > "$HOME/.config/voltix/native.yaml"
install -m 0644 "$ROOT/deploy/systemd/voltix.service" "$HOME/.config/systemd/user/"

# Demo-only tokens (600); override later if wiring a real mock.
if [ ! -f "$HOME/.config/voltix/env" ]; then
  umask 077
  printf 'VOLTIX_DISPATCH_TOKEN=%s\nVOLTIX_TELEMETRY_TOKEN=%s\n' \
      "${VOLTIX_DISPATCH_TOKEN:-voltix-native-dispatch}" \
      "${VOLTIX_TELEMETRY_TOKEN:-voltix-native-telemetry}" \
      > "$HOME/.config/voltix/env"
fi

systemctl --user daemon-reload
systemctl --user enable voltix.service
echo "installed. start with: systemctl --user start voltix"

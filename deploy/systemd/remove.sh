#!/usr/bin/env bash
# Remove everything install.sh created (service, unit, config, binary,
# env, and the test state dir). Touches nothing else.
set -euo pipefail
systemctl --user stop voltix.service 2>/dev/null || true
systemctl --user disable voltix.service 2>/dev/null || true
rm -f "$HOME/.config/systemd/user/voltix.service" \
      "$HOME/.local/libexec/voltix" \
      "$HOME/.config/voltix/native.yaml" \
      "$HOME/.config/voltix/env"
rm -rf "$HOME/.local/state/voltix"
systemctl --user daemon-reload
echo "voltix user service removed"

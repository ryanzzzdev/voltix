#!/usr/bin/env bash
# Three-pane live demo (doc 09 layout) using zellij:
#   top-left  — controller logs   |  top-right — simulated site physics
#   bottom    — interactive runner (or --runner to auto-run the narrative)
#
# Usage:
#   ./scripts/demo-watch.sh            # interactive bottom pane
#   ./scripts/demo-watch.sh --runner   # auto-run the demo narrative (used
#                                      # for the asciinema/agg GIF capture)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
command -v zellij >/dev/null || { echo "zellij not found (~/.local/bin/zellij)"; exit 1; }

# Bring the isolated site up if it isn't already.
if ! curl -sf -H "Authorization: Bearer voltix-demo-dispatch" \
        http://127.0.0.1:8180/api/status >/dev/null 2>&1; then
  make demo
fi

RUNNER_SCRIPT=$(mktemp --suffix=.sh)
if [ "${1:-}" = "--runner" ]; then
  # Used by the recording path: run the narrative, hold a moment, then end
  # the session so the capture terminates (env var is set inside zellij).
  cat > "$RUNNER_SCRIPT" <<'EOF'
./scripts/demo-runner.sh
echo; echo "### narrative complete"
sleep 4
zellij kill-session "$ZELLIJ_SESSION_NAME"
EOF
else
  printf 'cat scripts/demo-cheatsheet.txt; exec bash\n' > "$RUNNER_SCRIPT"
fi

LAYOUT=$(mktemp --suffix=.kdl)
sed "s|@RUNNER@|bash $RUNNER_SCRIPT|" scripts/demo-watch.kdl > "$LAYOUT"
trap 'rm -f "$LAYOUT" "$RUNNER_SCRIPT"' EXIT
exec zellij --config scripts/demo-zellij-config.kdl attach --create voltix-demo \
  options --default-layout "$LAYOUT"

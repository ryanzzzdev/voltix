#!/usr/bin/env bash
# VTX-017 native verification: voltix as an unprivileged systemd --user
# service. Starts the sim + telemetry mock as plain host processes on the
# native harness ports, installs the unit under PID-unique names, and
# checks user identity, state dir, journal, MemoryMax, restart behaviour
# and post-restart session rejection.
#
# Isolation (audit R3): every filesystem resource the harness creates is
# tagged with $$ and routed through install.sh/remove.sh overrides, and
# install refuses to run if a same-named path already exists. The
# canonical install paths are snapshotted before/after so the script can
# prove it never touched a real installation. Resources are removed by an
# EXIT trap — cleanup also runs when a prerequisite fails mid-way.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DISPATCH="http://127.0.0.1:8190"
MOCK="http://127.0.0.1:8191"
DTOK="voltix-native-dispatch"
TTOK="voltix-native-telemetry"

TAG="voltix-nt-$$"
UNIT_NAME="$TAG.service"
export VOLTIX_UNIT_NAME="$UNIT_NAME" \
       VOLTIX_BIN_PATH="$HOME/.local/libexec/$TAG" \
       VOLTIX_CONFIG_DIR="$HOME/.config/$TAG" \
       VOLTIX_STATE_NAME="$TAG" \
       VOLTIX_OWNER_TAG="$TAG" \
       VOLTIX_REFUSE_EXISTING=1
UNIT_PATH="$HOME/.config/systemd/user/$UNIT_NAME"
UNITDIR="$HOME/.local/state/$TAG"

# Canonical install paths the harness must never touch.
GUARD_PATHS=("$HOME/.config/systemd/user/voltix.service"
             "$HOME/.local/libexec/voltix"
             "$HOME/.config/voltix"
             "$HOME/.local/state/voltix")
# Content fingerprints (not mere existence) so the guard detects the
# harness modifying a pre-existing install even in passing.
guard() {
  for p in "${GUARD_PATHS[@]}"; do
    if [ -d "$p" ]; then
      echo "dir $p $(find -L "$p" -type f -exec sha256sum {} + 2>/dev/null | sort | sha256sum | cut -d' ' -f1)"
    elif [ -f "$p" ]; then
      echo "file $p $(sha256sum "$p" | cut -d' ' -f1)"
    elif [ -L "$p" ]; then
      echo "link $p -> $(readlink "$p")"
    fi
  done
}

SIM_PID=""; MOCK_PID=""; MOCK_DIR=""
PASS=0; FAIL=0

ok()   { echo "PASS  $1"; PASS=$((PASS+1)); }
bad()  { echo "FAIL  $1"; FAIL=$((FAIL+1)); }
say()  { echo "--- $*"; }

status() { curl -sf -H "Authorization: Bearer $DTOK" "$DISPATCH/api/status"; }
jget()   { python3 -c "import json,sys;print(json.load(sys.stdin)$1)" 2>/dev/null; }

cleanup() {
  say "cleanup"
  # Ownership-tagged removal: a refusal means the resources aren't ours —
  # report it rather than force-delete.
  if ! "$ROOT/deploy/systemd/remove.sh" >/dev/null 2>&1; then
    echo "cleanup: remove.sh refused — leaving foreign resources untouched" >&2
  fi
  [ -n "$SIM_PID" ] && kill "$SIM_PID" 2>/dev/null
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null
  [ -n "$MOCK_DIR" ] && rm -rf "$MOCK_DIR"
}
trap cleanup EXIT

GUARD_BEFORE=$(guard)

say "prereqs"
command -v systemctl >/dev/null || { echo "no systemctl"; exit 1; }
systemctl --user show-environment >/dev/null 2>&1 || { echo "no user manager"; exit 1; }
(cd "$ROOT" && make build >/dev/null) || { echo "build failed"; exit 1; }
for port in 2512 2513 2514 8190 8191; do
  (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && { exec 3>&- 3<&-; echo "port $port already in use"; exit 1; } || true
done

say "start sim (2512-2514) + upstream-mock (8191)"
(cd "$ROOT/sim" && uv run python -m voltix_sim --host 127.0.0.1 \
    --inverter-port 2512 --meter-port 2513 --control-port 2514) >/dev/null 2>&1 &
SIM_PID=$!
MOCK_DIR=$(mktemp -d)
VOLTIX_TELEMETRY_TOKEN=$TTOK "$ROOT/bin/upstream-mock" \
    -listen 127.0.0.1:8191 -dir "$MOCK_DIR" >/dev/null 2>&1 &
MOCK_PID=$!
sleep 2

say "refusal safety: foreign same-name resources"
# Plant foreign resources at THIS run's paths (no owner tag — simulates a
# stale install after PID reuse). install must refuse to overwrite and
# remove.sh must refuse to delete them.
mkdir -p "$VOLTIX_CONFIG_DIR"
echo foreign > "$VOLTIX_CONFIG_DIR/native.yaml"
mkdir -p "$UNITDIR"
if "$ROOT/deploy/systemd/install.sh" >/dev/null 2>&1; then
  bad "install overwrote a pre-existing untagged path"
else
  ok "install refused pre-existing untagged path"
fi
if "$ROOT/deploy/systemd/remove.sh" >/dev/null 2>&1; then
  bad "remove.sh deleted resources it did not own"
else
  ok "remove.sh refused untagged resources"
fi
[ "$(cat "$VOLTIX_CONFIG_DIR/native.yaml")" = "foreign" ] \
  && ok "foreign config left intact" \
  || bad "foreign config was modified"
rm -rf "$VOLTIX_CONFIG_DIR" "$UNITDIR" # planted by this script — ours

say "install + start user service ($UNIT_NAME)"
# A refused/failed install must stop the run immediately — proceeding
# would exercise (and then "clean up") resources we did not create.
if ! "$ROOT/deploy/systemd/install.sh" >/dev/null; then
  echo "install refused or failed — aborting (pre-existing paths?)" >&2
  exit 1
fi
systemctl --user start "$UNIT_NAME"
for i in $(seq 1 20); do S=$(status) && break; sleep 1; done
[ -n "${S:-}" ] && ok "service started, status endpoint live" || bad "status endpoint never came up"
SESSION1=$(echo "$S" | jget "['boot_session_id']")
say "boot session: $SESSION1"

MPID=$(systemctl --user show "$UNIT_NAME" -p MainPID --value)
OWNER=$(stat -c %u "/proc/$MPID" 2>/dev/null || echo -1)
[ "$OWNER" = "$(id -u)" ] && ok "runs as unprivileged user (uid $OWNER)" || bad "process owner uid=$OWNER"

# stat -L: the user manager may implement StateDirectory as a symlink
# (~/.local/state/<name> -> ~/.config/<name>); check the real dir.
MODE=$(stat -L -c %a "$UNITDIR" 2>/dev/null || echo missing)
U=$(stat -L -c %u "$UNITDIR" 2>/dev/null || echo -1)
[ "$MODE" = "700" ] && [ "$U" = "$(id -u)" ] \
  && ok "state dir mode=0700 owner=user" || bad "state dir mode=$MODE uid=$U"

journalctl --user -u "$UNIT_NAME" --no-pager -n 5 2>/dev/null | grep -q voltix \
  && ok "journal visible via journalctl --user" || bad "no journal lines found"

MEM=$(systemctl --user show "$UNIT_NAME" -p MemoryMax --value)
CG=$(systemctl --user show "$UNIT_NAME" -p ControlGroup --value)
CGMAX=""
[ -n "$CG" ] && CGMAX=$(cat "/sys/fs/cgroup/$CG/memory.max" 2>/dev/null)
if [ "$MEM" != "infinity" ] && [ -n "$MEM" ]; then
  [ "$CGMAX" = "$MEM" ] \
    && ok "MemoryMax=$MEM enforced in cgroup" \
    || bad "MemoryMax=$MEM but cgroup memory.max=$CGMAX"
else
  bad "MemoryMax not set"
fi

say "dispatch under session 1"
NOW=$(date -u +%FT%TZ); END=$(date -u -d '+25 seconds' +%FT%TZ)
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$DISPATCH/api/dispatch" \
  -H "Authorization: Bearer $DTOK" \
  -d "{\"controller_id\":\"voltix-native\",\"boot_session_id\":\"$SESSION1\",\"command_id\":\"11111111-1111-1111-1111-111111111111\",\"revision\":1,\"issued_at\":\"$NOW\",\"valid_until\":\"$END\",\"target_kw\":150}")
[ "$CODE" = "202" ] && ok "dispatch accepted (rev 1)" || bad "dispatch -> HTTP $CODE"
sleep 3
STATE=$(status | jget "['loop']['State']")
[ "$STATE" = "ACTIVE" ] && ok "loop ACTIVE with accepted command" || bad "loop state=$STATE"

say "SIGKILL -> systemd restart"
T0=$(date +%s.%N)
kill -9 "$MPID"
NEW=""; for i in $(seq 1 40); do
  sleep 0.5
  S2=$(status) && NEW=$(echo "$S2" | jget "['boot_session_id']") \
    && [ -n "$NEW" ] && [ "$NEW" != "$SESSION1" ] && break
done
RT=$(python3 -c "print(f'{$(date +%s.%N)-$T0:.2f}')")
if [ -n "$NEW" ] && [ "$NEW" != "$SESSION1" ]; then
  ok "restarted with new boot session in ${RT}s"; say "restart session: $NEW"
else
  bad "no new session within 20s"
fi

CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$DISPATCH/api/dispatch" \
  -H "Authorization: Bearer $DTOK" \
  -d "{\"controller_id\":\"voltix-native\",\"boot_session_id\":\"$SESSION1\",\"command_id\":\"11111111-1111-1111-1111-111111111111\",\"revision\":1,\"issued_at\":\"$NOW\",\"valid_until\":\"$END\",\"target_kw\":150}")
[ "$CODE" = "409" ] && ok "old-session replay rejected (409)" || bad "old-session replay -> $CODE (want 409)"

CNT=$(curl -s "$MOCK/control/count" | python3 -c "import json,sys;print(json.load(sys.stdin)['count'])" 2>/dev/null || echo 0)
[ "$CNT" -gt 0 ] && ok "telemetry reached mock (count=$CNT)" || bad "mock count=$CNT"

sleep 1
TEL=$(status | jget "['telemetry']['uploaded']")
[ "${TEL:-0}" -gt 0 ] && ok "uploader acked events (uploaded=$TEL)" || bad "uploaded=$TEL"

say "result: $PASS passed, $FAIL failed (restart=${RT}s)"
[ "$FAIL" -eq 0 ] || exit 1

# Explicit cleanup + post-checks (the EXIT trap is idempotent: remove.sh
# tolerates absent resources and the helper pids are already dead).
cleanup
say "isolation checks"
LEAK=""
for p in "$UNIT_PATH" "$VOLTIX_BIN_PATH" "$VOLTIX_CONFIG_DIR" "$UNITDIR" \
         "$HOME/.config/systemd/user/default.target.wants/$UNIT_NAME"; do
  { [ -e "$p" ] || [ -L "$p" ]; } && LEAK="$LEAK $p"
done
[ -z "$LEAK" ] && ok "all test-created resources removed" || bad "leaked:$LEAK"
GUARD_AFTER=$(guard)
[ "$GUARD_AFTER" = "$GUARD_BEFORE" ] \
  && ok "canonical install paths untouched" \
  || bad "canonical paths changed: before='$GUARD_BEFORE' after='$GUARD_AFTER'"

say "result: $PASS passed, $FAIL failed (restart=${RT}s)"
[ "$FAIL" -eq 0 ]

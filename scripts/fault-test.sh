#!/usr/bin/env bash
# VTX-018 fault harness: repeatable fault scenarios with machine-checkable
# results against the isolated Compose demo (fresh containers each run).
#
# Coverage (acceptance matrix): A telemetry outage, B dispatch expiry,
# C3 field-link outage, C4 sustained controller absence, D lost ack.
# C5 fast-restart is delegated to scripts/native-test.sh — the spec
# requires native restart evidence that container restarts cannot supply.
#
# BLK-003 boundary: no unprivileged netns/NET_ADMIN/CAP_NET_RAW on this
# host, so faults are application/process-level: sim control faults,
# receiver pause/drop, docker pause (SIGSTOP). Packet capture is
# unavailable; request/timeout/reconnect is explained from app logs.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE="docker compose -f $ROOT/deploy/compose.yaml"
DISPATCH="http://127.0.0.1:8180"
MOCK="http://127.0.0.1:8081"
DTOK="voltix-demo-dispatch"
REV=0
PASS=0; FAIL=0

ok()  { echo "PASS  $1"; PASS=$((PASS+1)); }
bad() { echo "FAIL  $1"; FAIL=$((FAIL+1)); }
say() { echo ""; echo "=== $* ==="; }

status() { curl -sf -H "Authorization: Bearer $DTOK" "$DISPATCH/api/status"; }
jget()   { python3 -c "import json,sys;print(json.load(sys.stdin)$1)" 2>/dev/null; }
field()  { status | jget "$1"; }
mockctl() { curl -sf -X POST "$MOCK/control/$1"; }
simctl() {
  python3 - "$1" "$2" <<'PY'
import socket, json, sys
s = socket.create_connection(('127.0.0.1', 1504), 3); s.settimeout(3)
s.sendall(json.dumps({'cmd': sys.argv[1], 'value': json.loads(sys.argv[2])}).encode() + b'\n')
print(s.recv(65536).decode().strip())
PY
}
simstate() { simctl get_state null; }

post_cmd() { # target_kw, duration_s, revision -> http code
  # Callers must bump REV in their own shell: $(...) is a subshell.
  local sess now end resp code rev=$3
  sess=$(field "['boot_session_id']")
  now=$(date -u +%FT%TZ); end=$(date -u -d "+$2 seconds" +%FT%TZ)
  resp=$(curl -s -w '\n%{http_code}' -X POST "$DISPATCH/api/dispatch" \
    -H "Authorization: Bearer $DTOK" \
    -d "{\"controller_id\":\"voltix-demo\",\"boot_session_id\":\"$sess\",\"command_id\":\"00000000-0000-4000-8000-$(printf %012d $rev)\",\"revision\":$rev,\"issued_at\":\"$now\",\"valid_until\":\"$end\",\"target_kw\":$1}")
  code=${resp##*$'\n'}
  [ "$code" != "202" ] && echo "  post_cmd rev=$rev sess=${sess:0:8}.. -> $code ${resp%$'\n'*}" >&2
  echo "$code"
}

wait_state() { # expected loop state, max seconds
  for i in $(seq 1 "$2"); do
    [ "$(field "['loop']['State']")" = "$1" ] && return 0
    sleep 1
  done
  return 1
}

cleanup() { $COMPOSE down -v >/dev/null 2>&1; }
trap cleanup EXIT

say "fresh stack"
$COMPOSE up -d --build >/dev/null 2>&1
for i in $(seq 1 30); do status >/dev/null && break; sleep 1; done
status >/dev/null || { echo "voltix never came up"; exit 1; }
wait_state STANDBY 10 && ok "baseline STANDBY" || bad "no STANDBY baseline"

# --- B: dispatch expiry ------------------------------------------------
say "B: dispatch expiry -> fallback starts by expiry+1 cycle, zero within ramp"
T_EXP=$(date +%s)
REV=$((REV+1)); CODE=$(post_cmd 100 25 $REV)   # expires ~T_EXP+25
[ "$CODE" = "202" ] && ok "command accepted" || { bad "dispatch -> $CODE"; }
wait_state ACTIVE 8 && ok "loop ACTIVE" || bad "loop not ACTIVE"
# poll: fallback start = dispatch_active flips false; zero = sim output 0
T_FB=""; T_ZERO=""
for i in $(seq 1 40); do
  NOW=$(date +%s)
  [ -z "$T_FB" ] && [ "$(field "['dispatch_active']")" = "False" ] && T_FB=$NOW
  OUT=$(simstate | python3 -c "import json,sys;print(json.load(sys.stdin)['state']['output_kw'])" 2>/dev/null)
  [ -n "$T_FB" ] && [ -z "$T_ZERO" ] && [ "${OUT:-1}" = "0.0" ] && T_ZERO=$NOW
  [ -n "$T_ZERO" ] && break
  sleep 1
done
if [ -n "$T_FB" ]; then
  LAG=$((T_FB - T_EXP - 25))
  [ "$LAG" -le 2 ] && ok "fallback start at expiry+${LAG}s (<=1 cycle+work)" \
    || bad "fallback started expiry+${LAG}s (>2s)"
else bad "dispatch never expired"; fi
if [ -n "$T_ZERO" ]; then
  ZERO=$((T_ZERO - T_EXP - 25))
  # 100 kW at 50 kW/s slew = 2 s; allowance = slew + cycle slack
  [ "$ZERO" -le 8 ] && ok "output reached 0 at expiry+${ZERO}s" \
    || bad "zero took expiry+${ZERO}s"
  # Audit R6: zero inside the slew window while the sim watchdog is NOT
  # expired proves the fallback was a controller write — output could not
  # have reached 0 via watchdog ramp inside 8s (trip is 10s).
  WE=$(simstate | python3 -c "import json,sys;print(json.load(sys.stdin)['state']['watchdog_expired'])" 2>/dev/null)
  [ "$WE" = "False" ] && ok "fallback was a controller write (watchdog not expired)" \
    || bad "zero may have come from watchdog trip, not a fallback write (we=$WE)"
else bad "output never reached 0"; fi

# --- C3: field-link outage ----------------------------------------------
say "C3: device unreachable -> voltix degraded AND sim watchdog trips independently"
REV=$((REV+1)); CODE=$(post_cmd 50 25 $REV)
[ "$CODE" = "202" ] || { bad "dispatch -> $CODE"; }
wait_state ACTIVE 8
T_OFF=$(date +%s)  # last successful WSet within the previous cycle
simctl set_online true >/dev/null   # ensure clean baseline
simctl set_online false >/dev/null
sleep 2
INVOK=$(field "['loop']['InverterOK']")
[ "$INVOK" = "False" ] && ok "voltix reports inverter failure" \
  || bad "InverterOK still $INVOK"
T_TRIP=""
for i in $(seq 1 20); do
  WE=$(simstate | python3 -c "import json,sys;print(json.load(sys.stdin)['state']['watchdog_expired'])" 2>/dev/null)
  [ "$WE" = "True" ] && { T_TRIP=$(date +%s); break; }
  sleep 1
done
if [ -n "$T_TRIP" ]; then
  DT=$((T_TRIP - T_OFF))
  # watchdog = 10 s after last accepted write; write happened <=1 s before
  # T_OFF, so observable window is [9, 12] s
  { [ "$DT" -ge 8 ] && [ "$DT" -le 13 ]; } \
    && ok "sim watchdog tripped ${DT}s after last refresh (10s bound)" \
    || bad "watchdog trip at ${DT}s"
  OUT=$(simstate | python3 -c "import json,sys;print(json.load(sys.stdin)['state']['output_kw'])" 2>/dev/null)
  [ "${OUT:-1}" = "0.0" ] && ok "inverter independently ramped to 0" \
    || bad "inverter output=$OUT after watchdog"
else bad "watchdog never tripped"; fi
simctl set_online true >/dev/null

# --- C4: sustained controller absence ------------------------------------
say "C4: controller paused > watchdog -> inverter falls back alone"
REV=$((REV+1)); CODE=$(post_cmd 50 28 $REV)  # <=30s contract bound
[ "$CODE" = "202" ] || { bad "dispatch -> $CODE"; }
wait_state ACTIVE 8
$COMPOSE pause voltix >/dev/null
T_PAUSE=$(date +%s)
T_TRIP=""
for i in $(seq 1 25); do
  WE=$(simstate | python3 -c "import json,sys;print(json.load(sys.stdin)['state']['watchdog_expired'])" 2>/dev/null)
  [ "$WE" = "True" ] && { T_TRIP=$(date +%s); break; }
  sleep 1
done
if [ -n "$T_TRIP" ]; then
  DT=$((T_TRIP - T_PAUSE))
  { [ "$DT" -ge 8 ] && [ "$DT" -le 13 ]; } \
    && ok "watchdog tripped ${DT}s after controller pause (sustained absence)" \
    || bad "trip at ${DT}s"
else bad "watchdog never tripped under pause"; fi
$COMPOSE unpause voltix >/dev/null
sleep 4
# post-SIGSTOP gap > max_loop_gap -> intent dropped, STANDBY, no jump
ST=$(field "['loop']['State']")
[ "$ST" = "STANDBY" ] && ok "resume after gap -> STANDBY (intent dropped, no jump)" \
  || bad "post-resume state=$ST"

# --- A: telemetry-only outage -------------------------------------------
say "A: telemetry outage -> queue grows, dispatch/control unaffected"
mockctl pause >/dev/null
D0=$(field "['telemetry']['depth_status']")
sleep 12
D1=$(field "['telemetry']['depth_status']")
U1=$(field "['telemetry']['upload_failed']")
{ [ "${D1:-0}" -gt "${D0:-0}" ] && [ "${U1:-0}" -gt 0 ]; } \
  && ok "queue grew ($D0->$D1) and upload failed ($U1) during outage" \
  || bad "depth $D0->$D1 failed=$U1"
REV=$((REV+1)); CODE=$(post_cmd 25 20 $REV)
[ "$CODE" = "202" ] && ok "dispatch still accepted during telemetry outage" \
  || bad "dispatch -> $CODE during outage"
wait_state ACTIVE 8 && ok "control loop ACTIVE during telemetry outage" \
  || bad "loop not ACTIVE"
mockctl resume >/dev/null
# poll for drain: first upload after resume may wait out accumulated
# backoff, then batches drain one per round
D2=$D1; U2=""
for i in $(seq 1 30); do
  D2=$(field "['telemetry']['depth_status']")
  U2=$(field "['telemetry']['uploaded']")
  { [ "${U2:-0}" -gt 0 ] && [ "${D2:-999}" -le 5 ]; } && break
  sleep 2
done
{ [ "${U2:-0}" -gt 0 ] && [ "${D2:-999}" -lt "${D1:-0}" ]; } \
  && ok "after resume: uploaded=$U2, queue drained $D1->$D2" \
  || bad "no recovery: uploaded=$U2 depth=$D2"

# --- D: lost acknowledgement ---------------------------------------------
say "D: commit-then-drop -> retry with same identities, receiver dedups"
# Audit R6: all assertions are relative to THIS scenario's baselines —
# upload_failed is cumulative and already nonzero from scenario A.
UF0=$(field "['telemetry']['upload_failed']")
C0=$(curl -sf "$MOCK/control/count" | jget "['count']")
UF0=${UF0:-0}; C0=${C0:-0}
mockctl drop-after-commit >/dev/null
# Wait for the scenario itself: receiver committed new rows (C>C0) AND a
# new upload failure was recorded (UF>UF0) — together that is
# commit-then-drop actually exercised, not a stale counter.
C1=""; UF=""
for i in $(seq 1 30); do
  C1=$(curl -sf "$MOCK/control/count" | jget "['count']")
  UF=$(field "['telemetry']['upload_failed']")
  [ "${C1:-0}" -gt "$C0" ] && [ "${UF:-0}" -gt "$UF0" ] && break
  sleep 1
done
{ [ "${C1:-0}" -gt "$C0" ] && [ "${UF:-0}" -gt "$UF0" ]; } \
  && ok "commit-then-drop exercised (rows $C0->$C1, failures $UF0->$UF)" \
  || bad "drop not exercised: rows $C0->$C1 failures $UF0->$UF"
# Now wait for the replay to drain: dedup means receiver count equals
# sender-acked total exactly — a duplicate would leave C > uploaded, a
# conflicted retry would leave upload_failed still climbing.
CF=""; UP=""; UF2=""
for i in $(seq 1 40); do
  CF=$(curl -sf "$MOCK/control/count" | jget "['count']")
  UP=$(field "['telemetry']['uploaded']")
  UF2=$(field "['telemetry']['upload_failed']")
  [ -n "$UP" ] && [ "${CF:-0}" = "$UP" ] && [ "${CF:-0}" -gt "$C0" ] && break
  sleep 2
done
if [ -n "$UP" ] && [ "${CF:-0}" = "$UP" ] && [ "${CF:-0}" -gt "$C0" ]; then
  ok "replay deduped: receiver rows=$CF == uploaded=$UP (failures +$((UF2-UF0)))"
else
  bad "dedup not proven: rows=$CF uploaded=$UP failures $UF0->$UF2"
fi

say "results: $PASS passed, $FAIL failed"
echo "unavailable prerequisites: NET_ADMIN/netns (denied unprivileged),"
echo "CAP_NET_RAW packet capture (denied) — faults are app/process-level;"
echo "request/timeout/reconnect evidence is application-observable"
echo "(syncer backoff counters, loop InverterOK transitions, sim watchdog)."
say "C5 fast-restart: delegated to make native-test (native evidence)"
[ "$FAIL" -eq 0 ]

#!/usr/bin/env bash
# VTX-020 scenario H: 30-minute 1 Hz run with a 5-minute telemetry outage,
# then measured catch-up. Records depth/RSS/loop timings to a JSON file.
# Uses the native harness ports (2512-2514 sim, 8190 dispatch, 8191 mock).
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/evidence/demo/resource-run.json}"
DISPATCH="http://127.0.0.1:8190"; MOCK="http://127.0.0.1:8191"
DTOK="voltix-native-dispatch"; TTOK="voltix-native-telemetry"
STATE_DIR=$(mktemp -d); MOCK_DIR=$(mktemp -d)
SIM_PID=""; MOCK_PID=""; VT_PID=""
cleanup() {
  [ -n "$VT_PID" ] && kill "$VT_PID" 2>/dev/null
  [ -n "$SIM_PID" ] && kill "$SIM_PID" 2>/dev/null
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null
  rm -rf "$STATE_DIR" "$MOCK_DIR"
}
trap cleanup EXIT
jget() { python3 -c "import json,sys;print(json.load(sys.stdin)$1)" 2>/dev/null; }
status() { curl -sf -H "Authorization: Bearer $DTOK" "$DISPATCH/api/status"; }
rss() { awk '/VmRSS/{print $2}' "/proc/$VT_PID/status" 2>/dev/null; }

(cd "$ROOT/sim" && uv run python -m voltix_sim --host 127.0.0.1 \
  --inverter-port 2512 --meter-port 2513 --control-port 2514) >/dev/null 2>&1 &
SIM_PID=$!
VOLTIX_TELEMETRY_TOKEN=$TTOK "$ROOT/bin/upstream-mock" \
  -listen 127.0.0.1:8191 -dir "$MOCK_DIR" >/dev/null 2>&1 &
MOCK_PID=$!
# voltix fail-fasts at Discover if devices aren't listening yet
for i in $(seq 1 20); do
  (echo > /dev/tcp/127.0.0.1/2512) 2>/dev/null && break; sleep 0.5; done
sed "s|@STATE_DIR@|$STATE_DIR|" "$ROOT/deploy/systemd/native.yaml.tpl" > "$STATE_DIR/cfg.yaml"
VOLTIX_DISPATCH_TOKEN=$DTOK VOLTIX_TELEMETRY_TOKEN=$TTOK \
  "$ROOT/bin/voltix" -config "$STATE_DIR/cfg.yaml" >>/tmp/resource-run-voltix.log 2>&1 &
VT_PID=$!
for i in $(seq 1 20); do status >/dev/null && break; sleep 1; done

echo "{\"samples\":[" > "$OUT"; FIRST=1
samp() {
  S=$(status); [ -z "$S" ] && return
  [ $FIRST = 0 ] && echo "," >> "$OUT"
  printf '{"t":%d,"state":"%s","depth":%s,"up":%s,"uf":%s,"rss_kb":%s,"rd_ms":%s,"wr_ms":%s}' \
    "$(date +%s)" \
    "$(echo "$S"|jget "['loop']['State']" || echo null)" \
    "$(echo "$S"|jget "['telemetry']['depth_status']" || echo null)" \
    "$(echo "$S"|jget "['telemetry']['uploaded']" || echo null)" \
    "$(echo "$S"|jget "['telemetry']['upload_failed']" || echo null)" \
    "$(rss || echo null)" \
    "$(echo "$S"|jget "['loop']['ReadDuration']//1000000" || echo null)" \
    "$(echo "$S"|jget "['loop']['WriteDuration']//1000000" || echo null)" >> "$OUT"
  FIRST=0
}
T0=$(date +%s); PHASE=run
while :; do
  NOW=$(date +%s); EL=$((NOW-T0))
  [ $EL -ge 600 ] && [ $EL -lt 900 ] && [ "$PHASE" = run ] && {
    echo "pausing mock at +${EL}s"; curl -sf -X POST "$MOCK/control/pause" >/dev/null; PHASE=outage; }
  [ $EL -ge 900 ] && [ "$PHASE" = outage ] && {
    echo "resuming mock at +${EL}s (drain boundary)"; curl -sf -X POST "$MOCK/control/resume" >/dev/null
    echo ",\"drain_boundary_t\":$NOW" >> "$OUT"; PHASE=catchup; }
  samp
  [ $EL -ge 1800 ] && break
  sleep 10
done
echo "]}" >> "$OUT"
echo "done -> $OUT"

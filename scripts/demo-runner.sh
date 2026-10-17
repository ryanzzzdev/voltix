#!/usr/bin/env bash
# Condensed demo narrative for the three-pane runner (and the GIF
# capture). Same story as demo-session.sh, paced for live watching:
# dispatch -> link loss -> watchdog -> recovery -> telemetry outage ->
# drain. Assumes `make demo` is up.
set -uo pipefail
DISPATCH="http://127.0.0.1:8180"
DTOK="voltix-demo-dispatch"
step() { echo ""; echo "### $*"; sleep 2; }
jget() { python3 -c "import json,sys;print(json.load(sys.stdin)$1)" 2>/dev/null; }
stat() { curl -sf -H "Authorization: Bearer $DTOK" "$DISPATCH/api/status"; }
simctl() {
  python3 - "$1" "$2" <<'PY'
import socket, json, sys
s = socket.create_connection(('127.0.0.1', 1504), 3); s.settimeout(3)
s.sendall(json.dumps({'cmd': sys.argv[1], 'value': json.loads(sys.argv[2])}).encode() + b'\n')
print(s.recv(256).decode().strip())
PY
}

step "baseline: STANDBY, devices healthy"
stat | jget "['loop']['State']"

step "dispatch -200 kW charge (25 s lease)"
SESS=$(stat | jget "['boot_session_id']")
curl -s -X POST "$DISPATCH/api/dispatch" -H "Authorization: Bearer $DTOK" \
  -d "{\"controller_id\":\"voltix-demo\",\"boot_session_id\":\"$SESS\",\"command_id\":\"$(python3 -c 'import uuid;print(uuid.uuid4())')\",\"revision\":$(date +%s),\"issued_at\":\"$(date -u +%FT%TZ)\",\"valid_until\":\"$(date -u -d '+25 seconds' +%FT%TZ)\",\"target_kw\":-200}" \
  | python3 -c "import json,sys;d=json.load(sys.stdin);print('accepted:',d.get('accepted'))"
sleep 7
stat | python3 -c "import json,sys;l=json.load(sys.stdin)['loop'];print(' state',l['State'],'| req',l['RequestedW'],'| ack',l['Acknowledged'],'| measured',l['MeasuredInvW'])"

step "field link drops (sim set_online=false)"
simctl set_online false
sleep 5
stat | python3 -c "import json,sys;l=json.load(sys.stdin)['loop'];print(' loop:',l['State'],'InverterOK',l['InverterOK'])"

step "independent watchdog trips 10 s after last write"
sleep 8
simctl get_state 'null' | python3 -c "import json,sys;d=json.load(sys.stdin)['state'];print(' sim: output',d['output_kw'],'kW | watchdog_expired',d['watchdog_expired'])"

step "recover the link — STANDBY, intent dropped, no jump"
simctl set_online true
sleep 5
stat | jget "['loop']['State']"

step "telemetry outage: mock paused, queue grows, control unaffected"
curl -s -X POST http://127.0.0.1:8081/control/pause >/dev/null
sleep 6
stat | python3 -c "import json,sys;t=json.load(sys.stdin)['telemetry'];print(' queue depth',t['depth_status'],'| failed',t['upload_failed'])"

step "resume upstream: backlog drains, deduplicated"
curl -s -X POST http://127.0.0.1:8081/control/resume >/dev/null
sleep 8
stat | python3 -c "import json,sys;t=json.load(sys.stdin)['telemetry'];print(' uploaded',t['uploaded'],'| depth',t['depth_status'])"
curl -s http://127.0.0.1:8081/control/count | python3 -c "import json,sys;print(' receiver rows:',json.load(sys.stdin)['count'])"

echo ""; echo "### demo narrative complete"

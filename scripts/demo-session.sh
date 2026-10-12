#!/usr/bin/env bash
# Scripted demonstration for the terminal recording (script/scriptreplay
# capture). Runs normal operation, a field-link fault, recovery, and a
# telemetry outage against the Compose demo. Read-only w.r.t. host.
set -uo pipefail
DISPATCH="http://127.0.0.1:8180"
DTOK="voltix-demo-dispatch"
step() { echo ""; echo "### $*"; sleep 1.5; }
jget() { python3 -c "import json,sys;print(json.load(sys.stdin)$1)" 2>/dev/null; }
statfield() { curl -sf -H "Authorization: Bearer $DTOK" "$DISPATCH/api/status" | jget "$1"; }

step "1. Start the isolated site"
make demo
sleep 4

step "2. Baseline: STANDBY, both devices healthy, telemetry draining"
curl -s -H "Authorization: Bearer $DTOK" $DISPATCH/api/status \
  | python3 -m json.tool | grep -E '"(State|InverterOK|MeterOK|dispatch_active|uploaded)"' | head -6

step "3. Dispatch a -200 kW charge command (25 s lease)"
SESS=$(statfield "['boot_session_id']")
curl -s -X POST $DISPATCH/api/dispatch -H "Authorization: Bearer $DTOK" \
  -d "{\"controller_id\":\"voltix-demo\",\"boot_session_id\":\"$SESS\",\"command_id\":\"$(python3 -c 'import uuid;print(uuid.uuid4())')\",\"revision\":1,\"issued_at\":\"$(date -u +%FT%TZ)\",\"valid_until\":\"$(date -u -d '+25 seconds' +%FT%TZ)\",\"target_kw\":-200}"
sleep 6

step "4. Ramped charge: requested/constrained/ack/measured agree"
curl -s -H "Authorization: Bearer $DTOK" $DISPATCH/api/status \
  | python3 -c "import json,sys; l=json.load(sys.stdin)['loop']; print('  state',l['State'],'| req',l['RequestedW'],'| con',l['ConstrainedW'],'| ack',l['Acknowledged'],'| inv',l['MeasuredInvW'],'| site',l['MeasuredSiteW'])"

step "5. Fault: field link drops (sim set_online=false)"
python3 -c "
import socket,json
s=socket.create_connection(('127.0.0.1',1504),3);s.settimeout(3)
s.sendall(json.dumps({'cmd':'set_online','value':False}).encode()+b'\n');print(' ',s.recv(256).decode().strip())"
sleep 4
curl -s -H "Authorization: Bearer $DTOK" $DISPATCH/api/status \
  | python3 -c "import json,sys; l=json.load(sys.stdin)['loop']; print('  loop sees: state',l['State'],'InverterOK',l['InverterOK'])"

step "6. Independent fallback: sim watchdog trips 10 s after last write"
sleep 8
python3 -c "
import socket,json
s=socket.create_connection(('127.0.0.1',1504),3);s.settimeout(3)
s.sendall(json.dumps({'cmd':'get_state'}).encode()+b'\n')
d=json.loads(s.recv(65536))['state']
print('  sim side: output_kw',d['output_kw'],'| watchdog_expired',d['watchdog_expired'])"

step "7. Recover the link"
python3 -c "
import socket,json
s=socket.create_connection(('127.0.0.1',1504),3);s.settimeout(3)
s.sendall(json.dumps({'cmd':'set_online','value':True}).encode()+b'\n');print(' ',s.recv(256).decode().strip())"
sleep 4
curl -s -H "Authorization: Bearer $DTOK" $DISPATCH/api/status \
  | python3 -c "import json,sys; l=json.load(sys.stdin)['loop']; print('  state',l['State'],'InverterOK',l['InverterOK'],'(intent dropped — no jump)')"

step "8. Telemetry outage: mock paused, queue grows, control unaffected"
curl -s -X POST http://127.0.0.1:8081/control/pause
sleep 6
curl -s -H "Authorization: Bearer $DTOK" $DISPATCH/api/status \
  | python3 -c "import json,sys; t=json.load(sys.stdin)['telemetry']; print('  depth',t['depth_status'],'| failed',t['upload_failed'],'| loop still runs')"

step "9. Resume upstream: backlog drains, deduplicated"
curl -s -X POST http://127.0.0.1:8081/control/resume
sleep 8
curl -s -H "Authorization: Bearer $DTOK" $DISPATCH/api/status \
  | python3 -c "import json,sys; t=json.load(sys.stdin)['telemetry']; print('  uploaded',t['uploaded'],'| depth',t['depth_status'])"
curl -s http://127.0.0.1:8081/control/count

step "10. Deterministic teardown"
make demo-down
echo ""; echo "### demo complete"

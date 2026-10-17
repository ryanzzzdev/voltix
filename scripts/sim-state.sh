#!/usr/bin/env bash
# Pretty-printed simulator state for the demo watch pane — polls the sim
# control socket once per invocation (drive with `watch -n1` or a loop).
python3 - <<'PY'
import socket, json, datetime
try:
    s = socket.create_connection(('127.0.0.1', 1504), 2); s.settimeout(2)
    s.sendall(json.dumps({'cmd': 'get_state'}).encode() + b'\n')
    d = json.loads(s.recv(65536))['state']
except Exception as e:
    print("simulator unreachable:", e)
    raise SystemExit
print(f"[{datetime.datetime.now():%H:%M:%S}] SIMULATED SITE")
print(f"  inverter output : {d['output_kw']:>8} kW")
print(f"  target          : {d['target_kw']:>8} kW")
print(f"  site power      : {d['site_kw']:>8} kW")
print(f"  SoC             : {d['soc_pct']:>8.1f} %")
print(f"  online          : {d['online']}")
print(f"  fault           : {d['fault']}")
wr = d['watchdog_remaining_s']
print(f"  watchdog        : {'EXPIRED -> output forced to 0' if d['watchdog_expired'] else (f'{wr} s remaining' if wr is not None else 'idle (no commands)')}")
PY

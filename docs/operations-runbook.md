# Voltix operations runbook

Verified 2026-10-12 on Linux x86_64 (Ubuntu, systemd 259, Docker 29.7.2,
Go 1.27.2, uv-managed CPython 3.13). Commands were executed as written
on the development host.

## 1. Setup

```bash
git clone <repo> && cd voltix
# Go 1.27.2 (tarball to ~/.local/go works without PATH changes)
curl -sLO https://go.dev/dl/go1.27.2.linux-amd64.tar.gz
tar -C ~/.local -xzf go1.27.2.linux-amd64.tar.gz
make build            # bin/voltix + bin/upstream-mock
make sim-env          # pinned sim environment (uv)
```

Prerequisites per mode:

| Mode | Needs |
| --- | --- |
| Unit/static | `make test`, `make check` — Go only |
| Simulator | `make sim-test` — uv (creates sim/.venv) |
| Integration | `make integration` — Go + sim env |
| Compose demo / faults | Docker + compose plugin; no root, no NET_ADMIN |
| Native service | `systemctl --user` (user manager); cgroup v2 for MemoryMax |

## 2. Credentials

Bearer tokens are environment variables, never in config files:

- `VOLTIX_DISPATCH_TOKEN` — `POST /api/dispatch` + `GET /api/status`
- `VOLTIX_TELEMETRY_TOKEN` — telemetry upload to the mock

Demo defaults (local-only placeholders): `voltix-demo-dispatch` /
`voltix-demo-telemetry` (Compose), `voltix-native-dispatch` /
`voltix-native-telemetry` (native harness). Override via the environment.

## 3. Compose demo

```bash
make demo        # build, start, bounded readiness
make demo-down   # containers + networks + volumes removed (state is disposable)
```

Loopback endpoints: dispatch `127.0.0.1:8180` (8080 avoided — commonly
occupied), status same, sim control `127.0.0.1:1504`, mock `127.0.0.1:8081`.

Send a command (fields from `GET /api/status`; max lifetime 30 s):

```bash
S=$(curl -s -H "Authorization: Bearer voltix-demo-dispatch" \
     http://127.0.0.1:8180/api/status | python3 -c "import json,sys;print(json.load(sys.stdin)['boot_session_id'])")
curl -X POST http://127.0.0.1:8180/api/dispatch \
  -H "Authorization: Bearer voltix-demo-dispatch" \
  -d "{\"controller_id\":\"voltix-demo\",\"boot_session_id\":\"$S\",\"command_id\":\"$(python3 -c 'import uuid;print(uuid.uuid4())')\",\"revision\":1,\"issued_at\":\"$(date -u +%FT%TZ)\",\"valid_until\":\"$(date -u -d '+25 seconds' +%FT%TZ)\",\"target_kw\":-200}"
```

## 4. Native service

```bash
make native-test   # installs user unit, runs 11 checks, removes everything
```

Manual: `deploy/systemd/install.sh` → `systemctl --user start voltix`
(dispatch on `127.0.0.1:8190`, harness ports 2512–2514/8191) →
`journalctl --user -u voltix -f` → `deploy/systemd/remove.sh`.

## 5. Fault playbook

`make fault-test` runs all scenarios on a fresh stack (~3 min). Manual
equivalents:

| Fault | Command | Expected |
| --- | --- | --- |
| Telemetry outage (A) | `curl -X POST :8081/control/pause` (resume: `/control/resume`) | queue grows, `upload_failed` up, dispatch unaffected |
| Dispatch expiry (B) | simply stop refreshing | `dispatch_active=false` by expiry+cycle, output→0 in slew allowance |
| Field link loss (C3) | `{"cmd":"set_online","value":false}` to `127.0.0.1:1504` | `InverterOK=false`; sim watchdog trips 10 s after last write |
| Controller absence (C4) | `docker compose -f deploy/compose.yaml pause voltix` | watchdog trips 10 s; unpause → STANDBY (no jump) |
| Lost ack (D) | `curl -X POST :8081/control/drop-after-commit` (one-shot) | `upload_failed++`, replay deduplicates — receiver rows == uploaded |
| Fast restart (C5) | `kill -9 <pid>` under `native-test` | systemd restarts; new boot session; old session → 409 |

Sim control socket (`:1504`, JSON lines): `ping`, `get_state`,
`set_baseline_kw`, `set_soc_pct`, `set_fault`, `set_online`,
`set_delay_ms`, `set_corrupt`, `set_watchdog_s`, `force_output_kw`,
`reset`. Mock test-only routes: `/control/pause|resume|reset|
drop-after-commit|count`.

## 6. State reset

| Component | Reset |
| --- | --- |
| Compose | `make demo-down` (volumes deleted) then `make demo` |
| Native | `deploy/systemd/remove.sh` removes unit, config, env, `~/.local/state/voltix` |
| Local dev | stop voltix, `rm -rf .state/voltix` (new stream_id on next start) |
| Mock | `POST /control/reset` (test envs only) or delete its dir |

## 7. Cleanup traps and safety

- `fault-test`/`native-test` trap-remove their resources; only
  test-created files are touched.
- No host firewall, routes, or interfaces are modified; all faults are
  application/process-level.
- No service is installed at host scope — the unit is `systemd --user`.

## 8. Known limitations

- Packet-level fault injection (netns/tc/iptables) is unavailable
  without root on this host; CAP_NET_RAW packet capture likewise.
- ENOSPC/disk-full and hardware power loss are unproven (SIGKILL +
  `ulimit` are the exercised bounds).
- Exactly-once delivery is not claimed — at-least-once with receiver
  dedup.
- Loopback-only topology; demo tokens are placeholders, not secrets.
- Measured numbers are dev-host values, not sizing guarantees.

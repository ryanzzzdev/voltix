# Deployment assets

## Isolated Compose demo

Prerequisites: Docker with the Compose plugin (verified with Docker
29.7.2 / Compose v2); no other privileges — all containers run
unprivileged and no NET_ADMIN or host network changes are needed.

```
make demo        # build, start, bounded readiness checks
make demo-down   # remove containers, networks AND volumes (disposable state)
```

Topology:

- `field` network: voltix <-> sim (Modbus inverter :1502, meter :1503)
- `upstream` network: voltix <-> mock (telemetry :8081)

The controller is the only service on both networks. Host bindings are
loopback-only:

| Port | Service | Purpose |
| --- | --- | --- |
| 127.0.0.1:8180 | voltix | dispatch API + authenticated `/api/status` |
| 127.0.0.1:1504 | sim | JSON-line fault control (`{"cmd":"set_fault","value":true}` etc.) |
| 127.0.0.1:8081 | mock | telemetry receiver + test-only `/control/*` routes |

Port 8180 is used for dispatch because 8080 is commonly occupied; change
the `ports:` mapping in `deploy/compose.yaml` if it also conflicts.

Demo tokens default to `voltix-demo-dispatch` / `voltix-demo-telemetry`
(local-only placeholders); override with `VOLTIX_DISPATCH_TOKEN` and
`VOLTIX_TELEMETRY_TOKEN`. Never put real tokens in compose files.

Sim control ops (one JSON object per line): `ping`, `get_state`,
`set_baseline_kw`, `set_soc_pct`, `set_fault`, `set_online`,
`set_delay_ms`, `set_corrupt`, `set_watchdog_s`, `force_output_kw`,
`reset`.

Mock test-only routes (documented as non-production): `POST
/control/pause`, `POST /control/resume`, `POST /control/reset`,
`POST /control/drop-after-commit` (one-shot), `GET /control/count`.

## Dispatch command schema

`POST /api/dispatch` requires
`{"controller_id","boot_session_id","command_id","revision","issued_at","valid_until","target_kw"}`;
`boot_session_id` comes from `GET /api/status`. `issued_at`/`valid_until`
are RFC 3339 timestamps; the controller converts validity to monotonic
time at acceptance.

## Notes

- The Compose demo is not the systemd deployment; native service
  verification lives under `deploy/systemd/` (separate evidence).
- Config decoding is strict: unknown keys in the YAML are rejected, so
  `configs/compose.yaml` lists only wired settings.

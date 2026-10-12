# Terminal demonstration recording

Artifact policy: recordings are util-linux `script` typescripts +
timing files kept under `evidence/demo/` and committed when small
(<1 MB). No hosted upload; replay locally.

Recorded 2026-10-12 against the Compose demo via `scripts/demo-session.sh`.

## Playback

```bash
scriptreplay -t evidence/demo/demo.timing -d 1 evidence/demo/demo.typescript
```

(`scriptreplay` is part of util-linux; `-d` sets the speed divisor.)

## Contents

1. `make demo` — isolated stack bring-up with bounded readiness.
2. Baseline STANDBY, devices healthy.
3. Dispatch −200 kW charge, 25 s lease → accepted.
4. ACTIVE with requested/constrained/acknowledged/measured all −200 kW.
5. Field-link fault injected via sim control (`set_online=false`) →
   loop reports DEGRADED / InverterOK=false.
6. Independent fallback: sim watchdog_expired, output ramped to 0.
7. Link recovery → STANDBY, intent dropped, no jump.
8. Telemetry outage (mock pause) → queue depth grows, loop unaffected.
9. Resume → backlog drains to the deduplicating receiver.
10. `make demo-down` — all containers/networks/volumes removed.

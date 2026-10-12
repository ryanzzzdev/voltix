# M2 control verification findings

Date: 2026-10-12
Milestone: M2 — Supervisory control ([roadmap](../../docs/project/roadmap.md))
Scope: end-to-end verification of the supervisory control slice — dispatch
API, operating states, policy bounds and recovery — against the real
`voltix` binary and the Python simulator over TCP. See
[control contract](../../docs/project/features/control.md) and
[verification](../../docs/project/features/verification.md).

These findings describe the *simulated* slice only. They do not constitute
physical safety guarantees; a 1 Hz sampled policy cannot prevent transient
site-limit exceedance between readings.

## Workload

- Controller: `bin/voltix -config <generated>` with 200 ms cycle period,
  200 ms endpoint deadline, 2.5 s max loop gap, 5%/95% SoC bounds,
  400 kW export limit, 50 kW/s slew.
- Simulator: `python -m voltix_sim` on independent inverter/meter ports;
  faults, offline, delay, SoC and baseline injected via its control socket.
- Driver: `internal/control/e2e_test.go` (`make integration`, build tag
  `integration`) — one `TestControlEndToEnd` covering all scenarios below.

## Results

All scenarios pass; total e2e wall time ~26 s. Unit suites: policy 11,
dispatch 10, control 9 tests; `go test -race ./...` clean.

| Scenario | Evidence |
|---|---|
| Standby | `loop.State=STANDBY`, output stays 0 kW with no dispatch |
| Valid discharge | +200 kW accepted (202); measured output ramps at ≤50 kW/s (mid-ramp <150 kW at ~1.2 s), settles ≥195 kW |
| Valid charge | −100 kW accepted; output crosses zero, settles ≤−95 kW |
| Expiry fallback | 4 s lease: `dispatch_active` flips false at expiry; time-to-zero measured *separately* (>300 ms after fallback start) |
| SoC floor | At 4% SoC a fresh +80 kW command yields `ConstrainedW=0` |
| Infeasible baseline | 450 kW baseline export + SoC 95%: charge prohibited and export over limit → `SITE_LIMIT_INFEASIBLE` reported, target 0 |
| Device fault | `set_fault` → `FAULT` state, output → <5 kW; clears on `set_fault false` |
| Offline/stale reads | `set_online false` → `DEGRADED`; recovers on reconnect |
| Restart replay | Old `boot_session_id` + rev 99 → 409 after restart; new session accepts rev 1 (revision state dies with the boot) |
| Malformed/auth | Unknown field → 400; missing token → 401 |
| Delayed writes | 400 ms device delay (>200 ms deadline) → `Acknowledged=null` (uncertain), not silently retried; recovers when delay cleared |
| Loop pause | SIGSTOP 3 s > 2.5 s gap → intent invalidated on resume (`dispatch_active=false`) |
| No recovery jump | Fresh +200 kW after recovery ramps from measured ~0; never exceeds 130 kW within 2 s |

## Contract notes and limits

- Expiry/gap `DEGRADED` lasts exactly one cycle: the intent is dropped and
  subsequent cycles are `STANDBY`. Durable evidence of fallback is
  `dispatch_active=false`, not the transient state — the e2e asserts on it.
- `Acknowledged` means the device accepted the write at the protocol level;
  it does not prove the inverter tracked it (`MeasuredInvW` covers that).
- A delayed device costs up to ~2× the endpoint deadline per cycle
  (timeout → reconnect → timeout) before the write reports uncertain.
- `MeasuredAt` is the cycle read-start time, so input staleness equals
  read latency; both endpoint reads share one deadline.
- `SITE_EXPORT_LIMIT` is flagged only when the export bound actually clamps
  the requested target, not merely when headroom shrinks.

## M2 exit review

| Exit criterion | Status |
|---|---|
| Valid charge/discharge; malformed/expired/conflicting/old-session/out-of-order cannot revive output | Met — e2e scenarios 2, 3, 8, 9 + dispatch unit tests |
| SoC/device/export boundary tests; infeasible reported not satisfied | Met — policy unit tests + e2e scenarios 5, 5b |
| Expiry begins fallback within polling tolerance; arrival does not renew replays | Met — e2e scenario 4 + idempotent-retry test |
| Invalid/stale input, gaps, failed writes, restarts per documented policy | Met — e2e scenarios 7, 10, 11, 8 |
| Slow consumers cannot block control; timings include reads and writes | Met — bounded snapshot channel test + `ReadDuration`/`WriteDuration` |

# M4 exit review — reproducible checks and demo evidence

Date: 2026-10-12. Reviewer: Devin (SWE-2 High). Scope: roadmap M4 exit
criteria and the verification-matrix rows, reconciled against actual
results on the development host (Ubuntu, systemd 259, Docker 29.7.2,
Go 1.27.2, uv/CPython 3.13).

Revised 2026-10-12 after the independent audit
(`docs/project/evidence/review/m4-independent-audit.md`): row F
re-labelled, fault/native check counts updated to the re-run totals, and
the unavailable checks remain explicitly open rather than claimed.

## Acceptance-matrix reconciliation

| ID | Scenario | Result | Evidence |
| --- | --- | --- | --- |
| P1 | Protocol fixtures | Met (M1) | 17 golden PDUs from vendored sunspec/models@90b4a33; decode tests; evidence/device-integration |
| P2 | Device timeout | Met | Sim watchdog trips 10 s after last accepted write — verified M1 e2e and twice in fault-test (C3, C4) |
| C1 | Dispatch + limits | Met (M2) | e2e: ramp ≤ slew, SoC floor, infeasible-baseline rejection; evidence/control |
| C2 | Replayed/stale commands | Met | 409 on old session/revision; identical retry does not extend expiry (unit + e2e + native-test) |
| A | Telemetry-only outage | Met | fault-test: queue 5→17, upload_failed=4, dispatch accepted, loop ACTIVE, drained after resume |
| B | Dispatch expiry | Met | fault-test: fallback start expiry+0 s (≤1 cycle), output 0 at expiry+2 s (within slew allowance) |
| C3 | Field-link outage | Met | fault-test: InverterOK=false reported; independent watchdog trip at 10 s; inverter self-ramps to 0 |
| C4 | Sustained absence | Met | fault-test: docker pause → watchdog 10 s; unpause → STANDBY (loop-gap drop, no jump) — distinct from C5 |
| C5 | Fast restart | Met (native) | native-test: SIGKILL → new boot session in 2.68 s; old session → 409 |
| D | Lost acknowledgement | Met | fault-test: upload_failed recorded; replay deduplicated (rows == uploaded); also M3 unit/e2e |
| E | Saturation/alarm flood | Met (M3) | e2e small-cap overflow: per-tier eviction counters, control intact; all-alarms unit test |
| F | Disk full/busy | Met (EFBIG class); true ENOSPC open | `TestStorageWriteFailureSurfacedAndRecoverable`: RLIMIT_FSIZE subprocess, kernel write refusal → unhealthy + volatile drop + intact reads + recovery. Filesystem ENOSPC needs a bounded mount — unavailable; see [decision 004](../../docs/project/decisions/004-local-fault-injection-scope.md) |
| G | Loop gap/invalid samples | Met | SIGSTOP e2e (M2) + fault-test C4 resume → STANDBY, no dt-jump |
| H | Resource/catch-up run | Met | 30-min run + 5-min outage; see measurements below |

## Resource exercise (H)

`scripts/resource-run.sh`: 30 min at 1 Hz (production-default limits:
90 000/10 000 events, 900/100 handoff), mock paused +10 min→+15 min.
Samples every 10 s in `resource-run.json` (183 samples).

Measured 2026-10-12, duration 1806 s, voltix on native harness ports:

| Metric | Result |
| --- | --- |
| Events produced/uploaded | 1805 status+alarm events, all acked |
| Outage window | +607 s → +904 s (mock pause ≈ 5 min) |
| Queue at resume | depth 298 (~1 event/s during outage) |
| Catch-up drain | 298 → ≤5 depth in ~10 s — replay far faster than production |
| upload_failed during outage | 14 total — bounded backoff, no hot loop |
| voltix RSS | 14.4 MB start → 21.1 MB peak → 20.0 MB end (bounded) |
| Loop read duration | 1–8 ms throughout incl. catch-up — no jitter degradation |
| Loss during outage | zero persistent loss, zero evictions |

## Reproducible checks

| Command | What it proves | Ran locally |
| --- | --- | --- |
| `make test` / `check` | unit + gofmt/vet | yes 2026-10-12 |
| `go test -race ./...` | race-clean | yes |
| `make sim-test` | sim physics + wire (26) | yes |
| `make integration` | device + control + telemetry e2e | yes |
| `make demo` / `demo-down` | isolated stack, bounded readiness, deterministic teardown | yes (twice fresh) |
| `make fault-test` | A/B/C3/C4/D — 17 checks | yes — 17/17 (2026-10-12, post-audit baselines) |
| `make native-test` | user unit, restart, MemoryMax, journal + isolation — 13 checks | yes — 13/13 (2026-10-12) |

CI configuration: `.github/workflows/ci.yml` covers gofmt/vet/test/-race/
build + sim tests + loopback integration — nonprivileged checks only.
Demo/fault/native remain the documented local path (require a Docker
daemon / user systemd). **Hosted CI is unverified** — no remote is
authorised or configured; no run has been observed.

## Limitations and follow-ups

- **F (disk-full)**: kernel-enforced write failure (EFBIG via
  RLIMIT_FSIZE) exercised at the store durability boundary — surfaced,
  contained, recoverable. A genuine filesystem ENOSPC still requires a
  bounded mount this host cannot provide; tracked as open follow-up under
  decision 004, not claimed as passed.
- **Packet-level faults**: unprivileged netns/NET_ADMIN/CAP_NET_RAW
  denied on this host; faults are application/process-level. A packet
  trace could not be captured; request→timeout→reconnect is evidenced
  through `upload_failed`/`InverterOK`/watchdog counters instead.
- **MemoryMax**: cgroup enforcement verified; actual OOM-under-load
  not exercised.
- **Hardware power loss** and physical storage guarantees unproven
  (SIGKILL ≈ crash only).
- **Hosted CI / publication**: unverified, no remote; ARM and real
  hardware remain out of scope.
- 30 s command-lifetime bound and loopback-only demo are design
  constraints, not failures.

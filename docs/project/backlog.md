# Voltix backlog

Updated: 2026-10-12

Task bodies own status; the ascending index mirrors it. Follow the [task conventions](documentation-guide.md#task-records). No application implementation has started.

## Task index

- ✅ [VTX-001 — Establish project-management standards](#vtx-001)
- ✅ [VTX-002 — Establish the implementation plan and initial backlog](#vtx-002)
- ✅ [VTX-003 — Bootstrap the minimal development workspace](#vtx-003)
- ✅ [VTX-004 — Verify and freeze the simulator wire profile](#vtx-004)
- ✅ [VTX-005 — Implement the battery-site simulator and independent watchdog](#vtx-005)
- ✅ [VTX-006 — Implement bounded Modbus device adapters](#vtx-006)
- ✅ [VTX-007 — Verify the complete device integration slice](#vtx-007)
- ✅ [VTX-008 — Implement pure supervisory operating policies](#vtx-008)
- ✅ [VTX-009 — Implement authenticated dispatch sessions and leases](#vtx-009)
- ✅ [VTX-010 — Integrate the control loop and diagnostic snapshots](#vtx-010)
- ✅ [VTX-011 — Verify control boundaries and recovery end to end](#vtx-011)
- ✅ [VTX-012 — Implement the bounded durable event store](#vtx-012)
- ✅ [VTX-013 — Implement the transactional upstream mock](#vtx-013)
- ✅ [VTX-014 — Implement bounded upload and acknowledgement handling](#vtx-014)
- ✅ [VTX-015 — Integrate telemetry and verify resource and storage failures](#vtx-015)
- ✅ [VTX-016 — Package the isolated container demonstration](#vtx-016)
- ✅ [VTX-017 — Package and verify the native Linux service](#vtx-017)
- ✅ [VTX-018 — Automate isolated Linux failure scenarios](#vtx-018)
- ✅ [VTX-019 — Document operation and record the portfolio demonstration](#vtx-019)
- ✅ [VTX-020 — Review milestone exits and reproducible checks](#vtx-020)

- ✅ [VTX-021 — Independently audit milestone completion](#vtx-021)

## Delivery notes

Independent audit VTX-021 found unmet completion criteria and two reproduced control regressions; all five reopened tasks were corrected and re-verified 2026-10-12 (see per-task notes, commits e05caa2/6715588/8ba02d9/695846f). Remaining: [VTX-022](#vtx-022) — privileged-environment verification of true ENOSPC and packet-level faults, open follow-up under decision 004. Findings and priorities: [audit](evidence/review/m4-independent-audit.md). Next unused task ID: VTX-023.

<a id="vtx-001"></a>

## VTX-001 — Establish project-management standards

- Repository: voltix
- Area: documentation
- Milestone: Documentation setup
- Priority: High
- Status: Done ✅
- Depends on: None
- References: [Guide](documentation-guide.md), [decision 001](decisions/001-markdown-project-tracking.md)

### Outcome

Adapt NEMLab's agent-assisted Markdown project workflow to Voltix before planning implementation.

### Done when

- Root README and AGENTS instructions point to one canonical documentation guide.
- Guide defines record ownership, stable IDs/statuses, templates, acceptance/evidence requirements, session updates, and UTC model/agent-attributed journaling.
- Roadmap, backlog, architecture, blockers, decision, and journal accurately record the setup without inventing implementation progress or approving background proposals.
- Local links, task/index status, and preservation of existing discussion files are checked.

### Completion evidence

Reviewed NEMLab's README, AGENTS, documentation guide, Markdown-tracking decision, milestone-separation decision, and related task, milestone, blocker, architecture, specification, and journal records. Created the Voltix documentation listed in the [README](../../README.md).

Local documentation links and anchors, task/index status, required journal fields, and preservation of existing discussion files were checked successfully. No application tests apply to this documentation-only change. Files are uncommitted; Git status reported no usable repository and Git was not initialised.

### Notes

Implementation planning and bootstrapping are deliberately deferred to the next session. Existing proposals remain under `docs/initial-discussion/` without edits.

<a id="vtx-002"></a>

## VTX-002 — Establish the implementation plan and initial backlog

- Repository: voltix
- Area: documentation
- Milestone: Planning baseline
- Priority: High
- Status: Done ✅
- Depends on: VTX-001
- References: [Roadmap](roadmap.md), [architecture](architecture.md)

### Outcome

Reconcile the reviewed proposals into canonical scope, architecture, behavioural contracts, and a dependency-ordered initial backlog.

### Done when

- Milestones have scope, dependencies, exclusions, and measurable exit criteria.
- Architecture and contracts resolve reviewed contradictions and label implementation gates.
- Initial tasks have outcomes, acceptance criteria, dependencies, and truthful status.
- Links, task dependency consistency, source preservation, and journal attribution are checked.

### Completion evidence

Established the roadmap, architecture, four behavioural/verification specifications, decisions 002/003, three bounded implementation gates and tasks VTX-003–020. Checked local links/anchors, sequential task IDs, dependency consistency, Ready-state prerequisites, journal attribution and unchanged discussion sources. No application implementation or tests were performed; these documents are uncommitted.

### Notes

The user requested detailed planning on 2026-10-12. Bootstrap follows planning as a separate task.

<a id="vtx-003"></a>

## VTX-003 — Bootstrap the minimal development workspace

- Repository: voltix
- Area: integration
- Milestone: M1
- Priority: High
- Status: Done ✅
- Depends on: [VTX-002](#vtx-002)
- References: [architecture](architecture.md)

### Outcome

Establish reproducible Go and Python development commands without scaffolding future packages.

### Done when

- Inspect the existing directory/Git state and preserve all documents; initialise a local repository only when beginning the authorised bootstrap, without inventing a remote or making a commit.
- Select and pin supported Go/Python versions and dependencies with compatibility evidence; record the actual module path decision without assuming the example GitHub username is verified.
- Create one root Go module, minimal voltix entry point, pinned simulator dependency configuration, ignore rules and example configuration without secrets.
- Implement working make build/test/check commands for the code that exists, plus clean-setup instructions; run build, smoke and applicable checks. Do not present unimplemented later targets as passing.

### Completion evidence

Directory/Git state inspected; existing documents preserved and no remote invented. Go 1.27.2 installed from the official linux-amd64 tarball to `~/.local/go` with sha256 checked against the published digest (`ecbadb99…45fe5`); no sudo required. Module path `voltix` chosen by the user; `go.mod`, `cmd/voltix/main.go`, `Makefile`, `.gitignore`, `configs/example.yaml` (no secrets) and `sim/pyproject.toml` created. Simulator dependencies pinned: pymodbus 3.15.0 and pytest 9.1.1 under uv-managed CPython 3.13.14 with `sim/uv.lock` committed; pymodbus 3.16.x deliberately skipped because it was published 2026-10-11, under seven days old. Verified on 2026-10-12: `go mod tidy`, `make build`, `make test` (no test files yet — reported honestly), `make check` (gofmt + vet), and `./bin/voltix --config configs/example.yaml` smoke run all pass. Clean-setup instructions added to README.

### Notes

Done in one checkpoint of the M1 campaign. `make test` has no Go tests yet; first test code arrives with VTX-005/006.

<a id="vtx-004"></a>

## VTX-004 — Verify and freeze the simulator wire profile

- Repository: voltix
- Area: documentation
- Milestone: M1
- Priority: High
- Status: Done ✅
- Depends on: [VTX-002](#vtx-002)
- References: [device profile](features/device-profile.md), [blockers](blockers.md#blk-001)

### Outcome

Turn the selected SunSpec subset into an independently checked register contract before either side depends on it.

### Done when

- Pin upstream model definitions/specification revision; produce a manifest for discovery/common/end layout and models 103, 124, 704 and 203 with address base, unit IDs, offsets, lengths, types, access, scaling and invalid values.
- Specify watt target enable/mode/write sequence, multi-register atomicity, readback, meter sign normalisation, mandatory values and declared unsupported fields; resolve BLK-001 or narrow the profile explicitly.
- Provide independently reviewed golden request/response fixtures for charge/discharge, SoC, site power, sentinel and malformed cases; document provenance/license for any copied source assets.
- Keep simulator-specific watchdog behaviour distinct from claimed manufacturer or full-standard conformance.

### Completion evidence

Pinned `sunspec/models` at `90b4a331dcca1d6eac69c1bead952fddcc5852e0` (2026-08-20, Apache-2.0) and vendored models 1/103/124/203/704 plus LICENSE under `contracts/upstream/`. `contracts/generate_profile.py` derives `contracts/sunspec-profile.json` — base 40000, unit IDs, model order and per-point offsets/types/access/scale factors/sentinels, plus the atomic FC16 WSetEna..WSet write window — and 17 golden PDU fixtures covering discovery, discharge/charge (negative watts), SoC boundaries, meter import/export, sentinels and exception cases. Fixtures were computed from upstream JSONs before either implementation exists and spot-checked by hand (e.g. -250000 W = 0xFFFC2F70; 40170 = 0x9CEA). BLK-001 resolved. Enums verified from upstream symbols (WSetMod WATTS=1, ChaSt DISCHARGING/CHARGING, St MPPT=4/FAULT=7).

### Notes

Meter positive=export and the 10 s watchdog refresh rule are declared simulator policy in the manifest, not manufacturer claims. Adapter must read SF registers dynamically even though the profile fixes their values.

<a id="vtx-005"></a>

## VTX-005 — Implement the battery-site simulator and independent watchdog

- Repository: voltix
- Area: simulator
- Milestone: M1
- Priority: High
- Status: Done ✅
- Depends on: [VTX-003](#vtx-003), [VTX-004](#vtx-004)
- References: [device profile](features/device-profile.md)

### Outcome

Provide changing battery and meter values plus an independently timed command fallback.

### Done when

- Serve inverter and meter on separate configurable unprivileged endpoints using the checked manifest; supply deterministic scenario configuration and a test-only fault interface.
- Evolve bounded energy and measured output separately from targets; compute site power from shared battery state and configurable baseline.
- Use monotonic time and ensure accepted power refresh alone renews the ten-second watchdog; blocking clients cannot stop the simulator clock.
- Unit tests cover energy/sign boundaries, invalid writes, unrelated-write/poll non-refresh, timeout start and time-to-zero; record passing checks.

### Completion evidence

`sim/voltix_sim/` implements a pure `Site` physics core (bounded energy/SoC, 250 kW/s track, 100 kW/s fallback, monotonic watchdog) plus a pymodbus 3.15 `SimDevice` layer: separate inverter (models 1/103/124/704) and meter (1/203) endpoints rendered from the frozen manifest, with a single `action` hook that advances physics per request, rejects malformed/partial/unrelated writes via exception codes, and refreshes the watchdog only inside the atomic FC16 WSetEna..WSet window (40170–40173). A line-oriented JSON control socket (`control.py`) provides test-only state/fault/delay/baseline control. Wire tests replay the golden fixtures over real TCP and also caught two fixture bugs (203 W/W_SF not adjacent; -2500 int16 is 0xF63C). Verified 2026-10-12: `uv run pytest -q` → 26 passed; `python -m voltix_sim` smoke run serves SunS discovery and control state.

### Notes

Simulator watchdog/ramp policy remains declared sim behaviour, not manufacturer conformance. `sentinel-sunssf` is retained as a decoder-side fixture for VTX-006; the sim's corrupt flag emits value sentinels only.

<a id="vtx-006"></a>

## VTX-006 — Implement bounded Modbus device adapters

- Repository: voltix
- Area: controller
- Milestone: M1
- Priority: High
- Status: Done ✅
- Depends on: [VTX-003](#vtx-003), [VTX-004](#vtx-004)
- References: [device profile](features/device-profile.md), [control](features/control.md), [blockers](blockers.md#blk-002)

### Outcome

Expose narrow typed inverter/meter operations with explicit errors and bounded socket behaviour.

### Done when

- Select/pin a maintained Go Modbus library after verifying deadline and connection-ownership semantics; document the result and resolve the Modbus part of BLK-002.
- Implement discovery/profile validation, read scaling/sentinels, sign conversion and absolute target writes using checked fixtures.
- Serialise operations per endpoint; enforce actual read/write deadlines and bounded reconnect; cancellation must not leak goroutines or leave unbounded socket operations.
- Tests cover delayed/partial/malformed responses, exceptions, disconnect/reconnect, overflow and signed targets. A write timeout is reported as uncertain rather than assumed unapplied.

### Completion evidence

Selected `github.com/simonvetter/modbus` v1.6.4 (MIT, maintained): verified in source that `tcpTransport.ExecuteRequest` applies a real `socket.SetDeadline` per request, requests are mutex-serialised per client, and i/o timeouts map to `ErrRequestTimedOut`. `internal/device` owns one lazily-connected `Endpoint` per device; reads reconnect+retry once, writes never replay and report `ErrWriteUncertain` on timeout. `Discover` validates the SunS marker and walks the bounded model chain; `ReadInverter`/`ReadMeter` decode W/SoC/St/Evt1 with per-point sentinel rejection; `SetPowerTargetW` emits the atomic FC16 window. Addresses come from `internal/device/profile.go`, generated by `contracts/generate_profile.py` from the frozen manifest. Verified 2026-10-12: `make test` passes discovery, typed reads, signed/negative writes, sentinel rejection (incl. `sentinel-sunssf`), delayed→timeout, malformed→bounded error, hangup→reconnect and uncertain-write cases against a minimal fake Modbus server plus golden-fixture decode tests. Modbus sub-gate of BLK-002 resolved.

### Notes

The library's dial uses a fixed 5 s connect timeout (per-connect bound, documented). Malformed garbage that cannot form a frame surfaces as `ErrTimeout` at the socket deadline — bounded either way.

<a id="vtx-007"></a>

## VTX-007 — Verify the complete device integration slice

- Repository: voltix
- Area: integration
- Milestone: M1
- Priority: High
- Status: Done ✅
- Depends on: [VTX-005](#vtx-005), [VTX-006](#vtx-006)
- References: [device profile](features/device-profile.md), [verification](features/verification.md), [findings](../../evidence/device-integration/findings.md)

### Outcome

Demonstrate Go-to-Python reads, writes and failure recovery and review the M1 exit evidence.

### Done when

- Start the simulator and Go adapter reproducibly; verify changing SoC/site readings and accepted versus measured positive/negative output.
- Cross-check observed wire values against independent fixtures, not only shared implementation assumptions.
- Exercise dropped/delayed device replies and loss of command refresh, measuring driver return bounds and device fallback independently.
- Record commands/environment/results in evidence/device-integration/findings.md and confirm each M1 exit criterion; leave gaps open rather than claiming completion.

### Completion evidence

`make integration` (new target, `go test -tags=integration`) launches `python -m voltix_sim` as a subprocess and exercises the full slice in `TestDeviceIntegrationSlice` (14.3 s): discovery on both endpoints, live SoC/baseline changes, accepted-vs-measured +250/-250 kW including the negative charging path, WSet-window readback matching the golden write fixture, watchdog expiry under continuous polling to zero output with Evt1 bit0, bounded delayed/offline replies and recovery, and corrupt-value rejection. All M1 exit criteria confirmed; details and limitations in [evidence/device-integration/findings.md](../../evidence/device-integration/findings.md).

### Notes

The test found one real test-authoring bug during development: the watchdog deadline runs from the accepted write, not from when tracking was first observed — corrected before acceptance.

<a id="vtx-008"></a>

## VTX-008 — Implement pure supervisory operating policies

- Repository: voltix
- Area: controller
- Milestone: M2
- Priority: High
- Status: Done ✅
- Depends on: [VTX-007](#vtx-007)
- References: [control](features/control.md)

### Outcome

Implement deterministic policy calculation isolated from sockets, HTTP and storage.

### Done when

- Represent validated measurements with ages, command expiry and explicit state/reason outcomes using injectable time.
- Test charge/discharge limits, exact SoC boundaries, sampled site-export bounds and infeasible intervals.
- Test hard-limit precedence over normal slew, long-gap fallback, and fresh measured ramp baseline after startup/recovery.
- Boundary tests cover invalid/non-finite values, stale input and expired intent; no hidden device or database side effects.

### Completion evidence

`internal/policy` implements `Supervisor.Step` as a pure function over injected `Inputs` (readings + validity flags + monotonic `Now`/`MeasuredAt`/`Intent.ExpiresAt`), returning `Decision{State, TargetW, Invalidate, Reasons}`. The ACTIVE path forms the hard interval (charge/discharge limits, exact 5%/95% SoC edges, export bound via `baseline = meter − inverter`), clamps the intent, applies ≤1 s slew (measured baseline on first/recovery cycles), then re-applies hard bounds — matching the contract's precedence. Gaps >2.5 s invalidate intent without multiplying the stale elapsed time; fault/stale/invalid/expired inputs yield DEGRADED/FAULT with ramp-to-zero. Verified 2026-10-12: 11 tests pass covering all boundary cases in the Done-when list; `go vet` clean; the package imports only `math`/`time`.

### Notes

Deliberate simplification for testability: snapshot freshness is a single `MeasuredAt` rather than per-endpoint ages; VTX-010 wires it from both read results.

<a id="vtx-009"></a>

## VTX-009 — Implement authenticated dispatch sessions and leases

- Repository: voltix
- Area: controller
- Milestone: M2
- Priority: High
- Status: Done ✅
- Depends on: [VTX-007](#vtx-007)
- References: [control](features/control.md)

### Outcome

Accept bounded local commands with replay-resistant expiry and observable acknowledgement semantics.

### Done when

- Implement POST /api/dispatch and authenticated redacted status with a random boot session and strict bounded schema.
- Validate identity, monotonic command revisions, finite targets, UTC expiry/lifetime and bearer token; responses distinguish API acceptance from device application.
- Identical retries preserve the original deadline; changed duplicates/old revisions/old sessions fail; test forward/backward clock changes with injected clocks.
- Tests cover restart/session invalidation, concurrent requests, expired commands, malformed/oversized input and secret redaction; a heartbeat cannot renew dispatch implicitly.

### Completion evidence

`internal/dispatch` implements `POST /api/dispatch` and `GET /api/status` behind a constant-time bearer check, with a random 16-byte boot session ID per start. `Session.accept` enforces controller/boot-session identity (409), UUID `command_id` and finite `target_kw` (422), strictly increasing revisions (409 — old revisions stay rejected after expiry, per-boot-session state), exact-retry idempotence preserving the original monotonic deadline (202), same-revision changed-fields conflict (409), `issued_at <= now < valid_until` with a 0<life<=30 s bound (422), and converts the remaining wall lifetime to a monotonic deadline at acceptance. `Active` checks BOTH the mono deadline and wall `valid_until`, so forward clock jumps expire conservatively and backward steps cannot extend. Bodies are capped at 4 KiB with unknown-field rejection (400); status is redacted (no token/secret fields); heartbeats (status polls) do not extend leases. Verified 2026-10-12: 10 tests pass including injected forward/backward clock shifts, restart replay rejection, idempotent retry, and 64-way concurrent accepts; `go test -race ./...` clean.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-010"></a>

## VTX-010 — Integrate the control loop and diagnostic snapshots

- Repository: voltix
- Area: controller
- Milestone: M2
- Priority: High
- Status: Done ✅
- Depends on: [VTX-008](#vtx-008), [VTX-009](#vtx-009)
- References: [architecture](architecture.md), [control](features/control.md)

### Outcome

Wire the controller to real adapters while keeping one owner for state and device writes.

### Done when

- Implement startup fresh-read/zero policy, nominal 1 Hz orchestration, concurrent independent reads, serialised writes, and bounded shutdown.
- Include read/write phases in timing metrics; expose requested, constrained, acknowledged/uncertain and measured output plus input ages and reason codes.
- Invalidate active intent on stale input/long stalls; failed writes do not trigger stale retries. Recover only under the documented state/command rules.
- Publish snapshots through a bounded nonblocking interface; a fake slow consumer cannot stall the loop. Run relevant tests and Go race checks.

### Completion evidence

`internal/control` implements the single-owner 1 Hz loop: concurrent inverter/meter reads (per-cycle goroutines bounded by endpoint deadlines) feed `policy.Step`; writes are serialised through the one loop goroutine and issued fresh each cycle — a failed write sets `Acknowledged=nil` (uncertain) and is never replayed. `Decision.Invalidate` drops the dispatch intent (stale/invalid reads, >2.5 s gaps, lease expiry — the latter mapped from the session's monotonic deadline onto cycle `Now`). `Snapshot` carries state, reasons, requested/constrained/acknowledged/measured watts, read+write phase durations (input age == read latency), and publishes via a bounded nonblocking channel — a stopped consumer costs snapshots, never loop time. `Run` shuts down on ctx cancel with one bounded zero write. `cmd/voltix` now wires the real adapters: YAML config (yaml.v3), SunSpec model-chain identity validation at startup, bearer token from `VOLTIX_DISPATCH_TOKEN` only, loopback dispatch listener, and the loop snapshot merged into `GET /api/status` as `loop`. Verified 2026-10-12: 9 control tests pass (active/standby/degraded/fault paths, uncertain-write, slow-consumer non-stall, gap invalidation, lease expiry, bounded shutdown); `go test -race ./...` clean; `go vet` clean.

### Notes

Simplification recorded: `MeasuredAt` is the cycle read-start time, so "input age" equals read latency this cycle — per-endpoint ages were judged redundant given both reads share one deadline. `Acknowledged` is only set after the device accepts the write; it does not prove the inverter tracked it — honest uncertainty, per contract.

<a id="vtx-011"></a>

## VTX-011 — Verify control boundaries and recovery end to end

- Repository: voltix
- Area: integration
- Milestone: M2
- Priority: High
- Status: Done ✅
- Depends on: [VTX-010](#vtx-010)
- References: [control](features/control.md), [verification](features/verification.md)

### Outcome

Prove the M2 behaviour against real simulator and dispatch processes.

### Done when

- Exercise valid charge/discharge, SoC/export/ramp limits, infeasible baseline and device fault cases.
- Verify expiry starts fallback within the documented tolerance, measure time to zero, and reject replay/old-session commands after restart.
- Inject stale/invalid readings, delayed writes and a prolonged loop pause; verify uncertain output is reported truthfully and recovery does not jump to an old target.
- Record M2 exit findings with workload and timing evidence in evidence/control/findings.md; do not extrapolate to physical safety guarantees.

### Completion evidence

`internal/control/e2e_test.go` (build tag `integration`, `make integration`) runs the real `bin/voltix` against the real simulator and covers every required scenario in a single 26 s pass: standby, discharge/charge ramping at ≤50 kW/s to target, expiry fallback with fallback-start vs time-to-zero measured separately, SoC floor inhibit, infeasible baseline reported as `SITE_LIMIT_INFEASIBLE`, device fault inhibit+clear, offline/stale degradation, old-session replay rejection after restart with fresh-session rev-1 acceptance, 400/401 paths, delayed-write `Acknowledged=null` uncertainty and recovery, SIGSTOP-induced gap invalidation, and no recovery jump to an old target. Findings recorded in `evidence/control/findings.md` with the M2 exit review — all five exit criteria met. `go vet` clean; unit+races green (policy 11, dispatch 10, control 9).

### Notes

Reopened by the independent 2026-10-12 [audit](evidence/review/m4-independent-audit.md). Prior completion evidence below/above remains historical; address the audit findings assigned to this task before claiming its acceptance criteria met.

Corrective cycle 2026-10-12 (commit e05caa2): device fault now sets `Invalidate` — intent dropped, FAULT output ramped to 0, and post-clear recovery requires a newly accepted dispatch (was: stale intent silently resumed). Read/write phases are bounded by `read_deadline_ms`/`write_deadline_ms` (now actually wired through strict config); `MeasuredAt` is acquisition-start and `DecisionAt` is post-read, so a slow successful read beyond `max_input_age_s` reports STALE_INPUT and invalidates; a failed/expired write calls `Policy.WriteFailed`, resetting the ramp baseline to measured output rather than the unacknowledged request. New regressions: fault drop+zero+no-auto-resume, slow-read staleness, bounded read/write phases, write-failure rebaseline. `go test ./internal/control ./internal/policy` green; `-race ./...` clean; full integration e2e 45.8 s.

Design truth surfaced by the test: expiry/gap DEGRADED is a single-cycle state — after intent invalidation the loop is STANDBY, so the e2e asserts durable evidence (`dispatch_active=false`) rather than racing the transient state.

<a id="vtx-012"></a>

## VTX-012 — Implement the bounded durable event store

- Repository: voltix
- Area: controller
- Milestone: M3
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-003](#vtx-003)
- References: [telemetry](features/telemetry.md), [blockers](blockers.md#blk-002)

### Outcome

Build a SQLite queue with persistent identity, bounded retention and explicit loss accounting.

### Done when

- Select/pin the SQLite Go driver, document build/CGO implications and verify WAL/FULL pragmas on actual connections; resolve the SQLite part of BLK-002.
- Implement atomic stream/sequence allocation and insert, stable metadata after pruning, versioned schema, and no silent reset on incompatible/corrupt stores.
- Bound record sizes and both priority tiers; eviction and persistent counters are atomic. Test all-alarms saturation, all-rows-deleted restart and sequence non-reuse.
- Provide bounded writer handoff, store-health/volatile drop accounting, short transaction access and checked checkpoint results; run crash-reopen and busy/full-store tests in disposable locations.

### Completion evidence

`internal/store` uses `modernc.org/sqlite` v1.53.0 (BSD-3, pure Go — no CGO needed, resolving that build implication). `Open` verifies `journal_mode=wal` and `synchronous=2` (FULL) on the actual connection, rejects `user_version`≠0/1 and corrupt files without modifying them, and stamps a random 16-byte stream_id into `meta`. `Insert` runs one short tx: `meta.next_seq` allocation (survives deletion of all rows — never `MAX(seq)`), per-tier count check, atomic oldest-row eviction + `counters.evicted` increment, event insert. `Offer`/`RunWriter` give the bounded nonblocking handoff (900/100 defaults) with per-tier volatile drop counters persisted on drain/Close/recovery; `Batch` is a short bounded read for the syncer; `DeleteAcked` deletes only matching stream+seq (gaps/foreign ignored); `Checkpoint` runs bounded `wal_checkpoint(PASSIVE)` with busy reporting; `Depth` exposes queue/age monitoring. Write failure marks storage unhealthy and rejects new offers as volatile losses. Verified 2026-10-12: 11 tests — pragma verification, seq non-reuse across full-delete+reopen, atomic per-tier eviction with counters, all-alarms saturation tier isolation, payload bound, handoff overflow volatile accounting, writer drain on shutdown, corrupt-file no-reset, incompatible version, ack-gap/foreign safety, bounded checkpoint; `go test -race` clean.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-013"></a>

## VTX-013 — Implement the transactional upstream mock

- Repository: voltix
- Area: simulator
- Milestone: M3
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-003](#vtx-003)
- References: [control](features/control.md), [telemetry](features/telemetry.md)

### Outcome

Provide a Go dispatcher and durable receiver for acceptance testing, in the root module.

### Done when

- Use separate dispatch/control and telemetry ports with local token configuration; dispatcher learns boot session and issues fresh revisions.
- Validate bounded telemetry batches and transactionally insert unique stream/sequence events with FULL persistence; identical duplicates are no-ops and conflicting content is rejected.
- Return exact acknowledged IDs only after commit; add deterministic test mode that commits then drops the response.
- Test receiver restart persistence, schema/auth failures, duplicate conflicts and ack-loss behaviour; pause/reset controls are test-only and documented.

### Completion evidence

`internal/upstream` provides `Receiver` and `Dispatcher` in the root module. The receiver serves `POST /api/telemetry` (bearer auth via constant-time compare; ≤100 events, ≤256 KiB body, strict unknown-field rejection → 400; envelope/event validation → 422; conflicting content → 409) plus test-only controls on distinct routes: pause/resume (503 while paused), reset (row wipe, disposable envs only), drop-after-commit (commits then `panic(http.ErrAbortHandler)` — deterministic lost response), and count. `commitBatch` is one atomic tx on a WAL+FULL `received` table keyed (stream_id, seq_id): identical payloads are no-op acks, differing payloads reject the batch; acked IDs return only after `tx.Commit`. `Dispatcher` learns `boot_session_id` from `GET /api/status` and issues increasing revisions with UUID command IDs. Verified 2026-10-12: 6 tests — commit/ack/dedup/conflict, restart persistence, auth+schema+bounds, commit-then-drop ack loss with idempotent retry, pause/reset; `go test -race` clean.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-014"></a>

## VTX-014 — Implement bounded upload and acknowledgement handling

- Repository: voltix
- Area: controller
- Milestone: M3
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-012](#vtx-012), [VTX-013](#vtx-013)
- References: [telemetry](features/telemetry.md)

### Outcome

Forward retained events reliably without holding database transactions across network requests.

### Done when

- Batch up to 100 events with a five-second maximum flush wait, one in-flight upload per stream and configured HTTP size/deadline bounds.
- Implement jittered bounded retry for transient failures and slow visible rechecks for configuration/client failures; successful sends reset backoff.
- Validate exact IDs/stream against the sent batch and prune only acknowledged rows; handle eviction races and sequence gaps without deleting unsent data.
- Test lost responses, partial/invalid/foreign acknowledgements, shutdown, timeout and eventual replay; demonstrate receiver deduplication with stable identities.

### Completion evidence

`internal/syncer.Uploader` runs a single drain loop per stream — inherent one-in-flight. `round` reads ≤BatchMax (default 100) via a short bounded `Batch` tx, POSTs to the receiver (3 s default HTTP deadline, bearer token), and validates the ack envelope before pruning: stream_id must match the local stream, every acked seq must be a member of the sent batch (subset semantics — partial acks prune only acked rows), undecodable/foreign/unsent acks are `ErrAckMismatch` and delete nothing. Transient failures (transport, 5xx, 429) take jittered (±25%) exponential backoff 1 s→30 s, reset on success; 4xx/ack-mismatch become `ConfigFault` with slow recheck (30 s) — no hot loop, no data deletion. Empty queues sleep FlushWait (5 s); full batches continue immediately. Verified 2026-10-12: 8 tests against the real store+receiver plus canned ack fakes — drain+prune, 250-event batching, one-shot lost-response replay with receiver dedup (stable identities, no duplicate rows), transient outage+resume, partial-ack pruning, foreign/unsent/undecodable acks deleting nothing, 401 config-fault flag, bounded shutdown mid-request; `go test -race` clean. Note: receiver `drop-after-commit` is now one-shot (auto-clears) — the realistic single lost response.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-015"></a>

## VTX-015 — Integrate telemetry and verify resource and storage failures

- Repository: voltix
- Area: integration
- Milestone: M3
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-011](#vtx-011), [VTX-014](#vtx-014)
- References: [telemetry](features/telemetry.md), [verification](features/verification.md)

### Outcome

Connect the real queue to the controller and establish M3 acceptance evidence.

### Done when

- Verify telemetry-only outage while dispatch stays valid, process crash/reopen, lost-ack replay, and catch-up during continued collection.
- Force small-capacity status/alarm overflow and store-full/checkpoint contention; validate persistent versus volatile loss counters and continued local control.
- Measure commit/replay throughput, memory, loop timing and DB/WAL footprint under a stated workload; ensure receiver throughput can drain the chosen backlog.
- Record evidence/telemetry/findings.md with all M3 criteria and explicit pre-commit, overflow and hardware power-loss limitations.

### Completion evidence

`cmd/voltix` now runs the full telemetry path: `internal/telemetry.Producer` consumes loop snapshots into status events plus transition-only alarms (no storm); `store.RunWriter` drains bounded handoffs; `syncer.Uploader` uploads to `telemetry_url` behind `VOLTIX_TELEMETRY_TOKEN`; a 5 s probe restores storage health; `GET /api/status` exposes a `telemetry` block (healthy, stream_id, depths, ages, evictions, volatile losses, upload stats). Startup store-init failure is fatal; mid-run failure degrades health without touching control. `internal/control/telemetry_e2e_test.go` verifies end to end against real processes (~25 s): baseline drain, telemetry-only outage with dispatch valid + catch-up under continued collection, lost-ack replay deduped, SIGKILL crash/reopen with stable stream_id and resumed upload, small-cap overflow with persistent evictions + continued control, plus measured commit throughput (4151 ev/s), RSS (15.4 MB) and db+wal footprint (264 KB). `evidence/telemetry/findings.md` records the M3 exit review — all six criteria met with explicit ENOSPC/power-loss limitations. Verified 2026-10-12: full unit suite + `go test -race` + `make integration` green.

### Notes

Reopened by the independent 2026-10-12 [audit](evidence/review/m4-independent-audit.md). Prior completion evidence below/above remains historical; address the audit findings assigned to this task before claiming its acceptance criteria met.

Corrective cycle 2026-10-12 (commit 8ba02d9): `TestStorageWriteFailureSurfacedAndRecoverable` re-execs under `RLIMIT_FSIZE` with SIGXFSZ ignored — a real kernel write refusal (EFBIG, surfaced as SQLite disk I/O error 778) proving the commit-error -> unhealthy -> volatile-drop -> recovery path. A genuine filesystem ENOSPC requires a bounded mount this host cannot provide; recorded as accepted scope revision in decision 004 with an explicit open follow-up ([VTX-022](#vtx-022)) rather than claimed equivalence.

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-016"></a>

## VTX-016 — Package the isolated container demonstration

- Repository: voltix
- Area: deployment
- Milestone: M4
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-011](#vtx-011), [VTX-014](#vtx-014)
- References: [architecture](architecture.md), [verification](features/verification.md)

### Outcome

Provide a one-command local site with independently controllable communication paths.

### Done when

- Build controller/simulator/mock images from pinned dependencies; wire separate field/upstream networks and distinct dispatch/telemetry endpoints.
- Run services without unnecessary privileges and with isolated disposable state; management ports bind only to host loopback.
- Implement make integration/demo with readiness checks, bounded startup and deterministic cleanup; tmux/recording tools are optional.
- Verify from a clean test environment and document exact prerequisites; do not claim these tests cover systemd.

### Completion evidence

`cmd/upstream-mock` (new binary) serves internal/upstream with its test-only /control/* routes. deploy/docker/go.Dockerfile builds voltix + upstream-mock into one debian-slim image (uid 10001); sim.Dockerfile pins pymodbus==3.15.0 on python:3.13-slim (uid 10002); deploy/compose.yaml wires field (voltix<->sim) + upstream (voltix<->mock) networks with only voltix straddling both; host bindings loopback-only (dispatch 8180 — 8080 was occupied on the dev host —, sim control 1504, mock 8081); disposable named volumes removed by `down -v`. `make demo` does bounded readiness (30 s auth'd status probe, 10 s control-socket probe); `make demo-down` removes containers/networks/volumes. deploy/README.md documents prerequisites, ports, sim-control ops, mock test routes and the dispatch schema. Side fixes: config decode is now strict (KnownFields) — example.yaml's silently-ignored keys (unit_id, write_deadline_ms, etc.) were removed; upstream-mock Addr bug fixed. Verified 2026-10-12 ~03:00 UTC on the dev host (Docker 29.7.2): clean down -v + up cycle, dispatch accepted -> ACTIVE ack=inv=site -200 kW across networks, sim set_fault -> FAULT/DEVICE_FAULT in loop status, telemetry drained to mock (count>0), deterministic teardown.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-017"></a>

## VTX-017 — Package and verify the native Linux service

- Repository: voltix
- Area: deployment
- Milestone: M4
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-015](#vtx-015)
- References: [architecture](architecture.md), [verification](features/verification.md), [blockers](blockers.md#blk-003)

### Outcome

Verify the controller as an unprivileged systemd service in an explicitly selected disposable Linux environment.

### Done when

- Resolve native test environment/capabilities under BLK-003 before privileged execution; no silent host service installation.
- Provide a unit, example configuration, bounded restart/shutdown behaviour, dedicated state permissions and documented install/remove steps.
- Verify actual process user, state access, logs, resource configuration and kill/restart behaviour with new-session command rejection; measure restart time.
- Record native evidence separately from containers; preserve simulation state as required and remove only test-created resources.

### Completion evidence

`deploy/systemd/voltix.service` is a real `systemd --user` unit (BLK-003 resolved this way — the host runs a user manager on cgroup v2; no root or host service needed): Restart=on-failure/RestartSec=2, MemoryMax=128M, NoNewPrivileges+PrivateTmp+ProtectSystem=strict, journal output, StateDirectory=voltix. install.sh/remove.sh are idempotent and confined to ~/.config/{systemd/user,voltix}, ~/.local/libexec and ~/.local/state. `make native-test` (scripts/native-test.sh) starts sim on 2512-2514 + upstream-mock on 8191 as plain processes, then verifies everything machine-checkably. Verified 2026-10-12 ~04:00 UTC — 11/11 checks: status live, uid=1000, state dir 0700 owner=user, journal lines, MemoryMax=134217728 enforced in cgroup memory.max, dispatch rev1 -> ACTIVE, SIGKILL -> new boot session in 2.68 s, old-session replay -> 409, telemetry reached+acked at mock. Quirks recorded in deploy/README: user-manager StateDirectory is a ~/.local/state->~/.config symlink (stat -L needed; ExecStartPre chmods the real dir 0700); command lifetime bound is 30 s. Actual OOM-under-load not exercised — MemoryMax cgroup enforcement verified only. This is distinct native evidence, not container restart evidence.

### Notes

Reopened by the independent 2026-10-12 [audit](evidence/review/m4-independent-audit.md). Prior completion evidence below/above remains historical; address the audit findings assigned to this task before claiming its acceptance criteria met.

Corrective cycle 2026-10-12 (commit 6715588): the harness can no longer touch a developer install. install.sh/remove.sh accept `VOLTIX_UNIT_NAME`/`VOLTIX_BIN_PATH`/`VOLTIX_CONFIG_DIR`/`VOLTIX_STATE_NAME` overrides, `VOLTIX_REFUSE_EXISTING=1` aborts on any pre-existing path, and native-test runs under PID-unique names with an EXIT-trap cleanup, canonical-path guard snapshots, and post-cleanup leak assertions. remove.sh resolves the StateDirectory symlink layout and only removes resolved paths under the per-user trees. Verified: 13/13 including `all test-created resources removed` and `canonical install paths untouched`.

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-018"></a>

## VTX-018 — Automate isolated Linux failure scenarios

- Repository: voltix
- Area: integration
- Milestone: M4
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-015](#vtx-015), [VTX-016](#vtx-016), [VTX-017](#vtx-017)
- References: [verification](features/verification.md), [blockers](blockers.md#blk-003)

### Outcome

Provide repeatable network/process faults with machine-checkable acceptance results.

### Done when

- Implement make fault-test and native-test orchestration for A, B, C3, C4, C5 and D, reusing existing storage/command tests.
- Keep NET_ADMIN in the disposable harness; target only test interfaces/routes. Use cleanup traps and avoid host-wide firewall changes.
- Distinguish sustained absence from fast restart and fallback start from reaching zero; observe inverter output independently.
- Record per-scenario results, diagnostics and unavailable prerequisites; packet-capture example explains one request/timeout/reconnect sequence.

### Completion evidence

`make fault-test` (scripts/fault-test.sh) brings up a fresh Compose stack and asserts A, B, C3, C4, D machine-checkably; C5 is delegated to `make native-test` because the verification plan requires native restart evidence that container restarts cannot supply. BLK-003 boundary: unprivileged netns/NET_ADMIN and CAP_NET_RAW are denied on this host, so faults are application/process-level (sim set_online/delay, mock pause/drop-after-commit, docker pause=SIGSTOP) — recorded as unavailable prerequisites in the harness output, not silently skipped; request/timeout/reconnect is evidenced through app counters (upload_failed, InverterOK transitions, sim watchdog_expired). Verified 2026-10-12 ~04:30 UTC — 16/16: B fallback start expiry+0s, output zero expiry+2s; C3 voltix reports inverter failure AND sim watchdog trips 10s after last refresh with independent ramp to 0; C4 docker-pause -> watchdog 10s, unpause -> STANDBY (loop-gap intent drop, no jump); A queue grows during mock pause with dispatch still accepted and loop ACTIVE, drains after resume; D lost response recorded and replay deduplicated (receiver rows == uploaded). Harness found+fixed: REV increment inside $() subshell (now bumped in caller), 28s duration within the 30s contract bound, drain polls instead of fixed sleeps.

### Notes

Reopened by the independent 2026-10-12 [audit](evidence/review/m4-independent-audit.md). Prior completion evidence below/above remains historical; address the audit findings assigned to this task before claiming its acceptance criteria met.

Corrective cycle 2026-10-12 (commit 695846f): scenario D now baselines receiver rows and `upload_failed` before arming drop-after-commit, requires both to advance (commit-then-drop actually exercised), then requires `receiver rows == uploaded` exactly after replay — duplicates or unresolved conflicts break equality. Scenario B asserts output reached 0 within the slew window while the sim watchdog was NOT expired, proving a controller fallback write rather than a watchdog ramp. Verified 17/17 on a fresh Compose stack.

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-019"></a>

## VTX-019 — Document operation and record the portfolio demonstration

- Repository: voltix
- Area: documentation
- Milestone: M4
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-018](#vtx-018)
- References: [verification](features/verification.md)

### Outcome

Make Voltix understandable and reproducible to another engineer.

### Done when

- Write verified running-locally/native-operation instructions including setup, credentials, state reset, fault execution, cleanup and known limitations.
- Update README with current architecture, runnable commands and measured claims; record a short terminal demonstration of selected normal/failure/recovery paths.
- Keep large recordings outside Git or use an explicitly chosen artifact policy; no public upload is assumed.
- Walk through instructions in a clean test environment and fix broken commands/links; retain full acceptance evidence beyond the video.

### Completion evidence

docs/operations-runbook.md covers verified setup, credentials (env-only tokens + demo placeholders), Compose/native operation, state reset, the per-fault playbook, cleanup traps and explicit limitations. README rewritten: current architecture, all make targets, measured claims (commit rate, RSS, restart time, watchdog/fallback timings) with the dev-host caveat, updated doc index. Terminal recording: evidence/demo/demo.typescript + .timing (util-linux `script`, replayable via scriptreplay — chosen artifact policy: committed <1MB, no hosted upload); recorded 2026-10-12, shows dispatch -200 kW ACTIVE, link-loss -> DEGRADED + independent watchdog trip, no-jump recovery, telemetry outage + drain. All README/runbook/deploy links checked programmatically; every documented command was exercised during this campaign on the dev host. Verified 2026-10-12 ~05:15 UTC.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-020"></a>

## VTX-020 — Review milestone exits and reproducible checks

- Repository: voltix
- Area: integration
- Milestone: M4
- Priority: Normal
- Status: Done ✅
- Depends on: [VTX-019](#vtx-019)
- References: [roadmap](roadmap.md), [verification](features/verification.md)

### Outcome

Close the local portfolio milestone only when its documented evidence supports completion.

### Done when

- Provide CI configuration for suitable nonprivileged checks and a separately documented privileged/native test path; run corresponding commands locally.
- Reconcile every verification-matrix row with actual results and run the stated resource/catch-up exercise; record limitations and actionable follow-ups.
- Check clean setup, doc links, task status and milestone criteria; write evidence/demo/exit-review.md without labelling missing checks passed.
- State hosted CI/publication as unverified if no remote/run is available; do not create accounts, push, or deploy as part of this task.

### Completion evidence

`evidence/demo/exit-review.md` reconciles all 14 verification-matrix rows with actual results; `.github/workflows/ci.yml` defines nonprivileged hosted checks (gofmt/vet/test/-race/build, sim tests, loopback integration) while demo/fault/native stay the documented local path — hosted execution explicitly unverified (no remote authorised or observed). Resource exercise (matrix H): `scripts/resource-run.sh` — 30 min at 1 Hz with ~5-min mock outage: 1805 events all uploaded, depth peaked 298 at resume then drained to <=5 in ~10 s (replay far faster than production), upload_failed=14 bounded, RSS 14.4->21.1->20.0 MB, loop reads 1-8 ms throughout, zero persistent loss. Re-ran all check commands locally 2026-10-12: gofmt clean, vet clean, go test + -race green, sim 26/26, make demo/fault-test (16/16)/native-test (11/11). Open follow-ups recorded: real ENOSPC fault injection, packet-level faults (netns denied), OOM-under-load, hosted CI. No accounts created, nothing pushed.

### Notes

Reopened by the independent 2026-10-12 [audit](evidence/review/m4-independent-audit.md). Prior completion evidence below/above remains historical; address the audit findings assigned to this task before claiming its acceptance criteria met.

Corrective cycle 2026-10-12 (commit 8ba02d9): `make check` resolves `gofmt` beside the selected `$(GO)` toolchain and fails explicitly when the formatter is missing or errors — verified with missing and erroring shims; a real `gofmt` drift it then caught in cmd/voltix/main.go was fixed. Exit review updated: row F re-labelled, fault/native counts refreshed, unavailable checks stay open. Stale docs/evidence links: scan found none remaining; roadmap header/table already reconciled by the audit pass.

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-021"></a>

## VTX-021 — Independently audit milestone completion

- Repository: voltix
- Area: integration
- Milestone: M4
- Priority: High
- Status: Done ✅
- Depends on: None (independent review of claimed completed work)
- References: [Audit findings](evidence/review/m4-independent-audit.md)

### Outcome

Check whether actual implementation and evidence support the claimed milestone completion.

### Done when

- Compare contracts and exits with source, artifacts and existing tests.
- Independently verify relevant checks where feasible, recording unavailable checks.
- Record actionable findings and reconcile affected task/milestone status without changing production code.

### Completion evidence

Reviewed baseline 95745cb, source, scripts, exit artifacts and contracts. Existing uncached race tests and all 26 simulator tests pass with loopback access. Two targeted regressions fail: retained intent after device fault and ineffective read-freshness calculation. See the audit for integration results and verification limitations. Documentation changes are uncommitted; no code fix or commit performed.

### Notes

Audit completion is not M4 completion. Reopened VTX-011, 015, 017, 018 and 020 own the remaining work.

<a id="vtx-022"></a>

## VTX-022 — Verify privileged-environment fault scenarios

- Repository: voltix
- Area: verification
- Milestone: M4
- Priority: Normal
- Status: Backlog 📋
- Depends on: [VTX-018](#vtx-018), [decision 004](decisions/004-local-fault-injection-scope.md)
- References: [verification](features/verification.md), [blockers](blockers.md#blk-004)

### Outcome

In a privileged or disposable environment (unprivileged userns with
networking, or root), exercise the checks this host cannot run.

### Done when

- True filesystem ENOSPC on the telemetry store (bounded mount or quota),
  observing surfaced errors, contained loss and recovery.
- Packet-level network faults (tc/netem or iptables inside a netns) for
  delayed/dropped/reordered device and upstream traffic, with optional
  packet capture.
- OOM-under-load on the native unit while telemetry is queued.
- Update the exit-review matrix rows and close BLK-004's remainder.

### Completion evidence

Not run — no privileged environment is available on the development host
(`unshare -r -n` uid_map denied; /run/user tmpfs unsafe to fill). The
EFBIG-class store test and application/process-level fault suite cover
the same handling paths meanwhile; see decision 004.

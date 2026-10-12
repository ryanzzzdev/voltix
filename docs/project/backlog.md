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
- 📋 [VTX-011 — Verify control boundaries and recovery end to end](#vtx-011)
- 📋 [VTX-012 — Implement the bounded durable event store](#vtx-012)
- 📋 [VTX-013 — Implement the transactional upstream mock](#vtx-013)
- 📋 [VTX-014 — Implement bounded upload and acknowledgement handling](#vtx-014)
- 📋 [VTX-015 — Integrate telemetry and verify resource and storage failures](#vtx-015)
- 📋 [VTX-016 — Package the isolated container demonstration](#vtx-016)
- 📋 [VTX-017 — Package and verify the native Linux service](#vtx-017)
- 📋 [VTX-018 — Automate isolated Linux failure scenarios](#vtx-018)
- 📋 [VTX-019 — Document operation and record the portfolio demonstration](#vtx-019)
- 📋 [VTX-020 — Review milestone exits and reproducible checks](#vtx-020)

## Delivery notes

Next execution tasks: **VTX-008** and **VTX-009** (both Ready; disjoint — pure policy package vs dispatch API — and may proceed in parallel). Then VTX-010 → VTX-011 closes M2. Refine unresolved details using evidence rather than extending the initial scope.

M2: VTX-008 and VTX-009 may proceed in parallel after M1, then VTX-010 → VTX-011. M3 queue/receiver work (VTX-012/013) can overlap M1/M2 after bootstrap; VTX-014 joins them, and VTX-015 joins telemetry with verified control. M4 packaging and native verification feed fault automation, documentation and exit review.

Tasks with unfinished dependencies remain Backlog; promote to Ready when dependencies and remaining scope checks are satisfied. Later packaging tasks should be refined at the M3 boundary. Keep active work small and coordinate file ownership if multiple agents work in parallel. Next unused ID: VTX-021.

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
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-012"></a>

## VTX-012 — Implement the bounded durable event store

- Repository: voltix
- Area: controller
- Milestone: M3
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-013"></a>

## VTX-013 — Implement the transactional upstream mock

- Repository: voltix
- Area: simulator
- Milestone: M3
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-014"></a>

## VTX-014 — Implement bounded upload and acknowledgement handling

- Repository: voltix
- Area: controller
- Milestone: M3
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-015"></a>

## VTX-015 — Integrate telemetry and verify resource and storage failures

- Repository: voltix
- Area: integration
- Milestone: M3
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-016"></a>

## VTX-016 — Package the isolated container demonstration

- Repository: voltix
- Area: deployment
- Milestone: M4
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-017"></a>

## VTX-017 — Package and verify the native Linux service

- Repository: voltix
- Area: deployment
- Milestone: M4
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-018"></a>

## VTX-018 — Automate isolated Linux failure scenarios

- Repository: voltix
- Area: integration
- Milestone: M4
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-019"></a>

## VTX-019 — Document operation and record the portfolio demonstration

- Repository: voltix
- Area: documentation
- Milestone: M4
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

<a id="vtx-020"></a>

## VTX-020 — Review milestone exits and reproducible checks

- Repository: voltix
- Area: integration
- Milestone: M4
- Priority: Normal
- Status: Backlog 📋
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

Not started; no implementation or verification claimed.

### Notes

Execute when dependencies are complete; capture actual evidence before marking Done.

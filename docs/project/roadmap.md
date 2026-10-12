# Voltix roadmap

Updated: 2026-10-12
Status: M1 complete — simulator, frozen wire profile and bounded Go adapter verified end to end ([findings](../evidence/device-integration/findings.md)).

Build a reproducible, simulated battery-site controller that demonstrates device integration, explicit operating policies, and failure recovery. This is an engineering portfolio, not a certified controller or a Tesla-compatible product. Scope and rationale: [decision 002](decisions/002-scope-and-delivery.md).

## Milestones

| Milestone | Outcome | Status | Depends on |
| --- | --- | --- | --- |
| Documentation setup | Canonical project-management workflow | Complete | None |
| Planning baseline | Scope, architecture, contracts, initial tasks | Complete | Documentation setup |
| M1 — Device integration | Reproducible simulator and Go device adapter | Complete | Planning baseline |
| M2 — Supervisory control | Validated dispatch, limits, expiry and recovery | Planned | M1 |
| M3 — Durable telemetry | Bounded persistent queue and deduplicated replay | Planned | M2 for integrated acceptance; queue work may overlap M2 |
| M4 — Linux demo and verification | Reproducible container demo, native service checks, failure evidence | Planned | M1–M3 |

Documentation setup evidence: [VTX-001](backlog.md#vtx-001). Planning evidence: [VTX-002](backlog.md#vtx-002). These completions do not imply application functionality. No calendar delivery dates are committed; reassess effort after M1.

## M1 — Device integration

Outcome: a Go client reads a changing simulated battery and meter, writes a power target, and observes an independent device command timeout.

Scope: minimal single-module scaffold; pinned development dependencies; verified SunSpec profile; Python site simulator with separate inverter/meter endpoints; bounded Go Modbus adapter; automated interoperability tests.

Tasks: [VTX-003](backlog.md#vtx-003) through [VTX-007](backlog.md#vtx-007). Start with VTX-003; VTX-004 can proceed independently.

Exit criteria:

- Clean documented setup can run a smoke-tested Go entry point and simulator using pinned dependencies.
- A checked register manifest cites an immutable upstream revision and defines discovery, offsets, scales, invalid values, writes, and sign conversion.
- Independent fixtures verify decoding and writes, including negative charging power; two implementations agreeing alone is insufficient evidence.
- Simulator energy evolves within its configured bounds; only accepted power refreshes reset its watchdog; polls and unrelated writes do not.
- Adapter demonstrates bounded failure and reconnect after delayed, malformed, and disconnected responses.

Excluded: upstream dispatch, persistent telemetry, production service installation, public hosting. Protocol interoperability is demonstrated against the simulator only.

## M2 — Supervisory control

Outcome: fresh authorised dispatch drives the simulator through explicit operating states with observable limits and fallback.

Scope: pure policy functions, authenticated local dispatch API, command ordering/session freshness, one loop owner, operating-limit enforcement, stale-input and restart behaviour. See [control contract](features/control.md).

Tasks: [VTX-008](backlog.md#vtx-008) through [VTX-011](backlog.md#vtx-011).

Exit criteria:

- Valid charge/discharge commands work; malformed, expired, duplicate-conflicting, old-session, and out-of-order requests cannot revive previous output.
- SoC, device-power and site-export policies pass boundary tests; infeasible site targets are reported rather than claimed satisfied.
- Expiry begins fallback within the stated polling tolerance; arrival time does not renew replayed commands.
- Invalid/stale inputs, prolonged scheduling gaps, failed writes, and restarts follow documented policies.
- Slow telemetry consumers cannot block the control path; timings include field reads and writes.

Excluded: hard real-time guarantees, protective relays/BMS implementation, grid compliance, autonomous market optimisation.

## M3 — Durable telemetry

Outcome: measurements survive tested process crashes, replay after interruptions, and remain bounded under extended outage.

Scope: SQLite persistence, two bounded event classes, durable identity/sequence, transactional mock receiver, acknowledgement validation, retry worker, and storage-failure evidence. See [telemetry contract](features/telemetry.md).

Tasks: [VTX-012](backlog.md#vtx-012) through [VTX-015](backlog.md#vtx-015). Storage and receiver work can overlap M2 once their dependencies are complete.

Exit criteria:

- A committed event is retried until acknowledgement or explicit retention eviction; the limitations before commit are visible.
- Lost acknowledgements cause retransmission but no duplicate receiver rows.
- Both event classes are bounded; persisted overflow counters agree with test inputs; repeated faults cannot create unlimited alarm storage.
- Sequence identities are not reused after pruning or tested restart; acknowledgements cannot remove unsent/uncommitted records.
- Disk-full and unavailable-store handling preserve control independence and distinguish persistent from volatile loss counters.
- Measured backlog, replay throughput, memory and database/WAL footprint are recorded for a stated workload. No claim of tested power-loss durability from process-kill tests.

Excluded: custom storage engines, global event ordering, exactly-once transport, unlimited retention, cloud infrastructure.

## M4 — Linux demo and verification

Outcome: another developer can reproduce the working site and its failure scenarios, and inspect separate native service evidence.

Scope: isolated Compose topology, native systemd packaging, fault harness, operational instructions, CI configuration, terminal recording, final evidence review. See [verification plan](features/verification.md).

Tasks: [VTX-016](backlog.md#vtx-016) through [VTX-020](backlog.md#vtx-020). Refine packaging tasks at the M3 boundary based on actual runtime behaviour.

Exit criteria:

- A clean Linux environment follows verified setup instructions and runs the demo; prerequisites and required privileges are explicit.
- Independent telemetry, dispatch, device-link, process-crash, and lost-ack tests pass with machine-checkable assertions.
- Native systemd restart, user identity, configured resource limits and log access are tested separately from container restart behaviour.
- Check scripts are runnable locally and CI configuration exists; actual hosted results remain explicitly unverified unless a remote and run are available.
- README, runbook, short recording and final evidence describe measured results and known limitations accurately.

Public publication, paid hosting, ARM hardware and actual energy equipment are outside this milestone. Local demo completion is not hosted release readiness.

## Deferred scope

Rust daemon, Protobuf/gRPC, a dashboard/TUI, NEMLab integration, price optimisation, multiple batteries, redundant controllers, additional industrial protocols, hardware deployment, and real-device commissioning. Add them only as explicit follow-up scope; do not expand the initial backlog to cover them.

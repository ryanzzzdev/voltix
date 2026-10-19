# Verification and demonstration plan

Date: 2026-10-12
Status: Required checks for implementation; none have run against application code yet.

## Layers and intended commands

| Layer | Planned command | Environment and claim |
| --- | --- | --- |
| Unit | make test | Go and simulator unit tests; pure policy, decoding, queue logic |
| Static/race | make check | Formatting, Go vet/race checks, selected Python checks after bootstrap |
| Integration | make integration | Real processes and SQLite; no host-wide network mutation |
| Network faults | make fault-test | Disposable Linux namespaces/containers with explicitly scoped NET_ADMIN privileges |
| Native service | make native-test | Disposable Linux VM with systemd, or explicitly selected test host; not the container demo |
| Demonstration | make demo | Start the site and print/stream useful state; optional recording tooling |

These commands are targets to implement, not instructions claimed to work today. Separate setup prerequisites, cleanup and failure diagnostics. Fault tests fail clearly or report unavailable capability; they must not silently pass by skipping required checks. A full failure recording may call fault-test separately; tmux is optional.

## Acceptance matrix

| ID | Scenario | Required observation | Task |
| --- | --- | --- | --- |
| P1 | Protocol fixtures | Signed/scaled values, discovery, sentinels and target write/readback agree with independently checked manifest | VTX-004/007 |
| P2 | Device timeout | Polls/config writes do not renew power lifetime; fallback begins after last accepted refresh within tolerance | VTX-005/007 |
| C1 | Normal dispatch and limits | Requested, constrained, acknowledged and measured output are distinguished; SoC/export/ramp boundaries pass | VTX-011 |
| C2 | Replayed/stale commands | Old session/revision and changed duplicate rejected; identical retry never extends expiry | VTX-009/011 |
| A | Telemetry-only outage | Dispatch and field links remain available; control continues under valid lease; bounded queue grows | VTX-015/018 |
| B | Dispatch expiry | Stop command refreshes; fallback begins by expiry plus one period/work budget; zero reached within configured ramp allowance | VTX-011/018 |
| C3 | Field-link outage | Controller reports stale/unknown state; simulator independently begins fallback 10 seconds after last accepted power refresh, within 500 ms test tolerance | VTX-007/018 |
| C4 | Sustained controller absence | Disable auto-restart or suspend process beyond watchdog duration; independent fallback verified | VTX-018 |
| C5 | Fast restart | Kill controller; native service restarts; new session, fresh reads and no stale dispatch replay; record actual restart time | VTX-017/018 |
| D | Lost acknowledgement | Receiver commits then drops response; identical retry stores once and exact acknowledgement permits pruning | VTX-014/015 |
| E | Saturation and alarm flood | Both tiers bounded, intended victims evicted and counters correct; no unbounded fault-event loop | VTX-012/015 |
| F | Disk full/busy checkpoint | Storage degrades observably; control remains independent; no unbounded retries or silent database reset | VTX-015 |
| G | Long loop gap/invalid samples | Freshness invalidates active intent; no large dt-induced jump; startup/recovery ramp baseline valid | VTX-011 |
| H | Resource and catch-up run | Declared workload, bounded queue/memory, replay faster than production, DB/WAL footprint and loop jitter measured | VTX-015/020 |

Use tiny queue capacities for deterministic saturation tests and a separate production-default workload for resource evidence. Wall-clock assertions are for a documented unloaded Linux environment and include scheduling tolerances; unit tests use virtual time. Export limits are sampled supervisory behaviour, not a protection guarantee.

## Native and container boundaries

Container topology uses two networks, controller on both, inverter/meter on field network and mock on upstream network. Bind management ports only to loopback on the host. Dispatch and telemetry have distinct endpoints so Test A does not cut dispatch accidentally. Controller runs without networking-administration privileges; a disposable helper owns fault injection.

Native service tests verify user identity, state-directory permissions, restart policy, resource limit configuration/effect, startup session reset and journal visibility. Container restart evidence cannot satisfy these checks. Service installation and privileged commands must be clearly documented and limited to the chosen test environment. Cleanup restores only resources created by the harness; never flush host firewall rules.

## Minimum operational evidence

The final report names OS/toolchain/library versions, workload and durations, exact commands, outcomes, skipped checks and known limitations. Include a representative packet trace explanation (request, response, timeout/reconnect) to demonstrate practical network diagnosis without asserting a simulated packet fault proves physical Ethernet expertise.

Initial resource exercise: 30-minute 1 Hz run with a five-minute telemetry outage, declared event size and limit configuration. Treat this as bounded test evidence, not proof of no memory leaks or unlimited uptime. Measure end-to-end loop time during catch-up and under memory constraints. If MemoryMax=128M is unsuitable, record the evidence and chosen limit rather than disguising OOM failures.

Hosted CI is an optional execution venue: first green hosted run observed 2026-10-19 (go, sim and loopback integration jobs all passing). Power-cut/hardware, ARM deployment and real equipment remain excluded.

Fault-injection scope on the development host is application/process-level plus a kernel-enforced file-size-limit storage fault; true packet-level injection, packet capture and filesystem ENOSPC are unavailable without privileges and are carried as follow-up work — see [decision 004](../decisions/004-local-fault-injection-scope.md).

## Presentation deliverables

README with purpose, architecture, limitations and verified prerequisites; native operations runbook; reproducible demo; three-minute recording of selected failures with readable explanations; final evidence linking each required scenario to its result. The full suite supplies correctness evidence beyond the recording.

Tasks: [VTX-016](../backlog.md#vtx-016) through [VTX-020](../backlog.md#vtx-020).

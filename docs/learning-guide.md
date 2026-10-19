# Learning Voltix

This guide is the shortest path from “the demo runs” to understanding why the
system is built this way. Read it alongside the code; the linked project
contracts contain the precise rules and evidence.

## 1. The one-sentence mental model

Voltix is a **supervisory controller for one simulated battery site**. An
upstream system asks for a battery power level, Voltix checks whether that
request is fresh and safe, writes a constrained target to the inverter once per
second, and records what happened without allowing telemetry problems to stop
control.

It deliberately separates three responsibilities:

1. **Authority:** is this dispatch command authentic, current, and intended for
   this process instance?
2. **Control:** given current measurements and limits, what target may the
   inverter receive now?
3. **Evidence:** can observations survive an upstream outage and be delivered
   without duplicates at the receiver?

The project demonstrates those boundaries on a simulator. It is not a
certified battery controller, protection system, market optimiser, or claim of
compatibility with arbitrary equipment.

## 2. Domain knowledge

### Battery and site terms

- **BESS:** battery energy storage system. Here it is represented by an
  inverter plus stored energy.
- **Inverter:** converts between battery-side DC and grid-side AC and accepts a
  power setpoint.
- **Meter:** measures total site power at the grid connection.
- **SoC:** state of charge, as a percentage of usable energy capacity.
- **Setpoint/target:** the power Voltix asks the inverter to produce or absorb.
- **Supervisory control:** relatively slow, application-level coordination.
  Voltix runs nominally at 1 Hz; it is not a protective relay or hard real-time
  control loop.
- **Fallback:** deliberate movement toward zero power when authority or valid
  measurements are lost.

Voltix uses these sign conventions:

- battery power `> 0`: discharging to the site/grid;
- battery power `< 0`: charging from the site/grid;
- site power `> 0`: exporting to the grid;
- site power `< 0`: importing from the grid.

The simulator updates stored energy with:

```text
next energy = current energy - battery power × elapsed hours
```

Positive discharge therefore lowers SoC; negative charging raises it.

The site meter combines controllable battery power with everything Voltix does
not control:

```text
site export = battery power + baseline generation-minus-load
```

Example: if the uncontrollable baseline exports 300 kW and the site limit is
400 kW, the battery may discharge at no more than 100 kW. A +200 kW request is
constrained to +100 kW. If the battery charges at -200 kW, site export becomes
100 kW.

### Industrial protocol terms

- **Modbus TCP:** a request/response protocol for reading and writing numbered
  16-bit registers over TCP. It supplies transport, not semantic meaning.
- **SunSpec:** standardised model layouts layered on Modbus. Voltix uses a
  deliberately small, frozen profile: inverter models 103, 124 and 704, plus
  meter model 203.
- **Scale factor:** a register value often needs another register to determine
  its decimal scale.
- **Sentinel:** a reserved raw value meaning invalid or not implemented. Voltix
  rejects these instead of treating them as measurements.
- **Atomic FC16 write:** the enable, mode, and 32-bit power target are written as
  one four-register command. A partial write is not a valid power command.

The checked source of truth for addresses and types is
`contracts/sunspec-profile.json`; generated Go constants and the Python
simulator both consume that frozen contract. Golden fixtures keep the two
implementations from merely agreeing on the same accidental mistake.

### Distributed-systems terms hiding inside the demo

- **Lease:** a dispatch command is valid only until its explicit expiry. Voltix
  never turns it into a permanent setting.
- **Boot session:** every controller process start gets a random identity. A
  command for the previous process cannot be replayed after restart.
- **Revision:** commands must move forward monotonically within a boot session.
- **Idempotent retry:** retrying the exact same command is safe and does not
  extend its lifetime.
- **At-least-once delivery:** telemetry may be retransmitted until acknowledged.
- **Deduplication:** the receiver uses `(stream_id, seq_id)` so a retry after a
  lost acknowledgement does not create a second row.
- **Backpressure:** slow storage or networking must not stall the control loop.
  Voltix uses bounded handoffs and counts loss instead of waiting forever.

## 3. Runtime architecture

There are three processes in the Compose demonstration:

| Process | Responsibility |
| --- | --- |
| `voltix` | Dispatch API, device adapter, 1 Hz policy loop, local event store, telemetry uploader |
| `sim` | Battery physics, SunSpec/Modbus inverter and meter, independent command watchdog, test-only fault controls |
| `upstream-mock` | Durable telemetry receiver and test-only outage/lost-ack controls |

Inside `voltix`, the important package boundaries are:

| Package | Question it answers |
| --- | --- |
| `internal/dispatch` | May this command become current intent? |
| `internal/device` | Can typed inverter/meter values be safely exchanged over the frozen wire profile? |
| `internal/policy` | What state and constrained target follow from these inputs? |
| `internal/control` | How are bounded reads, policy, write, and snapshot sequenced each second? |
| `internal/telemetry` | Which control observations become status or transition events? |
| `internal/store` | How are events durably and finitely queued? |
| `internal/syncer` | How are retained events retried and acknowledged? |
| `internal/upstream` | How does the test receiver commit and deduplicate batches? |

The controller straddles two logical networks. The **field network** contains
the inverter and meter; the **upstream network** contains telemetry. This is
why an upstream outage can be demonstrated without pretending the inverter
also disappeared.

## 4. One complete control cycle

Every nominal one-second cycle does this:

1. Read the inverter and meter concurrently, each through a bounded device
   endpoint.
2. Assemble one input snapshot: measured inverter power, SoC, site power,
   device fault status, freshness, and the currently active dispatch lease.
3. Run the pure policy function. It chooses `STANDBY`, `ACTIVE`, `DEGRADED`, or
   `FAULT`, plus reason codes and a target.
4. If the inputs or timing invalidate authority, drop the active dispatch. An
   old target cannot spring back after recovery.
5. Write the newly computed target to the inverter with a bounded deadline.
   A timeout is “uncertain,” because the device may have received the write.
6. Publish a diagnostic snapshot through a bounded, nonblocking channel.

Only the loop goroutine owns device writes and cross-cycle control state. HTTP
handlers may validate intent, but they never write the inverter directly.

### Policy order matters

For a healthy active command, Voltix:

1. forms the hard allowable interval from charge/discharge capacity, SoC
   boundaries, and the site-export limit;
2. clamps the requested target into that interval;
3. applies the normal slew-rate limit;
4. reapplies hard bounds in case a newly tightened bound must override the
   preferred ramp.

Safety/freshness checks happen before normal target tracking. Invalid data is
not silently changed to zero and treated as healthy.

### The four operating states

| State | Meaning |
| --- | --- |
| `STANDBY` | Devices are usable but there is no fresh dispatch; target moves toward zero. |
| `ACTIVE` | Fresh authority and valid measurements exist; constrained dispatch is applied. |
| `DEGRADED` | Expiry, bad/stale reads, timing gap, or communication trouble invalidated intent; fallback is attempted. |
| `FAULT` | The device reports a fault; nonzero output is inhibited and intent is dropped. |

Being alive is not the same as being healthy or active. The status API exposes
that distinction.

## 5. Two independent fallback mechanisms

This is the most important safety idea in Voltix.

1. **Controller lease fallback:** when dispatch expires or inputs become
   invalid, Voltix computes and writes targets toward zero.
2. **Inverter watchdog fallback:** only a complete accepted power write refreshes
   the simulator's 10-second watchdog. If Voltix is dead or the field link is
   broken, the simulator independently ramps output to zero.

The second mechanism exists because software cannot command a device it cannot
reach. In real equipment, this responsibility would live in independently
engineered inverter/BMS/protection behaviour; Voltix only simulates the idea.

## 6. Telemetry path and delivery claim

A control snapshot is offered without blocking. The telemetry producer emits:

- one status event per cycle;
- one alarm-tier event on each operating-state transition, avoiding an alarm
  storm every failed cycle.

SQLite holds two finite tiers. When a tier is full, its oldest row is evicted
atomically and a counter is incremented. Sequence IDs are never reused after
acknowledgement or eviction.

The uploader takes at most 100 rows, performs HTTP outside any database
transaction, validates the exact receiver acknowledgement, and deletes only
acknowledged IDs. If the receiver commits but its response is lost, the same
identities are retried and the receiver treats identical duplicates as no-ops.

The honest guarantee is:

> A locally committed event that remains inside retention is retried until a
> valid acknowledgement, assuming configuration becomes valid and the receiver
> eventually returns.

This is not exactly-once transport, infinite retention, or proof against
physical power loss.

## 7. What the demo means

The scripted demonstration tells a failure-containment story:

1. **STANDBY:** the controller starts with a new boot session, reads devices,
   and writes toward zero. Nothing persisted from an old process authorises
   output.
2. **Dispatch -200 kW:** a fresh 25-second lease asks the battery to charge.
   Negative power is intentional. Requested, constrained, acknowledged, and
   measured values eventually agree.
3. **Field link loss:** Modbus requests fail. The loop becomes `DEGRADED` and
   drops intent, but cannot itself deliver zero through a broken link.
4. **Watchdog trip:** ten seconds after the last accepted write, the simulated
   inverter independently ramps to zero. This demonstrates defence beyond
   controller liveness.
5. **Link recovery:** Voltix returns to `STANDBY`. It does not resurrect the old
   -200 kW command; a new dispatch is required.
6. **Telemetry outage:** the receiver pauses. The local queue grows and retry
   failures are visible, but valid control continues because HTTP upload and
   SQLite work are outside the control loop.
7. **Receiver recovery:** the backlog drains. Lost/repeated transmissions do
   not duplicate receiver rows.

The visual demo is explanatory evidence, not the whole proof. Unit tests,
cross-process integration tests, the fault harness, the native systemd test,
and the acceptance matrix supply the stronger checks.

## 8. Best source-reading order

Read the project as a set of vertical slices, not directory by directory:

1. **Physics and signs:** `sim/voltix_sim/site.py`, then
   `docs/project/features/device-profile.md`.
2. **Policy without I/O:** `internal/policy/policy.go` and its table-driven
   tests. Work examples by hand before reading assertions.
3. **Command authority:** `internal/dispatch/session.go`, especially boot
   session, revision, idempotence, and dual wall/monotonic expiry.
4. **Wire boundary:** `contracts/sunspec-profile.json`,
   `internal/device/sunspec.go`, and one fixture.
5. **Orchestration:** `internal/control/loop.go`, then `cmd/voltix/main.go` to
   see composition and lifecycle.
6. **Durability:** `internal/store/store.go`, `internal/syncer/syncer.go`, and
   `internal/upstream/receiver.go`.
7. **System meaning:** `scripts/demo-runner.sh`, `scripts/fault-test.sh`, and
   `evidence/demo/exit-review.md`.

## 9. A practical learning lab

Use this sequence and predict the result before running each step:

1. Run `make sim-test`; change only fake-clock inputs in a simulator test and
   explain the energy result.
2. Run one policy test with `go test ./internal/policy -run <name> -v`; draw the
   hard interval and expected slew result.
3. Run `make demo`, inspect `/api/status`, and identify authority, control, and
   telemetry fields.
4. Send -200 kW, then explain the difference among requested, constrained,
   acknowledged, and measured power.
5. Change baseline generation so the export limit clamps a positive dispatch.
6. Stop command refresh and observe controller lease fallback before the
   device watchdog expires.
7. Break the field link and explain why the watchdog, not Voltix, must bring
   output down.
8. Pause telemetry and verify that queue depth changes while the control state
   remains valid.
9. Read the lost-ack scenario and explain why receiver row count does not grow
   twice.
10. Finish with `make integration`; treat the tests as executable system
    documentation.

## 10. Questions that prove real understanding

You understand Voltix when you can answer these without memorising code:

- Why does +200 kW lower battery SoC?
- Why can dispatch acceptance return 202 before the inverter changes output?
- Why must a timed-out write not be blindly retried?
- Why is the boot session separate from command revision?
- Why is the export limit based on `meter - inverter`?
- Why does a telemetry outage not force `DEGRADED`?
- Why can a committed telemetry event still be lost under bounded retention?
- Why are receiver deduplication and sender acknowledgement validation both
  necessary?
- Why does recovery require a new dispatch instead of reviving the last one?
- Which claims come from simulation evidence, and which would require real
  hardware or a privileged test environment?

## Further reference

- [Architecture](project/architecture.md)
- [Control contract](project/features/control.md)
- [Telemetry contract](project/features/telemetry.md)
- [Operations runbook](operations-runbook.md)
- [Demo evidence](../evidence/demo/README.md)
- [Acceptance review](../evidence/demo/exit-review.md)


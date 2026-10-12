# Technical Specification & Implementation Plan: `Voltix` (v1.2 Final)

**Resilient Industrial Edge Controller & Telemetry Gateway for Battery Energy Storage Systems**

## 1. System Overview & Objectives

**`Voltix`** is an offline-first, high-reliability industrial edge daemon running on Linux. It interfaces with local battery energy storage system (BESS) inverters and site power meters over Modbus TCP, executes a periodic (1 Hz) supervisory power control loop with physical safety limits, trips to a safe state upon signal loss, and reliably buffers high-frequency telemetry to disk with at-least-once delivery and deduplication over intermittent network links.

### Strategic Alignment

* **Primary Target:** Tesla — *Sr. Software Engineer, Industrial Energy Site Controller (APAC)* ([Tesla Req 276061](https://www.tesla.com/careers/search/job/sr-software-engineer-industrial-energy-site-controller-apac-276061)).

* **Industry Portability:** Architectural patterns directly transferable to **Fluence** (*Fluence OS*), **Wärtsilä** (*GEMS*), **SwitchDin** (*Droplet*), **KrakenFlex**, and Australian DNSP Dynamic Operating Envelope (DOE) controllers.

## 2. Definitive Tech Stack & Architecture Decisions

| **Component** | **Selected Technology** | **Specific Implementation Rules** | 
| **Language** | **Go (Golang 1.22+)** | Core daemon, concurrency via goroutines, channels, and `context.Context`. | 
| **Upstream Protocol** | **HTTP/1.1 with JSON** | Simple, inspectable, and sufficient for 1 Hz telemetry and low-frequency dispatch. Telemetry batched (50–100 items). | 
| **Fieldbus Protocol** | **Modbus TCP (Verified SunSpec Subset)** | SunSpec standard register layouts without arbitrary compression:  • **Model 103** (Three-Phase Inverter: Power, Voltage, Frequency, Status)  • **Model 124 / 713** (Storage: State of Charge `SoC`, Charge/Discharge Limits)  • **Model 704** (DER AC Controls: Active Power Setpoint `WSetPct` / `WMaxLimPct`) | 
| **Local Storage Engine** | **Embedded SQLite (WAL Mode)** | • `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`  • Monotonic 64-bit sequence IDs (`seq_id`) + UUIDs.  • Bounded application queue (max 100k rows, max 1 KB/payload).  • Checked checkpoint execution (`PRAGMA wal_checkpoint(PASSIVE / TRUNCATE)`). | 
| **Deployment Target** | **Embedded Linux / Systemd** | Unprivileged Linux user, `systemd` restart policy, `MemoryMax=128M`, structured `journald` logging. | 
| **Test & Fault Harness** | **Docker Compose + Linux `tc netem` / `ip netns`** | Isolated dual network bridges: upstream WAN vs fieldbus LAN. Simulates latency, jitter, loss, and connection drops. | 

## 3. High-Level Architecture

```
                  +-------------------------------------------------+
                  |          Upstream Cloud / Dispatcher Mock       |
                  +-------------------------------------------------+
                                           ^
                     Heartbeat & Setpoints |  HTTP/gRPC Telemetry
                      (POST /api/dispatch) |  (Batches with Sequence IDs)
                                           v
+=====================================================================================+
|  `voltix` Linux Daemon (Go)                                                         |
|                                                                                     |
|   +-----------------------+     +--------------------+     +---------------------+  |
|   | Dispatch Receiver     | --> | Safety & Limits    | --> | Supervisory Control |  |
|   | * Command ID & Lease  |     | * SoC (5%-95%)     |     | * 1 Hz Periodic Tick|  |
|   | * Watchdog (30s)      |     | * Ramp Rate (dt)   |     | * Overrun & Miss Mon|  |
|   +-----------------------+     +--------------------+     +---------------------+  |
|                                                                     |               |
|                                                                     v               |
|   +-----------------------+                            +-------------------------+  |
|   | Telemetry Syncer      |                            | Device Driver Interface |  |
|   | * At-least-once       |                            | (HAL Abstraction)       |  |
|   | * Exponential Backoff |                            +-------------------------+  |
|   +-----------------------+                                         |               |
|               ^                                                     v               |
|               | (Read batches)                             Modbus TCP (Port 502)    |
|   +-----------------------+                                         |               |
|   | SQLite WAL App Queue  | <--- [Telemetry Snapshot] --------------+               |
|   | * Monotonic Seq IDs   |                                         |               |
|   | * Bounded Storage     |                                         |               |
+===+=======================+=========================================+===============+
                                                                      |
                                                                      v
                                                    +---------------------------------+
                                                    | Mock Inverter & Power Meter     |
                                                    | * Independent Comms Watchdog    |
                                                    | * Auto-shutdown on link loss    |
                                                    +---------------------------------+

```

## 4. Key Subsystems & Behavioral Contracts

### 4.1. The 1 Hz Supervisory Control Loop & I/O Deadlines

* **Periodic Scheduling:** Runs via `time.NewTicker(1 * time.Second)`.

* **Parallel I/O Phase:** Polling Inverter metrics (Model 103 & 124) and Power Meter metrics is executed **concurrently** using Go's `errgroup` under a strict `200ms` context deadline. Total I/O phase is capped at 200ms rather than 400ms sequential latency.

* **Overrun & Latency Instrumentation:**

  * Record `tick_start_jitter` (delay between scheduled tick and execution).

  * Record `loop_duration_ms` (time to complete I/O + calculation).

  * If loop duration exceeds 300ms, increment a `control_loop_deadline_missed_total` metric.

* **Ramp Calculation & Long-Stall Protection:**

  * Normal operation: Ramp rate applies elapsed time (`dt = time.Since(lastTick)`): `max_delta = ramp_rate_kw_per_sec * dt.Seconds()`.

  * **Prolonged Stall Guard:** If `dt > 2.5s` (due to severe CPU starvation or process suspension), **do not scale `max_delta` with the large `dt`**. Clamp `dt` to 1.0s, log a warning, revalidate fresh device telemetry, and do not execute sudden power step jumps.

### 4.2. Dual Watchdog & Safety Trip Architecture

* **Upstream Dispatch Lease (Controller-side):**

  * Dispatch commands arrive with an expiry lease: `valid_until = now + 30s`.

  * If `now > valid_until` without a fresh valid dispatch command, the controller enters `DEGRADED_EXPIRED` mode and commands `0 kW` (ramping down safely).

* **Inverter-Side Autonomous Command Watchdog:**

  * The mock inverter implements an independent **10-second watchdog timer**.

  * **Refresh Rule:** The watchdog is reset **only by an accepted active power command write**, never by generic status polling or configuration reads.

  * If no power command is received for 10s (due to field link sever or controller crash), the inverter autonomously ramps to `0 kW` standby.

* **Crash & Fast-Restart Policy:**

  * When `voltix` starts or revives under `systemd`, it **must not replay previous setpoints**.

  * It initializes in `STANDBY`, polls fresh inverter state, and commands `0 kW` (or holds zero) until a freshly signed, non-expired upstream dispatch command arrives.

### 4.3. Telemetry Delivery & Overflow Contract

* **Delivery Semantics:**

  * Persisted records are retried until acknowledged upstream or explicitly evicted under the overflow policy.

  * Upstream cloud receiver acknowledges batches transactionally by ID range.

  * Receiver deduplicates retransmitted records using the controller ID + `seq_id`.

* **Overflow & Eviction Policy:**

  * Storage limit is enforced logically at 100,000 records.

  * When capacity is reached during an extended outage, the oldest unacknowledged low-priority metrics are dropped in FIFO order.

  * Dropped records increment a persistent counter: `telemetry_records_dropped_total`.

  * High-priority alarms/trip events are quarantined in an unevictable priority tier.

## 5. Four Isolated Fault-Injection Scenarios

The test harness (`scripts/run_fault_tests.sh`) executes four distinct, non-overlapping tests:

```
+---------------------------------------------------------------------------------------+
| Test A: Telemetry Outage (Field Comms Active, Dispatch Active, Telemetry Link Down)  |
| * Upstream telemetry port blocked via iptables.                                       |
| * Pass Criteria: Inverter continues regulating power; queue fills; zero RAM leak.    |
+---------------------------------------------------------------------------------------+
| Test B: Dispatch Lease Expiry (Telemetry Active, Dispatch Stopped)                    |
| * Dispatcher stops issuing heartbeats.                                                |
| * Pass Criteria: At t = 30s (+/- 1s), controller detects expiry and ramps to 0 kW.    |
+---------------------------------------------------------------------------------------+
| Test C1: Field Link Severed (Modbus Cable Disconnected)                               |
| * Modbus TCP port dropped on field bridge.                                            |
| * Pass Criteria: Inverter internal watchdog trips at t = 10s (+/- 500ms) to 0 kW;     |
|   controller logs link loss and buffers a FAULT telemetry record.                     |
+---------------------------------------------------------------------------------------+
| Test C2: Controller Crash & Fast Revival (kill -9)                                    |
| * Controller killed; systemd restarts it in < 1 second.                               |
| * Pass Criteria: Controller starts in STANDBY, reads fresh state, does NOT replay old |
|   dispatch, and requires fresh authorization before driving load.                     |
+---------------------------------------------------------------------------------------+
| Test D: Lost ACK & Reconnect Deduplication                                            |
| * Backend receives and commits batch, but network drops before HTTP 200 reaches edge. |
| * Controller reconnects and resends batch.                                            |
| * Pass Criteria: Backend identifies duplicate seq_ids, stores once, returns 200;      |
|   controller prunes queue to zero.                                                    |
+---------------------------------------------------------------------------------------+

```

## 6. Implementation Roadmap (Milestones)

1. **Milestone 1 — Mock Inverter & SunSpec Client**

   * Python/pymodbus simulator implementing Models 103, 124, and 704 with a 10s power-write watchdog.

   * Go Modbus client polling Models 103/124 concurrently with 200ms timeout.

2. **Milestone 2 — Supervisory Control Loop & Safety Limiter**

   * 1 Hz loop with `dt` ramp calculations and stall-protection clamping.

   * Safety envelope: SoC bounding (Model 124) and grid capacity export limiting.

   * Dispatch receiver with 30s lease validation.

3. **Milestone 3 — SQLite Telemetry Queue & Deduplication**

   * Schema with `seq_id`, timestamp, payload, priority, and committed status.

   * Background sync worker with jittered exponential backoff.

   * FIFO eviction on queue saturation with `telemetry_records_dropped_total`.

4. **Milestone 4 — Systemd Packaging, Automated Harness & Demo**

   * Systemd unit with `MemoryMax=128M` and unprivileged execution (`User=voltix`).

   * Bash test harness executing Tests A, B, C1, C2, and D.

   * 3-minute recording demonstrating resilience and terminal telemetry inspectability.
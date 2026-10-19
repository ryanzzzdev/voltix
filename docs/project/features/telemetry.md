# Telemetry persistence and delivery contract

Date: 2026-10-12
Status: Implemented and verified for process-crash, bounded-retention, retry, and measured local resource scenarios; physical power-loss durability remains unproven.

## Event identity and durable boundary

Each event has stream_id (random UUID created with a new store), seq_id (positive monotonic 64-bit integer never reused within that store), controller_id, UTC observed_at, priority and a versioned payload. The stream and sequence form the deduplication key; a separate event UUID is unnecessary. Sequence order is local to a stream, not a global or wall-clock order. A deliberate store reset creates a new stream identity.

Assign identity and insert the event atomically. Use persistent sequence allocation that survives deletion of all queue rows; do not allocate from MAX(seq_id). Store identity, sequence metadata and persistent loss counters remain when events are pruned.

The durability promise starts only after a successful local commit. Loop snapshots waiting in memory can be lost on crash. Report pre-persistence drops separately from retention evictions. One SQLite writer owns the store; syncer uses short bounded reads and acknowledgement operations. Never hold a transaction over HTTP.

Use WAL and synchronous FULL for both event store and mock receiver, and verify pragmas on the actual connections. FULL asks SQLite to synchronise WAL commits, assuming the storage stack honours it; it is not proof of hardware power-loss behaviour. [SQLite synchronous documentation](https://www.sqlite.org/pragma.html#pragma_synchronous). Choice rationale: [decision 003](../decisions/003-failure-and-persistence.md).

## Bounded retention and failures

Initial configurable bounds:

- 90,000 ordinary status events and 10,000 alarm/transition events; neither tier is unevictable.
- Maximum encoded event payload 1 KiB; bound envelope fields too.
- Bounded in-memory handoff: 900 status and 100 alarm slots. When full, drop the incoming event in that tier and count it; do not block control.
- At persistent tier capacity, atomically evict its oldest row, insert the new row and increment its persistent eviction counter. Tier isolation preserves alarm space without promising infinite retention.

Emit faults on transitions and periodic summaries so every failed poll does not create an alarm storm. Loss counter reporting must not recursively enqueue unlimited events.

Logical row bounds are not a hard filesystem quota. Monitor main DB, WAL, queue depth, oldest age and free space. Use short transactions and checked checkpoints; a busy checkpoint is observable and retried with bounded work. Do not run blocking checkpoint operations in the control loop. Test long-lived readers and disk-full conditions. Record a measured storage envelope; exact physical quotas are outside v1.

On write failure, rollback, mark storage unhealthy, drop/reject new snapshots after bounded handoff capacity, and increment volatile counters. Persist counters when storage recovers when possible; a crash during storage failure can lose them. Do not advertise exact durable loss counts in that interval. Corrupt/unopenable databases must never be silently deleted/reset. Fail startup if required store initialisation fails; if it fails during operation, local control remains independent and health is degraded.

## Upload API and acknowledgement

POST /api/telemetry on the mock receiver's separate telemetry port, with a bearer token. A batch has schema_version, stream_id and ordered events. Maximum 100 events, 256 KiB request body. Flush when 100 records are ready or at least every 5 seconds; do not wait indefinitely for a full batch at 1 Hz. Keep one upload in flight per stream.

The receiver validates all events and atomically commits the full batch to its SQLite store with a unique key on (stream_id, seq_id). Repeated identity with identical content is a no-op; conflicting content is an error. Only after commit return 200 with the exact acknowledged sequence IDs and stream identity. Reject malformed/oversized batches; a deliberately faulty test mode may commit and close the connection without a response.

The uploader validates acknowledgements against the sent batch and deletes only acknowledged records of the matching stream. Acknowledgement loss causes retry with unchanged identities. Gaps from evictions are legal; do not treat a maximum sequence as proof that all earlier records were received. Ignore already-evicted local IDs safely without deleting anything else.

Use HTTP deadlines (initially 3 seconds) and jittered exponential retry (initial 1 second, cap 30 seconds) for transport errors, 429 and server failures; bounded Retry-After support may be added with tests. Authentication/schema/client errors become observable configuration faults with slow bounded rechecks, not hot retry loops or automatic data deletion. Successful upload resets transient backoff.

## Delivery claim and measurements

Retained committed events are retried until acknowledged, assuming eventual receiver availability and valid configuration. Receiver deduplication makes retries idempotent. Overflow eviction, store reset, storage failure and pre-commit loss are explicit exceptions. Do not claim exactly-once transport or unlimited lossless retention.

Measure commit cost, replay throughput, memory, DB/WAL sizes and event-loss counters with stated payload, production rate, outage length and hardware. Show backlog reduction while collection continues; to assert queue empty, pause producers and wait for acknowledgements. Keep the receiver store durable across test restarts, and reset it only in disposable test environments.

Tasks: [VTX-012](../backlog.md#vtx-012) through [VTX-015](../backlog.md#vtx-015).

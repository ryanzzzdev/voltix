# M3 telemetry verification findings

Date: 2026-10-12
Milestone: M3 — Durable telemetry ([roadmap](../../docs/project/roadmap.md))
Scope: durable buffering and at-least-once delivery of control-loop
telemetry against the real `bin/voltix`, simulator and Go mock receiver.
See [telemetry contract](../../docs/project/features/telemetry.md) and
[verification](../../docs/project/features/verification.md).

## Workload

- Controller: `bin/voltix` at 200 ms cycle — ~5 status events/s plus
  transition alarms; store: `modernc.org/sqlite` v1.53.0, WAL + FULL.
- Receiver: `internal/upstream` mock on its own port, same pragmas.
- Uploader: ≤100-event batches, ≤5 s flush, 3 s HTTP deadline, jittered
  1 s→30 s backoff, 30 s config-fault recheck.
- Driver: `internal/control/telemetry_e2e_test.go` plus unit suites
  (store 11, upstream 6, syncer 8, dispatch 10, control 9, policy 11;
  all race-clean except trivial pure tests).

## Results

End-to-end (~20 s standalone; ~51 s with the M2 e2e in the same run):

| Scenario | Evidence |
|---|---|
| Baseline collect+upload | `uploaded` grows continuously; queue hovers at ≤1 flush interval (~25) |
| Telemetry-only outage | receiver paused → dispatch still accepted (202), loop stays ACTIVE, output ramps; queue grows (depth>2); backlog drains on resume while collection continues |
| Lost-ack replay | one-shot drop-after-commit → retry resends identical identities → receiver dedups as no-op, acks, rows pruned; `upload_failed` increments, no duplicate receiver rows |
| Crash/reopen (SIGKILL) | stream_id identical after restart; pre-kill committed events upload; WAL recovery no data loss |
| Store/queue overflow | max_status_events=10: `evicted_status` increments per overflow; dispatch accepted and loop ACTIVE under a full store; bounded handoff keeps control independent |
| Measurements | commit throughput 4151 events/s (241 µs/commit incl. seq alloc + cap check, 2000×320 B events); db+wal ≈ 264 KB at ~12 committed events (~22 KB/row at small n); receiver drained 250-event backlog in <1 s; voltix RSS ≈ 15.4 MB under collection+upload |

Unit evidence additionally: seq non-reuse across delete-all + reopen;
all-alarms saturation tier isolation; payload bounds; handoff volatile
drops; corrupt/incompatible store never silently reset; bounded PASSIVE
checkpoint; partial/foreign/undecodable acks prune nothing; receiver
restart persistence.

## M3 exit review

| Criterion | Status |
|---|---|
| Committed event retried until ack or eviction; pre-commit loss visible | Met — retry loop + volatile loss counters vs persistent evictions |
| Lost ack → retransmit, no duplicate receiver rows | Met — e2e lost-ack + receiver dedup tests |
| Both classes bounded; persisted counters agree with inputs; no alarm storm | Met — per-tier caps, transition-only alarms, atomic eviction counters |
| No sequence reuse after pruning/restart; acks can't remove unsent records | Met — meta-allocated seq (never MAX), subset-validated ack pruning |
| Disk-full/unavailable store keeps control independent; persistent vs volatile loss distinguished | Met at available-store level (unhealthy→volatile drops, handoff bound); see limitations |
| Measured backlog/throughput/memory/footprint recorded | Met — commit 4151 ev/s (241 µs), RSS 15.4 MB, db+wal 264 KB, drain <1 s/250 events |
| No power-loss durability claim from kill tests | Met — SIGKILL tests show WAL recovery, not hardware durability |

## Limitations

- **Pre-commit loss**: events between handoff offer and SQLite commit are
  lost on crash; counted as volatile drops, not guaranteed durable.
- **Disk-full / checkpoint contention**: exercised only at unit level
  (bounded busy checkpoint, unhealthy-store rejection). A real ENOSPC
  fault-injection is deferred — flagged as remaining risk for M4 fault
  automation.
- **Memory/commit-cost**: measured on this dev host only
  (4151 ev/s, RSS 15.4 MB); representative payload 320 B; numbers are
  workload-dependent and not production sizing.
- **Hardware power loss**: WAL+FULL assumes the storage stack honours
  fsync; no physical power-cut testing was performed.
- Receiver `drop-after-commit` is a one-shot test mode, adequate for
  single lost-response verification; sustained-drop behaviour is covered
  by the transient-retry path instead.

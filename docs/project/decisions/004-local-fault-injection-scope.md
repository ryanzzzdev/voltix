# 004 — Local fault-injection scope on an unprivileged host

Date: 2026-10-12
Status: Accepted scope revision after the M4 independent audit.

## Context

The acceptance matrix originally assumed packet-level fault injection
and a real filesystem ENOSPC scenario. The M4 audit correctly flagged
that the delivered fault suite substitutes process/application-level
faults and never injected true disk-full, yet rows were presented as
verified equivalents.

Measured host limits (2026-10-12):

- `unshare -r -n` fails on `/proc/self/uid_map` — no unprivileged network
  namespaces, so `tc`/`iptables`-class packet faults are impossible here.
- No `CAP_NET_RAW`, so packet capture (`tcpdump`) is unavailable.
- No mount permission and no usable tiny filesystem: `/tmp` and
  `/dev/shm` are 16 GiB tmpfs; `/run/user/1000` is 3.1 GiB but holds the
  live user-manager sockets — filling it would break the session.
- SQLite `PRAGMA max_page_count`/`journal_size_limit` do not produce a
  full-disk error under WAL (verified empirically).

## Decision

1. Fault injection on this host is application/process-level: sim
   `set_fault`/`set_online`/`delay`, receiver pause/drop-after-commit,
   SIGSTOP/SIGKILL, and bounded file-size limits. These are recorded as
   the delivered scope — not as packet-level equivalents.
2. Storage-write failure is exercised with a real kernel-enforced
   failure: `RLIMIT_FSIZE` + ignored `SIGXFSZ` in a re-execed test
   subprocess (`TestStorageWriteFailureSurfacedAndRecoverable`). This is
   an EFBIG-class "disk I/O error" (SQLite error 778) — the same commit
   error surface ENOSPC produces, labelled honestly rather than claimed
   as ENOSPC.
3. True filesystem ENOSPC, packet-level network faults, packet capture,
   hosted CI, physical power loss, and real hardware remain open
   follow-up work requiring a privileged/disposable environment. They are
   not marked met in the acceptance matrix.

## Consequences

- `evidence/demo/exit-review.md` rows F and the network-fault rows carry
  explicit scope language; nothing unavailable is reported as passed.
- `scripts/fault-test.sh` prints the fault-scope notice every run.
- If a privileged environment becomes available, the same harness gains
  `tc`/netns and real-ENOSPC variants without contract changes.

## References

- `docs/project/evidence/review/m4-independent-audit.md` (R4)
- `internal/store/store_test.go` — `TestStorageWriteFailureSurfacedAndRecoverable`
- `docs/project/blockers.md`

# Independent completion audit

Date: 2026-10-12
Reviewer: Codex desktop, GPT-6 family (exact variant unavailable).
Reviewed baseline: commit 95745cb; working tree initially clean.

## Conclusion

Substantial implementation and genuine recorded demo/resource artifacts exist, but M2–M4 are not fully complete against the unchanged contracts. Existing tests passing does not cover the control regressions below. Preserve Devin's historical results; this review supersedes the blanket completion conclusion, not the existence of those results.

## Findings

### R1 — Device fault does not invalidate dispatch (high)

`internal/policy/policy.go:117–119` sets FAULT but leaves Invalidate false. `internal/control/loop.go` only drops the session when Invalidate is true. An otherwise-live command therefore survives a device fault and can resume when fault status clears, without the new dispatch required by the control contract. The fault branch can also calculate a nonzero ramp target while the contract says nonzero commands are inhibited.

Independent temporary regression: fake inverter returns Evt1=2 with a valid 300 kW/30-second session; execute one cycle and require session Drop. FAILED: no Drop occurred. Existing TestDeviceFaultInhibits asserts only the state, not invalidation or recovery without fresh dispatch.

Required: invalidate prior intent on device fault, define/test fault target inhibition and require clear fault plus fresh dispatch before ACTIVE. Owner: VTX-011, with policy/loop fixes within that reopened outcome.

### R2 — Freshness calculation uses a pre-read clock (high)

`internal/control/loop.go:117–119` passes the ticker's earlier timestamp as Now and the subsequent read-start timestamp as MeasuredAt. Their difference is normally negative; it does not grow with read latency. The stale-input comparison in policy therefore does not protect the actual loop from a slow successful read.

Independent temporary regression: MaxSnapshotAge=10 ms, fake inverter read sleeps 50 ms and returns valid data, live dispatch. FAILED: cycle remains ACTIVE and requests 50,000 W. This scales the configured threshold to keep the test fast; it demonstrates the clock-order bug directly.

The read phase waits unconditionally for both goroutines. Endpoint retries and a documented five-second connection timeout are not bounded by the 200 ms phase budget; see `internal/device/endpoint.go`. No complete-cycle deadline/missed-deadline instrumentation closes that gap. Write failure marks acknowledgement nil but leaves policy's previous target advanced and intent active, contrary to recovery from a known acknowledgement/fresh measured baseline.

Required: consistent actual decision/receive timing, phase-wide connection/read/write budgets, and regression tests for slow successful reads, failed writes and recovery. Owner: VTX-011.

### R3 — Native test cleanup can delete pre-existing installation data (high)

`scripts/native-test.sh` installs into the normal voltix user-unit/configuration/binary/state paths and always invokes remove.sh on exit. install.sh overwrites existing unit/config/binary; remove.sh deletes env/config and recursively removes ~/.local/state/voltix without establishing that the harness created them. Even a prerequisites failure triggers the cleanup trap. This violates the task requirement to remove only test-created resources.

Confirmed by source inspection; deliberately did not run the native harness or deletion script. Required: unique test unit/state paths or explicit refusal to touch pre-existing resources, with ownership-aware cleanup and failure-path tests. Owner: VTX-017.

### R4 — Required acceptance checks remain unverified (medium)

The original `evidence/demo/exit-review.md` labels row F partially met: no actual ENOSPC injection. Packet-level faults and packet capture were unavailable and replaced with app/process faults. These are useful substitutes but do not satisfy all unchanged M3/M4 criteria; no accepted scope-change decision was found. The resource run measures read duration, which does not by itself establish total-cycle jitter/deadline performance.

Hosted CI, physical power loss and real hardware were already explicitly outside required local completion; their absence is not a new blocker. OOM-under-load should remain an honest limitation, not automatically a new acceptance requirement.

Required: close storage/network verification or obtain and record an explicit scope revision, preserving gaps as follow-up work. Owners: VTX-015, VTX-018 and VTX-020.

### R5 — Quality gate succeeds when formatter is unavailable (medium)

Running `make check` with Go found via the Makefile's fallback but gofmt absent from PATH prints `gofmt: command not found` and continues successfully to vet. At Makefile:17, command substitution produces an empty string and `test -z` passes. Resolve formatter from the same toolchain or fail explicitly on missing/execution-failed tools. Owner: VTX-020.

### R6 — Fault harness can report a false lost-ack pass (medium)

Test D reads cumulative upload_failed, which already increased during Test A; it does not require a new increment from D. It can satisfy count/upload checks before the requested dropped response has actually been exercised. Capture per-scenario baselines and prove commit, lost response, retry and dedup occurred. Other timing checks use coarse polling and broader bounds than the specification; Test B observes dispatch_active rather than an actual fallback command. Owner: VTX-018.

### R7 — Documentation state and evidence links are stale (low)

Roadmap header still says M3 complete while the table marks M4 complete. Backlog delivery notes still direct work to VTX-008/009. Roadmap links use docs/evidence although artifacts live at repository-root evidence; several findings links similarly resolve incorrectly. Correct current status/navigation without rewriting prior journal history. Owner: VTX-020.

## Independent checks

- Initial `make test`: passed using cached results; not treated as fresh execution evidence.
- Initial simulator/race runs in sandbox could not open sockets; environment limitation, not product failure.
- With PATH set to the installed Go toolchain and writable uv cache, `make check` passed and simulator tests passed 26/26 outside the socket-restricted sandbox.
- Uncached `go test -race -count=1 ./...`: passed all existing Go packages outside the socket-restricted sandbox.
- Two independent no-socket control regressions described above: both failed. Temporary test file removed afterwards; no production code changed.
- Uncached `go test -count=1 -tags=integration ./internal/device/ ./internal/control/`: passed (device 14.866 s, control 51.264 s).
- Native harness, Docker fault suite, and 30-minute resource test were not rerun in this audit. Recorded artifacts were inspected, not relabelled as independently reproduced.

## Next action

Fix control semantics/timing and native-harness isolation first. Then close the storage/network evidence gaps, strengthen fault assertions and the check target, and rerun the exit review. Do not restart the project; most implementation is already present.

## Resolution status (2026-10-12, Devin)

All findings were verified against source before fixing. Corrective
commits on `main`: e05caa2 (R1/R2 control), 6715588 (R3 isolation),
8ba02d9 (R4 store fault + R5 quality gate), 695846f (R6 harness).

- R1/R2 (VTX-011): fault now invalidates intent (`Invalidate=true`,
  inhibited-zero output, fresh dispatch required after clear); staleness
  uses acquisition-start vs post-read decision time; read/write phases
  bounded by configured deadlines; failed writes reset the ramp baseline
  to measured output. New regression tests cover every reproduced defect.
- R3 (VTX-017): PID-unique install names, `VOLTIX_REFUSE_EXISTING`,
  canonical-path guard, post-cleanup leak assertions. 13/13.
- R4 (VTX-015): `RLIMIT_FSIZE`/EFBIG subprocess test exercises the real
  commit-error path (SQLite 778). True ENOSPC and packet faults are an
  accepted scope revision — [decision 004](../../decisions/004-local-fault-injection-scope.md)
  — carried as follow-up VTX-022, not claimed as verified.
- R5 (VTX-020): `make check` resolves `gofmt` beside `$(GO)`; missing or
  erroring formatter now fails the gate (negative-shim tested).
- R6 (VTX-018): per-scenario baselines; D proves commit-then-drop then
  strict rows==uploaded dedup; B proves the zero was a controller write
  (watchdog unexpired). 17/17.
- R7 (VTX-020): link scan clean; status reconciled; exit review revised.

Re-verified this cycle: `make test`, `make check`, `-race ./...`,
`make sim-test` (see verification notes), `make integration` (45.8 s),
native-test 13/13, fault-test 17/17. The 30-minute resource run was not
repeated; its recorded artifact stands as prior evidence.

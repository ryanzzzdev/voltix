# Follow-up completion audit

Date: 2026-10-12
Baseline: 6d57fc1 (clean working tree at start).
Reviewer: Codex desktop / GPT-6 family; exact variant unavailable.

## Verdict

Corrections address several previous findings, but M2/M4 should not yet be closed. Two focused regression checks still fail, and native cleanup has remaining edge cases. This review checks the corrective changes rather than adding portfolio features.

## Improvements confirmed

- Fault now invalidates dispatch; clearing it requires a new command.
- Slow successful read freshness uses acquisition-start versus post-read time; an explicit regression exists.
- Failed writes request a measured ramp baseline for the next cycle.
- Formatter discovery/error handling now fails closed.
- A real RLIMIT_FSIZE/EFBIG storage-failure test and recovery check exist; this is correctly distinguished from ENOSPC.
- Native test uses unique names rather than normal installation names.
- Fault suite uses scenario-local counter baselines and checks controller-origin zero versus watchdog fallback.
- Decision 004 and VTX-022 explicitly retain unavailable privileged tests as follow-up. Its acceptance source should be recorded per the guide; this reviewer cannot infer a separate user approval from the supplied record. The reduced scope is transparent, but does not excuse the defects below.

## Remaining findings

### F1 — Timed-out operations continue and may issue stale commands (high)

`internal/control/loop.go:184–199` starts a write goroutine and stops waiting after WriteBudget, but never cancels the operation. Endpoint methods still acquire their mutex and perform connection/request work without the loop deadline. Later cycles spawn additional reads/writes while earlier calls remain pending. A write waiting behind a slow read or connection attempt can begin executing after its budget and after a newer decision. Returning promptly is not equivalent to bounding the device operation. Read goroutines likewise accumulate behind endpoint locks during persistent faults.

Focused reproduction: a fake adapter blocks *before applying* its target; WriteBudget is 1 ms. Step returns an uncertain acknowledgement. Releasing the adapter afterwards applies the old 50,000 W target. Regression FAILED. This is not a demand to undo a request already applied remotely: locally queued work must be cancellable/fenced before it reaches the device.

Required: deadline-aware connection/lock/I/O ownership; prevent expired queued writes from executing; bounded outstanding operations; test timeout followed by newer zero/dispatch, repeated outage cycles and shutdown. Tests must assert no late side effects, not only how quickly Step returns. Owner: VTX-011.

### F2 — FAULT still produces a nonzero command at nonzero output (high)

`internal/policy/policy.go:131` calls rampTo in the fault branch despite the explicit inhibit-nonzero contract. Its new test starts with zero measured power, so it cannot reveal the defect. A device fault at 200,000 W results in a 150,000 W requested target.

Focused regression with Evt1=2, measured inverter/site power 200,000 W, valid session and normal 50,000 W/s slew: FAILED, state FAULT with target 150,000 W. Required: issue the fault-policy zero target; distinguish that target from the simulator's physical ramp. Test positive and negative nonzero output and fault-clear recovery. Owner: VTX-011.

### F3 — Native cleanup still lacks reliable ownership/failure handling (medium)

`deploy/systemd/remove.sh:24` uses rm -f on STATE_DIR before directory removal. If StateDirectory is an ordinary directory instead of the dev-host symlink, rm returns an error and set -e exits before cleanup completes. Test both directory and symlink forms.

Also, native-test's EXIT trap unconditionally calls remove.sh even if install.sh refuses pre-existing PID-tagged paths. The harness does not check install success before continuing. A unique PID tag reduces collisions but is not ownership proof; a stale same-name installation after PID reuse can be removed despite refusal. Only cleanup resources claimed/created by this invocation; fail immediately on unsuccessful installation and test refusal/prerequisite failures. The current canonical-path guard compares existence only, not contents.

Source-inspected; no user services or installation data were changed by this review. Owner: VTX-017.

## Verification

- make check: passed with the installed Go toolchain.
- Simulator suite: 26 passed.
- Uncached go test -race -count=1 ./...: all existing packages passed, including the storage-failure subprocess.
- Uncached integration suites passed: device 14.860 s, control 50.841 s (loopback-enabled execution outside the restricted sandbox).
- Two temporary no-socket targeted regression tests: both failed as described. Temporary test source removed, production code unchanged.
- Native/Docker fault harness and 30-minute resource exercise were not rerun. Their existing artifacts remain historical evidence, not independent passes in this review.

## Next work

VTX-011: repair operation lifetime and nonzero-fault behaviour, then verify boundary tests. VTX-017: make cleanup ownership-aware and portable. VTX-020: reconcile and rerun completion review once these pass. Keep VTX-022 explicit; do not conflate deferred environment tests with corrected code.

## Resolution status (2026-10-12, Devin)

All three findings verified against source before fixing.

- F1: endpoints are ctx-deadline-aware. Operations serialise through a
  one-slot semaphore acquired via the caller's context — waiters exit at
  their deadline (bounded outstanding ops) and expired queued writes are
  fenced before issuing I/O. Verified: `TestExpiredQueuedWriteIsFenced`,
  `TestAdmissionBoundedByContext`, and `TestWritePhaseBounded` asserting
  the expired write never applied. In-flight writes remain bounded by the
  socket deadline and reported as uncertain — per the documented contract.
- F2: the fault branch now emits an immediate zero target, not a slew.
  Regression: `TestFaultAtNonzeroOutputRequestsZero` (200 kW measured →
  target 0); policy-level assertion updated to require `Invalidate` and
  `TargetW == 0`.
- F3: remove.sh handles plain-dir and symlink StateDirectory layouts and
  verifies `VOLTIX_OWNER_TAG` markers (unit + `.voltix-owner`) before
  deleting anything; native-test aborts on refused install, fingerprints
  canonical paths by content, and self-tests both refusal paths. 16/16.

Verification this cycle: `go test -count=1 ./...` and `-race` all green;
integration suites passed (device 14.9 s, control 51.4 s); native-test
16/16; `make check` clean. VTX-022 privileged-environment checks remain
open follow-up — not conflated with these corrections.

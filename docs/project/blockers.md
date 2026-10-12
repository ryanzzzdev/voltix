# Voltix blockers and open questions

Updated: 2026-10-12

No obstacle blocks beginning bootstrap or protocol investigation. These are explicit implementation gates, not reports of failed attempts. Task execution status remains in the [backlog](backlog.md).

<a id="blk-001"></a>

## BLK-001 — Exact simulator protocol profile

- Identified: 2026-10-12
- Status: Resolved
- Affected tasks: [VTX-004](backlog.md#vtx-004), VTX-005, VTX-006; M1 interoperability acceptance.
- Impact: Selected SunSpec models still need an immutable revision, exact register manifest, mandatory-value treatment, enable/write sequence and independently checked fixtures.
- Blocking scope: Gates adapter/simulator implementation that relies on exact wire fields; does not block bootstrap or protocol research.
- Evidence: Official model definitions were inspected during planning to distinguish SoC in 124, watt targets in 704, and the 203 meter. See [device contract](features/device-profile.md).
- Next action / owner: Implementing agent completes VTX-004, documenting source revision and narrowing unsupported behaviour if needed.
- Resolution: Resolved 2026-10-12. Vendored `sunspec/models@90b4a33` (Apache-2.0) and generated `contracts/sunspec-profile.json` plus 17 golden fixtures via `contracts/generate_profile.py`, derived from upstream definitions before either implementation exists. Atomic power command = one FC16 over the 4-register WSetEna..WSet window; meter sign convention declared sim policy.

<a id="blk-002"></a>

## BLK-002 — Driver compatibility and persistence build requirements

- Identified: 2026-10-12
- Status: Open
- Affected tasks: [VTX-006](backlog.md#vtx-006), [VTX-012](backlog.md#vtx-012).
- Impact: Go Modbus cancellation/socket behaviour and SQLite driver build/pragma support must be verified rather than inferred from API names.
- Blocking scope: Gates those adapters' acceptance; bootstrap and pure policy/specification work can proceed. No library failure is known.
- Evidence: No dependency has been installed or tested in Voltix. Architecture requires bounded socket I/O and WAL/FULL commit behaviour.
- Next action / owner: Implementing agent selects supported libraries and records small compatibility tests in the owning tasks. Resolve each sub-gate independently and close this record when both pass.
- Resolution: Pending; Modbus and SQLite sub-gates both unverified.

<a id="blk-003"></a>

## BLK-003 — Native systemd and network-fault execution environment

- Identified: 2026-10-12
- Status: Open
- Affected tasks: [VTX-017](backlog.md#vtx-017), [VTX-018](backlog.md#vtx-018); M4 native/fault evidence.
- Impact: Native systemd tests need an appropriate disposable Linux environment; network fault tests need scoped capabilities. Container lifecycle tests alone do not satisfy native acceptance.
- Blocking scope: Future native/privileged execution gate, not local application development. Do not mark the project blocked.
- Evidence: Current workspace permits documentation writes; systemd/VM/container privileges have not been inspected or exercised for this project.
- Next action / owner: Implementing agent inspects available local facilities at packaging time and chooses a disposable VM/test environment. Request missing environment input only if necessary; record unavailable checks accurately.
- Resolution: Pending environment verification. No service installation or network changes performed during planning.

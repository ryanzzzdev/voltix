# Device register contract

This directory is the frozen wire contract between the Voltix Go adapter and the
Python site simulator. It resolves blocker BLK-001.

## Contents

- `upstream/` — verbatim copies of the SunSpec model definitions used as the
  source of truth: `model_1.json`, `model_103.json`, `model_124.json`,
  `model_203.json`, `model_704.json`, and `LICENSE` (Apache-2.0).
- `sunspec-profile.json` — the generated register manifest: address base,
  unit IDs, model order, per-point offsets/addresses/types/access/scale
  factors, sentinels, the atomic WSet write window, and sim-only policies.
- `fixtures/*.json` — golden Modbus request/response vectors (PDU-level hex)
  with expected domain decodes: discovery, charge/discharge power, SoC
  boundaries, meter import/export, signed WSet writes, sentinels, and
  exception cases.
- `generate_profile.py` — regenerates the manifest and fixtures from the
  vendored upstream JSONs. Stdlib only: `python3 contracts/generate_profile.py`.

## Provenance and independence

- Source: `github.com/sunspec/models`, revision
  `90b4a331dcca1d6eac69c1bead952fddcc5852e0` (2026-08-20), Apache-2.0.
- The manifest and fixtures are computed from the vendored upstream
  definitions by `generate_profile.py` — they were produced before, and
  independently of, the simulator implementation (VTX-005) and the Go adapter
  (VTX-006). Both sides must conform to these fixtures; fixture bytes were
  hand-checked against the model layouts.
- Meter sign convention (positive = export) is a Voltix simulator policy, not
  a claimed manufacturer convention. The 10 s power-command watchdog is
  likewise simulator policy; do not read it as universal inverter behaviour.
- Fixed scale factors (e.g. `W_SF=2` for 500 kW range in int16) are demo
  choices recorded in the manifest; the adapter reads SF registers dynamically.

## Wire assumptions

- Big-endian register data; 32-bit points use high-word-first order (SunSpec
  default), as exercised by the int32 WSet fixtures.
- Function codes: FC3 reads, FC16 writes. The only valid power command is one
  atomic FC16 over the 4-register `WSetEna..WSet` window.
- Exceptions: 02 for out-of-map/read-only writes, 03 for malformed or
  partial-window writes.

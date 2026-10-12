# M1 device-integration findings

Date: 2026-10-12 (UTC)
Scope: VTX-007 — the complete device integration slice; Go adapter against the
Python simulator over real Modbus TCP.

## Environment

- Linux x86_64, Go 1.27.2 (`~/.local/go`), uv-managed CPython 3.13.14.
- `github.com/simonvetter/modbus` v1.6.4; `pymodbus` 3.15.0, pytest 9.1.1
  (pinned in `sim/pyproject.toml`/`sim/uv.lock`).
- Wire profile: `contracts/sunspec-profile.json` from `sunspec/models`
  @ `90b4a33` (vendored under `contracts/upstream/`).

## Commands

```bash
make check            # gofmt + go vet
make test             # unit + fake-server + golden-fixture tests
cd sim && uv run pytest -q          # simulator physics + wire tests
make integration      # go test -tags=integration: drives the real sim
```

## Results

- `go vet` clean; `go test ./...` — 13 adapter tests pass (discovery, typed
  reads, signed writes, sentinels, delayed/malformed/hangup, reconnect,
  uncertain write).
- `uv run pytest -q` — 26 simulator tests pass, including replaying the
  golden PDUs over real TCP.
- `TestDeviceIntegrationSlice` — 14.3 s against a live `python -m
  voltix_sim` subprocess:
  - Discovery walks both model chains (`{1,103,124,704}` / `{1,203}`).
  - Changing state observed: `set_soc_pct 60` → SoC reads 60.0;
    `set_baseline_kw 10` → meter reads +10 000 W.
  - Accepted vs measured power: +250 000 W and −250 000 W targets both
    accepted and reached; the WSet window reads back `[1, 1, 0x0003,
    0xD090]` matching the `write-target-discharge` golden fixture.
  - Independent watchdog: continuous polls across the window do not
    refresh it; ~10 s after the last accepted write the device ramps to
    0 W (100 kW/s fallback) and raises Evt1 bit0.
  - Bounded failure: `set_delay_ms 400` → `ErrTimeout` within ~0.5 s
    (two bounded attempts: timeout → reconnect → timeout);
    `set_online false` → bounded exception; restoring online recovers;
    `set_corrupt` → `ErrInvalidData`.

## Cross-checks against independent fixtures

The golden PDUs were generated from vendored upstream SunSpec model JSONs
before either implementation existed (`contracts/generate_profile.py`).
Independence held up in practice: the wire replay caught two fixture errors
(model 203 `W`/`W_SF` non-adjacency; −2500 int16 is `0xF63C`), which were
corrected in the generator and regenerated — the implementations were not
permitted to define the fixtures.

## M1 exit review

| Exit criterion | Status |
| --- | --- |
| Clean documented setup, smoke-tested entry point and simulator, pinned deps | Met — README commands; `bin/voltix` smoke; `python -m voltix_sim` smoke; `uv.lock` |
| Checked manifest citing immutable upstream revision | Met — manifest pins `sunspec/models@90b4a33` |
| Independent fixtures verify decode/write incl. negative watts | Met — 17 golden PDUs; both implementations conform; two fixture bugs caught by wire tests |
| Bounded simulator energy; only accepted power refreshes watchdog | Met — unit + wire + live integration evidence |
| Adapter bounded failure and reconnect (delayed/malformed/disconnected) | Met — unit tests and live delay/offline/corrupt injection |

## Known limitations

- The sim's 10 s watchdog and ramp rates are declared simulator policy, not
  vendor behaviour; interoperability is demonstrated against the simulator
  only (per M1 scope).
- The library's connect dial has a fixed 5 s bound (per connection attempt).
- Corrupt-scale-factor (`sentinel-sunssf`) is verified decode-side only; the
  sim's corrupt flag emits value sentinels.

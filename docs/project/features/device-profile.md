# Device profile and simulation contract

Date: 2026-10-12
Status: Selected simulator scope; register-level manifest and interoperability remain VTX-004 verification gates.

## Scope and model selection

One Python process models a battery inverter and a site meter, exposed on distinct Modbus TCP endpoints. Use configurable base address and unit IDs, a SunSpec identifier/common discovery layout, standard model lengths/offsets, and an end marker. Do not compress selected fields into custom offsets or claim full SunSpec certification.

| Endpoint | Model | Required use |
| --- | --- | --- |
| Inverter | 103 | Measured active power, status, illustrative voltage/frequency |
| Inverter | 124 | ChaState with its scale factor for state of charge |
| Inverter | 704 | Absolute active-power control through WSet in watts, WSetMod and WSetEna |
| Site meter | 203 | Total site active power and its scale factor |

Model 124 provides ChaState; do not invent a generic SoC register. In model 704, WSet is distinct from the maximum-power limit WMaxLimPct. This profile chooses watt-based targets rather than treating those controls as interchangeable. Model 203 is the selected three-phase meter model. Sources: [124](https://github.com/sunspec/models/blob/master/json/model_124.json), [704](https://github.com/sunspec/models/blob/master/json/model_704.json), [203](https://github.com/sunspec/models/blob/master/json/model_203.json), [103](https://github.com/sunspec/models/blob/master/json/model_103.json).

VTX-004 pinned upstream `sunspec/models` revision `90b4a331dcca1d6eac69c1bead952fddcc5852e0` and produced the checked manifest and golden fixtures in [contracts/](../../../contracts/). The manifest fixes base address 40000, unit IDs, model order, per-point layout, fixed scale factors, sentinels and the atomic FC16 `WSetEna..WSet` write window. This is a restricted simulator profile, not universal model conformance.

Fixture expectations must be reviewed independently of simulator generation, including positive/negative watts, scaling, sentinel values and malformed responses. Do not infer physical units or word order from field names alone. Normalise any meter sign convention at the adapter boundary and test it.

## Domain conventions

Voltix domain/API power is in kW, energy in kWh, SoC in percent, durations in seconds. Battery power is positive for discharge and negative for charging. Site power is positive for export. Wire values are converted explicitly using the checked model scale factors; reject invalid/non-finite values and integer overflow rather than silently rounding out of range.

The fixture site uses:

- Battery energy capacity 1,000 kWh and initial SoC 50%.
- Charge/discharge capability 500 kW in each direction.
- Controller operating SoC interval 5–95%; simulator physical energy bounds 0–100%.
- Normal target slew 50 kW/s; device fallback slew 100 kW/s.
- Illustrative charge/discharge efficiency 1.0 for the first simulator; no thermal/chemistry/degradation model.
- Site baseline net generation configurable, initially 0 kW; export limit 400 kW.

These are deterministic test inputs, not equipment recommendations. Keep tests able to override them to exercise boundaries quickly.

## Simulation dynamics

Advance energy from measured output and elapsed monotonic time: E_next = E - P_battery * dt / 3600 for the initial unity-efficiency model. Bound energy and physical output locally; do not allow numerical overshoot outside the physical energy range. Report measured output separately from requested/accepted target.

Site export = battery output + baseline net generation (generation minus local load). Meter and inverter samples derive from one site model; separately served endpoints allow link failures independently. Scenario controls can change the baseline, readings, fault state, delay and availability through a test-only interface inaccessible to the controller's normal driver.

## Independent command timeout

An accepted write of a complete valid power target refreshes the simulator's 10-second deadline, even when the target is unchanged. Reading registers, malformed writes, unsupported addresses, or changing unrelated configuration do not refresh it. Describe the refresh rule as simulator policy; do not assume every physical inverter implements it.

On expiry the device begins moving toward zero at its fallback rate. The start tolerance is 500 ms in the designated unloaded test environment; time to zero also includes abs(start_power)/fallback_rate. Its clock and watchdog must keep running when a client stalls or disconnects. Log/watchdog observations come from the simulator itself, not only the controller.

The simulator's own constraints operate when the controller is down. Emergency simulated faults may force output to zero immediately; normal rate limits do not override physical bounds.

## Acceptance ownership

[VTX-004](../backlog.md#vtx-004) fixes the wire profile; [VTX-005](../backlog.md#vtx-005) implements/tests the simulator; [VTX-006](../backlog.md#vtx-006) implements the adapter; [VTX-007](../backlog.md#vtx-007) proves the slice together.

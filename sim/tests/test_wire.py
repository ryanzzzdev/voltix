"""Wire-level tests: real pymodbus client against the running simulator.

Golden fixtures in contracts/fixtures are replayed verbatim where a site
precondition can be arranged; this is the independent cross-check the
contract requires (fixtures derive from upstream model definitions, not from
this implementation).
"""

import dataclasses
import json
import time
from pathlib import Path

import pytest
from pymodbus.client import ModbusTcpClient

FIXTURES = Path(__file__).resolve().parents[2] / "contracts" / "fixtures"


def read_regs(client, hex_req: str):
    parts = hex_req.split()
    fc, addr, count = int(parts[0], 16), int(parts[1], 16), int(parts[2], 16)
    assert fc == 0x03
    return client.read_holding_registers(addr, count=count, device_id=1)


def expected_fc3_regs(hex_resp: str) -> list[int]:
    b = bytes.fromhex(hex_resp)
    assert b[0] == 0x03
    return [int.from_bytes(b[2 + i:4 + i], "big") for i in range(0, len(b) - 2, 2)]


def fixture(name):
    return json.loads((FIXTURES / f"{name}.json").read_text())


def client_for(port):
    c = ModbusTcpClient("127.0.0.1", port=port)
    assert c.connect()
    return c


def drive(site, watts: float):
    """Make the device hold `watts` output: instant tracking, long watchdog."""
    site.cfg = dataclasses.replace(
        site.cfg, track_rate_wps=1e9, fallback_rate_wps=1e9, watchdog_s=3600.0)
    site.accept_power_command(time.monotonic(), ena=1, mod=1, watts=int(watts))


# ---------------------------------------------------------------- discovery


def test_suns_marker_and_model_headers(rig):
    c = client_for(rig.inv_port)
    fx = fixture("discovery-suns-marker")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    fx = fixture("discovery-model-headers")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    # walk the model chain headers
    assert c.read_holding_registers(40070, count=2, device_id=1).registers == [103, 50]
    assert c.read_holding_registers(40122, count=2, device_id=1).registers == [124, 24]
    assert c.read_holding_registers(40148, count=2, device_id=1).registers == [704, 65]
    assert c.read_holding_registers(40215, count=2, device_id=1).registers == [0xFFFF, 0]
    c.close()


def test_meter_headers(rig):
    c = client_for(rig.met_port)
    assert c.read_holding_registers(40000, count=2, device_id=1).registers == [0x5375, 0x6E53]
    assert c.read_holding_registers(40070, count=2, device_id=1).registers == [203, 105]
    assert c.read_holding_registers(40177, count=2, device_id=1).registers == [0xFFFF, 0]
    c.close()


# ----------------------------------------------------------------- readings


def test_inverter_discharge_fixture(rig):
    drive(rig.site, 250_000)
    c = client_for(rig.inv_port)
    fx = fixture("inverter-discharge")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    c.close()


def test_inverter_charge_negative_fixture(rig):
    drive(rig.site, -250_000)
    c = client_for(rig.inv_port)
    fx = fixture("inverter-charge")
    resp = read_regs(c, fx["request_pdu_hex"])
    assert resp.registers == expected_fc3_regs(fx["expected_response_pdu_hex"])
    assert resp.registers[0] == 0xF63C  # -2500 int16 on the wire
    c.close()


def test_soc_fixtures(rig):
    c = client_for(rig.inv_port)
    rig.site.energy_kwh = rig.site.cfg.capacity_kwh * 0.50
    fx = fixture("soc-normal")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    rig.site.energy_kwh = rig.site.cfg.capacity_kwh * 0.05
    fx = fixture("soc-boundaries")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    c.close()


def test_meter_fixtures(rig):
    drive(rig.site, 180_000)
    c = client_for(rig.met_port)
    fx = fixture("meter-export")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    drive(rig.site, -120_000)
    fx = fixture("meter-import")
    assert read_regs(c, fx["request_pdu_hex"]).registers == \
        expected_fc3_regs(fx["expected_response_pdu_hex"])
    c.close()


def test_sentinel_fixtures(rig):
    rig.site.corrupt = True  # value sentinels on W and ChaState
    c = client_for(rig.inv_port)
    for name in ("sentinel-w", "sentinel-chastate"):
        fx = fixture(name)
        assert read_regs(c, fx["request_pdu_hex"]).registers == \
            expected_fc3_regs(fx["expected_response_pdu_hex"]), name
    c.close()
    # sentinel-sunssf is exercised against the Go adapter decoder in VTX-006;
    # the sim's corrupt flag does not emit SF sentinels.


# ------------------------------------------------------------------- writes


def _write_window(client, rig, vals):
    return client.write_registers(rig.inv_addr("WSetEna"), vals, device_id=1)


def test_write_target_discharge_fixture(rig):
    c = client_for(rig.inv_port)
    fx = fixture("write-target-discharge")
    req = bytes.fromhex(fx["request_pdu_hex"])
    vals = [int.from_bytes(req[6 + i:8 + i], "big") for i in range(0, 8, 2)]
    resp = _write_window(c, rig, vals)
    assert not resp.isError()
    rb = c.read_holding_registers(rig.inv_addr("WSetEna"), count=4, device_id=1)
    assert rb.registers == vals  # readback: enable, mode, int32 target
    assert rig.site.target_w == 250_000
    assert rig.site.enabled
    c.close()


def test_write_target_charge_negative_fixture(rig):
    c = client_for(rig.inv_port)
    fx = fixture("write-target-charge")
    req = bytes.fromhex(fx["request_pdu_hex"])
    vals = [int.from_bytes(req[6 + i:8 + i], "big") for i in range(0, 8, 2)]
    resp = _write_window(c, rig, vals)
    assert not resp.isError()
    assert rig.site.target_w == -250_000
    c.close()


def test_write_target_zero_fixture(rig):
    c = client_for(rig.inv_port)
    resp = _write_window(c, rig, [1, 1, 0, 0])
    assert not resp.isError()
    assert rig.site.target_w == 0
    c.close()


def test_write_readonly_rejected(rig):
    c = client_for(rig.inv_port)
    resp = c.write_registers(rig.inv_addr("St"), [4], device_id=1)
    assert resp.isError()
    c.close()


def test_bad_address_rejected(rig):
    c = client_for(rig.inv_port)
    resp = c.read_holding_registers(39999, count=2, device_id=1)
    assert resp.isError()
    c.close()


def test_partial_wset_window_rejected_no_refresh(rig):
    c = client_for(rig.inv_port)
    # writing only the two WSet registers is not a valid power command
    resp = c.write_registers(rig.inv_addr("WSet"), [0x0003, 0xD090], device_id=1)
    assert resp.isError()
    assert rig.site.last_accept is None
    assert rig.site.target_w == 0
    c.close()


def test_unrelated_write_does_not_refresh(rig):
    c = client_for(rig.inv_port)
    resp = c.write_registers(rig.inv_addr("StorCtl_Mod"), [1], device_id=1)
    assert not resp.isError()
    assert rig.site.last_accept is None  # config writes never refresh
    c.close()


# ----------------------------------------------------------------- watchdog


def test_polls_do_not_refresh_and_fallback_starts(rig):
    rig.site.cfg = type(rig.site.cfg)(
        **{**rig.site.cfg.__dict__, "watchdog_s": 0.5,
           "track_rate_wps": 250_000, "fallback_rate_wps": 100_000})
    c = client_for(rig.inv_port)
    resp = _write_window(c, rig, [1, 1, 0x0003, 0xD090])  # +250 kW
    assert not resp.isError()
    for _ in range(4):  # keep polling past the deadline: polls must not refresh
        c.read_holding_registers(rig.inv_addr("W"), count=1, device_id=1)
        time.sleep(0.15)
    time.sleep(0.4)  # now ~1.0 s past the last accepted write
    assert rig.site.watchdog_expired(time.monotonic())
    # Evt1 bit0 (low word) = watchdog tripped flag rendered on the wire
    evt = c.read_holding_registers(rig.inv_addr("Evt1"), count=2, device_id=1)
    assert evt.registers[1] & 0x0001
    c.close()


def test_control_port_state(rig):
    out = rig.control({"cmd": "get_state"})
    assert out["ok"]
    assert out["state"]["soc_pct"] == 50.0
    out = rig.control({"cmd": "set_baseline_kw", "value": -50})
    assert out["ok"]
    out = rig.control({"cmd": "get_state"})
    assert out["state"]["baseline_kw"] == -50.0

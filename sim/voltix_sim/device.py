"""Build pymodbus SimDevices for the simulated site from the checked manifest.

Register contents are recomputed from Site state on every request through the
pymodbus `action` hook, so a slow or absent client never freezes the physics:
`Site.advance(now)` is driven by monotonic time deltas and catches up lazily.
"""

from __future__ import annotations

import asyncio
import time
from collections.abc import Callable

from pymodbus.constants import ExcCodes
from pymodbus.simulator import SimData, SimDevice
from pymodbus.simulator.simutils import DataType

from .manifest import EndpointMap, Point, Profile
from .site import Site

# SunSpec "not implemented" sentinels per wire type.
_SENTINEL = {
    "uint16": 0xFFFF, "enum16": 0xFFFF, "bitfield16": 0xFFFF,
    "int16": 0x8000, "sunssf": 0x8000,
    "uint32": 0xFFFFFFFF, "acc32": 0xFFFFFFFF, "bitfield32": 0xFFFFFFFF,
    "int32": 0x80000000, "string": 0x0000, "pad": 0x0000,
}


def encode(point_type: str, size: int, value) -> list[int]:
    """Encode a Python value into 16-bit registers for one manifest point."""
    if point_type in ("uint16", "int16", "enum16", "bitfield16", "sunssf"):
        return [int(value) & 0xFFFF]
    if point_type in ("uint32", "int32", "acc32", "bitfield32"):
        v = int(value) & 0xFFFFFFFF
        return [(v >> 16) & 0xFFFF, v & 0xFFFF]
    if point_type == "string":
        data = str(value).encode("utf-8")[: size * 2].ljust(size * 2, b"\x00")
        return [(data[i] << 8) | data[i + 1] for i in range(0, size * 2, 2)]
    if point_type == "pad":
        return [0] * size
    raise ValueError(f"unsupported point type {point_type}")


def _decode_int32(hi: int, lo: int) -> int:
    v = (hi << 16) | lo
    return v - 0x100000000 if v & 0x80000000 else v


def _initial_value(p: Point, ep_common: dict) -> int | str:
    if p.fixed_value is not None:
        return p.fixed_value
    if p.type == "string":
        return ep_common.get(p.name, "")
    if p.type == "pad" or p.name == "DA":
        return 0
    if p.name in _DYNAMIC:
        return 0  # rendered live on every request
    if p.writable:
        return 0  # writable config registers start at zero
    return _SENTINEL.get(p.type, 0)


# Points rendered live from Site state. Everything else keeps its initial
# (fixed or sentinel) value, except client-written writable config registers.
_DYNAMIC = {
    "A", "AphA", "AphB", "AphC", "W", "Hz", "WH", "TmpCab", "St", "Evt1",
    "Evt2", "ChaState", "ChaSt", "WSetEna", "WSetMod", "WSet",
    "PhV", "PhVphA", "PhVphB", "PhVphC", "PPV", "PPVphAB", "PPVphBC",
    "PPVphCA", "TotWhExp", "TotWhImp", "Evt", "WChaMax",
}


def _scaled(raw_phys: float, sf: int) -> int:
    return int(round(raw_phys / (10.0 ** sf)))


def _inverter_live(site: Site, now: float) -> dict[str, float]:
    out = site.output_w
    if site.fault:
        st, cha = 7, 1
    elif site.soc_pct <= 0.001:
        st, cha = 8, 2
    elif site.soc_pct >= 99.999:
        st, cha = 8, 5
    elif out > 0:
        st, cha = 4, 3
    elif out < 0:
        st, cha = 4, 4
    else:
        st, cha = 8, 6
    evt1 = (1 if site.watchdog_expired(now) else 0) | (2 if site.fault else 0)
    return {
        "A": out / 400.0, "AphA": out / 1200.0, "AphB": out / 1200.0,
        "AphC": out / 1200.0, "PhVphA": 400.0, "PhVphB": 400.0,
        "PhVphC": 400.0, "W": out, "Hz": 50.0, "WH": site.discharged_wh,
        "TmpCab": 25.0, "St": st, "Evt1": evt1, "Evt2": 0,
        "WChaMax": site.cfg.max_power_w, "ChaState": site.soc_pct,
        "ChaSt": cha,
        "WSetEna": 1 if site.enabled else 0, "WSetMod": 1,
        "WSet": site.target_w,
    }


def _meter_live(site: Site) -> dict[str, float]:
    sw = site.site_w
    return {
        "A": sw / 400.0, "AphA": sw / 1200.0, "AphB": sw / 1200.0,
        "AphC": sw / 1200.0, "PhV": 400.0, "PhVphA": 400.0,
        "PhVphB": 400.0, "PhVphC": 400.0, "PPV": 400.0,
        "PPVphAB": 400.0, "PPVphBC": 400.0, "PPVphCA": 400.0,
        "Hz": 50.0, "W": sw, "TotWhExp": site.exported_wh,
        "TotWhImp": site.imported_wh, "Evt": 0,
    }


def build_device(
    site: Site,
    profile: Profile,
    endpoint: str,
    clock: Callable[[], float] = time.monotonic,
) -> SimDevice:
    """Create the SimDevice for one endpoint (inverter or meter)."""
    ep = profile.endpoints[endpoint]
    common = profile.common_block
    common_ep = dict(common.get(endpoint, {}))
    common_ep.update({k: v for k, v in common.items() if k in ("Mn", "Vr", "DA")})

    # Model-id -> SF map for raw conversion; SF regs themselves are fixed.
    sf_by_model = profile.scale_factors

    model_of_point: dict[int, int] = {}
    for model in profile.raw["models"][endpoint]:
        for p in model["points"]:
            model_of_point[p["address"]] = model["id"]

    def point_sf(p: Point) -> int:
        if p.sf_ref:
            return sf_by_model[model_of_point[p.address]].get(p.sf_ref, 0)
        return 0

    # ---- initial register contents + SimData blocks by writability -------
    values: dict[int, int] = {}
    writable: dict[int, bool] = {}
    for p in ep.points:
        init = _initial_value(p, common_ep)
        regs = encode(p.type, p.size, init)
        for i, r in enumerate(regs):
            values[p.address + i] = r
            writable[p.address + i] = p.writable
    for i, m in enumerate(profile.suns_marker):
        values[ep.span_start + i] = m
        writable[ep.span_start + i] = False
    values[ep.end_marker_address] = 0xFFFF
    values[ep.end_marker_address + 1] = 0
    writable[ep.end_marker_address] = writable[ep.end_marker_address + 1] = False

    simdata: list[SimData] = []
    run_start = ep.span_start
    run_writable = writable.get(run_start, False)
    for addr in range(ep.span_start + 1, ep.span_end + 1):
        w = writable.get(addr, False)
        if addr == ep.span_end or w != run_writable:
            simdata.append(SimData(
                address=run_start,
                values=[values[a] for a in range(run_start, addr)],
                datatype=DataType.REGISTERS,
                readonly=not run_writable,
            ))
            run_start, run_writable = addr, w

    # ---- live render ------------------------------------------------------
    def render(regs: list[int], base: int, now: float) -> None:
        dyn = _inverter_live(site, now) if endpoint == "inverter" else _meter_live(site)
        for p in ep.points:
            if p.name not in dyn:
                continue
            if site.corrupt and p.name in ("W", "ChaState"):
                raw = _SENTINEL[p.type]
            else:
                raw = _scaled(dyn[p.name], point_sf(p))
            for i, r in enumerate(encode(p.type, p.size, raw)):
                regs[p.address + i - base] = r

    # ---- request gate ------------------------------------------------------
    wset = ep.wset_window

    async def action(fc, start, address, count, regs, set_values):
        if site.delay_s > 0:
            await asyncio.sleep(site.delay_s)
        if not site.online:
            return ExcCodes.DEVICE_BUSY
        now = clock()
        site.advance(now)
        render(regs, start, now)
        if set_values is None:
            return None
        if wset is not None:
            exact = fc == 16 and address == wset[0] and count == wset[1]
            overlaps = address < wset[0] + wset[1] and address + count > wset[0]
            if exact:
                try:
                    site.accept_power_command(
                        now, set_values[0], set_values[1],
                        _decode_int32(set_values[2], set_values[3]))
                except ValueError:
                    return ExcCodes.ILLEGAL_VALUE
                return None
            if overlaps:
                # Partial power-command writes are not valid commands and never
                # refresh the watchdog.
                return ExcCodes.ILLEGAL_VALUE
        return None

    return SimDevice(id=ep.unit_id, simdata=simdata, action=action)

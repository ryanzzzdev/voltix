"""Test-only fault/scenario control interface.

JSON-lines over TCP on its own port; unreachable through the Modbus device
endpoints, matching the contract that scenario controls are inaccessible to
the controller's normal driver path.
"""

from __future__ import annotations

import asyncio
import json
import time
from collections.abc import Callable

from .site import Site


def dispatch(site: Site, cmd: dict, clock: Callable[[], float]) -> dict:
    c = cmd.get("cmd")
    v = cmd.get("value")
    if c == "ping":
        return {"ok": True}
    if c == "get_state":
        site.advance(clock())
        return {"ok": True, "state": site.snapshot(clock())}
    if c == "set_baseline_kw":
        site.baseline_w = float(v) * 1000.0
    elif c == "set_soc_pct":
        site.energy_kwh = site.cfg.capacity_kwh * float(v) / 100.0
    elif c == "set_fault":
        site.fault = bool(v)
    elif c == "set_online":
        site.online = bool(v)
    elif c == "set_delay_ms":
        site.delay_s = float(v) / 1000.0
    elif c == "set_corrupt":
        site.corrupt = bool(v)
    elif c == "set_watchdog_s":
        site.cfg = type(site.cfg)(**{**site.cfg.__dict__, "watchdog_s": float(v)})
    elif c == "force_output_kw":
        site.output_w = float(v) * 1000.0
    elif c == "reset":
        site.now0 = clock()
        site.__post_init__()
    else:
        return {"ok": False, "error": f"unknown cmd {c!r}"}
    return {"ok": True}


async def _handle(reader: asyncio.StreamReader, writer: asyncio.StreamWriter,
                  site: Site, clock: Callable[[], float]) -> None:
    try:
        while line := await reader.readline():
            try:
                cmd = json.loads(line)
            except json.JSONDecodeError:
                resp = {"ok": False, "error": "bad json"}
            else:
                resp = dispatch(site, cmd, clock)
            writer.write(json.dumps(resp).encode() + b"\n")
            await writer.drain()
    finally:
        writer.close()


async def start_control_server(site: Site, host: str, port: int,
                               clock: Callable[[], float] = time.monotonic
                               ) -> asyncio.AbstractServer:
    return await asyncio.start_server(
        lambda r, w: _handle(r, w, site, clock), host, port)

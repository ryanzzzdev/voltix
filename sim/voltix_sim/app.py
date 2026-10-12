"""Run the simulated battery site: inverter + meter Modbus TCP endpoints and
the test-only control interface.

    python -m voltix_sim [--host H] [--inverter-port N] [--meter-port N]
                         [--control-port N] [--watchdog-s S] [--soc-pct P]
"""

from __future__ import annotations

import argparse
import asyncio
import logging
import time

from pymodbus import FramerType
from pymodbus.server import ModbusTcpServer

from .control import start_control_server
from .device import build_device
from .manifest import load
from .site import Site, SiteConfig

log = logging.getLogger("voltix-sim")


async def _ticker(site: Site, clock) -> None:
    """Advance physics and log transitions; never required for correctness."""
    last = None
    while True:
        site.advance(clock())
        snap = site.snapshot(clock())
        state = (snap["enabled"], snap["fault"], snap["watchdog_expired"],
                 round(site.output_w / 1000.0, 1))
        if state != last:
            log.info("site state: %s", snap)
            last = state
        await asyncio.sleep(0.25)


async def run(host: str, inverter_port: int, meter_port: int, control_port: int,
              site: Site, clock=time.monotonic) -> None:
    profile = load()
    inv_dev = build_device(site, profile, "inverter", clock)
    met_dev = build_device(site, profile, "meter", clock)

    inv_srv = ModbusTcpServer(inv_dev, framer=FramerType.SOCKET,
                            address=(host, inverter_port))
    met_srv = ModbusTcpServer(met_dev, framer=FramerType.SOCKET,
                              address=(host, meter_port))
    await inv_srv.serve_forever(background=True)
    await met_srv.serve_forever(background=True)
    ctl = await start_control_server(site, host, control_port, clock)
    ticker = asyncio.create_task(_ticker(site, clock))

    log.info("inverter on %s:%d, meter on %s:%d, control on %s:%d",
             host, inverter_port, host, meter_port, host, control_port)
    try:
        await asyncio.Event().wait()
    finally:
        ticker.cancel()
        ctl.close()
        await inv_srv.shutdown()
        await met_srv.shutdown()


def main() -> None:
    p = argparse.ArgumentParser(prog="voltix-sim")
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--inverter-port", type=int, default=1502)
    p.add_argument("--meter-port", type=int, default=1503)
    p.add_argument("--control-port", type=int, default=1504)
    p.add_argument("--watchdog-s", type=float, default=10.0)
    p.add_argument("--soc-pct", type=float, default=50.0)
    p.add_argument("--baseline-kw", type=float, default=0.0)
    args = p.parse_args()

    logging.basicConfig(level=logging.INFO,
                        format="%(asctime)s %(name)s %(message)s")
    site = Site(SiteConfig(watchdog_s=args.watchdog_s,
                           initial_soc_pct=args.soc_pct,
                           baseline_w=args.baseline_kw * 1000.0),
                now0=time.monotonic())
    try:
        asyncio.run(run(args.host, args.inverter_port, args.meter_port,
                        args.control_port, site))
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()

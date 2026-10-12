"""Real-server fixture: runs both Modbus endpoints + control on free ports."""

import asyncio
import socket
import threading
import time

import pytest
from pymodbus import FramerType
from pymodbus.server import ModbusTcpServer

from voltix_sim.control import start_control_server
from voltix_sim.device import build_device
from voltix_sim.manifest import load
from voltix_sim.site import Site, SiteConfig


def free_port() -> int:
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


class Rig:
    def __init__(self, site, profile, ports):
        self.site = site
        self.profile = profile
        self.inv_port, self.met_port, self.ctl_port = ports

    def inv_addr(self, point):
        return self.profile.endpoints["inverter"].addr(point)

    def met_addr(self, point):
        return self.profile.endpoints["meter"].addr(point)

    def control(self, cmd: dict) -> dict:
        with socket.create_connection(("127.0.0.1", self.ctl_port), timeout=5) as c:
            c.sendall((json.dumps(cmd) + "\n").encode())
            return json.loads(c.makefile().readline())


import json  # noqa: E402  (used by Rig.control)


@pytest.fixture
def rig():
    site = Site(now0=time.monotonic())
    profile = load()
    ports = (free_port(), free_port(), free_port())
    loop = asyncio.new_event_loop()
    servers = {}

    def _thread():
        asyncio.set_event_loop(loop)

        async def boot():
            inv = ModbusTcpServer(
                build_device(site, profile, "inverter"),
                framer=FramerType.SOCKET, address=("127.0.0.1", ports[0]))
            met = ModbusTcpServer(
                build_device(site, profile, "meter"),
                framer=FramerType.SOCKET, address=("127.0.0.1", ports[1]))
            await inv.serve_forever(background=True)
            await met.serve_forever(background=True)
            ctl = await start_control_server(site, "127.0.0.1", ports[2])
            servers.update(inv=inv, met=met, ctl=ctl)

        loop.run_until_complete(boot())
        loop.run_forever()

    t = threading.Thread(target=_thread, daemon=True)
    t.start()
    deadline = time.monotonic() + 5
    while "ctl" not in servers and time.monotonic() < deadline:
        time.sleep(0.01)
    yield Rig(site, profile, ports)

    async def _stop():
        servers["ctl"].close()
        await servers["inv"].shutdown()
        await servers["met"].shutdown()
    asyncio.run_coroutine_threadsafe(_stop(), loop).result(timeout=5)
    loop.call_soon_threadsafe(loop.stop)
    t.join(timeout=5)

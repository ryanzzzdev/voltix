"""Load the checked wire profile (contracts/sunspec-profile.json).

The simulator is built from this manifest so that the register map, scale
factors and the power-command window cannot drift from the frozen contract.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

PROFILE_PATH = (
    Path(__file__).resolve().parents[2] / "contracts" / "sunspec-profile.json"
)


@dataclass(frozen=True)
class Point:
    name: str
    offset: int
    address: int
    size: int
    type: str
    writable: bool
    sf_ref: str | None
    fixed_value: int | None


@dataclass
class EndpointMap:
    name: str
    unit_id: int
    default_port: int
    span_start: int
    span_end: int  # one past the end-marker registers
    end_marker_address: int
    points: list[Point]
    wset_window: tuple[int, int] | None  # (start address, register count)

    def addr(self, name: str) -> int:
        for p in self.points:
            if p.name == name:
                return p.address
        raise KeyError(f"{self.name}: no point {name}")

    def point(self, name: str) -> Point:
        for p in self.points:
            if p.name == name:
                return p
        raise KeyError(f"{self.name}: no point {name}")


@dataclass
class Profile:
    raw: dict
    base: int
    suns_marker: tuple[int, int]
    endpoints: dict[str, EndpointMap]
    scale_factors: dict[int, dict[str, int]]
    common_block: dict


def load(path: str | Path | None = None) -> Profile:
    raw = json.loads(Path(path or PROFILE_PATH).read_text())
    base = raw["address_base"]

    endpoints: dict[str, EndpointMap] = {}
    for ep_name, ep_info in raw["endpoints"].items():
        points: list[Point] = []
        for model in raw["models"][ep_name]:
            for p in model["points"]:
                points.append(Point(
                    name=p["name"],
                    offset=p["offset"],
                    address=p["address"],
                    size=p["size"],
                    type=p["type"],
                    writable=p["access"] == "RW",
                    sf_ref=p.get("scale_factor_ref"),
                    fixed_value=p.get("fixed_value"),
                ))
        wset_window = None
        if ep_name == "inverter":
            wset_window = (raw["control_write"]["start_address"], 4)
        span_end = ep_info["end_marker_address"] + 2
        endpoints[ep_name] = EndpointMap(
            name=ep_name,
            unit_id=ep_info["unit_id"],
            default_port=ep_info["default_port"],
            span_start=base,
            span_end=span_end,
            end_marker_address=ep_info["end_marker_address"],
            points=points,
            wset_window=wset_window,
        )

    return Profile(
        raw=raw,
        base=base,
        suns_marker=tuple(raw["suns_marker"]["value"]),
        endpoints=endpoints,
        scale_factors={int(k): v for k, v in raw["scale_factors"].items()},
        common_block=raw["common_block"],
    )

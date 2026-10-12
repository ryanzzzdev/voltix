#!/usr/bin/env python3
"""Generate the Voltix SunSpec wire profile manifest and golden fixtures.

Derivation source: vendored upstream model definitions in contracts/upstream/
(sunspec/models @ pinned revision). Running this script regenerates
sunspec-profile.json and fixtures/ deterministically. It is the independent
cross-check: the simulator and the Go adapter must conform to its output, not
to each other.

Usage: python3 contracts/generate_profile.py
"""

import json
import struct
from pathlib import Path

HERE = Path(__file__).resolve().parent
UPSTREAM = HERE / "upstream"
FIXTURES = HERE / "fixtures"
REVISION = "90b4a331dcca1d6eac69c1bead952fddcc5852e0"

ADDRESS_BASE = 40000
SUNS_MARKER = (0x5375, 0x6E53)
END_ID = 0xFFFF

# Fixed scale factors chosen for the demo site (500 kW battery, 1 Hz control).
# SF must let the value fit its wire type: 103/203 W int16 max 32767, so SF=2
# (100 W resolution) covers +/-3.27 MW. These are simulator constants; the
# adapter must still read SF registers and apply them dynamically.
FIXED_SF = {
    1: {},
    103: {"A_SF": 0, "V_SF": 0, "W_SF": 2, "Hz_SF": -2, "VA_SF": 0,
          "VAr_SF": 0, "PF_SF": -3, "WH_SF": 0, "DCA_SF": 0, "DCV_SF": 0,
          "DCW_SF": 0, "Tmp_SF": 0},
    124: {"WChaMax_SF": 1, "WChaDisChaGra_SF": 0, "VAChaMax_SF": 0,
          "MinRsvPct_SF": 0, "ChaState_SF": -2, "StorAval_SF": 0,
          "InBatV_SF": -1, "InOutWRte_SF": 0},
    203: {"A_SF": 0, "V_SF": 0, "Hz_SF": -2, "W_SF": 2, "VA_SF": 0,
          "VAR_SF": 0, "PF_SF": -3, "TotWh_SF": 0, "TotVAh_SF": 0,
          "TotVArh_SF": 0},
    704: {"PF_SF": -3, "WMaxLimPct_SF": -2, "WSet_SF": 0, "WSetPct_SF": -2,
          "VarSet_SF": 0, "VarSetPct_SF": -2},
}

# Type widths in 16-bit registers and the "not implemented" sentinel per
# SunSpec Information Model specification.
TYPE_SENTINEL = {
    "uint16": 0xFFFF, "enum16": 0xFFFF, "bitfield16": 0xFFFF,
    "int16": 0x8000, "sunssf": 0x8000,
    "uint32": 0xFFFFFFFF, "acc32": 0xFFFFFFFF, "bitfield32": 0xFFFFFFFF,
    "int32": 0x80000000, "acc64": 0xFFFFFFFFFFFFFFFF,
    "string": 0x0000, "pad": 0x0000,
}


def load_model(mid):
    return json.loads((UPSTREAM / f"model_{mid}.json").read_text())


def iter_points(group):
    """Yield (point, offset) for a group including nested groups, in order."""
    off = 0
    for p in group["points"]:
        yield p, off
        off += p.get("size", 1)
    for sg in group.get("groups", []):
        for p in sg["points"]:
            yield p, off
            off += p.get("size", 1)


def model_layout(mid, start_addr):
    """Return manifest entry for a model placed with its ID at start_addr."""
    d = load_model(mid)
    points = []
    for p, off in iter_points(d["group"]):
        name = p["name"]
        entry = {
            "name": name,
            "offset": off,
            "address": start_addr + off,
            "size": p.get("size", 1),
            "type": p["type"],
            "access": p.get("access", "R"),
            "mandatory": p.get("mandatory") in (True, "M", "m"),
            "sentinel": TYPE_SENTINEL.get(p["type"]),
        }
        if "sf" in p:
            entry["scale_factor_ref"] = p["sf"]
        if name.endswith("_SF") or p["type"] == "sunssf":
            if name in FIXED_SF.get(mid, {}):
                entry["fixed_value"] = FIXED_SF[mid][name]
        if name in ("ID", "L"):
            entry["fixed_value"] = mid if name == "ID" else None
        syms = {s["value"]: s["name"] for s in p.get("symbols", [])}
        if syms:
            entry["symbols"] = syms
        if p.get("comments"):
            entry["comments"] = p["comments"]
        points.append(entry)
    data_len = max(p["offset"] + p["size"] for p in points) - 2
    for p in points:
        if p["name"] == "L":
            p["fixed_value"] = data_len
    return {
        "id": mid,
        "start_address": start_addr,
        "declared_length_L": data_len,
        "total_registers": data_len + 2,
        "points": points,
    }


def chain(models, first_id_addr):
    """Place models contiguously starting with the ID register at first_id_addr."""
    out = []
    addr = first_id_addr
    for mid in models:
        m = model_layout(mid, addr)
        out.append(m)
        addr += m["total_registers"]
    return out, addr


def reg16(v):
    return f"{v & 0xFFFF:04X}"


def regs(*vals):
    return "".join(reg16(v) for v in vals)


def fc3_req(addr, count):
    return f"03 {addr:04X} {count:04X}"


def fc3_resp(*vals):
    return f"03 {2*len(vals):02X} " + " ".join(f"{v & 0xFFFF:04X}" for v in vals)


def fc16_req(addr, vals):
    data = " ".join(f"{v & 0xFFFF:04X}" for v in vals)
    return f"10 {addr:04X} {len(vals):04X} {2*len(vals):02X} {data}"


def fc16_resp(addr, count):
    return f"10 {addr:04X} {count:04X}"


def exception(fc, code):
    return f"{fc | 0x80:02X} {code:02X}"


def main():
    # --- layout ------------------------------------------------------------
    # base: SunS marker(2) -> model1 ID/L at base+2 -> chain -> end marker
    m1_start = ADDRESS_BASE + 2

    inv_models, inv_end = chain([1, 103, 124, 704], m1_start)
    met_models, met_end = chain([1, 203], m1_start)

    def header_addr(models, mid):
        for m in models:
            if m["id"] == mid:
                return m["start_address"]
        raise KeyError(mid)

    manifest = {
        "profile": "voltix-sunspec-v1",
        "description": "Restricted SunSpec profile for the Voltix simulated site. "
                       "Not universal model conformance; unsupported points serve "
                       "the type sentinel or a documented fixed value.",
        "upstream": {
            "repository": "github.com/sunspec/models",
            "revision": REVISION,
            "license": "Apache-2.0 (see upstream/LICENSE)",
            "files": [f"json/model_{m}.json" for m in (1, 103, 124, 203, 704)],
        },
        "address_base": ADDRESS_BASE,
        "suns_marker": {"address": ADDRESS_BASE, "value": [0x5375, 0x6E53]},
        "end_marker": {"id": END_ID, "length": 0},
        "endpoints": {
            "inverter": {
                "unit_id": 1, "default_port": 1502,
                "model_order": [1, 103, 124, 704],
                "end_marker_address": inv_end,
            },
            "meter": {
                "unit_id": 1, "default_port": 1503,
                "model_order": [1, 203],
                "end_marker_address": met_end,
            },
        },
        "models": {"inverter": inv_models, "meter": met_models},
        "sentinels": {k: f"0x{v:X}" for k, v in TYPE_SENTINEL.items()},
        "common_block": {
            "Mn": "Voltix", "Vr": "1.0.0", "DA": 0,
            "inverter": {"Md": "VoltixSim-Inverter", "SN": "VTX-SIM-INV-001", "Opt": ""},
            "meter": {"Md": "VoltixSim-Meter", "SN": "VTX-SIM-MET-001", "Opt": ""},
        },
        "scale_factors": {str(k): v for k, v in FIXED_SF.items()},
        "domain_conventions": {
            "battery_power_kw": "positive = discharging, negative = charging",
            "site_power_kw": "positive = exporting to grid (simulator-defined meter convention; the adapter normalises at its boundary)",
            "soc_pct": "ChaState x 10^-2 with ChaState_SF=-2",
        },
        "control_write": {
            "description": "One atomic FC16 request covering WSetEna..WSet "
                           "(4 registers) sets enable, mode and target together.",
            "function": 16,
            "start_address": header_addr(inv_models, 704) + 22,
            "registers": ["WSetEna", "WSetMod", "WSet.hi", "WSet.lo"],
            "values": {"WSetEna": "ENABLED(1)", "WSetMod": "WATTS(1)",
                       "WSet": "signed int32 watts, WSet_SF=0"},
            "readback": "read WSetEna/WSetMod/WSet after write to confirm",
            "watchdog": "only an accepted complete write of this window refreshes "
                        "the simulator's 10 s power-command deadline",
        },
        "disallowed": [
            "writes to read-only points -> Modbus exception 02",
            "partial write of the WSet window (WSet without Ena/Mod) -> rejected, no watchdog refresh",
            "points marked unsupported in this profile serve the type sentinel",
        ],
    }

    (HERE / "sunspec-profile.json").write_text(
        json.dumps(manifest, indent=2) + "\n")

    # --- fixtures -----------------------------------------------------------
    # Absolute addresses used below (from the layout above).
    inv103 = header_addr(inv_models, 103)
    inv124 = header_addr(inv_models, 124)
    inv704 = header_addr(inv_models, 704)
    met203 = header_addr(met_models, 203)
    A = {
        "sunS": ADDRESS_BASE,
        "inv_W": inv103 + 14, "inv_W_SF": inv103 + 15,
        "inv_Hz": inv103 + 16, "inv_St": inv103 + 38,
        "chaState": inv124 + 8, "chaState_SF": inv124 + 22,
        "wSet_window": inv704 + 22,
        "met_W": met203 + 18, "met_W_SF": met203 + 22,
    }

    def fx(name, desc, request, response, decode=None, notes=None):
        f = {"name": name, "description": desc,
             "unit_id": 1, "request_pdu_hex": request,
             "expected_response_pdu_hex": response}
        if decode:
            f["decode"] = decode
        if notes:
            f["notes"] = notes
        (FIXTURES / f"{name}.json").write_text(json.dumps(f, indent=2) + "\n")

    FIXTURES.mkdir(exist_ok=True)

    fx("discovery-suns-marker",
       "Read the SunSpec identifier at the base address.",
       fc3_req(A["sunS"], 2), fc3_resp(0x5375, 0x6E53),
       {"bytes": "0x53756E53", "ascii": "SunS"})

    fx("discovery-model-headers",
       "Walk model ID/length headers on the inverter endpoint.",
       fc3_req(m1_start, 2), fc3_resp(1, 66),
       {"ID": 1, "L": 66,
        "next_header": "103 at 40070; then 124 at 40122; 704 at 40148; "
                       "end marker 0xFFFF at 40215"})

    fx("inverter-discharge",
       "Inverter W register reading +250.0 kW (discharging). "
       "W int16 raw 2500 with W_SF=2 -> 2500 x 10^2 = 250000 W.",
       fc3_req(A["inv_W"], 2), fc3_resp(2500, 2),
       {"W_raw": 2500, "W_SF": 2, "watts": 250000, "kw": 250.0})

    fx("inverter-charge",
       "Inverter W register reading -250.0 kW (charging). "
       "raw -2500 -> int16 0xF63C; W_SF=2 -> -250000 W.",
       fc3_req(A["inv_W"], 2), fc3_resp(0xF63C, 2),
       {"W_raw": -2500, "W_SF": 2, "watts": -250000, "kw": -250.0})

    fx("soc-normal",
       "ChaState uint16 raw 5000 with ChaState_SF=-2 -> 50.00 %.",
       fc3_req(A["chaState"], 1), fc3_resp(5000),
       {"ChaState_raw": 5000, "ChaState_SF": -2, "soc_pct": 50.0},
       "ChaState_SF is read separately at its own address (fixed -2).")

    fx("soc-boundaries",
       "Operating limits: raw 500 -> 5.00 % (discharge floor) and "
       "raw 9500 -> 95.00 % (charge ceiling).",
       fc3_req(A["chaState"], 1), fc3_resp(500),
       {"ChaState_raw": 500, "ChaState_SF": -2, "soc_pct": 5.0},
       "Second case identical with response register 9500 -> 95.0 %.")

    # NB: in model 203 W (offset 18) is NOT adjacent to W_SF (offset 22);
    # WphA/B/C sit between. Read W alone; read W_SF at its own address.
    fx("meter-export",
       "Site meter W int16 raw 1800, W_SF=2 -> +180.0 kW export.",
       fc3_req(A["met_W"], 1), fc3_resp(1800),
       {"W_raw": 1800, "W_SF": 2, "watts": 180000, "kw": 180.0,
        "W_SF_address": A["met_W_SF"]})

    fx("meter-import",
       "Site meter W int16 raw -1200 (0xFB50), W_SF=2 -> -120.0 kW import.",
       fc3_req(A["met_W"], 1), fc3_resp(0xFB50),
       {"W_raw": -1200, "W_SF": 2, "watts": -120000, "kw": -120.0})

    fx("write-target-discharge",
       "Atomic FC16 write of the WSet window: enable + watts mode + "
       "int32 target +250000 W = 0x0003D090.",
       fc16_req(A["wSet_window"], [1, 1, 0x0003, 0xD090]),
       fc16_resp(A["wSet_window"], 4),
       {"WSetEna": "ENABLED(1)", "WSetMod": "WATTS(1)", "WSet": 250000,
        "kw": 250.0},
       "This is the only write pattern that refreshes the 10 s watchdog.")

    fx("write-target-charge",
       "Atomic FC16 write with int32 target -250000 W = 0xFFFC2F70 "
       "(negative charging power exercises the signed path).",
       fc16_req(A["wSet_window"], [1, 1, 0xFFFC, 0x2F70]),
       fc16_resp(A["wSet_window"], 4),
       {"WSetEna": "ENABLED(1)", "WSetMod": "WATTS(1)", "WSet": -250000,
        "kw": -250.0})

    fx("write-target-zero",
       "Atomic FC16 write commanding 0 W (fallback target).",
       fc16_req(A["wSet_window"], [1, 1, 0x0000, 0x0000]),
       fc16_resp(A["wSet_window"], 4),
       {"WSet": 0, "kw": 0.0})

    fx("sentinel-w",
       "Inverter W register returns int16 sentinel 0x8000 = not implemented; "
       "decoder must reject, not scale it.",
       fc3_req(A["inv_W"], 2), fc3_resp(0x8000, 2),
       {"W_raw": "0x8000", "decode": "INVALID"})

    fx("sentinel-chastate",
       "ChaState returns uint16 sentinel 0xFFFF = not implemented.",
       fc3_req(A["chaState"], 1), fc3_resp(0xFFFF),
       {"ChaState_raw": "0xFFFF", "decode": "INVALID"})

    fx("sentinel-sunssf",
       "W_SF returns sunssf sentinel 0x8000; any scaled value is unusable.",
       fc3_req(A["inv_W_SF"], 1), fc3_resp(0x8000),
       {"W_SF": "0x8000", "decode": "INVALID"})

    fx("error-write-readonly",
       "Write to read-only inverter status register St is rejected.",
       fc16_req(A["inv_St"], [4]), exception(0x10, 0x02),
       None,
       "Exception 02 ILLEGAL DATA ADDRESS for read-only/out-of-map writes.")

    fx("error-bad-address",
       "Read of an unmapped register inside a declared gap is rejected.",
       fc3_req(39999, 2), exception(0x03, 0x02))

    fx("error-partial-wset-write",
       "Writing WSet alone without the Ena/Mod window is rejected; "
       "it must not refresh the watchdog either.",
       fc16_req(A["wSet_window"] + 2, [0x0003, 0xD090]),
       exception(0x10, 0x03),
       None,
       "Simulator policy: only the full 4-register window is a valid "
       "power command.")

    # A small index so consumers can enumerate fixtures without a directory scan.
    index = sorted(p.name for p in FIXTURES.glob("*.json") if p.name != "index.json")
    (FIXTURES / "index.json").write_text(json.dumps(
        {"fixtures": index,
         "format": "PDU-level hex without MBAP (TCP) framing or RTU CRC",
         "byte_order": "big-endian per register; register order high->low word "
                       "for 32-bit points (SunSpec default)"},
        indent=2) + "\n")

    print(f"wrote manifest + {len(index)} fixtures")
    for name in index:
        print("  ", name)


if __name__ == "__main__":
    main()

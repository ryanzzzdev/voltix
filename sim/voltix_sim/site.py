"""Battery-site physics: energy, output ramping and the power-command watchdog.

Pure logic with an injected monotonic timestamp on every call, so unit tests
drive it with a fake clock and the wire server uses time.monotonic. No I/O.

Conventions (docs/project/features/device-profile.md):
- battery power positive = discharging, negative = charging
- site power positive = exporting to grid
- an accepted complete power command is the only watchdog refresh
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class SiteConfig:
    capacity_kwh: float = 1000.0
    initial_soc_pct: float = 50.0
    max_power_w: float = 500_000.0
    watchdog_s: float = 10.0
    track_rate_wps: float = 250_000.0
    fallback_rate_wps: float = 100_000.0
    baseline_w: float = 0.0


@dataclass
class Site:
    cfg: SiteConfig = field(default_factory=SiteConfig)
    now0: float = 0.0  # monotonic timestamp at construction

    def __post_init__(self) -> None:
        self.energy_kwh = self.cfg.capacity_kwh * self.cfg.initial_soc_pct / 100.0
        self.output_w = 0.0
        self.target_w = 0.0
        self.enabled = False
        self.last_accept: float | None = None
        self.fault = False
        self.online = True
        self.delay_s = 0.0
        self.corrupt = False  # test hook: serve sentinel values on key points
        self.baseline_w = self.cfg.baseline_w
        self.discharged_wh = 0.0
        self.exported_wh = 0.0
        self.imported_wh = 0.0
        self._ts = self.now0

    # -- command intake --------------------------------------------------

    def accept_power_command(self, now: float, ena: int, mod: int, watts: int) -> None:
        """Record an accepted atomic power command; the only watchdog refresh."""
        if mod != 1 or ena not in (0, 1):
            raise ValueError("unsupported WSetMod/WSetEna")
        self.last_accept = now
        self.enabled = ena == 1
        target = float(watts) if self.enabled else 0.0
        lim = self.cfg.max_power_w
        self.target_w = max(-lim, min(lim, target))

    # -- physics ----------------------------------------------------------

    def watchdog_expired(self, now: float) -> bool:
        return self.last_accept is not None and now - self.last_accept >= self.cfg.watchdog_s

    def advance(self, now: float) -> None:
        """Advance physics to monotonic `now`; safe to call sparsely.

        If the watchdog deadline falls inside the interval, the step is split
        there: tracking until expiry, then fallback — expiry is not retroactive.
        """
        while True:
            boundary = now
            if (not self.fault and self.enabled and self.last_accept is not None
                    and self._ts < (expiry := self.last_accept + self.cfg.watchdog_s) < now):
                boundary = expiry
            self._advance_to(boundary)
            if boundary == now:
                return

    def _advance_to(self, now: float) -> None:
        dt = now - self._ts
        if dt <= 0:
            return
        # The operating mode is fixed by the state at the segment start;
        # advance() splits at the watchdog deadline so it cannot change
        # mid-segment.
        expired = self.watchdog_expired(self._ts)
        self._ts = now

        if self.fault:
            # Emergency fault drives output to zero immediately.
            self.output_w = 0.0
        else:
            goal = self.target_w if (self.enabled and not expired) else 0.0
            rate = self.cfg.fallback_rate_wps if expired else self.cfg.track_rate_wps
            step = rate * dt
            delta = goal - self.output_w
            self.output_w += max(-step, min(step, delta))
            lim = self.cfg.max_power_w
            self.output_w = max(-lim, min(lim, self.output_w))

        e = self.energy_kwh - self.output_w * dt / 3.6e6
        if e <= 0.0 and self.output_w > 0:
            self.output_w, e = 0.0, 0.0
        elif e >= self.cfg.capacity_kwh and self.output_w < 0:
            self.output_w, e = 0.0, self.cfg.capacity_kwh
        self.energy_kwh = e

        self.discharged_wh += max(self.output_w, 0.0) * dt / 3600.0
        sw = self.site_w
        if sw > 0:
            self.exported_wh += sw * dt / 3600.0
        else:
            self.imported_wh += -sw * dt / 3600.0

    # -- views ------------------------------------------------------------

    @property
    def site_w(self) -> float:
        """Meter reading: battery output plus baseline net generation."""
        return self.output_w + self.baseline_w

    @property
    def soc_pct(self) -> float:
        return self.energy_kwh / self.cfg.capacity_kwh * 100.0

    def snapshot(self, now: float) -> dict:
        return {
            "soc_pct": round(self.soc_pct, 3),
            "output_kw": round(self.output_w / 1000.0, 3),
            "target_kw": round(self.target_w / 1000.0, 3),
            "site_kw": round(self.site_w / 1000.0, 3),
            "enabled": self.enabled,
            "fault": self.fault,
            "online": self.online,
            "watchdog_expired": self.watchdog_expired(now),
            "watchdog_remaining_s": (
                None if self.last_accept is None
                else round(max(0.0, self.cfg.watchdog_s - (now - self.last_accept)), 3)
            ),
            "baseline_kw": round(self.baseline_w / 1000.0, 3),
        }

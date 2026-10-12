"""Pure-physics tests for the site model; fake clock, no I/O."""

from voltix_sim.site import Site, SiteConfig


def make(**kw) -> Site:
    return Site(SiteConfig(**kw))


def test_discharge_reduces_energy_and_soc():
    s = make()
    s.accept_power_command(0.0, ena=1, mod=1, watts=250_000)
    s.advance(0.0)
    s.advance(1.0)  # track rate 250 kW/s reaches target in 1 s
    assert s.output_w == 250_000
    e0 = s.energy_kwh
    s.advance(2.0)
    assert s.energy_kwh < e0  # discharging drains the battery
    assert s.soc_pct < 50.0


def test_charge_negative_target_raises_energy():
    s = make()
    s.accept_power_command(0.0, ena=1, mod=1, watts=-250_000)
    s.advance(1.0)
    assert s.output_w == -250_000
    e0 = s.energy_kwh
    s.advance(2.0)
    assert s.energy_kwh > e0


def test_energy_bounded_and_output_stops_at_empty():
    s = make(capacity_kwh=10.0, initial_soc_pct=1.0, watchdog_s=5000.0)
    s.accept_power_command(0.0, ena=1, mod=1, watts=500_000)
    s.advance(1.0)   # 250 kW/s track rate -> 250 kW
    s.advance(3.0)   # reaches 500 kW and drains the remaining 0.03 kWh
    assert s.energy_kwh == 0.0
    assert s.output_w == 0.0  # cannot discharge an empty battery


def test_energy_bounded_at_full_charge():
    s = make(capacity_kwh=10.0, initial_soc_pct=99.0, watchdog_s=5000.0)
    s.accept_power_command(0.0, ena=1, mod=1, watts=-500_000)
    s.advance(1.0)
    s.advance(3.0)
    assert s.energy_kwh == 10.0
    assert s.output_w == 0.0


def test_watchdog_trip_ramps_to_zero_at_fallback_rate():
    s = make(watchdog_s=10.0, track_rate_wps=250_000, fallback_rate_wps=100_000)
    s.accept_power_command(0.0, ena=1, mod=1, watts=250_000)
    s.advance(1.0)
    assert s.output_w == 250_000
    # Expiry at t=10.0: between 1.0 and 10.5 the first 9 s are normal tracking,
    # then 0.5 s of fallback at 100 kW/s -> 250 - 50 = 200 kW.
    s.advance(10.5)
    assert s.output_w == 200_000
    s.advance(12.5)
    assert s.output_w == 0.0


def test_watchdog_only_refreshed_by_accepted_command():
    s = make(watchdog_s=10.0)
    s.accept_power_command(0.0, ena=1, mod=1, watts=100_000)
    s.advance(9.0)
    assert not s.watchdog_expired(9.0)
    s.advance(9.5)
    assert not s.watchdog_expired(9.5)  # advancing/polling is not a refresh
    s.accept_power_command(9.5, ena=1, mod=1, watts=100_000)
    s.advance(19.4)
    assert not s.watchdog_expired(19.4)  # refresh at 9.5 holds until 19.5
    s.advance(19.6)
    assert s.watchdog_expired(19.6)


def test_fault_forces_immediate_zero():
    s = make()
    s.accept_power_command(0.0, ena=1, mod=1, watts=250_000)
    s.advance(1.0)
    s.fault = True
    s.advance(1.1)
    assert s.output_w == 0.0


def test_disable_command_ramps_at_track_rate():
    s = make(track_rate_wps=250_000)
    s.accept_power_command(0.0, ena=1, mod=1, watts=250_000)
    s.advance(1.0)
    s.accept_power_command(1.0, ena=0, mod=1, watts=0)
    s.advance(1.5)
    assert s.output_w == 125_000  # halfway down at track rate, not fallback


def test_invalid_mode_rejected():
    s = make()
    try:
        s.accept_power_command(0.0, ena=1, mod=0, watts=100)
    except ValueError:
        pass
    else:
        raise AssertionError("mod=0 must be rejected")
    assert s.last_accept is None


def test_meter_site_power_includes_baseline():
    s = make()
    s.baseline_w = -80_000  # net load
    s.accept_power_command(0.0, ena=1, mod=1, watts=250_000)
    s.advance(1.0)
    assert s.site_w == 170_000

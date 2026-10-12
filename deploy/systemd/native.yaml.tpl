# Template for the native-test controller configuration. install.sh
# replaces @STATE_DIR@ with the systemd StateDirectory path.
# Ports are the native harness range (2512-2514 sim, 8190 dispatch,
# 8191 telemetry mock) — distinct from the Compose demo bindings.

controller_id: voltix-native

devices:
  inverter:
    host: 127.0.0.1
    port: 2512
  meter:
    host: 127.0.0.1
    port: 2513

limits:
  soc_min_pct: 5
  soc_max_pct: 95
  site_export_max_kw: 400
  charge_max_kw: 500
  discharge_max_kw: 500
  normal_slew_kw_per_s: 50

timing:
  cycle_period_ms: 1000
  read_deadline_ms: 200
  max_input_age_s: 2
  max_loop_gap_s: 2.5

telemetry:
  state_dir: @STATE_DIR@
  max_status_events: 90000
  max_alarm_events: 10000
  max_payload_bytes: 1024
  handoff_status_slots: 900
  handoff_alarm_slots: 100

upstream:
  dispatch_listen: 127.0.0.1:8190
  telemetry_url: http://127.0.0.1:8191/api/telemetry

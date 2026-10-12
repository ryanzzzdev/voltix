# Supervisory control and dispatch contract

Date: 2026-10-12
Status: Implementation baseline; defaults require measured verification, not claimed real-time guarantees.

## Single-controller scope

One loop owns active dispatch, device writes and operating state. User intent is a battery power target; grid limiting may reduce it. The upstream mock is a single dispatcher. No optimisation, priority arbitration between multiple dispatchers, fleet scheduling, or autonomous restoration of old commands is included.

## Dispatch API and freshness

Local API: POST /api/dispatch with a configured bearer token. An authenticated status endpoint exposes the current boot_session_id, generated randomly for each process start. Requests contain controller_id, boot_session_id, command_id, revision, issued_at, valid_until, and target_kw.

- Require finite target, correct identity/session, positive increasing revision, UUID command ID, and UTC timestamps. Reject bodies above 4 KiB and malformed/unknown fields.
- Require issued_at <= receive_time < valid_until and a positive issued-to-expiry duration no greater than 30 seconds. Demo processes share a host clock; cross-host clock synchronisation is outside v1. Do not reset valid_until to receipt time plus 30 seconds.
- Convert remaining lifetime at acceptance to a monotonic deadline. A backward wall-clock step cannot extend that deadline; check absolute expiry too so a forward jump expires conservatively.
- The exact latest accepted command can be retried idempotently while valid; its deadline is unchanged. The same revision/ID with changed fields is a conflict. Older revisions are rejected, including after the active command expires. Revision state lasts for the boot session; restart changes the session and rejects old requests.
- Return 202 for accepted dispatch with command ID and expiry; acceptance is not proof of a device write. Use 400 for malformed payload, 401 for missing/invalid token, 409 for session/revision conflicts, and 422 for expired or semantically invalid values. Status distinguishes requested, limited, acknowledged and measured output.
- A heartbeat does not extend a command. The dispatcher refreshes intent by sending a new revision with new timestamps; tests stop that stream to exercise expiry.

Bind to loopback by default. Secrets are not checked into example config or logged. The isolated demo uses shared-token authentication, not digital signatures. Public TLS deployment is deferred.

## Operating states

| State | Condition and action |
| --- | --- |
| STANDBY | No fresh dispatch; read device state and request fallback zero when communication permits |
| ACTIVE | Fresh command and valid current measurements; apply constrained target |
| DEGRADED | Expired command, stale input, timing gap or communication issue; invalidate active intent and request zero if possible |
| FAULT | Device-reported fault or invalid device identity/configuration; inhibit nonzero commands and report reason |

Keep telemetry-store/upload health separate: loss of the backend alone does not invalidate otherwise valid control intent. Configuration errors fail startup. Record reason codes rather than creating a state for every combination.

Startup always creates a new boot session and starts STANDBY. Read fresh inverter state before issuing writes; never refresh a persisted nonzero target. Request zero, using measured power as the ramp baseline, until a valid new-session dispatch is available. A valid command plus a complete healthy cycle can restore ACTIVE after a transient issue. A device fault requires clear device status and a newly accepted dispatch. New requests may be accepted during degraded conditions but do not authorise output until required inputs are valid.

## Freshness and I/O

Require successful valid inverter and meter readings in the current cycle; also reject snapshots older than 2 seconds measured from monotonic receive times. A successful Modbus read alone cannot prove a real device sensor is fresh; the simulator profile and measured freshness claim are limited to communication and documented device status.

Read each endpoint serially within its owner; read different endpoints concurrently. Initial read-phase deadline: 200 ms total. Write-phase deadline: 100 ms, including command transaction sequence. Record poll duration, write duration, tick-start delay and total cycle duration. Target cycle work budget: 350 ms. These are engineering defaults to test; adjust with evidence if the validated library/profile cannot satisfy them. No synchronous reconnect/retry loop may run past its deadline.

A timed-out write may have been applied: mark its acknowledgement uncertain, obtain fresh state, and do not blindly replay it after a newer dispatch. On recovery re-establish device identity/profile before further commands.

## Limits and priority

First validate sensor values, device status, command freshness and configuration. A failed required check causes fallback/inhibition; never replace an invalid reading with zero and continue normal control.

For healthy inputs, form an allowable interval from charge/discharge bounds and SoC: prohibit discharge at/below 5%, prohibit charge at/above 95%. Apply the site-export upper bound using baseline = measured_site_export - measured_battery_power, so target <= export_limit - baseline. Clamp the requested power to the feasible interval, then apply normal slew based on elapsed time. Reapply hard operating bounds after slew: newly tightened hard bounds override the normal ramp preference and must emit a reason. Test this precedence explicitly.

If the interval is empty (for example solar alone exceeds export capacity while charging is unavailable), choose zero as the demo fallback and report the remaining site-limit violation as infeasible. Voltix cannot control unmodelled generation. A 1 Hz sampled policy cannot guarantee no transient site-limit exceedance between readings.

Normal slew is 50 kW/s, based on the previous acknowledged target (fresh measured output at startup/recovery). For a gap over 2.5 seconds, invalidate active dispatch, revalidate inputs and enter fallback; do not multiply a stale target by the full elapsed gap. During this recovery use at most one second of ramp allowance per cycle.

## Expiry, shutdown and device timeout

Command expiry invalidates intent and begins fallback within one control period plus the 350 ms work budget in the declared test environment. Reaching zero takes additional ramp time; assert both separately. If communication is unavailable the controller cannot enact zero: report that fact, and observe the simulator's independent timeout separately.

On graceful shutdown attempt bounded zero/fallback and stop workers; on kill -9 no handler is assumed. The simulator refreshes its timeout only on accepted power commands. Do not confuse controller-side lease expiry with device-side communications expiry.

## Control independence

Emit immutable snapshots to a bounded telemetry handoff without blocking the loop. Track rejected snapshots and store health separately. No database commit or upstream HTTP call runs in the control loop. Unit tests use an injected clock; integration tests measure real scheduling under a declared workload.

Tasks: [VTX-008](../backlog.md#vtx-008) through [VTX-011](../backlog.md#vtx-011). Device semantics: [device profile](device-profile.md).

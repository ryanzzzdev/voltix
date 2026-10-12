# 002 — Scope and delivery of the simulated site controller

Date: 2026-10-12
Status: Accepted as the implementation planning baseline; not implemented.

## Context

The user selected Voltix and a single repository, reviewed a Go site-controller proposal, and requested detailed roadmap, architecture, and initial tasks. The original discussions include optional extensions and claims that later reviews narrowed.

## Decision

Deliver one simulated battery site through four milestones: device integration, supervisory control, durable telemetry, and Linux demonstration/verification. Use one Go module for the controller and upstream mock, Python/pymodbus for the simulator, HTTP/JSON upstream, Modbus/SunSpec downstream, SQLite locally, and native systemd plus a distinct Compose demo.

Select a specific small simulator profile rather than broad vendor compatibility. Put behavioural contracts in canonical feature documents. Preserve initial discussions as historical background. Build the first vertical slice before UI, additional languages, optimisation, or redundancy.

## Alternatives

Multiple repositories would add coordination without independent release needs. Two controller languages and gRPC add scope before measurement establishes a benefit. Public hosting and physical hardware are optional future demonstrations, not requirements for a reproducible local portfolio.

## Consequences

Initial tasks are detailed through the control and telemetry work; final packaging tasks can be refined against the resulting system. No committed timeline, certified safety claim, manufacturer interoperability claim, or automatic publication. Native systemd evidence cannot be substituted by container restart tests.

## Acceptance source

The user selected Voltix and accepted the single-repository direction in this conversation, then requested detailed planning on 2026-10-12. Component organisation and sequencing are selected under that delegated planning scope, incorporating the reviewed simplifications; runtime validation remains outstanding.

## References

- [Roadmap](../roadmap.md)
- [Architecture](../architecture.md)
- [VTX-002](../backlog.md#vtx-002)
- [Historical discussions](../../initial-discussion/)

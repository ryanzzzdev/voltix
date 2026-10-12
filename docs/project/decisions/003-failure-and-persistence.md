# 003 — Explicit command and telemetry failure boundaries

Date: 2026-10-12
Status: Accepted as implementation policy under delegated planning; not implemented or verified.

## Context

The reviewed proposal left replayed command lifetimes ambiguous, allowed an unlimited alarm tier, and paired persistent sequence deduplication with potentially lost commits after power failure. These contracts must be precise before implementation.

## Decision

Commands target a per-process boot session and carry an increasing revision and bounded absolute expiry. Replays do not renew expiry; restart invalidates old-session commands. The simulator has its own power-refresh timeout independent of controller liveness.

Use SQLite WAL with synchronous FULL for the controller event store and mock receiver. This deliberately replaces the historical NORMAL proposal: at this workload, a stronger commit boundary is preferable to an unmeasured throughput optimisation. Keep a persistent stream identity and monotonic sequence allocation; acknowledge receiver commits only after successful transaction completion. FULL still relies on the storage stack honouring synchronisation; process crash tests do not prove physical power-loss behaviour. See [SQLite durability documentation](https://www.sqlite.org/pragma.html#pragma_synchronous).

Both ordinary telemetry and alarm events have finite retention. Upload delivery is at least once only for committed records still retained, assuming eventual receiver availability. Evictions and pre-persistence handoff drops are explicit losses. Exact loss accounting cannot be durably promised while storage itself is unavailable.

## Alternatives

NORMAL remains a potential measured optimisation only with an explicitly revised durability/identity contract. An unevictable alarm tier violates bounded storage. Global exactly-once messaging, signed-command infrastructure, consensus, and an external broker are unnecessary for the single-controller isolated demo.

## Consequences

Benchmark commit costs and use a bounded writer handoff. Test restart identity, lost acknowledgements, bounded alarm floods and disk-full failures. Distinguish transport acceptance, committed telemetry and measured equipment output. Protocol-specific fallback and operating constraints remain documented simulation policies.

## Acceptance source

Selected under the user's 2026-10-12 request to plan project details, resolving the contradictions identified in prior reviews. This is an engineering planning choice, not a user claim about real equipment requirements.

## References

- [Control contract](../features/control.md)
- [Telemetry contract](../features/telemetry.md)
- [VTX-002](../backlog.md#vtx-002)

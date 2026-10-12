# 001 — Markdown project tracking

Date: 2026-10-12
Status: Accepted

## Context

Voltix needs durable project memory for agent-assisted engineering before project planning and implementation begin. The user requested the same project-management standards used by sibling NEMLab.

## Decision

Keep canonical project records in Voltix's `docs/project/`, with the repository README linking to them and root `AGENTS.md` activating the workflow. Use the project-management skill when available and the documentation guide as its source of truth.

Keep task bodies and authoritative status in `backlog.md`, milestone progress in `roadmap.md`, design in architecture/specifications, reasoning in numbered decisions, obstacles in `blockers.md`, and work history in monthly UTC journals. Use sequential `VTX` task IDs. Every journal entry identifies the LLM model and coding agent separately.

Preserve `docs/initial-discussion/` as background. Adoption of this workflow does not approve every technical proposal contained there.

## Alternatives

- External issue tracker: unnecessary for this initial setup; may replace Markdown task ownership later.
- Several independent trackers or copied plans: rejected because status and requirements would drift.
- Copying NEMLab's product milestones and technical setup: inappropriate for Voltix; reuse its workflow only.

## Consequences and follow-up

Agents update affected records after meaningful work without a separate documentation request. Keep the workflow proportional to each task and avoid duplicate journals. Implementation planning remains the next user-directed step. This decision does not initialise Git or authorise commits/publication.

## Acceptance source

The user requested NEMLab's project-management standards and corresponding files in Voltix on 2026-10-12, explicitly placing planning after this setup. Voltix-specific paths and the `VTX` prefix are routine adaptations under that request.

## References

- [Documentation guide](../documentation-guide.md)
- [VTX-001](../backlog.md#vtx-001)

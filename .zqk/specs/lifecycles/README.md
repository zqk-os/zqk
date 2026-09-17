## Lifecycle Specification Guide

Every Workstream OS object that participates in a lifecycle must define its
state machine in a YAML file inside this directory.  The goal is to make the
rules machine-readable so that CLI commands, validators, and status-check tools
can rely on the exact same definition.

Each lifecycle file should answer these questions:

**Canonical exam:** [docs/architecture/LIFECYCLE_STATE_MACHINE_RUBRIC.md](../../architecture/LIFECYCLE_STATE_MACHINE_RUBRIC.md) — enumerate \(n(n-1)\) edges, prune to valid; class roles vs kind tokens; catalyst / preconditions / postconditions / shockwave / similarity. Start with **policy** membranes even when policy occupancy is thin.

**Shockwave catalog (do not clone PRI):** [LIFECYCLE_SHOCKWAVE_MAP.md](../../architecture/LIFECYCLE_SHOCKWAVE_MAP.md) → [family](../../architecture/lifecycle_shockwave/README.md) → [kind index](../../architecture/lifecycle_shockwave/KIND_INDEX.md). PRI filled exam: [kind_priority_plan.md](../../architecture/lifecycle_shockwave/kind_priority_plan.md).

1. **What are the valid statuses?**  
   Include display labels, `role` (class), whether the status is origin, terminal
   (ending), archive, or system-only/error. For each: catalyst, hold preconditions,
   postconditions of hops that enter it.
2. **What transitions are allowed?**  
   Prune from the complete directed graph. For every kept edge: origin, destination,
   manual and/or automatic, qualifying `preconditions`, hold-after-hop `postconditions`,
   shockwave (`on_dependent_status` / side_effects).
3. **What preconditions exist for originating the object?**  
   If an object cannot be created unless certain references exist, capture it in
   the lifecycle file or its `metadata` section.
4. **What is the initial / origin status?**  
   Explicitly mark one status with `origin: true` (and `initial: true` where used).
5. **What are the ending statuses?**  
   Mark any terminal states with `terminal: true`.
6. **What status represents archival?**  
   Either set `archive: true` on a status or document it inside the metadata
   block.  Many lifecycles allow `* → archived`.
7. **What status represents an error / halt?**  
   System `error` (`system: true`) vs kind `halted` (`paused`/`blocked`). Halt resume
   must not launder a check valve (see rubric). Not every kind should copy
   priority_plan's `halted ↛ shovel_ready` rule.

### YAML Structure

```
object_type: backlog_item
metadata:
  description: >
    High-level explanation of the lifecycle and any originating preconditions.
  originating_preconditions:
    - "Link to at least one goal (optional)"
  archive_status: archived
  error_status: error

statuses:
  - value: exploring
    display: "Exploring"
    initial: true
  - value: validated
    display: "Validated"
  - value: roadmap
    display: "Roadmap"
  - value: in_progress
    display: "In Progress"
  - value: complete
    display: "Complete"
    terminal: true
  - value: archived
    display: "Archived"
    archive: true
    terminal: true
  - value: error
    display: "Error"
    system: true

transitions:
  - from: exploring
    to: validated
    manual: true
    description: "Idea reviewed and ready for deeper planning"
  - from: roadmap
    to: in_progress
    manual: true
    preconditions:
      - "At least one milestone_ref"
      - "Owner assigned"
```

Additional sections (e.g., `percent_complete`, `auto_transition_rules`) are
optional but encouraged.

### Error Handling Pattern

When the CLI detects an illegal transition or a lifecycle incoherency it should:

1. Move the object to the lifecycle’s `error_status`.
2. Emit a structured error using a shared template  
   (e.g., `LIFECYCLE_INVALID_TRANSITION`, `LIFECYCLE_STATE_UNKNOWN`).
3. Surface the error differently depending on context:
   - **View mode**: Detailed explanation plus remediation guidance.
   - **List mode**: Concise summary with the error code.
4. Increment telemetry counters per error code so we can track trends and feed
   them into `status-check` or future health dashboards.

Capturing lifecycles in a single, consistent format unlocks:

- Uniform validation across CLI, MCP server, and automation agents.
- `status-check` commands for every object type with shared heuristics.
- Future top-level `workstream-os status-check` runs that aggregate findings.

When authoring a new lifecycle file, copy the template above and fill in the
object-specific details.  Keep the information in sync with the documentation in
`docs/architecture/LIFECYCLE_DEFINITIONS_EXPLAINED.md`.


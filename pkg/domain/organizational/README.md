# Organizational Domain

Organizational structure and change impact analysis for ZQK.

## Purpose

- **Impact analysis**: Given an organizational change (e.g. division restructure), find ZQK kernel objects that reference the affected organizational entities (divisions, teams).
- **Propagation**: Record that propagation was applied to those kernel objects (e.g. set `last_propagated_change_ref`).

## Integration with ZQK Kernel Objects (BLI-743)

Organizational change impact and propagation are integrated with the following **kernel object kinds**:

| Kind          | Reference fields used for impact | Propagation field          |
|---------------|-----------------------------------|----------------------------|
| workstream    | `division_ref`, `team_ref`        | `last_propagated_change_ref` |
| goal          | `division_ref`, `team_ref`        | `last_propagated_change_ref` |
| backlog_item  | `division_ref`, `team_ref`        | `last_propagated_change_ref` |
| milestone     | `division_ref`, `team_ref`        | `last_propagated_change_ref` |

- **Finding affected objects**: `reference_traversal.findAffectedNexosObjects` lists objects of each kind that reference any of the organizational IDs from the change’s `affected_objects` (or that reference divisions/teams implied by the change). Matching is done by inspecting `division_ref` and `team_ref` (single string or list).
- **Propagation**: The propagate command updates each affected object with `last_propagated_change_ref` set to the organizational change ID, so downstream tools can see which change was last applied.

## Components

- **ImpactAnalyzer** (`impact_analyzer.go`): Reads an `organizational_change`, finds affected kernel objects, creates an `impact_analysis` object.
- **reference_traversal.go**: Finds workstreams, goals, backlog_items, and milestones that reference given organizational object IDs.

## CLI

- `zqk organizational analyze-impact --change <id>`: Create impact_analysis for a change.
- `zqk organizational propagate --change <id> [--confirm] [--dry-run]`: Apply propagation to affected kernel objects.

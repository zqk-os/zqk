# Gantt SVG interaction contract (minimal)

**Last Verified:** 2026-08-31


**Backlog:** BLI-214 — interactive Gantt (zoom, filter, drill-down).

**Status:** Contract and tests are in-tree; full renderer integration remains aligned with `docs/marketing/strategic-pivot/SVG_GANTT_WORK_PRESERVATION.md` (graph-backend pivot).

## Required hooks

Rendered SVG (or the minimal example in `pkg/gantt/svg_contract.go`) MUST include:

- Root: `data-gantt-root="1"`.
- Lanes: `class="gantt-lane"` and `data-lane-id`.
- Task bars: `class="gantt-task-bar"`, `data-task-id`, `data-gantt-start`, `data-gantt-end`.
- Milestones: `class="gantt-milestone"`, `data-milestone-id`, `data-gantt-at` on a child shape.

Consumers may rely on these attributes for CSS/JS filtering without inferring geometry from untagged paths.

## Tests

`pkg/gantt/svg_contract_test.go` asserts presence of the fragments above.

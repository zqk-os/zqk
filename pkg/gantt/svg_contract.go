package gantt

// MinimalInteractionSVG returns a deterministic SVG fragment that satisfies the interaction contract
// for BLI-214 (stable hooks for zoom/filter). Full rendering remains deferred per strategic pivot;
// consumers and tests use this shape as the canonical minimal example.
func MinimalInteractionSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" data-gantt-root="1" role="img" aria-label="gantt">
  <g class="gantt-lane" data-lane-id="lane-a">
    <rect class="gantt-task-bar" data-task-id="task-1" data-gantt-start="2026-01-01" data-gantt-end="2026-01-02" x="0" y="0" width="40" height="12"/>
  </g>
  <g class="gantt-milestone" data-milestone-id="ms-1">
    <polygon data-gantt-at="2026-01-03" points="60,0 70,12 50,12"/>
  </g>
</svg>`
}

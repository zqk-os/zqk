package gantt

import (
	"strings"
	"testing"
)

func TestMinimalInteractionSVG_HasContractHooks(t *testing.T) {
	t.Parallel()
	s := MinimalInteractionSVG()
	for _, sub := range []string{
		`data-gantt-root="1"`,
		`class="gantt-lane"`,
		`data-lane-id="lane-a"`,
		`class="gantt-task-bar"`,
		`data-task-id="task-1"`,
		`data-gantt-start="2026-01-01"`,
		`data-gantt-end="2026-01-02"`,
		`class="gantt-milestone"`,
		`data-milestone-id="ms-1"`,
		`data-gantt-at="2026-01-03"`,
	} {
		if !strings.Contains(s, sub) {
			t.Fatalf("missing contract fragment %q in SVG", sub)
		}
	}
}

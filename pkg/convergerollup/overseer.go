package convergerollup

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// CVSTreeNode is one convergence_session in the coordinator's descendant tree (BFS, depth-limited).
type CVSTreeNode struct {
	ID           string `json:"id"`
	Depth        int    `json:"depth"`
	Status       string `json:"status"`
	CurrentPhase string `json:"current_phase,omitempty"`
}

// ActiveChildIDsUnderCoordinator returns CVS ids in tree with depth > 0 and status active.
func ActiveChildIDsUnderCoordinator(tree []CVSTreeNode) []string {
	var out []string
	for _, n := range tree {
		if n.Depth == 0 {
			continue
		}
		if n.Status == objects.ObjectStatusActive {
			out = append(out, n.ID)
		}
	}
	return out
}

// BuildArbitratedParentMessage merges rollup_core (from the same measure as convergence JSON) with a
// coordinator descendant tree so parent-exit suggestions stay gated on child completion.
// rollup map keys match buildRollupStatusCore output: recommended_next_action, ready_for_parent_completion, rollup_status.
func BuildArbitratedParentMessage(rollup map[string]any, tree []CVSTreeNode, coordinatorID string) string {
	if rollup == nil {
		rollup = map[string]any{}
	}
	rec, _ := rollup["recommended_next_action"].(string)
	rec = strings.TrimSpace(rec)
	ready, _ := rollup["ready_for_parent_completion"].(bool)
	active := ActiveChildIDsUnderCoordinator(tree)

	var b strings.Builder
	b.WriteString("**Overseer (coordinator ")
	b.WriteString(coordinatorID)
	b.WriteString("):** ")
	if len(active) > 0 && !ready {
		b.WriteString(fmt.Sprintf(
			"%d active child session(s) in tree under related_object_refs: %s. "+
				"Do not treat parent as ready to complete until children are completed or archived (or scope is explicitly revised). ",
			len(active),
			strings.Join(active, ", "),
		))
	}
	if rec != "" {
		b.WriteString("Measured rollup directive: ")
		b.WriteString(rec)
	} else {
		b.WriteString("Run zqk scheduler convergence measure --format json --session-id ")
		b.WriteString(coordinatorID)
		b.WriteString(" for recommended_next_action.")
	}
	return b.String()
}

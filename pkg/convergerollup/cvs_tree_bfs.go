package convergerollup

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// DefaultOverseerCVSTreeMaxDepth caps BFS expansion from the coordinator (nested CVS;
// docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md Appendix C).
const DefaultOverseerCVSTreeMaxDepth = 3

// CollectCVSTreeBFS returns root and descendants reachable via child CVS ids, BFS order, depth-limited.
// Nodes at depth == maxDepth are included; their children are not enqueued.
//
// nodeFor loads each visited id once: return child CVS-* ids (refs), lifecycle status, and current_phase.
// Only refs with the "CVS-" prefix are enqueued; other entries are ignored.
func CollectCVSTreeBFS(
	rootID string,
	maxDepth int,
	nodeFor func(id string) (refs []string, status string, phase string, err error),
) ([]CVSTreeNode, error) {
	if rootID == "" || nodeFor == nil {
		return nil, errfmt.Errorf("convergerollup: CollectCVSTreeBFS: root and nodeFor required")
	}
	if maxDepth < 0 {
		return nil, errfmt.Errorf("convergerollup: CollectCVSTreeBFS: maxDepth must be >= 0")
	}

	type qn struct {
		id    string
		depth int
	}
	queue := []qn{{rootID, 0}}
	seen := make(map[string]bool)
	var out []CVSTreeNode

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur.id] {
			continue
		}
		seen[cur.id] = true

		refs, st, phase, err := nodeFor(cur.id)
		if err != nil {
			return nil, errfmt.Errorf("nodeFor %s: %w", cur.id, err)
		}
		out = append(out, CVSTreeNode{
			ID:           cur.id,
			Depth:        cur.depth,
			Status:       st,
			CurrentPhase: phase,
		})
		if cur.depth >= maxDepth {
			continue
		}
		for _, cid := range refs {
			if cid == "" || !strings.HasPrefix(cid, "CVS-") {
				continue
			}
			if seen[cid] {
				continue
			}
			queue = append(queue, qn{cid, cur.depth + 1})
		}
	}
	return out, nil
}

package convergerollup

import (
	"slices"
	"strings"
)

// MaxRelatedCVSHopDepth is the default maximum hop count for DetectCVSRefCycle when walking
// parent→child related_object_refs (cycle detection). Aligns with cmd/zqk/scheduler rollup
// and overseer paths; doc: CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md Appendix C (max depth).
const MaxRelatedCVSHopDepth = 8

// DetectCVSRefCycle walks related_object_refs from root following only CONV-* ids.
// If following an edge would revisit an id already on the current path, it returns that cycle
// (path from root through the repeated node, ending with the closing edge target).
// maxDepth is the maximum hop count from root (root at depth 0); edges beyond that are not expanded.
func DetectCVSRefCycle(root string, refsFor func(id string) ([]string, error), maxDepth int) (cycle []string, ok bool) {
	if root == "" || refsFor == nil || maxDepth < 0 {
		return nil, false
	}
	var dfs func(path []string, id string, depth int) ([]string, bool)
	dfs = func(path []string, id string, depth int) ([]string, bool) {
		if slices.Contains(path, id) {
			out := append(slices.Clone(path), id)
			return out, true
		}
		newPath := append(slices.Clone(path), id)
		if depth >= maxDepth {
			return nil, false
		}
		refs, err := refsFor(id)
		if err != nil {
			return nil, false
		}
		for _, r := range refs {
			if r == "" || !strings.HasPrefix(r, "CONV-") {
				continue
			}
			if c, found := dfs(newPath, r, depth+1); found {
				return c, true
			}
		}
		return nil, false
	}
	return dfs(nil, root, 0)
}

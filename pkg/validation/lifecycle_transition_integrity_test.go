package validation

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// duplicateEdges partitions repeated from->to transitions into those whose preconditions differ
// and those that are exact repeats.
//
// A transition is looked up by its endpoints, so the first match answers and any other
// preconditions for that edge are never consulted. When the sets differ, the stricter one is
// unreachable and the barrier it describes does not exist — the same silent fail-open as a
// precondition no rule recognizes, but harder to see because both sentences are recognized.
// Exact repeats are untidy and harmless.
func duplicateEdges(lc objects.Lifecycle) (divergent []string, redundant int) {
	sets := map[string]map[string]bool{}
	for _, tr := range lc.Transitions {
		pre := append([]string(nil), tr.Preconditions...)
		sort.Strings(pre)
		edge := tr.From + "->" + tr.To
		if sets[edge] == nil {
			sets[edge] = map[string]bool{}
		}
		sets[edge][strings.Join(pre, "\x00")] = true
	}
	counts := map[string]int{}
	for _, tr := range lc.Transitions {
		counts[tr.From+"->"+tr.To]++
	}
	for edge, n := range counts {
		switch {
		case n < 2:
		case len(sets[edge]) > 1:
			divergent = append(divergent, fmt.Sprintf("%s (%d variants)", edge, len(sets[edge])))
		default:
			redundant++
		}
	}
	sort.Strings(divergent)
	return divergent, redundant
}

func TestLifecycleTransitions_noDivergentDuplicates(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	var scanned, redundant int
	err := filepath.WalkDir(dir, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		raw, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", d.Name(), err)
		}
		var lc objects.Lifecycle
		if err := yaml.Unmarshal(raw, &lc); err != nil {
			t.Fatalf("parse %s: %v", d.Name(), err)
		}
		scanned++
		divergent, red := duplicateEdges(lc)
		redundant += red
		for _, edge := range divergent {
			t.Errorf("%s declares %s with differing preconditions, so only the first is ever "+
				"consulted and the rest gate nothing", d.Name(), edge)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk lifecycles dir: %v", err)
	}
	if scanned == 0 {
		t.Fatal("no lifecycles scanned; this guard would be vacuous")
	}
	t.Logf("scanned %d lifecycles; %d redundant duplicate transitions (identical preconditions, harmless)", scanned, redundant)
}

// TestDuplicateEdges_detectsDivergence is the mutation test for the guard above. The shipped
// lifecycles have no divergent duplicates, so without synthetic input a broken detector would
// pass and the guard would be decorative.
func TestDuplicateEdges_detectsDivergence(t *testing.T) {
	t.Parallel()
	edge := func(from, to string, pre ...string) objects.Transition {
		return objects.Transition{From: from, To: to, Preconditions: pre}
	}
	for _, tc := range []struct {
		name          string
		transitions   []objects.Transition
		wantDivergent int
		wantRedundant int
	}{
		{"unique edges are clean", []objects.Transition{
			edge("a", "b", "x"), edge("b", "c", "y"),
		}, 0, 0},
		{"exact repeat is redundant not divergent", []objects.Transition{
			edge("a", "b", "x"), edge("a", "b", "x"),
		}, 0, 1},
		{"repeat with extra precondition is divergent", []objects.Transition{
			edge("a", "b", "x"), edge("a", "b", "x", "y"),
		}, 1, 0},
		{"repeat where one has none is divergent", []objects.Transition{
			edge("a", "b"), edge("a", "b", "x"),
		}, 1, 0},
		{"precondition order does not make a duplicate divergent", []objects.Transition{
			edge("a", "b", "x", "y"), edge("a", "b", "y", "x"),
		}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			divergent, redundant := duplicateEdges(objects.Lifecycle{Transitions: tc.transitions})
			if len(divergent) != tc.wantDivergent {
				t.Errorf("divergent: got %v (%d), want %d", divergent, len(divergent), tc.wantDivergent)
			}
			if redundant != tc.wantRedundant {
				t.Errorf("redundant: got %d, want %d", redundant, tc.wantRedundant)
			}
		})
	}
}

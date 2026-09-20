package convergerollup

import (
	"testing"
)

func TestCollectCVSTreeBFS_respectsMaxDepth(t *testing.T) {
	refs := map[string][]string{
		"CVS-a": {"CVS-b"},
		"CVS-b": {"CVS-c"},
		"CVS-c": {"CVS-d"},
		"CVS-d": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "active", "c1", nil
	}
	tree, err := CollectCVSTreeBFS("CVS-a", 2, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Fatalf("want 3 nodes (a,b,c), got %d: %#v", len(tree), tree)
	}
	if tree[0].ID != "CVS-a" || tree[0].Depth != 0 {
		t.Fatalf("root: %#v", tree[0])
	}
	if tree[2].ID != "CVS-c" || tree[2].Depth != 2 {
		t.Fatalf("deepest: %#v", tree[2])
	}
	for _, n := range tree {
		if n.ID == "CVS-d" {
			t.Fatal("CVS-d should not appear when maxDepth=2")
		}
	}
}

func TestCollectCVSTreeBFS_cycleDoesNotLoop(t *testing.T) {
	refs := map[string][]string{
		"CVS-a": {"CVS-b"},
		"CVS-b": {"CVS-a"},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "active", "", nil
	}
	tree, err := CollectCVSTreeBFS("CVS-a", DefaultOverseerCVSTreeMaxDepth, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 {
		t.Fatalf("want 2 nodes once each, got %d %#v", len(tree), tree)
	}
}

func TestCollectCVSTreeBFS_diamondDeduplicates(t *testing.T) {
	refs := map[string][]string{
		"CVS-p":    {"CVS-l", "CVS-r"},
		"CVS-l":    {"CVS-leaf"},
		"CVS-r":    {"CVS-leaf"},
		"CVS-leaf": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "", "", nil
	}
	tree, err := CollectCVSTreeBFS("CVS-p", DefaultOverseerCVSTreeMaxDepth, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	var leafCount int
	for _, n := range tree {
		if n.ID == "CVS-leaf" {
			leafCount++
		}
	}
	if leafCount != 1 {
		t.Fatalf("CVS-leaf should appear once, got %d in %#v", leafCount, tree)
	}
}

func TestCollectCVSTreeBFS_ignoresNonCVSPrefix(t *testing.T) {
	refs := map[string][]string{
		"CVS-a": {"CVS-b", "BLI-1", ""},
		"CVS-b": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "", "", nil
	}
	tree, err := CollectCVSTreeBFS("CVS-a", 3, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range tree {
		if n.ID == "BLI-1" {
			t.Fatal("non-CVS ref should be ignored")
		}
	}
	if len(tree) != 2 {
		t.Fatalf("got %#v", tree)
	}
}

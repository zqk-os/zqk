package convergerollup

import (
	"testing"
)

func TestCollectCVSTreeBFS_respectsMaxDepth(t *testing.T) {
	refs := map[string][]string{
		"CONV-a": {"CONV-b"},
		"CONV-b": {"CONV-c"},
		"CONV-c": {"CONV-d"},
		"CONV-d": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "active", "c1", nil
	}
	tree, err := CollectCVSTreeBFS("CONV-a", 2, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Fatalf("want 3 nodes (a,b,c), got %d: %#v", len(tree), tree)
	}
	if tree[0].ID != "CONV-a" || tree[0].Depth != 0 {
		t.Fatalf("root: %#v", tree[0])
	}
	if tree[2].ID != "CONV-c" || tree[2].Depth != 2 {
		t.Fatalf("deepest: %#v", tree[2])
	}
	for _, n := range tree {
		if n.ID == "CONV-d" {
			t.Fatal("CONV-d should not appear when maxDepth=2")
		}
	}
}

func TestCollectCVSTreeBFS_cycleDoesNotLoop(t *testing.T) {
	refs := map[string][]string{
		"CONV-a": {"CONV-b"},
		"CONV-b": {"CONV-a"},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "active", "", nil
	}
	tree, err := CollectCVSTreeBFS("CONV-a", DefaultOverseerCVSTreeMaxDepth, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 {
		t.Fatalf("want 2 nodes once each, got %d %#v", len(tree), tree)
	}
}

func TestCollectCVSTreeBFS_diamondDeduplicates(t *testing.T) {
	refs := map[string][]string{
		"CONV-p":    {"CONV-l", "CONV-r"},
		"CONV-l":    {"CONV-leaf"},
		"CONV-r":    {"CONV-leaf"},
		"CONV-leaf": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "", "", nil
	}
	tree, err := CollectCVSTreeBFS("CONV-p", DefaultOverseerCVSTreeMaxDepth, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	var leafCount int
	for _, n := range tree {
		if n.ID == "CONV-leaf" {
			leafCount++
		}
	}
	if leafCount != 1 {
		t.Fatalf("CONV-leaf should appear once, got %d in %#v", leafCount, tree)
	}
}

func TestCollectCVSTreeBFS_ignoresNonCVSPrefix(t *testing.T) {
	refs := map[string][]string{
		"CONV-a": {"CONV-b", "ITEM-1", ""},
		"CONV-b": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "", "", nil
	}
	tree, err := CollectCVSTreeBFS("CONV-a", 3, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range tree {
		if n.ID == "ITEM-1" {
			t.Fatal("non-CVS ref should be ignored")
		}
	}
	if len(tree) != 2 {
		t.Fatalf("got %#v", tree)
	}
}

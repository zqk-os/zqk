package convergerollup

import (
	"strings"
	"testing"
)

func TestBuildArbitratedParentMessage_blocksWhenActiveChildren(t *testing.T) {
	tree := []CVSTreeNode{
		{ID: "CONV-p", Depth: 0, Status: "active"},
		{ID: "CONV-c1", Depth: 1, Status: "active"},
	}
	rollup := map[string]any{
		"ready_for_parent_completion": false,
		"recommended_next_action":     "Fix tests.",
	}
	s := BuildArbitratedParentMessage(rollup, tree, "CONV-p")
	if s == "" {
		t.Fatal("empty message")
	}
	if !strings.Contains(s, "CONV-c1") || !strings.Contains(s, "Do not treat parent") {
		t.Fatalf("message %q", s)
	}
}

func TestActiveChildIDsUnderCoordinator(t *testing.T) {
	tree := []CVSTreeNode{
		{ID: "CONV-p", Depth: 0, Status: "active"},
		{ID: "CONV-c1", Depth: 1, Status: "active"},
		{ID: "CONV-c2", Depth: 2, Status: "completed"},
	}
	got := ActiveChildIDsUnderCoordinator(tree)
	if len(got) != 1 || got[0] != "CONV-c1" {
		t.Fatalf("got %#v", got)
	}
}

package convergerollup

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestBuildArbitratedParentMessage_blocksWhenActiveChildren(t *testing.T) {
	tree := []CVSTreeNode{
		{ID: "CVS-p", Depth: 0, Status: objects.ObjectStatusActive},
		{ID: "CVS-c1", Depth: 1, Status: objects.ObjectStatusActive},
	}
	rollup := map[string]any{
		"ready_for_parent_completion": false,
		"recommended_next_action":     "Fix tests.",
	}
	s := BuildArbitratedParentMessage(rollup, tree, "CVS-p")
	if s == "" {
		t.Fatal("empty message")
	}
	if !strings.Contains(s, "CVS-c1") || !strings.Contains(s, "Do not treat parent") {
		t.Fatalf("message %q", s)
	}
}

func TestActiveChildIDsUnderCoordinator(t *testing.T) {
	tree := []CVSTreeNode{
		{ID: "CVS-p", Depth: 0, Status: objects.ObjectStatusActive},
		{ID: "CVS-c1", Depth: 1, Status: objects.ObjectStatusActive},
		{ID: "CVS-c2", Depth: 2, Status: objects.ObjectStatusCompleted},
	}
	got := ActiveChildIDsUnderCoordinator(tree)
	if len(got) != 1 || got[0] != "CVS-c1" {
		t.Fatalf("got %#v", got)
	}
}

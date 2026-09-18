package convergerollup

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNestSpawnLinkStatus(t *testing.T) {
	t.Parallel()
	refs := map[string][]string{
		"CVS-parent": {},
	}
	status := map[string]string{"CVS-parent": "active"}
	phase := map[string]string{"CVS-parent": "c2_triage"}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], status[id], phase[id], nil
	}

	if err := ValidateNestLink("CVS-parent", "CVS-parent", "CVS-child", DefaultOverseerCVSTreeMaxDepth, nodeFor); err != nil {
		t.Fatalf("validate spawn link: %v", err)
	}
	refs["CVS-parent"] = AppendRelatedObjectRef(refs["CVS-parent"], "CVS-child")
	refs["CVS-child"] = []string{"CVS-parent"}
	status["CVS-child"] = "active"
	phase["CVS-child"] = "c4_act"

	st, err := NestStatus("CVS-parent", DefaultOverseerCVSTreeMaxDepth, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %#v", st.Nodes)
	}
	fields := ChildSessionFields(NestSpawnRequest{
		ParentID:        "CVS-parent",
		Title:           "child",
		Hypothesis:      "h",
		DesiredEndState: "d",
	})
	if fields[objects.FieldKeyTitle] != "child" {
		t.Fatalf("fields=%v", fields)
	}
	rel := RelatedObjectRefsFromMap(map[string]any{objects.FieldKeyRelatedObjectRefs: []any{"CVS-a", "CVS-b"}})
	if len(rel) != 2 || rel[0] != "CVS-a" {
		t.Fatalf("refs=%v", rel)
	}
}

func TestNestRejectsDepthAndCycle(t *testing.T) {
	t.Parallel()
	refs := map[string][]string{
		"CVS-p": {"CVS-a"},
		"CVS-a": {"CVS-b"},
		"CVS-b": {"CVS-c"},
		"CVS-c": {},
		"CVS-x": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "active", "", nil
	}
	err := ValidateNestLink("CVS-p", "CVS-c", "CVS-x", 3, nodeFor)
	if err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("expected max depth error, got %v", err)
	}

	refs["CVS-x"] = []string{"CVS-p"}
	err = ValidateNestLink("CVS-p", "CVS-p", "CVS-x", 3, nodeFor)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

package convergerollup

import (
	"strings"
	"testing"
)

func TestNestSpawnLinkStatus(t *testing.T) {
	t.Parallel()
	refs := map[string][]string{
		"CONV-parent": {},
	}
	status := map[string]string{"CONV-parent": "active"}
	phase := map[string]string{"CONV-parent": "c2_triage"}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], status[id], phase[id], nil
	}

	if err := ValidateNestLink("CONV-parent", "CONV-parent", "CONV-child", DefaultOverseerCVSTreeMaxDepth, nodeFor); err != nil {
		t.Fatalf("validate spawn link: %v", err)
	}
	refs["CONV-parent"] = AppendRelatedObjectRef(refs["CONV-parent"], "CONV-child")
	refs["CONV-child"] = []string{"CONV-parent"}
	status["CONV-child"] = "active"
	phase["CONV-child"] = "c4_act"

	st, err := NestStatus("CONV-parent", DefaultOverseerCVSTreeMaxDepth, nodeFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %#v", st.Nodes)
	}
	fields := ChildSessionFields(NestSpawnRequest{
		ParentID:        "CONV-parent",
		Title:           "child",
		Hypothesis:      "h",
		DesiredEndState: "d",
	})
	if fields["title"] != "child" {
		t.Fatalf("fields=%v", fields)
	}
	rel := RelatedObjectRefsFromMap(map[string]any{"related_object_refs": []any{"CONV-a", "CONV-b"}})
	if len(rel) != 2 || rel[0] != "CONV-a" {
		t.Fatalf("refs=%v", rel)
	}
}

func TestNestRejectsDepthAndCycle(t *testing.T) {
	t.Parallel()
	refs := map[string][]string{
		"CONV-p": {"CONV-a"},
		"CONV-a": {"CONV-b"},
		"CONV-b": {"CONV-c"},
		"CONV-c": {},
		"CONV-x": {},
	}
	nodeFor := func(id string) ([]string, string, string, error) {
		return refs[id], "active", "", nil
	}
	err := ValidateNestLink("CONV-p", "CONV-c", "CONV-x", 3, nodeFor)
	if err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("expected max depth error, got %v", err)
	}

	refs["CONV-x"] = []string{"CONV-p"}
	err = ValidateNestLink("CONV-p", "CONV-p", "CONV-x", 3, nodeFor)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

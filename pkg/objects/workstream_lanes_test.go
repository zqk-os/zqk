package objects

import "testing"

func TestWorkstreamLaneIDs_SingularPluralDedup(t *testing.T) {
	t.Parallel()
	if got := WorkstreamLaneIDs(nil); len(got) != 0 {
		t.Fatalf("nil: %v", got)
	}
	if got := WorkstreamLaneIDs(map[string]any{FieldKeyWorkstreamRefs: []string{"  "}}); len(got) != 0 {
		t.Fatalf("blank plural: %v", got)
	}
	got := WorkstreamLaneIDs(map[string]any{
		FieldKeyWorkstreamRef:  "WS-A",
		FieldKeyWorkstreamRefs: []string{"WS-A", "WS-B", " "},
	})
	if len(got) != 2 || got[0] != "WS-A" || got[1] != "WS-B" {
		t.Fatalf("dedup order: %v", got)
	}
	if !WorkstreamLaneContains(map[string]any{FieldKeyWorkstreamRefs: []any{"WS-B"}}, "WS-B") {
		t.Fatal("[]any membership")
	}
	if WorkstreamLaneContains(map[string]any{FieldKeyWorkstreamRef: "WS-A"}, "WS-B") {
		t.Fatal("miss should fail")
	}
	if !IsWorkstreamLaneField(FieldKeyWorkstreamRef) || !IsWorkstreamLaneField(FieldKeyWorkstreamRefs) {
		t.Fatal("lane field names")
	}
	if IsWorkstreamLaneField(FieldKeyPriorityPlanRef) {
		t.Fatal("priority_plan_ref is not a workstream lane field")
	}
	if got := WorkstreamLaneGroupKey(map[string]any{FieldKeyWorkstreamRefs: []string{"WS-A", "WS-B"}}); got != "WS-A,WS-B" {
		t.Fatalf("group key: %q", got)
	}
}

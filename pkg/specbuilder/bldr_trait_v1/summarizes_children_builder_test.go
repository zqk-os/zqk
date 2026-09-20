package bldr_trait_v1

import "testing"

func TestSummarizesChildrenBuilderIsObjectLevelBehavior(t *testing.T) {
	t.Parallel()
	trait := NewSummarizesChildrenBuilder().Build()
	if trait.Name != "summarizes_children" {
		t.Fatalf("name=%q want summarizes_children", trait.Name)
	}
	if !trait.ObjectLevel {
		t.Fatal("summarizes_children must be object-level, not a field trait")
	}
	if trait.FieldLevel {
		t.Fatal("summarizes_children must not be field-level")
	}
	if trait.Category != "behavior" {
		t.Fatalf("category=%q want behavior", trait.Category)
	}
	childKinds, ok := trait.Config["child_kinds"].([]string)
	if !ok || len(childKinds) == 0 || childKinds[0] != "backlog_item" {
		t.Fatalf("unexpected child_kinds config: %v", trait.Config["child_kinds"])
	}
}

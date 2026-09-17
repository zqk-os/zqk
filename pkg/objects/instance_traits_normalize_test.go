package objects

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestMaybeStripRedundantTopLevelTraits_BacklogItemSpecTraits(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	sl := NewSpecLoader(specsDir)
	tr := NewTraitRegistry()
	traitsDir := filepath.Join("..", "..", paths.ProcessInternalTraitsDir)
	if err := tr.LoadTraitsFromDirectory(traitsDir); err != nil {
		t.Fatalf("LoadTraitsFromDirectory: %v", err)
	}

	spec, err := sl.LoadSpecWithInheritance("backlog_item.yaml")
	if err != nil {
		t.Fatalf("LoadSpecWithInheritance: %v", err)
	}
	if len(spec.ResolvedTraits) == 0 {
		t.Fatal("expected backlog_item ResolvedTraits")
	}

	obj := map[string]any{
		FieldKeyKind:            "backlog_item",
		FieldKeyID:              "BLI-test-traits-strip",
		"traits":                append([]any{}, toAnySlice(spec.ResolvedTraits)...),
		FieldKeyTitle:           "x",
		FieldKeyStatus:          "draft",
		FieldKeyNamespaceID:     "zqk:kernel",
		FieldKeyPriorityTier:    "P3",
		FieldKeyPriorityPlanRef: "PRI-221",
	}

	MaybeStripRedundantTopLevelTraits(obj, sl, tr)
	if _, ok := obj["traits"]; ok {
		t.Fatal("expected redundant traits stripped")
	}

	// Different trait list must be kept
	obj2 := map[string]any{
		FieldKeyKind:            "backlog_item",
		FieldKeyID:              "BLI-test-traits-keep",
		"traits":                []any{"base_object_traits", "manipulatable"},
		FieldKeyTitle:           "y",
		FieldKeyStatus:          "draft",
		FieldKeyNamespaceID:     "zqk:kernel",
		FieldKeyPriorityTier:    "P3",
		FieldKeyPriorityPlanRef: "PRI-221",
	}
	MaybeStripRedundantTopLevelTraits(obj2, sl, tr)
	if _, ok := obj2["traits"]; !ok {
		t.Fatal("expected non-matching traits preserved")
	}
}

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

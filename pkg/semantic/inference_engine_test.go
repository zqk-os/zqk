package semantic

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestInferenceEngine_DynamicHeuristics(t *testing.T) {
	store := &mockStorage{}
	engine := NewInferenceEngine(store)
	ctx := context.Background()

	// 1. Level 0 - should have static fallback
	assessment := &MaturityAssessment{Level: 0}
	inferences := engine.Infer(ctx, assessment)

	foundStatic := false
	for _, inf := range inferences {
		if inf[objects.FieldKeyTitle] == "Standardize Core Objects" {
			foundStatic = true
		}
	}
	if !foundStatic {
		t.Errorf("expected static fallback for level 0")
	}

	// 2. Add a dynamic heuristic for Level 1
	heuristic := map[string]any{
		objects.FieldKeyID:                  "INF-HEU-001",
		objects.FieldKeyKind:                objects.KindInferenceHeuristic,
		objects.FieldKeyTargetMaturityLevel: 1,
		objects.FieldKeyProposedTitle:       "Custom Level 1 Item",
		objects.FieldKeyProposedDescription: "Test dynamic inference",
		objects.FieldKeyProposedPriority:    "P3",
	}
	_ = store.Create(ctx, nil, heuristic)

	// 3. Test Level 1
	assessment1 := &MaturityAssessment{Level: 1}
	inferences1 := engine.Infer(ctx, assessment1)

	foundDynamic := false
	for _, inf := range inferences1 {
		if inf[objects.FieldKeyTitle] == "Custom Level 1 Item" {
			foundDynamic = true
		}
	}
	if !foundDynamic {
		t.Errorf("did not find dynamic heuristic inference for level 1")
	}
}

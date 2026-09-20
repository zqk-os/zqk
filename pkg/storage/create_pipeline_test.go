package storage

import "testing"

func TestCreatePipelineOrderedStages(t *testing.T) {
	t.Helper()
	if len(CreatePipelineOrderedStages) < 3 {
		t.Fatalf("expected ordered stages slice to be populated, got len=%d", len(CreatePipelineOrderedStages))
	}
	if CreatePipelineOrderedStages[0] != PersistenceStepEntry {
		t.Fatalf("first stage: got %q want %q", CreatePipelineOrderedStages[0], PersistenceStepEntry)
	}
	if CreatePipelineOrderedStages[len(CreatePipelineOrderedStages)-1] != PersistenceStepExit {
		t.Fatalf("last stage: got %q want %q",
			CreatePipelineOrderedStages[len(CreatePipelineOrderedStages)-1], PersistenceStepExit)
	}
	seen := make(map[PersistenceStep]struct{}, len(CreatePipelineOrderedStages))
	for _, s := range CreatePipelineOrderedStages {
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate stage in CreatePipelineOrderedStages: %q", s)
		}
		seen[s] = struct{}{}
	}
}

package specorigination

import "testing"

func TestStageNames_unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for _, name := range []string{
		StageIngest,
		StageNormalize,
		StageDecide,
		StageCommit,
		StageMaterializeIndexes,
		StageTriggerSideEffects,
		StageFinalize,
	} {
		if name == "" {
			t.Fatal("empty stage name")
		}
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate stage name %q", name)
		}
		seen[name] = struct{}{}
	}
}

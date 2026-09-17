package primaryorch

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestMaybeWakeOnPlanBacklogError_Wakes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := WriteBinding(root, Binding{
		AgentID: "test-orchestrator",
		Adapter: AdapterNoop,
	}); err != nil {
		t.Fatal(err)
	}
	MaybeWakeOnPlanBacklogError(context.Background(), root, objects.KindBacklogItem, "in_progress", objects.ObjectStatusError, map[string]any{
		objects.FieldKeyID:              "BLI-TEST",
		objects.FieldKeyTitle:           "broken",
		objects.FieldKeyPriorityPlanRef: "PRI-TEST",
	})
}

func TestMaybeWakeOnPlanBacklogError_SkipsNonError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	MaybeWakeOnPlanBacklogError(context.Background(), root, objects.KindBacklogItem, "planned", "in_progress", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-TEST",
	})
}

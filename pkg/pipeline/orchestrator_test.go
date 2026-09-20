package pipeline

import "testing"

func TestOrchestrator(t *testing.T) {
	t.Parallel()

	orchestrator := NewOrchestrator(nil)
	if orchestrator == nil {
		t.Fatal("NewOrchestrator() returned nil")
	}
	if orchestrator.store != nil {
		t.Fatalf("store = %v, want nil", orchestrator.store)
	}
	if orchestrator.plugins == nil {
		t.Fatal("plugins map is nil")
	}
	if len(orchestrator.plugins) != 0 {
		t.Fatalf("plugins count = %d, want 0", len(orchestrator.plugins))
	}
}

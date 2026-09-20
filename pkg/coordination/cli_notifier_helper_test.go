package coordination

import (
	"testing"
)

func TestNewCLINotifierWithCoordinator_PanicsOnMissingPrereqs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when projectRoot is empty")
		}
	}()
	NewCLINotifierWithCoordinator(false, false, "", nil, "op-1", "test", "human")
}

func TestNewCLINotifierWithCoordinator_PanicsOnNilStorage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when storageProvider is nil")
		}
	}()
	NewCLINotifierWithCoordinator(false, false, "/project", nil, "op-1", "test", "human")
}

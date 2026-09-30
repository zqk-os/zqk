package coordination

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"
)

func TestNewCLINotifierWithCoordinator_SafeFallbackOnMissingPrereqs(t *testing.T) {
	notifier := NewCLINotifierWithCoordinator(false, false, "", nil, "op-1", "test", "human")
	if _, ok := notifier.(storage.NoopOperationNotifier); !ok {
		t.Errorf("expected NoopOperationNotifier when projectRoot is empty, got %T", notifier)
	}
}

func TestNewCLINotifierWithCoordinator_SafeFallbackOnNilStorage(t *testing.T) {
	notifier := NewCLINotifierWithCoordinator(false, false, "/project", nil, "op-1", "test", "human")
	if _, ok := notifier.(storage.NoopOperationNotifier); !ok {
		t.Errorf("expected NoopOperationNotifier when storageProvider is nil, got %T", notifier)
	}
}

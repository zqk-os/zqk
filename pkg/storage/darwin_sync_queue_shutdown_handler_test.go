package storage

import (
	"context"
	"testing"
	"time"
)

const (
	testQueueNameDarwinCAS     = "darwin_cas_fsync_queue"
	errTestExpectedQueueName   = "expected queue name 'darwin_cas_fsync_queue', got %q"
	errTestExpectedCritical    = "expected DarwinSyncQueueShutdownHandler to be critical (Phase 1 drain)"
	errTestExpectedDrained     = "expected handler to report drained after Drain, got pending: %d"
	errTestExpectedNonNilCoord = "expected non-nil global coordinator"
	errTestExpectedQueueCrit   = "expected registered darwin_cas_fsync_queue to be critical"
	errTestExpectedRegistered  = "expected darwin_cas_fsync_queue to be registered in global shutdown coordinator"
)

func TestDarwinSyncQueueShutdownHandler_Contract(t *testing.T) {
	handler := &DarwinSyncQueueShutdownHandler{}

	if handler.GetName() != testQueueNameDarwinCAS {
		t.Errorf(errTestExpectedQueueName, handler.GetName())
	}

	if !handler.IsCritical() {
		t.Error(errTestExpectedCritical)
	}

	if err := handler.InitiateShutdown(); err != nil {
		t.Errorf("InitiateShutdown returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := handler.Drain(ctx); err != nil {
		t.Errorf("Drain returned error: %v", err)
	}

	if !handler.IsDrained() {
		t.Errorf(errTestExpectedDrained, handler.GetPendingCount())
	}
}

func TestGlobalShutdownCoordinator_IncludesDarwinSyncQueue(t *testing.T) {
	coordinator := GetGlobalShutdownCoordinator()
	if coordinator == nil {
		t.Fatal(errTestExpectedNonNilCoord)
	}

	found := false
	for _, q := range coordinator.queues {
		if q.GetName() == testQueueNameDarwinCAS {
			found = true
			if !q.IsCritical() {
				t.Error(errTestExpectedQueueCrit)
			}
			break
		}
	}

	if !found {
		t.Error(errTestExpectedRegistered)
	}
}

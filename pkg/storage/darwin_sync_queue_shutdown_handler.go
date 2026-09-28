package storage

import (
	"context"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// DarwinSyncQueueShutdownHandler adapts the Darwin CAS fsync background queue
// into the QueueShutdownHandler lifecycle contract for QueueShutdownCoordinator.
type DarwinSyncQueueShutdownHandler struct{}

// GetName returns the canonical queue name for logging and telemetry.
func (h *DarwinSyncQueueShutdownHandler) GetName() string {
	return "darwin_cas_fsync_queue"
}

// InitiateShutdown transitions the darwin CAS sync queue into synchronous mode
// to prevent any new background descriptors from being scheduled.
func (h *DarwinSyncQueueShutdownHandler) InitiateShutdown() error {
	return filecas.InitiateDarwinSyncShutdown()
}

// Drain blocks until all queued Darwin CAS sync operations complete or ctx expires.
func (h *DarwinSyncQueueShutdownHandler) Drain(ctx context.Context) error {
	return filecas.DrainDarwinSyncQueueContext(ctx)
}

// IsDrained returns true if all pending fsync operations have completed.
func (h *DarwinSyncQueueShutdownHandler) IsDrained() bool {
	return filecas.IsDarwinSyncQueueDrained()
}

// GetPendingCount returns the current count of pending background sync operations.
func (h *DarwinSyncQueueShutdownHandler) GetPendingCount() int64 {
	return filecas.DarwinSyncQueuePendingCount()
}

// IsCritical marks this queue as critical (Phase 1 drain) to guarantee durability.
func (h *DarwinSyncQueueShutdownHandler) IsCritical() bool {
	return true
}

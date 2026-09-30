// Extracted from io_queue.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

func (m *IOQueueManager) InitiateShutdown() error {
	var queues []*ioQueue
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameIoQueueInitiateShutdown, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if m.cancel != nil {
			m.cancel()
		}
		queues = make([]*ioQueue, len(m.queues))
		copy(queues, m.queues)
		for _, queue := range queues {
			queue.cancel()
		}
		return nil
	})
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending operations
func (m *IOQueueManager) Drain(ctx context.Context) error {
	// Initiate shutdown first
	if err := m.InitiateShutdown(); err != nil {
		return err
	}

	var queues []*ioQueue
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueManagerDrainCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queues = make([]*ioQueue, len(m.queues))
		copy(queues, m.queues)
		return nil
	})

	if len(queues) == 0 {
		return nil // No queues to drain
	}

	// Wait for all workers to finish
	var drainWg sync.WaitGroup
	ioDrainBud := goroutinelabels.DefaultBudget()
	for _, queue := range queues {
		queueCopy := queue // Capture for goroutine
		ioDrainBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueDrainWait, fmt.Sprintf(ConstMiscWaitingForIoQueueWorkerS, queueCopy.queueID)).
			WithWaitGroup(&drainWg)
		if ioDrainBud != nil {
			ioDrainBuilder = ioDrainBuilder.WithBudget(ioDrainBud)
		}
		ioDrainBuilder.StartSimple(func() {
			// Wait for the queue's worker WaitGroup
			wgID := fmt.Sprintf("%s_worker", queueCopy.queueID)
			queueCopy.wgManager.Wait(wgID)
		})
	}

	done := make(chan struct{})
	ioCollectorBud := goroutinelabels.DefaultBudget()
	ioCollectorBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueDrainCollector, ConstMiscWaitingForAllIoQueueDrainsToComplete).
		WithCleanup(func() {
			close(done)
		})
	if ioCollectorBud != nil {
		ioCollectorBuilder = ioCollectorBuilder.WithBudget(ioCollectorBud)
	}
	ioCollectorBuilder.StartSimple(func() {
		drainWg.Wait()
	})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (m *IOQueueManager) IsDrained() bool {
	var drained bool
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueIsDrained, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		drained = true
		for _, queue := range m.queues {
			if len(queue.operations) > 0 || queue.workerRunning.Load() == 1 {
				drained = false
				break
			}
		}
		return nil
	})
	return drained
}

// GetPendingCount implements QueueShutdownHandler
func (m *IOQueueManager) GetPendingCount() int64 {
	var queues []*ioQueue
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueManagerGetPendingCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queues = make([]*ioQueue, len(m.queues))
		copy(queues, m.queues)
		return nil
	})
	var total int64
	for _, queue := range queues {
		total += int64(len(queue.operations))
	}
	return total
}

// GetName implements QueueShutdownHandler
func (m *IOQueueManager) GetName() string {
	return ConstMiscIoQueueManager
}

// IsCritical implements QueueShutdownHandler
// I/O operations are critical - must complete before shutdown
func (m *IOQueueManager) IsCritical() bool {
	return true
}

package storage

import (
	"context"
	"sort"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// Shutdown stops the buffer and flushes remaining events
func (b *AuditEventBuffer) Shutdown() error {
	// Cancel context to signal goroutine to exit
	if b.cancel != nil {
		b.cancel()
	}

	// Stop periodic flush ticker
	if b.flushTicker != nil {
		b.flushTicker.Stop()
	}

	// Close stop channel (idempotent check)
	select {
	case <-b.stopChan:
		// Already closed
	default:
		close(b.stopChan)
	}

	// Flush remaining events
	return b.Flush()
}

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new events and initiates shutdown
func (b *AuditEventBuffer) InitiateShutdown() error {
	return b.Shutdown()
}

// Drain implements QueueShutdownHandler
// Flushes all pending events
func (b *AuditEventBuffer) Drain(ctx context.Context) error {
	return b.Flush()
}

// IsDrained implements QueueShutdownHandler
// Returns true if buffer is empty
func (b *AuditEventBuffer) IsDrained() bool {
	var drained bool
	if err := concurrency.RunInRLockWithLogger(&b.mu, locknames.LockNameAuditBufferIsDrained, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		drained = len(b.buffer) == 0
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// GetPendingCount implements QueueShutdownHandler
			// Returns the number of pending event groups
			Error(ErrMsgLockIsDrained, err).Log()
	}
	return drained
}

func (b *AuditEventBuffer) GetPendingCount() int64 {
	var count int64
	if err := concurrency.RunInRLockWithLogger(&b.mu, locknames.LockNameAuditBufferGetPendingCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		count = int64(len(b.buffer))
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// GetName implements QueueShutdownHandler
			Error(ErrMsgLockGetPending, err).Log()
	}
	return count
}

func (b *AuditEventBuffer) GetName() string {
	return ValueAuditEventBuffer
}

// IsCritical implements QueueShutdownHandler
// Audit buffer is not critical - can be force-shutdown
func (b *AuditEventBuffer) IsCritical() bool {
	return false
}

// GetBufferStats returns statistics about the current buffer state
func (b *AuditEventBuffer) GetBufferStats() map[string]any {
	var stats map[string]any
	if err := concurrency.RunInRLockWithLogger(&b.mu, locknames.LockNameAuditBufferGetStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		totalEvents := 0
		groupCount := len(b.buffer)
		groupDetails := make([]map[string]any, 0, groupCount)
		enabled := b.enabled
		windowSize := b.windowSize
		threshold := b.threshold
		for _, group := range b.buffer {
			totalEvents += group.Count
			groupDetails = append(groupDetails, map[string]any{
				FieldKeyAggKey:             group.Key,
				objects.FieldKeyEventType:  group.EventType,
				objects.FieldKeyTargetKind: group.TargetKind,
				FieldKeyAggCount:           group.Count,
				objects.FieldKeyFirstSeen:  group.FirstSeen.Format(time.RFC3339),
				objects.FieldKeyLastSeen:   group.LastSeen.Format(time.RFC3339),
			})
		}
		sort.Slice(groupDetails, func(i, j int) bool {
			return groupDetails[i][FieldKeyAggCount].(int) > groupDetails[j][FieldKeyAggCount].(int)
		})
		stats = map[string]any{
			objects.FieldKeyEnabled: enabled,
			FieldKeyAggGroupCount:   groupCount,
			FieldKeyAggTotalEvents:  totalEvents,
			FieldKeyAggGroups:       groupDetails,
			FieldKeyAggWindowSize:   windowSize.String(),
			FieldKeyAggThreshold:    threshold,
		}
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetBufferStats, err).Log()
	}
	return stats
}

// GetAuditEventBufferStats returns lifetime counters for events added and events flushed.
func (b *AuditEventBuffer) GetAuditEventBufferStats() (eventsAdded, eventsFlushed int64) {
	return b.eventsAddedTotal.Load(), b.eventsFlushedTotal.Load()
}

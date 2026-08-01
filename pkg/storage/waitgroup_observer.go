package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// LoggingWaitGroupObserver implements WaitGroupObserver with logging
// This provides observability for WaitGroup lifecycle events
type LoggingWaitGroupObserver struct {
	logger    *logging.EventLogger
	ctx       context.Context
	enabled   bool
	mu        sync.RWMutex
	groups    map[string]*groupStats
	startTime time.Time
}

// groupStats tracks statistics for a WaitGroup
type groupStats struct {
	operation string
	createdAt time.Time
	addCount  int
	doneCount int
	waitCount int
	lastAdd   time.Time
	lastDone  time.Time
	lastWait  time.Time
}

// NewLoggingWaitGroupObserver creates a new logging observer for WaitGroup lifecycle events
// If ctx is nil, uses system context
func NewLoggingWaitGroupObserver(ctx context.Context) *LoggingWaitGroupObserver {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	return &LoggingWaitGroupObserver{
		logger:    logging.NewEventLogger(ctx),
		ctx:       ctx,
		enabled:   true,
		groups:    make(map[string]*groupStats),
		startTime: time.Now(),
	}
}

// SetEnabled enables or disables the observer
func (o *LoggingWaitGroupObserver) SetEnabled(enabled bool) {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverSetEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		o.enabled = enabled
		return nil
	})
}

// OnGroupCreated is called when a WaitGroup is created
func (o *LoggingWaitGroupObserver) OnGroupCreated(id, operation string) {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverOnCreated, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !o.enabled {
			return nil
		}
		o.groups[id] = &groupStats{
			operation: operation,
			createdAt: time.Now(),
		}
		StorageLog(o.logger.Logger()).Debug(LogEventStorageWaitGroupObserverCreatedDebug).
			String("group_id", id).
			String("operation", operation).
			Log()
		return nil
	})
}

// OnGroupAdd is called when Add() is called on a WaitGroup
func (o *LoggingWaitGroupObserver) OnGroupAdd(id string, delta int) {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverOnAdd, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !o.enabled {
			return nil
		}
		stats, exists := o.groups[id]
		if !exists {
			return nil
		}
		stats.addCount += delta
		stats.lastAdd = time.Now()
		StorageLog(o.logger.Logger()).Debug(LogEventStorageWaitGroupObserverAddDebug).
			String("group_id", id).
			String("operation", stats.operation).
			Int("delta", delta).
			Int("total_adds", stats.addCount).
			Log()
		return nil
	})
}

// OnGroupDone is called when Done() is called on a WaitGroup
func (o *LoggingWaitGroupObserver) OnGroupDone(id string) {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverOnDone, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !o.enabled {
			return nil
		}
		stats, exists := o.groups[id]
		if !exists {
			return nil
		}
		stats.doneCount++
		stats.lastDone = time.Now()
		if stats.doneCount > stats.addCount {
			StorageLog(o.logger.Logger()).Warn(LogEventStorageWaitGroupObserverDoneExceedsAddWarn).
				String("group_id", id).
				String("operation", stats.operation).
				Int("add_count", stats.addCount).
				Int("done_count", stats.doneCount).
				Log()
		}
		StorageLog(o.logger.Logger()).Debug(LogEventStorageWaitGroupObserverDoneDebug).
			String("group_id", id).
			String("operation", stats.operation).
			Int("done_count", stats.doneCount).
			Int("add_count", stats.addCount).
			Log()
		return nil
	})
}

// OnGroupWait is called when Wait() is called on a WaitGroup
func (o *LoggingWaitGroupObserver) OnGroupWait(id string) {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverOnWait, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !o.enabled {
			return nil
		}
		stats, exists := o.groups[id]
		if !exists {
			return nil
		}
		stats.waitCount++
		stats.lastWait = time.Now()
		if stats.addCount != stats.doneCount {
			StorageLog(o.logger.Logger()).Warn(LogEventStorageWaitGroupObserverWaitMismatchWarn).
				String("group_id", id).
				String("operation", stats.operation).
				Int("add_count", stats.addCount).
				Int("done_count", stats.doneCount).
				Int("difference", stats.addCount-stats.doneCount).
				Log()
		}
		StorageLog(o.logger.Logger()).Debug(LogEventStorageWaitGroupObserverWaitDebug).
			String("group_id", id).
			String("operation", stats.operation).
			Int("add_count", stats.addCount).
			Int("done_count", stats.doneCount).
			Log()
		return nil
	})
}

// OnGroupCompleted is called when Wait() completes
func (o *LoggingWaitGroupObserver) OnGroupCompleted(id string, duration time.Duration) {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverOnCompleted, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !o.enabled {
			return nil
		}
		stats, exists := o.groups[id]
		if !exists {
			return nil
		}
		StorageLog(o.logger.Logger()).Info(LogEventStorageWaitGroupObserverCompletedInfo).
			String("group_id", id).
			String("operation", stats.operation).
			String("duration", duration.String()).
			Int("total_adds", stats.addCount).
			Int("total_dones", stats.doneCount).
			Int("wait_calls", stats.waitCount).
			Log()
		if stats.addCount != stats.doneCount {
			StorageLog(o.logger.Logger()).Warn(LogEventStorageWaitGroupObserverCompletedMismatchWarn).
				String("group_id", id).
				String("operation", stats.operation).
				Int("add_count", stats.addCount).
				Int("done_count", stats.doneCount).
				Log()
		}
		if duration > 30*time.Second {
			StorageLog(o.logger.Logger()).Warn(LogEventStorageWaitGroupObserverWaitSlowWarn).
				String("group_id", id).
				String("operation", stats.operation).
				String("duration", duration.String()).
				Log()
		}
		return nil
	})
}

// GetStats returns statistics for a specific WaitGroup
func (o *LoggingWaitGroupObserver) GetStats(id string) (operation string, addCount, doneCount, waitCount int, exists bool) {
	var op string
	var adds, dones, waits int
	var found bool
	_ = concurrency.RunInRLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverGetStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		stats, ok := o.groups[id]
		if !ok {
			return nil
		}
		found = true
		op = stats.operation
		adds = stats.addCount
		dones = stats.doneCount
		waits = stats.waitCount
		return nil
	})
	if !found {
		return "", 0, 0, 0, false
	}
	return op, adds, dones, waits, true
}

// GetAllStats returns statistics for all tracked WaitGroups
func (o *LoggingWaitGroupObserver) GetAllStats() map[string]GroupStats {
	var result map[string]GroupStats
	_ = concurrency.RunInRLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverGetAllStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result = make(map[string]GroupStats, len(o.groups))
		for id, stats := range o.groups {
			result[id] = GroupStats{
				Operation: stats.operation,
				CreatedAt: stats.createdAt,
				AddCount:  stats.addCount,
				DoneCount: stats.doneCount,
				WaitCount: stats.waitCount,
				LastAdd:   stats.lastAdd,
				LastDone:  stats.lastDone,
				LastWait:  stats.lastWait,
			}
		}
		return nil
	})
	return result
}

// GroupStats represents statistics for a WaitGroup
type GroupStats struct {
	Operation string
	CreatedAt time.Time
	AddCount  int
	DoneCount int
	WaitCount int
	LastAdd   time.Time
	LastDone  time.Time
	LastWait  time.Time
}

// Summary returns a summary of all WaitGroup activity
func (o *LoggingWaitGroupObserver) Summary() string {
	var summary string
	var startTime time.Time
	var groups map[string]*groupStats
	_ = concurrency.RunInRLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverSummary, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(o.groups) == 0 {
			summary = ConstMiscNoWaitgroupsTracked
			return nil
		}
		summary = fmt.Sprintf(ConstMiscWaitgroupObserverSummaryTrackingDGroupsN, len(o.groups))
		startTime = o.startTime
		groups = make(map[string]*groupStats, len(o.groups))
		for id, stats := range o.groups {
			groups[id] = stats
		}
		return nil
	})
	if summary == ConstMiscNoWaitgroupsTracked {
		return summary
	}

	summary += fmt.Sprintf(ConstMiscUptimeVN, time.Since(startTime))
	for id, stats := range groups {
		summary += fmt.Sprintf(ConstMiscGroupQSN, id, stats.operation)
		summary += fmt.Sprintf(ConstMiscCreatedVN, stats.createdAt)
		summary += fmt.Sprintf(ConstMiscAddsDDonesDWaitsDN, stats.addCount, stats.doneCount, stats.waitCount)
		if stats.addCount != stats.doneCount {
			summary += fmt.Sprintf("    ⚠️  MISMATCH: Add/Done difference: %d\n", stats.addCount-stats.doneCount)
		}
		if !stats.lastAdd.IsZero() {
			summary += fmt.Sprintf(ConstMiscLastAddVN, stats.lastAdd)
		}
		if !stats.lastDone.IsZero() {
			summary += fmt.Sprintf(ConstMiscLastDoneVN, stats.lastDone)
		}
		if !stats.lastWait.IsZero() {
			summary += fmt.Sprintf(ConstMiscLastWaitVN, stats.lastWait)
		}
	}
	return summary
}

// ClearStats clears all tracked statistics
func (o *LoggingWaitGroupObserver) ClearStats() {
	_ = concurrency.RunInLockOrLog(&o.mu, locknames.LockNameWaitgroupObserverClearStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		o.groups = make(map[string]*groupStats)
		o.startTime = time.Now()
		return nil
	})
}

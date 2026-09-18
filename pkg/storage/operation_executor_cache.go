package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/when"
)

const (
	cacheInvalidationStatusCompleted  = "completed"
	cacheInvalidationStatusFailed     = "failed"
	cacheInvalidationStatusPending    = "pending"
	cacheInvalidationStatusProcessing = "processing"
)

// CacheManager manages cache operations and tracks consistency
type CacheManager struct {
	ctx                         context.Context
	storage                     ObjectStorageProvider
	schedulerManager            *SchedulerJobManager
	pendingInvalidations        map[string]*PendingInvalidation
	invalidationsTriggeredTotal atomic.Int64
	invalidationsCompletedTotal atomic.Int64
	invalidationsFailedTotal    atomic.Int64
	mu                          sync.RWMutex
	logger                      *logging.EventLogger
}

// GetCacheManagerStats returns lifetime counters for invalidations triggered, completed, and failed.
func (cm *CacheManager) GetCacheManagerStats() (triggered, completed, failed int64) {
	if cm == nil {
		return 0, 0, 0
	}
	return cm.invalidationsTriggeredTotal.Load(), cm.invalidationsCompletedTotal.Load(), cm.invalidationsFailedTotal.Load()
}

// NewCacheManager creates a new cache manager
// ctx: parent context from command entry point (should not be created here)
func NewCacheManager(ctx context.Context, storage ObjectStorageProvider) *CacheManager {
	return &CacheManager{
		ctx:                  ctx,
		storage:              storage,
		schedulerManager:     GetSchedulerJobManager(storage),
		pendingInvalidations: make(map[string]*PendingInvalidation),
		logger:               logging.NewEventLogger(ctx),
	}
}

// GetConsistencyStatus returns the current cache consistency status
func (cm *CacheManager) GetConsistencyStatus() *ConsistencyStatus {
	var status *ConsistencyStatus
	_ = concurrency.RunInRLockOrLog(&cm.mu, locknames.LockNameCacheManagerGetConsistency, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		status = &ConsistencyStatus{
			IsConsistent:         true,
			PendingCount:         len(cm.pendingInvalidations),
			PendingInvalidations: make([]string, 0),
			LastChecked:          time.Now(),
		}
		for id, inv := range cm.pendingInvalidations {
			if inv.Status == cacheInvalidationStatusPending || inv.Status == cacheInvalidationStatusProcessing {
				status.PendingInvalidations = append(status.PendingInvalidations, id)
				if time.Since(inv.StartedAt) > 30*time.Second {
					status.IsConsistent = false
					status.Warnings = append(status.Warnings,
						fmt.Sprintf(ConstMiscCacheInvalidationSHasBeenPendingForV, id, time.Since(inv.StartedAt)))
				}
			}
		}
		return nil
	})
	return status
}

// InvalidateAsync invalidates cache entries in the background
func (cm *CacheManager) InvalidateAsync(ctx context.Context, objectIDs []string, reason string) string {
	invID := fmt.Sprintf("inv-%d", time.Now().UnixNano())

	inv := &PendingInvalidation{
		ID:        invID,
		ObjectIDs: objectIDs,
		Reason:    reason,
		Status:    cacheInvalidationStatusPending,
		StartedAt: time.Now(),
	}

	_ = concurrency.RunInLockOrLog(&cm.mu, locknames.LockNameCacheManagerInvalidateAsync, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cm.pendingInvalidations[invID] = inv
		return nil
	})
	cm.invalidationsTriggeredTotal.Add(1)

	// Execute invalidation in background goroutine
	invBud := goroutinelabels.DefaultBudget()
	invBuilder := goroutinelabels.NewGoroutine(ConstMiscOperationExecutorCacheInvalidation, fmt.Sprintf(ConstMiscInvalidatingCacheForOperationS, invID))
	if invBud != nil {
		invBuilder = invBuilder.WithBudget(invBud)
	}
	invBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		cm.executeInvalidation(ctx, inv)
		return nil
	})

	return invID
}

// executeInvalidation executes cache invalidation in background
// Uses scheduler_job objects to manage the background process
func (cm *CacheManager) executeInvalidation(ctx context.Context, inv *PendingInvalidation) {
	inv.Status = cacheInvalidationStatusProcessing

	// Schedule cache invalidation via scheduler_job
	// This allows the scheduler to manage the background process,
	// track execution, handle retries, and provide audit events
	err := cm.schedulerManager.ScheduleCacheInvalidation(ctx, inv.ObjectIDs, inv.Reason)
	if err != nil {
		StorageLog(cm.logger.Logger()).Warn(LogEventStorageOperationExecutorCacheInvalidationScheduleFallbackWarn).
			String(ConstMiscInvalidationId, inv.ID).
			WithError(err).
			Log()
		// Fall back to direct execution when cacheOperationHandler != nil is future work
	}
	when.When(func() bool { return err != nil }).Then(func() {
		inv.Error = err
		inv.Status = cacheInvalidationStatusFailed
		cm.invalidationsFailedTotal.Add(1)
		StorageLog(cm.logger.Logger()).Error(LogEventStorageOperationExecutorCacheInvalidateFailedErr, err).
			String(ConstMiscInvalidationId, inv.ID).
			Log()
		now := time.Now()
		inv.CompletedAt = &now
	}).OrElse(func() {
		inv.Status = cacheInvalidationStatusCompleted
		cm.invalidationsCompletedTotal.Add(1)
		now := time.Now()
		inv.CompletedAt = &now
	}).Run()

	// Clean up old completed invalidations (keep last 100)
	cm.cleanupOldInvalidations()
}

// cleanupOldInvalidations removes old completed invalidations
func (cm *CacheManager) cleanupOldInvalidations() {
	_ = concurrency.RunInLockOrLog(&cm.mu, locknames.LockNameCacheManagerCleanupOldInvalidations, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(cm.pendingInvalidations) <= 100 {
			return nil
		}
		cutoff := time.Now().Add(-1 * time.Hour)
		for id, inv := range cm.pendingInvalidations {
			if inv.Status == cacheInvalidationStatusCompleted && inv.CompletedAt != nil && inv.CompletedAt.Before(cutoff) {
				delete(cm.pendingInvalidations, id)
			}
		}
		return nil
	})
}

// isRetryableErrorEnhanced checks if an error is retryable (includes version conflicts)
func isRetryableErrorEnhanced(err error) bool {
	// Use existing isRetryableError from operation_helper.go
	if isRetryableError(err) {
		return true
	}

	// Version conflicts are retryable (optimistic locking)
	if errors.Is(err, ErrVersionConflict) {
		return true
	}

	return false
}

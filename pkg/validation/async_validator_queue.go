package validation

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// Enqueue adds validation tasks to the queue
// Optimization: Check cache first to avoid unnecessary work
// Returns true if object was found in cache (cache hit), false if enqueued (cache miss)
func (av *AsyncValidator) Enqueue(objectID, objectKind, filePath string, priority int) bool {
	// Check if shutdown has been initiated
	select {
	case <-av.shutdown:
		logging.Fluent(av.logger).Warn(ConstMagic6fd76fb1).
			String("object_id", objectID).
			Log()
		return false
	default:
		// Continue
	}
	// Check cache first - if valid cached state exists, skip enqueueing
	if state, ok := av.GetCachedState(objectID); ok {
		// Quick mtime check before expensive checksum computation
		info, err := os.Stat(filePath)
		if err == nil {
			// File exists - check if mtime changed (quick check)
			// If mtime hasn't changed, file likely hasn't changed
			// Only compute checksum if mtime changed or we can't stat
			if !info.ModTime().After(state.LastValidated) {
				// File mtime hasn't changed - likely unchanged, but still check integrity issues
				hasIntegrityIssues := false
				for _, issue := range state.Issues {
					if issue.Category == "integrity" {
						hasIntegrityIssues = true
						break
					}
				}
				if !hasIntegrityIssues {
					// File likely unchanged and no integrity issues - use cached state
					av.sendProgress(ValidationProgress{
						Status:        "completed",
						CurrentObject: objectID,
					})
					return true // Cache hit
				}
			}
		}

		// File mtime changed or stat failed - verify with checksum
		currentChecksum := av.computeChecksum(filePath)
		if state.Checksum == currentChecksum {
			// CRITICAL: Even if file checksum matches, we must re-validate if:
			// 1. There are integrity issues (expected hash in registry may have been updated)
			// 2. Hash registry file was updated more recently than cache entry
			// This ensures cache stays in sync when hash registry is updated by auto-fix
			hasIntegrityIssues := false
			for _, issue := range state.Issues {
				if issue.Category == "integrity" {
					hasIntegrityIssues = true
					break
				}
			}

			// Check if hash registry was updated more recently than cache entry
			hashRegistryUpdated := av.checkHashRegistryUpdated(objectKind, filePath, state.LastValidated)

			if !hasIntegrityIssues && !hashRegistryUpdated {
				// File unchanged, no integrity issues, and hash registry hasn't been updated
				// Cached state is valid - skip validation
				// Send progress update for cached result (non-blocking - drop if channel full or closed)
				av.sendProgress(ValidationProgress{
					Status:        "completed",
					CurrentObject: objectID,
				})
				return true // Cache hit
			}
			// Has integrity issues or hash registry was updated - must re-validate
		}
	}

	// File changed or not in cache - enqueue for validation (dedupe by ObjectID across batches)
	var alreadyEnqueued bool
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorEnqueue,
		lockLoggerSystem(),
		func() error {
			if av.enqueuedObjectIDs[objectID] {
				alreadyEnqueued = true
				return nil
			}
			av.enqueuedObjectIDs[objectID] = true
			task := &ValidationTask{
				ObjectID:   objectID,
				ObjectKind: objectKind,
				FilePath:   filePath,
				Priority:   priority,
				Checksum:   "", // Will be computed during validation
				EnqueuedAt: time.Now(),
				MaxRetries: 3,
			}
			av.priorityQueue.Enqueue(task)
			av.lastQueueSize = av.priorityQueue.Size()
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ConstMagic2553e5e8, err).
			String("object_id", objectID).
			Log()
	}
	if alreadyEnqueued {
		return true // Already enqueued this run - treat as "no work" to avoid double-count
	}

	av.notifyWorkers() // Signal workers that tasks are available

	// Wake worker if needed (on-demand pattern)
	av.wakeWorkerIfNeeded()

	return false // Cache miss
}

// EnqueueBatch adds multiple validation tasks
// This is more efficient than calling Enqueue() multiple times as it:
// - Reduces lock contention (single lock acquisition for the batch)
// - Notifies workers once after the entire batch (instead of per-task)
// - Deduplicates by ObjectID (first occurrence wins) within batch and across all EnqueueBatch/Enqueue calls
func (av *AsyncValidator) EnqueueBatch(tasks []ValidationTask) {
	if len(tasks) == 0 {
		return
	}

	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorEnqueueBatch,
		lockLoggerSystem(),
		func() error {
			for i := range tasks {
				if av.enqueuedObjectIDs[tasks[i].ObjectID] {
					continue
				}
				av.enqueuedObjectIDs[tasks[i].ObjectID] = true
				// Heap-copy task so queue holds its own copy. Caller reuses the slice (slice[:0] + append),
				// so storing &tasks[i] would make queue entries point at overwritten data and cause
				// the same object to be validated many times (Nth batch validated N times).
				taskCopy := &ValidationTask{
					ObjectID:   tasks[i].ObjectID,
					ObjectKind: tasks[i].ObjectKind,
					FilePath:   tasks[i].FilePath,
					Priority:   tasks[i].Priority,
					Checksum:   tasks[i].Checksum,
					EnqueuedAt: tasks[i].EnqueuedAt,
					MaxRetries: tasks[i].MaxRetries,
					RetryCount: tasks[i].RetryCount,
				}
				av.priorityQueue.Enqueue(taskCopy)
			}
			av.lastQueueSize = av.priorityQueue.Size()
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ConstMagicc4c32c6b, err).
			Int("task_count", len(tasks)).
			Log()
	}

	// Notify workers once after batch is enqueued (more efficient than per-task)
	av.notifyWorkers()

	// Wake workers if needed (on-demand pattern)
	av.wakeWorkerIfNeeded()
}

// sendProgressUpdate sends a progress update for a cached/completed object
// This is used by batch enqueue optimization to send progress for cache hits
func (av *AsyncValidator) sendProgressUpdate(objectID string, status string) {
	av.sendProgress(ValidationProgress{
		Status:        status,
		CurrentObject: objectID,
	})
}

// GetProgress returns the progress channel
func (av *AsyncValidator) GetProgress() <-chan ValidationProgress {
	return av.progressChan
}

// GetContext returns the validator's context (cancelled when Stop() is called)
// This allows callers to detect when the validator is stopping
func (av *AsyncValidator) GetContext() context.Context {
	return av.ctx
}

// notifyWorkers signals workers that tasks are available
// Uses non-blocking send to avoid deadlocks if all workers are busy
func (av *AsyncValidator) notifyWorkers() {
	select {
	case av.taskAvailable <- struct{}{}:
		// Successfully notified workers
	default:
		// Channel already has notification (workers already aware)
		// This is fine - workers will check queue when they wake up
	}
}

// wakeWorkerIfNeeded starts a worker if there's work and we're under max workers
func (av *AsyncValidator) wakeWorkerIfNeeded() {
	// Check if we need to start a worker
	currentWorkers := int(av.activeWorkers.Load())
	if currentWorkers >= av.maxWorkers {
		return // Already at max workers
	}

	// Try to start a worker (atomic check-and-set)
	if err := concurrency.RunInLockWithLogger(
		&av.workerMu,
		LockNameAsyncValidatorStartWorker,
		lockLoggerSystem(),
		func() error {
			// Double-check after acquiring lock
			currentWorkers = int(av.activeWorkers.Load())
			if currentWorkers >= av.maxWorkers {
				return nil
			}

			// Check if there's work to do
			if av.priorityQueue.Size() == 0 {
				return nil // No work available
			}

			// Start a new worker
			workerID := currentWorkers
			av.activeWorkers.Add(1)

			// Emit worker start event
			callback := getValidationLifecycleEventCallback()
			if callback != nil && av.projectRoot != emptyValue {
				var projectRoot string
				if err := concurrency.RunInRLockWithLogger(
					&av.mu,
					LockNameAsyncValidatorWorkerStartGetRoot,
					lockLoggerSystem(),
					func() error {
						projectRoot = av.projectRoot
						return nil
					},
				); err != nil {
					logging.Fluent(av.logger).Error(ConstMagic73205531, err).Log()
					// Continue anyway, projectRoot will be empty string which is handled by callback
				}
				// Use validator's context (or derived context) instead of creating new Background()
				callback(
					av.ctx,
					projectRoot,
					nil, // Storage provider not available in validation package
					fmt.Sprintf("worker_%d", workerID),
					"worker_start",
					"started",
					int(av.activeWorkers.Load()),
					0,
					0,
					0,
				)
			}

			av.wg.Add(1)
			av.workerStates.Store(workerID, "starting")
			workerName := fmt.Sprintf(ConstMagic883e5ec2, workerID)
			workerPurpose := fmt.Sprintf(ConstMagic384a6a4c, workerID)
			queueBud := goroutinelabels.DefaultBudget()
			queueWorkerBuilder := goroutinelabels.NewGoroutine(workerName, workerPurpose).
				WithWaitGroup(&av.wg)
			if queueBud != nil {
				queueWorkerBuilder = queueWorkerBuilder.WithBudget(queueBud)
			}
			queueWorkerBuilder.StartWithContext(av.ctx, func(ctx context.Context) error {
				av.worker(workerID)
				return nil
			})
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ConstMagicd8246578, err).Log()
	}
}

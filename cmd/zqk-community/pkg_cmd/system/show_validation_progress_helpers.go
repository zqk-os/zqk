package system

import (
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/validation"
)

// checkCacheForCompletion checks cache for objects that didn't send progress
// If queue is empty, always recalculate to ensure accurate count (prevents hangs)
func checkCacheForCompletion(vpc *ValidationProgressContext, queueEmpty bool, progressReceivedCount int) int {
	// If queue is empty, always recalculate cached count (don't use stale value)
	// This prevents hangs when completion check runs before all objects are validated
	if queueEmpty {
		completedCopy := vpc.copyCompletedObjectIDs()
		allStates := vpc.Validator.GetAllCachedStates()
		cachedStateMap := make(map[string]bool, len(allStates))
		for _, state := range allStates {
			cachedStateMap[state.ObjectID] = true
		}

		cachedCount := 0
		for objectID := range vpc.EnqueuedObjectIDs {
			hasProgress := completedCopy[objectID]
			if !hasProgress && cachedStateMap[objectID] {
				cachedCount++
			}
		}
		return cachedCount
	}

	// If queue not empty, only check once to avoid performance overhead
	if vpc.CompletionCheckDone {
		return 0
	}

	if progressReceivedCount < len(vpc.EnqueuedObjectIDs)*9/10 && !queueEmpty {
		return 0
	}

	completedCopy := vpc.copyCompletedObjectIDs()
	allStates := vpc.Validator.GetAllCachedStates()
	cachedStateMap := make(map[string]bool, len(allStates))
	for _, state := range allStates {
		cachedStateMap[state.ObjectID] = true
	}

	cachedCount := 0
	for objectID := range vpc.EnqueuedObjectIDs {
		hasProgress := completedCopy[objectID]
		if !hasProgress && cachedStateMap[objectID] {
			cachedCount++
		}
	}

	vpc.CompletionCheckDone = true
	return cachedCount
}

// queueEmptyResetThreshold: only clear QueueEmptySince when queue size exceeds this.
// Prevents transient 1–2 items (e.g. race or last task) from resetting the force-complete timer and causing a hang.
const queueEmptyResetThreshold = 5

// updateQueueEmptyTracking updates tracking for when queue becomes empty.
// queueSize is used so we only clear QueueEmptySince when queue is meaningfully non-empty (> threshold),
// avoiding reset on transient 0/1/0 flapping so force-complete can fire after queue-empty duration.
func updateQueueEmptyTracking(vpc *ValidationProgressContext, queueEmpty bool, queueSize int) {
	if queueEmpty && vpc.QueueEmptySince == nil {
		now := time.Now()
		vpc.QueueEmptySince = &now
		logging.Fluent(vpc.Logger).Debug("Queue became empty, starting fallback timeout").
			Completed(vpc.Completed).
			FailedOps(vpc.Failed).
			TotalTasks(vpc.TotalTasks).
			Log()
	} else if !queueEmpty && queueSize > queueEmptyResetThreshold {
		vpc.QueueEmptySince = nil
	}
}

// allTasksAccountedFor returns true when (progressCount + cachedCount) >= totalTasks.
// Used for cache-based completion so we don't stall when progress updates are missing but cache has results.
func allTasksAccountedFor(progressCount, cachedCount, totalTasks int) bool {
	return totalTasks > 0 && (progressCount+cachedCount) >= totalTasks
}

// calculateWaitTime calculates wait time based on cached count
func calculateWaitTime(vpc *ValidationProgressContext, cachedCount int) time.Duration {
	waitTime := 1 * time.Second
	if cachedCount > len(vpc.EnqueuedObjectIDs)/2 {
		waitTime = 200 * time.Millisecond
	}
	return waitTime
}

// countCachedObjects counts cached objects that were enqueued
func countCachedObjects(vpc *ValidationProgressContext, allStates []*validation.ValidationState, completedCopy map[string]bool) int {
	cachedStateMap := make(map[string]bool, len(allStates))
	for _, state := range allStates {
		cachedStateMap[state.ObjectID] = true
	}

	cachedCount := 0
	for objectID := range vpc.EnqueuedObjectIDs {
		hasProgress := completedCopy[objectID]
		if !hasProgress && cachedStateMap[objectID] {
			cachedCount++
		}
	}

	return cachedCount
}

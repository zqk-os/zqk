package system

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// handleTickerUpdate handles a ticker update during validation.
//
// Completion/stuck pattern (single source of truth):
// - Total work = len(EnqueuedObjectIDs). "Accounted" = processed (Completed+Failed) + cached (enqueued IDs with validator cache, no progress yet).
// - Progress = any increase in accounted count (from progress channel or cache). Stuck = no increase in accounted for N seconds and still work left.
// - Completion = queue empty AND accounted >= total. No magic percentages; use one tolerance (e.g. 1% or 200 missing) when validator stopped.
func handleTickerUpdate(vpc *ValidationProgressContext, ticker *time.Ticker, ctxTimeout context.Context) (bool, error) {
	_, _, _, queueSize := vpc.Validator.GetValidationStats()
	vpc.Metrics.UpdateQueueSize(queueSize)

	vpc.Mu.RLock()
	currentCompleted := vpc.Completed
	currentFailed := vpc.Failed
	progressReceivedCount := len(vpc.CompletedObjectIDs)
	vpc.Mu.RUnlock()

	currentProgressCount := currentCompleted + currentFailed
	queueEmpty := queueSize == 0
	updateQueueEmptyTracking(vpc, queueEmpty, queueSize)

	// Compute cached count before stuck check so "progress" includes cache (avoids declaring stuck when queue empty but cache has more results).
	shouldCheckCache := queueEmpty ||
		currentProgressCount >= vpc.TotalTasks*8/10 ||
		time.Since(vpc.LastProgressTime) > 5*time.Second
	cachedCount := 0
	if shouldCheckCache {
		cachedCount = checkCacheForCompletion(vpc, queueEmpty, progressReceivedCount)
	}
	totalAccounted := currentProgressCount + cachedCount

	// When queue has been empty for a long time but not all tasks accounted for, force-complete so the run finishes and reports (avoids indefinite hang). Shorter duration (15s) so the command doesn't hang; stuck check still runs with 25s when queue empty.
	const queueEmptyForceCompleteDuration = 15 * time.Second
	if queueEmpty && totalAccounted < vpc.TotalTasks && vpc.TotalTasks > 0 && vpc.QueueEmptySince != nil &&
		time.Since(*vpc.QueueEmptySince) >= queueEmptyForceCompleteDuration && !vpc.ForceComplete {
		vpc.ForceComplete = true
		logging.Fluent(vpc.Logger).Warn("Queue empty for extended period with unaccounted tasks - force-completing with current results").
			Accounted(totalAccounted).
			TotalTasks(vpc.TotalTasks).
			QueueEmptyDuration(time.Since(*vpc.QueueEmptySince).String()).
			Log()
		if vpc.Validator.IsRunning() {
			_ = vpc.Validator.Stop()
		}
	}

	// Stuck = no increase in accounted count for N seconds and still work left. Skip if we already force-completed.
	stuck, stuckErr := checkStuckCondition(vpc, currentCompleted, currentFailed, queueSize, totalAccounted)
	if !stuck {
		// Stuck detected - error will propagate up and exit process
		if stuckErr != nil {
			// Force stop validator before returning error
			_ = vpc.Validator.Stop() // Best-effort, don't wait
			return false, stuckErr
		}
		// Force stop validator before returning error
		_ = vpc.Validator.Stop() // Best-effort, don't wait
		return false, errfmt.Errorf("validation appears stuck")
	}

	// Completion: queue empty AND (processed + cached) >= total, OR force-complete (queue empty for long time).
	allTasksProcessed := queueEmpty && (vpc.TotalTasks == 0 || allTasksAccountedFor(currentProgressCount, cachedCount, vpc.TotalTasks) || vpc.ForceComplete)
	if queueEmpty && allTasksProcessed {
		percentComplete := float64(currentProgressCount+cachedCount) / float64(vpc.TotalTasks) * 100
		timeSinceProgress := time.Since(vpc.LastProgressTime)

		// Stop validator when queue is empty and (progress + cached) >= TotalTasks (completion).
		when.When(func() bool { return vpc.Validator.IsRunning() }).Then(func() {
			logging.Fluent(vpc.Logger).Info("Queue empty and all tasks accounted for - stopping validator to force completion detection").
				PercentComplete(fmt.Sprintf("%.1f%%", percentComplete)).
				ProgressCount(currentProgressCount).
				CachedCount(cachedCount).
				TotalTasks(vpc.TotalTasks).
				TimeSinceProgress(timeSinceProgress.String()).
				ActiveWorkers(vpc.Validator.GetWorkerCount()).
				Log()

			stopErr := vpc.Validator.Stop()
			when.When(func() bool { return stopErr == nil }).Then(func() {
				logging.Fluent(vpc.Logger).Debug("Validator stopped successfully - workers will exit immediately").
					ActiveWorkers(vpc.Validator.GetWorkerCount()).
					Log()
			}).OrElse(func() {
				logging.Fluent(vpc.Logger).Warn("Failed to stop validator for completion").WithError(stopErr).Log()
			}).Run()
		}).OrElse(func() {
			logging.Fluent(vpc.Logger).Debug("Queue empty, all tasks processed, validator already stopped - proceeding with completion check").
				PercentComplete(fmt.Sprintf("%.1f%%", percentComplete)).
				ProgressCount(currentProgressCount).
				TotalTasks(vpc.TotalTasks).
				TimeSinceProgress(timeSinceProgress.String()).
				Log()
		}).Run()
	}

	// Update progress display with total count (processed + cached); cachedCount already computed above when needed
	// Use currentProgressCount (processed) as base, add cached for display accuracy
	totalCompleted := currentProgressCount + cachedCount
	updateProgressDisplay(vpc, totalCompleted, queueSize, queueEmpty, currentProgressCount)

	// Emit progress events via coordinator (milestones and periodic updates)
	if vpc.ProjectRoot != emptyValue && vpc.StorageProvider != nil {
		profile := systemProfileHuman // Default
		if vpc.Ctx != nil && vpc.Ctx.Profile != emptyValue {
			profile = vpc.Ctx.Profile
		}

		// Calculate current percentage
		percent := int(float64(totalCompleted) / float64(vpc.TotalTasks) * 100)
		if percent > 100 {
			percent = 100
		}

		// Emit milestone events at 25%, 50%, 75%, and near-completion (90%, 95%, 99%)
		milestonePercent := 0
		if percent >= 25 && vpc.LastMilestonePercent < 25 {
			milestonePercent = 25
		} else if percent >= 50 && vpc.LastMilestonePercent < 50 {
			milestonePercent = 50
		} else if percent >= 75 && vpc.LastMilestonePercent < 75 {
			milestonePercent = 75
		} else if percent >= 90 && vpc.LastMilestonePercent < 90 {
			milestonePercent = 90
		} else if percent >= 95 && vpc.LastMilestonePercent < 95 {
			milestonePercent = 95
		} else if percent >= 99 && vpc.LastMilestonePercent < 99 {
			milestonePercent = 99
		}

		if milestonePercent > 0 {
			emitCheckMilestoneEvent(
				pkgctx.NewSystemContext(),
				vpc.ProjectRoot,
				vpc.StorageProvider,
				vpc.OperationID,
				fmt.Sprintf("%d%%", milestonePercent),
				totalCompleted,
				vpc.TotalTasks,
				currentCompleted,
				currentFailed,
				queueSize,
				profile,
			)
			vpc.LastMilestonePercent = milestonePercent
		}

		// Emit periodic progress updates (every 10% or every 5 seconds, whichever comes first)
		// This provides metrics without flooding audit logs
		now := time.Now()
		progressInterval := vpc.TotalTasks / 10
		if progressInterval < 1 {
			progressInterval = 1
		}
		if now.Sub(vpc.LastProgressTime) >= 5*time.Second || totalCompleted%progressInterval == 0 {
			emitCheckProgressEventViaCoordinator(
				pkgctx.NewSystemContext(),
				vpc.ProjectRoot,
				vpc.StorageProvider,
				vpc.OperationID,
				"progress",
				totalCompleted,
				vpc.TotalTasks,
				currentCompleted,
				currentFailed,
				queueSize,
				fmt.Sprintf("Progress: %d/%d (%.1f%%)", totalCompleted, vpc.TotalTasks, float64(totalCompleted)/float64(vpc.TotalTasks)*100),
				profile,
			)
		}
	}

	// Check completion
	completed, err := checkCompletion(vpc, queueSize)
	if completed || err != nil {
		return completed, err
	}

	// Check fallback timeout
	completed, err = handleFallbackTimeout(vpc)
	if completed || err != nil {
		return completed, err
	}

	return false, nil
}

// checkStuckCondition checks if validation appears stuck.
// totalAccounted = processed (Completed+Failed) + cached; progress = any increase in totalAccounted (so cache discoveries count).
// Returns (notStuck, error) - if not stuck, returns (true, nil). If stuck, returns (false, error).
func checkStuckCondition(vpc *ValidationProgressContext, currentCompleted, currentFailed, queueSize, totalAccounted int) (bool, error) {
	if vpc.ForceComplete {
		return true, nil // Force-complete already set (queue empty for long time) - do not declare stuck
	}
	totalEnqueued := len(vpc.EnqueuedObjectIDs)
	if totalEnqueued == 0 {
		totalEnqueued = vpc.TotalTasks
	}

	// Progress = any increase in accounted count (from progress channel or cache). Prevents "stuck" when queue empty but cache has more.
	if totalAccounted > vpc.LastProgressCount {
		vpc.LastProgressTime = time.Now()
		vpc.LastProgressCount = totalAccounted
		return true, nil // Not stuck - progress detected
	}

	// Queue has work but no progress - check how long
	stuckThreshold := 20 * time.Second
	if os.Getenv(zqkenv.TestRoot()) != "" {
		stuckThreshold = 180 * time.Second
	}
	if queueSize > 0 && totalAccounted == vpc.LastProgressCount {
		if time.Since(vpc.LastProgressTime) > stuckThreshold {
			return false, errfmt.Errorf("validation appears stuck: queue has %d items but no progress for %v (accounted: %d/%d)",
				queueSize, time.Since(vpc.LastProgressTime), totalAccounted, totalEnqueued)
		}
	}

	// When queue is empty and we're not done, allow some grace before declaring stuck (discovery batching, cache drain) but keep it short so the command doesn't hang.
	effectiveStuckTimeout := vpc.StuckTimeout
	if queueSize == 0 && totalAccounted < totalEnqueued && totalEnqueued > 0 {
		effectiveStuckTimeout = 25 * time.Second
		if os.Getenv(zqkenv.TestRoot()) != "" {
			effectiveStuckTimeout = 180 * time.Second
		}
	}
	if time.Since(vpc.LastProgressTime) > effectiveStuckTimeout {
		stuckDuration := time.Since(vpc.LastProgressTime)
		queueEmptyDuration := ""
		if vpc.QueueEmptySince != nil {
			queueEmptyDuration = time.Since(*vpc.QueueEmptySince).String()
		}
		_, _, _, validatorQueueSize := vpc.Validator.GetValidationStats()

		// Log object IDs still in queue to identify stuck objects
		pendingIDs := vpc.Validator.GetPendingObjectIDs()
		const maxPendingLog = 30
		pendingSummary := ""
		if len(pendingIDs) > 0 {
			toLog := pendingIDs
			if len(toLog) > maxPendingLog {
				toLog = toLog[:maxPendingLog]
			}
			pendingSummary = strings.Join(toLog, ", ")
			if len(pendingIDs) > maxPendingLog {
				pendingSummary += fmt.Sprintf(" ... and %d more", len(pendingIDs)-maxPendingLog)
			}
		}

		logging.Fluent(vpc.Logger).Warn("No progress detected for extended period - possible deadlock").
			StuckDuration(stuckDuration.String()).
			Completed(currentCompleted).
			FailedOps(currentFailed).
			TotalAccounted(totalAccounted).
			TotalEnqueued(totalEnqueued).
			QueueSize(queueSize).
			ValidatorQueueSize(validatorQueueSize).
			QueueEmptyDuration(queueEmptyDuration).
			PendingInQueue(pendingSummary).
			DiagNote("Validation goroutines may be stuck waiting on locks (spec loader)").
			Log()

		// Dump goroutine stacks when stuck (using goroutine labels for identification)
		dumpGoroutineStacksOnStuck(vpc, stuckDuration)

		// Emit stuck event via coordinator
		if vpc.ProjectRoot != emptyValue && vpc.StorageProvider != nil {
			profile := systemProfileHuman // Default
			if vpc.Ctx != nil && vpc.Ctx.Profile != emptyValue {
				profile = vpc.Ctx.Profile
			}
			emitCheckStuckEvent(
				pkgctx.NewSystemContext(),
				vpc.ProjectRoot,
				vpc.StorageProvider,
				vpc.OperationID,
				totalAccounted,
				totalEnqueued,
				currentCompleted,
				currentFailed,
				queueSize,
				stuckDuration,
				profile,
			)
		}

		// CRITICAL: Always stop validator when stuck detected to free resources
		logging.Fluent(vpc.Logger).Warn("Validation stuck detected - forcing validator stop").
			Accounted(totalAccounted).
			TotalEnqueued(totalEnqueued).
			QueueSize(queueSize).
			ValidatorQueueSize(validatorQueueSize).
			StuckDuration(stuckDuration.String()).
			Log()

		stopCtx, stopCancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
		goroutinelabels.NewGoroutine("system", "force validator stop").
			StartSimple(func() {
				defer stopCancel()
				if err := vpc.Validator.Stop(); err != nil {
					logging.Fluent(vpc.Logger).Warn("Failed to stop validator on stuck detection").WithError(err).Log()
				}
			})

		select {
		case <-stopCtx.Done():
		case <-time.After(1 * time.Second):
		}

		errMsg := fmt.Sprintf("validation stuck: no progress for %v (accounted: %d/%d, queue: %d). Validator stopped to prevent further hangs",
			stuckDuration, totalAccounted, totalEnqueued, queueSize)
		if pendingSummary != emptyValue {
			errMsg += ". Pending in queue: " + pendingSummary
		}
		return false, errfmt.Errorf("%s", errMsg)
	}

	return true, nil // Not stuck
}

// validationProgressHeartbeatInterval is the maximum time between progress lines when count has not changed.
// Keeps notifications flowing so the command never appears hung (e.g. during slow validation of 14k+ objects).
const validationProgressHeartbeatInterval = 5 * time.Second

// updateProgressDisplay updates the progress display with descriptive status
// totalCompleted should include both progress updates and cached objects for accurate percentage.
// Prints a new line when count/queue changes or at least every validationProgressHeartbeatInterval (heartbeat).
func updateProgressDisplay(vpc *ValidationProgressContext, totalCompleted, queueSize int, queueEmpty bool, processedCount int) {
	countChanged := totalCompleted != vpc.LastPrintedCompleted || queueSize != vpc.LastPrintedQueueSize
	heartbeatDue := time.Since(vpc.LastPrintedTime) >= validationProgressHeartbeatInterval
	if countChanged {
		vpc.LastPrintedCompleted = totalCompleted
		vpc.LastPrintedQueueSize = queueSize
	}
	if !countChanged && !heartbeatDue {
		return
	}
	vpc.LastPrintedTime = time.Now()

	// Skip progress bar calculation and display if suppressed
	if vpc.SuppressProgress {
		return
	}

	percent := float64(totalCompleted) / float64(vpc.TotalTasks) * 100
	// Cap percentage at 100% to avoid showing >100% if count slightly exceeds total
	if percent > 100 {
		percent = 100
	}
	barLength := int(percent / 2)
	bar := ""
	for i := 0; i < 50; i++ {
		when.When(func() bool { return i < barLength }).Then(func() {
			bar += "="
		}).OrElse(func() {
			bar += " "
		}).Run()
	}

	// Throughput (objects/sec) for efficiency observability
	elapsed := time.Since(vpc.StartTime)
	throughputStr := ""
	if elapsed.Seconds() >= 1 && totalCompleted > 0 {
		rate := float64(totalCompleted) / elapsed.Seconds()
		when.When(func() bool { return rate >= 1000 }).Then(func() {
			throughputStr = fmt.Sprintf(" %.1fk/s", rate/1000)
		}).OrElse(func() {
			throughputStr = fmt.Sprintf(" %.0f/s", rate)
		}).Run()
	}

	// Add descriptive status to show what work is happening
	// Include worker count and active goroutines for better visibility
	status := ""
	workerCount := vpc.Validator.GetWorkerCount()
	// Note: getActiveGoroutines() is defined in async_check.go
	activeGoroutines := int(getActiveGoroutines())

	when.When(func() bool { return !queueEmpty }).Then(func() {
		// Queue has work - show active validation status
		status = fmt.Sprintf(" (validating, workers: %d, goroutines: %d)", workerCount, activeGoroutines)
	}).OrElseWhen(func() bool { return processedCount >= vpc.TotalTasks }).Then(func() {
		if queueEmpty {
			status = " (completed)"
		} else {
			status = fmt.Sprintf(" (completing, workers: %d)", workerCount)
		}
	}).OrElseWhen(func() bool { return !vpc.Validator.IsRunning() }).Then(func() {
		// Validator stopped - finalizing results
		status = fmt.Sprintf(" (finalizing, workers: %d)", workerCount)
	}).OrElseWhen(func() bool { return time.Since(vpc.LastProgressTime) < 5*time.Second }).Then(func() {
		status = fmt.Sprintf(" (processing results, workers: %d)", workerCount)
	}).OrElseWhen(func() bool { return time.Since(vpc.LastProgressTime) < 15*time.Second }).Then(func() {
		status = fmt.Sprintf(" (finalizing, workers: %d)", workerCount)
	}).OrElse(func() {
		status = fmt.Sprintf(" (waiting for workers: %d, goroutines: %d)", workerCount, activeGoroutines)
	}).Run()

	// POLICY-CODE-007: route progress through logger, not direct writer
	msg := fmt.Sprintf("Progress: [%s] %.1f%% (%d/%d, queue: %d)%s%s - %s", bar, percent, totalCompleted, vpc.TotalTasks, queueSize, throughputStr, status, time.Since(vpc.StartTime).Round(time.Second))
	logging.Fluent(vpc.Logger).Info(msg).Log()
	// Also send to terminal so validation phase doesn't appear hung (heartbeat every 5s or when count changes)
	if vpc.OutputQueue != nil && !vpc.SuppressProgress {
		_ = vpc.OutputQueue.EnqueueStderr(msg+"\n", false) //nolint:errcheck // best-effort
	}
}

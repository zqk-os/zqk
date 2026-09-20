package system

// : real-time progress feedback across discovery and aggregation phases

import (
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

const (
	validationCompletionDelay           = 10 * time.Millisecond
	validationCompletionLineFmt         = "\nValidation completed: %d/%d objects (%d succeeded, %d failed)\n"
	validationPercentFmt                = "%.1f%%"
	validationFlagVerbose               = "verbose"
	validationFlagAutoFixScheduler      = "auto-fix-scheduler"
	validationAutoFixBatchThreshold     = 10
	validationWarnBatchAutoFixFailed    = "Failed to batch auto-fix issues for scheduler"
	validationInfoAutoFixSubmitted      = "Auto-fix batches submitted to scheduler"
	validationDebugQueueAllAccounted    = "Queue empty and all objects accounted for - proceeding with completion"
	validationDebugQueueTolerance       = "Queue empty with tolerance - proceeding with completion"
	validationToleranceDivisor          = 100
	validationToleranceMax              = 200
	validationQueueEmptyExtendedSeconds = 20
)

// enqueueOrPrintStderr writes a terminal line through the validation output
// queue when one is attached so Progress, metrics, and the completion line
// stay in FIFO order. Metrics used to fmt.Fprintf stderr directly, which let
// a queued Progress line drain after the summary.
func enqueueOrPrintStderr(vpc *ValidationProgressContext, msg string, flush bool) {
	if vpc == nil {
		return
	}
	if vpc.OutputQueue != nil {
		_ = vpc.OutputQueue.EnqueueStderr(msg, flush) //nolint:errcheck // best-effort
		return
	}
	if vpc.Cmd != nil {
		fmt.Fprint(vpc.Cmd.ErrOrStderr(), msg)
	}
}

// emitValidationFinishTrailer prints the last Progress line, then verbose
// metrics, then "Validation completed". All three go through the same writer
// so a late Progress cannot land after the summary. SuppressProgress is set
// after the final Progress line so the 1s ticker cannot append another.
// The ticker never prints 100% (completed); this trailer owns that snapshot.
// If a prior path already printed this exact 100% line, skip a duplicate.
func emitValidationFinishTrailer(vpc *ValidationProgressContext, actualValidatedCount, failed int) {
	if vpc == nil || vpc.SuppressProgress {
		return
	}

	alreadyPrintedFinal := vpc.LastPrintedCompleted >= actualValidatedCount &&
		vpc.LastPrintedQueueSize == 0 &&
		validationProgressIsFinalSnapshot(true, actualValidatedCount, vpc.TotalTasks)
	if !alreadyPrintedFinal {
		writeValidationProgressLine(vpc, actualValidatedCount, 0, true, actualValidatedCount, true)
	}
	vpc.SuppressProgress = true

	if vpc.Metrics != nil {
		vpc.Metrics.Finalize()
	}

	if vpc.Cmd != nil {
		verbose, err := vpc.Cmd.Flags().GetBool(validationFlagVerbose)
		if err == nil && verbose {
			metricsText := ""
			if vpc.Metrics != nil {
				metricsText = vpc.Metrics.String()
			}
			enqueueOrPrintStderr(vpc, "\n"+metricsText+"\n", true)
			logSpecLoaderMetrics(vpc.Logger)
		}
	}

	msg := fmt.Sprintf(validationCompletionLineFmt,
		actualValidatedCount, vpc.TotalTasks, actualValidatedCount-failed, failed)
	enqueueOrPrintStderr(vpc, msg, true)
	enqueueOrPrintStderr(vpc, "Aggregating validation layers and checking CAS membrane...\n", true)

	if vpc.ProjectRoot != emptyValue && vpc.StorageProvider != nil {
		profile := systemProfileHuman
		if vpc.Ctx != nil && vpc.Ctx.Profile != emptyValue {
			profile = vpc.Ctx.Profile
		}
		emitCheckAggregationEvent(
			pkgctx.NewSystemContext(),
			vpc.ProjectRoot,
			vpc.StorageProvider,
			vpc.OperationID,
			profile,
		)
	}
}

// handleValidationCompletion handles successful validation completion
func handleValidationCompletion(vpc *ValidationProgressContext) error {
	// Deterministic delay (not completion-wait): allow in-flight cache writers to settle.
	time.Sleep(validationCompletionDelay)

	vpc.Metrics.RecordValidation(time.Since(vpc.ValidationStart))
	collectionStart := time.Now()

	finalFailedObjectIDs := vpc.copyFailedObjectIDs()
	results := collectResultsForCompletion(vpc, finalFailedObjectIDs)

	// Count actual validated objects from results (includes cached states)
	actualValidatedCount := len(results)
	vpc.Mu.RLock()
	finalFailed := vpc.Failed
	vpc.Mu.RUnlock()

	vpc.Metrics.RecordCollection(time.Since(collectionStart))

	if vpc.OutputQueue != nil {
		for vpc.OutputQueue.Size() > 0 {
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
	}

	emitValidationFinishTrailer(vpc, actualValidatedCount, finalFailed)

	// Emit completion milestone event via coordinator
	if vpc.ProjectRoot != emptyValue && vpc.StorageProvider != nil {
		profile := systemProfileHuman // Default
		if vpc.Ctx != nil && vpc.Ctx.Profile != emptyValue {
			profile = vpc.Ctx.Profile
		}
		duration := time.Since(vpc.ValidationStart)
		emitCheckCompletionEvent(
			pkgctx.NewSystemContext(),
			vpc.ProjectRoot,
			vpc.StorageProvider,
			vpc.OperationID,
			vpc.TotalTasks,
			actualValidatedCount-finalFailed,
			finalFailed,
			duration,
			profile,
		)

		// Emit high-level operation completion via OperationCallback
		opCallback := coordination.NewCoordinatorOperationCallback(
			pkgctx.NewSystemContext(),
			vpc.ProjectRoot,
			vpc.StorageProvider,
			"system_check",
			profile,
		)
		opCallback.OnComplete(vpc.OperationID, nil, duration)
	}

	// Handle auto-fix batching if enabled (before outputting results)
	if shouldAutoFix(vpc.Cmd) {
		projectRoot := vpc.ProjectRoot
		if projectRoot == emptyValue && vpc.Ctx != nil {
			projectRoot = vpc.Ctx.ProjectRoot
		}
		projectRoot = ProjectRootOrResolve(projectRoot)

		// Stale CAS cleanup: resolve "Stale CAS version" in one run (INTEGRITY_RESOLUTION_PLAN)
		staleCASResult := RunStaleCASCleanupForResults(projectRoot, results, vpc.Logger)

		// Resource hygiene auto-fix: reap stale locks, orphaned temp files, and enforce log limits
		_, _ = resourcehygiene.ExecuteHygiene(projectRoot, resourcehygiene.DefaultHygieneOptions())

		// Check if we should use scheduler batching
		useSchedulerBatching := true
		if flag, err := vpc.Cmd.Flags().GetBool(validationFlagAutoFixScheduler); err == nil {
			useSchedulerBatching = flag
		}

		if useSchedulerBatching {
			// Count fixable issues (AutoFixable, Tier 4, or with FixCommand; same as CollectAutoFixableIssues)
			autoFixableCount := 0
			for _, result := range results {
				for _, issue := range result.Issues {
					if IsIssueFixableForBatch(issue) {
						autoFixableCount++
					}
				}
			}

			// Use scheduler batching if we have enough issues (threshold: 10)
			if autoFixableCount >= validationAutoFixBatchThreshold {
				if err := handleAutoFixBatching(vpc.Ctx, vpc.Cmd, projectRoot, results); err != nil {
					logging.Fluent(vpc.Logger).Warn(validationWarnBatchAutoFixFailed).WithError(err).Log()
				} else {
					logging.Fluent(vpc.Logger).Info(validationInfoAutoFixSubmitted).
						AutoFixableCount(autoFixableCount).
						Log()
				}
			}
		}

		results = filterResultsByTierIfNeeded(vpc.Cmd, results)
		return outputResults(vpc.Cmd, vpc.Ctx, results, nil, &staleCASResult, nil)
	}

	results = filterResultsByTierIfNeeded(vpc.Cmd, results)
	return outputResults(vpc.Cmd, vpc.Ctx, results, nil, nil, nil)
}

// collectResultsForCompletion collects results when validation completes
// CRITICAL: Must iterate over ALL enqueued objects, not just cached states
// This ensures we report results for all discovered objects, not just those with cached validation states
func collectResultsForCompletion(vpc *ValidationProgressContext, failedCopy map[string]string) []CheckResult {
	// Iterate over ALL enqueued objects to ensure we report results for everything
	// Use GetCachedState for each object to get the most up-to-date state (may include recently validated objects)
	results := make([]CheckResult, 0, len(vpc.EnqueuedObjectIDs))
	missingStateCount := 0

	for objectID := range vpc.EnqueuedObjectIDs {
		// Try to get cached state for this object (may return stale states, but that's OK for reporting)
		state, hasState := vpc.Validator.GetCachedState(objectID)

		var result CheckResult
		if hasState && state != nil {
			// Object has cached validation state - use it
			result = convertValidationStateToCheckResult(state)
		} else if vpc.CachedCheckResults != nil {
			// Cache-hit snapshot from enqueue time (survives GetCachedState miss / maxAge).
			if snap, ok := vpc.CachedCheckResults[objectID]; ok {
				result = snap
			}
		}
		if result.ObjectID == emptyValue {
			// Object was enqueued but doesn't have cached state or snapshot
			// This can happen if:
			// 1. Object kind is excluded from validation cache (e.g. scheduler_health_metric, audit_event)
			// 2. Object was skipped due to cache hit but cache entry expired and no snapshot was taken
			// 3. Validation didn't complete for this object
			// Create a minimal result; use EnqueuedObjectInfo for kind/path when available
			kind, filePath := "", ""
			if vpc.EnqueuedObjectInfo != nil {
				if info, ok := vpc.EnqueuedObjectInfo[objectID]; ok {
					kind, filePath = info.Kind, info.FilePath
				}
			}
			result = CheckResult{
				ObjectID:   objectID,
				ObjectKind: kind,
				FilePath:   filePath,
				Issues:     []Issue{},
			}
			// Only count as "missing" when we would expect this object to be cached.
			// Cache-excluded kinds (scheduler_health_metric, audit_event, etc.) are never stored, so don't warn.
			if kind == emptyValue || validation.ShouldCacheValidationState(kind) {
				missingStateCount++
				// Pending CAS→cache true-up: soft-exclude instead of Tier-1 integrity hard fail.
				projectRoot := vpc.ProjectRoot
				if projectRoot == emptyValue && vpc.Ctx != nil {
					projectRoot = vpc.Ctx.ProjectRoot
				}
				projectRoot = ProjectRootOrResolve(projectRoot)
				if storage.IsObjectIDCachePending(projectRoot, objectID) {
					result.Issues = append(result.Issues, Issue{
						Tier:     3,
						Category: categoryCacheCoherence,
						Message:  "Excluded from blocking check: validation state missing while object-id-cache pending after CAS mutation; re-run after EnsureObjectIDCacheReady",
					})
				} else {
					// Summary-time miss is cache coherence, not CAS. Category integrity
					// is counted as Layer 0 and made SCH-autofix-run look like hash mismatch.
					result.Issues = append(result.Issues, Issue{
						Tier:     3,
						Category: categoryCacheCoherence,
						Message:  "validation state missing at summary time (cache miss after enqueue; re-run with --force or --clear-cache)",
					})
				}
			}
		}

		// Add validation error if object failed
		if errMsg, ok := failedCopy[objectID]; ok {
			if errMsg == "" {
				errMsg = "Validation failed - see logs for details"
			}
			category := "validation_error"
			tier := 1
			// Per-object budget expiry is infrastructure under fan-out, not an object
			// defect. Do not block system check; re-run or rely on prior cache retention.
			if strings.Contains(errMsg, "validation timeout after") {
				tier = 3
				category = "validation_timeout"
			} else if strings.Contains(errMsg, "Missing CAS file") ||
				strings.Contains(errMsg, "no such file or directory") ||
				strings.Contains(errMsg, "failed to read file") ||
				strings.Contains(errMsg, "integrity") {
				category = "integrity"
			}
			result.Issues = append(result.Issues, Issue{
				Tier:     tier,
				Category: category,
				Message:  errMsg,
			})
		}

		// Emit auto-fix events via coordinator (for debugging async auto-fix)
		// Also emit warning if metadata exists but AutoFixed is empty (indicates conversion issue)
		if hasState && state != nil && state.Metadata != nil {
			if autoFixedStr, ok := state.Metadata["auto_fixed"]; ok && autoFixedStr != emptyValue {
				operationID := fmt.Sprintf("auto_fix_%s_%d", result.ObjectID, time.Now().UnixNano())
				if len(result.AutoFixed) == 0 {
					// Conversion issue - metadata has auto_fixed but result doesn't
					emitAutoFixConversionIssueViaCoordinator(
						pkgctx.NewSystemContext(), vpc.ProjectRoot, vpc.StorageProvider, operationID,
						result.ObjectID, result.ObjectKind, autoFixedStr, vpc.Ctx.Profile,
					)
				} else {
					// Successfully collected auto-fix results
					emitAutoFixCollectedViaCoordinator(
						pkgctx.NewSystemContext(), vpc.ProjectRoot, vpc.StorageProvider, operationID,
						result.ObjectID, result.ObjectKind, result.AutoFixed, vpc.Ctx.Profile,
					)
				}
			}
		}

		results = append(results, result)

		for _, issue := range result.Issues {
			vpc.Metrics.RecordTierIssue(issue.Tier)
		}
	}

	// Log warning if many objects are missing cached states (indicates validation may not have completed)
	if missingStateCount > 0 && vpc.Logger != nil {
		missingPercent := float64(missingStateCount) / float64(len(vpc.EnqueuedObjectIDs)) * 100
		logging.Fluent(vpc.Logger).Warn("Some enqueued objects missing cached states").
			MissingCount(missingStateCount).
			TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
			MissingPercent(fmt.Sprintf(validationPercentFmt, missingPercent)).
			Log()
	}

	return results
}

// checkCompletion checks if validation is complete
func checkCompletion(vpc *ValidationProgressContext, queueSize int) (bool, error) {
	vpc.Mu.RLock()
	progressReceivedCount := len(vpc.CompletedObjectIDs)
	currentCompleted := vpc.Completed
	currentFailed := vpc.Failed
	vpc.Mu.RUnlock()

	queueEmpty := queueSize == 0
	// CRITICAL: Always check cache when checking completion
	// This ensures we account for cached validation results, not just progress updates
	cachedCount := checkCacheForCompletion(vpc, queueEmpty, progressReceivedCount)

	// Use processed count (currentCompleted + currentFailed) for completion check
	// This matches what the display shows and handles edge cases better
	currentProgressCount := currentCompleted + currentFailed
	totalAccountedFor := currentProgressCount + cachedCount

	// Force-complete: queue was empty for extended period with unaccounted tasks; proceed with current results
	if vpc.ForceComplete && queueEmpty {
		return handleFinalCompletion(vpc, cachedCount)
	}

	// EDGE CASE: If we have MORE than expected (e.g., 14366/14365), still trigger completion
	// This can happen when cached states include objects not in EnqueuedObjectIDs
	// or when objects are processed multiple times
	allObjectsAccountedFor := totalAccountedFor >= len(vpc.EnqueuedObjectIDs)

	updateQueueEmptyTracking(vpc, queueEmpty, queueSize)

	if queueEmpty {
		if allObjectsAccountedFor {
			// All objects accounted for (or more than expected) - proceed with completion
			logging.Fluent(vpc.Logger).Debug(validationDebugQueueAllAccounted).
				ProgressCount(currentProgressCount).
				CachedCount(cachedCount).
				TotalAccounted(totalAccountedFor).
				TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
				ProgressReceived(progressReceivedCount).
				Log()
			return handleFinalCompletion(vpc, cachedCount)
		}

		// If queue is empty but objects aren't all accounted for, check tolerance and time since queue empty
		// This handles timing issues where validation is in-flight but progress hasn't been sent yet
		// Also handles cases where some objects were filtered out or skipped during validation
		missingCount := len(vpc.EnqueuedObjectIDs) - totalAccountedFor
		percentAccountedFor := float64(totalAccountedFor) / float64(len(vpc.EnqueuedObjectIDs)) * 100

		// Increased tolerance: allow up to 1% missing or 200 objects (whichever is smaller)
		// This prevents hangs when some objects are legitimately skipped or filtered
		tolerance := len(vpc.EnqueuedObjectIDs) / validationToleranceDivisor // 1% tolerance
		if tolerance > validationToleranceMax {
			tolerance = validationToleranceMax // Cap at 200 objects
		}

		// EDGE CASE: If queue empty for extended period (>20s) and validator stopped, proceed with completion
		// This handles cases where workers are stuck or objects were filtered
		timeSinceQueueEmpty := time.Since(vpc.LastProgressTime)
		validatorStopped := !vpc.Validator.IsRunning()
		activeWorkers := vpc.Validator.GetWorkerCount()

		if missingCount <= tolerance {
			if activeWorkers > 0 {
				// Workers are still actively processing the remaining items - DO NOT STOP!
				logging.Fluent(vpc.Logger).Debug("Queue empty and missing count within tolerance, but workers still active - waiting").
					ProgressCount(currentProgressCount).
					CachedCount(cachedCount).
					TotalAccounted(totalAccountedFor).
					TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
					MissingCount(missingCount).
					ActiveWorkers(activeWorkers).
					Log()
				return false, nil
			}
			// Small number of objects unaccounted for - likely timing issue or filtered objects
			// and no workers are active to process them. Proceed with completion.
			logging.Fluent(vpc.Logger).Debug(validationDebugQueueTolerance).
				ProgressCount(currentProgressCount).
				CachedCount(cachedCount).
				TotalAccounted(totalAccountedFor).
				TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
				MissingCount(missingCount).
				Tolerance(tolerance).
				String("percent_accounted", fmt.Sprintf(validationPercentFmt, percentAccountedFor)).
				ProgressReceived(progressReceivedCount).
				Log()
			return handleFinalCompletion(vpc, cachedCount)
		}

		// Beyond tolerance but queue empty for extended period and validator stopped - proceed anyway
		// This prevents infinite hangs when completion detection is imperfect
		if validatorStopped && timeSinceQueueEmpty > validationQueueEmptyExtendedSeconds*time.Second && percentAccountedFor > validationQueueEmptyExtendedSeconds {
			// Validator stopped, queue empty for 20s+, and >20% accounted for - proceed with completion
			// This handles cases where many objects were filtered/skipped or workers were stuck
			logging.Fluent(vpc.Logger).Warn("Queue empty for extended period with validator stopped - proceeding with completion (prevents hang)").
				ProgressCount(currentProgressCount).
				CachedCount(cachedCount).
				TotalAccounted(totalAccountedFor).
				TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
				MissingCount(missingCount).
				String("percent_accounted", fmt.Sprintf(validationPercentFmt, percentAccountedFor)).
				String("time_since_queue_empty", timeSinceQueueEmpty.String()).
				ProgressReceived(progressReceivedCount).
				Log()
			return handleFinalCompletion(vpc, cachedCount)
		}

		// Queue empty but beyond tolerance and validator still running - log for debugging
		logging.Fluent(vpc.Logger).Debug("Queue empty but objects not accounted for - waiting").
			ProgressCount(currentProgressCount).
			CachedCount(cachedCount).
			TotalAccounted(totalAccountedFor).
			TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
			MissingCount(missingCount).
			String("percent_accounted", fmt.Sprintf(validationPercentFmt, percentAccountedFor)).
			String("time_since_progress", timeSinceQueueEmpty.String()).
			Bool("validator_stopped", validatorStopped).
			ProgressReceived(progressReceivedCount).
			Log()
	}

	return false, nil
}

// handleFinalCompletion handles final completion detection and result collection
func handleFinalCompletion(vpc *ValidationProgressContext, cachedCount int) (bool, error) {
	// Wait for validation goroutines to complete (they may still be running even though queue is empty)
	// This ensures all validation work is finished before declaring completion
	validationTimeout := 5 * time.Second
	if !vpc.Validator.WaitForValidationCompletion(validationTimeout) {
		logging.Fluent(vpc.Logger).Debug("Validation goroutines still running after timeout, proceeding anyway").Log()
	}

	_, _, _, finalQueueSize := vpc.Validator.GetValidationStats()
	vpc.Mu.RLock()
	finalProgressReceivedCount := len(vpc.CompletedObjectIDs)
	finalCurrentCompleted := vpc.Completed
	finalCurrentFailed := vpc.Failed
	vpc.Mu.RUnlock()

	finalCompletedCopy := vpc.copyCompletedObjectIDs()
	finalAllStates := vpc.Validator.GetAllCachedStates()
	finalCachedCount := countCachedObjects(vpc, finalAllStates, finalCompletedCopy)

	// Use processed count for consistency with display and completion check
	finalProgressCount := finalCurrentCompleted + finalCurrentFailed
	finalTotalAccountedFor := finalProgressCount + finalCachedCount

	// EDGE CASE: If we have MORE than expected, still proceed with completion
	// This can happen when cached states include objects not in EnqueuedObjectIDs
	finalAllObjectsAccountedFor := finalTotalAccountedFor >= len(vpc.EnqueuedObjectIDs)

	// If queue is empty, proceed with completion even if count doesn't perfectly match
	// This prevents hangs when cached states don't perfectly align with enqueued objects
	// (e.g., due to cache expiration, state filtering, or timing issues)
	if finalQueueSize == 0 {
		if finalAllObjectsAccountedFor {
			// All objects accounted for (or more than expected) - proceed
			logging.Fluent(vpc.Logger).Debug(validationDebugQueueAllAccounted).
				ProgressCount(finalProgressCount).
				CachedCount(finalCachedCount).
				TotalAccounted(finalTotalAccountedFor).
				TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
				ProgressReceived(finalProgressReceivedCount).
				Log()
			return collectAndOutputFinalResults(vpc)
		}
		// Queue is empty but count doesn't match - check tolerance before proceeding
		missingCount := len(vpc.EnqueuedObjectIDs) - finalTotalAccountedFor
		tolerance := len(vpc.EnqueuedObjectIDs) / validationToleranceDivisor // 1% tolerance
		if tolerance > validationToleranceMax {
			tolerance = validationToleranceMax // Cap at 200 objects
		}

		if missingCount <= tolerance {
			// Within tolerance - proceed with completion
			logging.Fluent(vpc.Logger).Debug(validationDebugQueueTolerance).
				ProgressCount(finalProgressCount).
				CachedCount(finalCachedCount).
				TotalAccounted(finalTotalAccountedFor).
				TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
				MissingCount(missingCount).
				Tolerance(tolerance).
				ProgressReceived(finalProgressReceivedCount).
				Log()
			return collectAndOutputFinalResults(vpc)
		}

		// Beyond tolerance but queue is empty - log warning and proceed anyway
		// This prevents infinite hangs when completion detection is imperfect

		logging.Fluent(vpc.Logger).Warn("Queue empty but object count mismatch beyond tolerance - proceeding with completion anyway (prevents hang)").
			ProgressCount(finalProgressCount).
			CachedCount(finalCachedCount).
			TotalAccounted(finalTotalAccountedFor).
			TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
			Int("expected_total", finalTotalAccountedFor).
			MissingCount(missingCount).
			Tolerance(tolerance).
			ProgressReceived(finalProgressReceivedCount).
			Log()
		return collectAndOutputFinalResults(vpc)
	}

	return false, nil
}

// collectAndOutputFinalResults collects and outputs final validation results
func collectAndOutputFinalResults(vpc *ValidationProgressContext) (bool, error) {
	if err := vpc.Validator.Stop(); err != nil {
		logging.Fluent(vpc.Logger).Warn("Failed to stop validator on completion").WithError(err).Log()
	}
	runIssues := &ValidationRunIssues{ForceComplete: vpc.ForceComplete}
	ri := vpc.Validator.GetRunIssues()
	runIssues.WorkerStopTimedOut = ri.WorkerStopTimedOut
	runIssues.CacheSaveTimedOut = ri.CacheSaveTimedOut

	select {
	case <-vpc.DrainDone:
	case <-time.After(5 * time.Second):
		logging.Fluent(vpc.Logger).Warn("Timeout waiting for drain goroutine (proceeding anyway)").Log()
	}

	vpc.Metrics.RecordValidation(time.Since(vpc.ValidationStart))
	collectionStart := time.Now()

	failedCopy := vpc.copyFailedObjectIDs()

	// Calculate actual validated count using the same logic as completion detection
	// This accounts for both progress updates and cached states
	vpc.Mu.RLock()
	progressReceivedCount := len(vpc.CompletedObjectIDs)
	vpc.Mu.RUnlock()

	// Count cached objects that were enqueued but didn't send progress
	allStates := vpc.Validator.GetAllCachedStates()
	cachedStateMap := make(map[string]bool, len(allStates))
	for _, state := range allStates {
		cachedStateMap[state.ObjectID] = true
	}

	completedCopy := vpc.copyCompletedObjectIDs()
	cachedCount := 0
	for objectID := range vpc.EnqueuedObjectIDs {
		hasProgress := completedCopy[objectID]
		if !hasProgress && cachedStateMap[objectID] {
			cachedCount++
		}
	}

	actualValidatedCount := progressReceivedCount + cachedCount
	vpc.Mu.RLock()
	finalCountFailed := vpc.Failed
	vpc.Mu.RUnlock()

	results := collectResultsForCompletion(vpc, failedCopy)

	// Hand-CAS / full-kind CAS re-peek was wired into every check completion by
	// 6526fdeb0f. That O(all hash YAML) scan after
	// validation is already complete made system check wall-time unacceptable and
	// piled heat onto the scheduler when night-duty fired checks in a loop.
	// Duplicate-ID coverage remains via checkDuplicateIDs + object ID cache.
	// reintroduce as opt-in / index-backed only.

	vpc.Metrics.RecordCollection(time.Since(collectionStart))

	if vpc.OutputQueue != nil {
		for vpc.OutputQueue.Size() > 0 {
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
	}

	emitValidationFinishTrailer(vpc, actualValidatedCount, finalCountFailed)

	// Handle auto-fix batching if enabled (before outputting results)
	if shouldAutoFix(vpc.Cmd) {
		projectRoot := vpc.ProjectRoot
		if projectRoot == emptyValue && vpc.Ctx != nil {
			projectRoot = vpc.Ctx.ProjectRoot
		}
		projectRoot = ProjectRootOrResolve(projectRoot)

		// Stale CAS cleanup: resolve "Stale CAS version" in one run (INTEGRITY_RESOLUTION_PLAN)
		staleCASResult := RunStaleCASCleanupForResults(projectRoot, results, vpc.Logger)

		// Resource hygiene auto-fix: reap stale locks, orphaned temp files, and enforce log limits
		_, _ = resourcehygiene.ExecuteHygiene(projectRoot, resourcehygiene.DefaultHygieneOptions())

		// Check if we should use scheduler batching
		useSchedulerBatching := true
		if flag, err := vpc.Cmd.Flags().GetBool(validationFlagAutoFixScheduler); err == nil {
			useSchedulerBatching = flag
		}

		if useSchedulerBatching {
			// Count fixable issues (AutoFixable, Tier 4, or with FixCommand; same as CollectAutoFixableIssues)
			autoFixableCount := 0
			for _, result := range results {
				for _, issue := range result.Issues {
					if IsIssueFixableForBatch(issue) {
						autoFixableCount++
					}
				}
			}

			// Use scheduler batching if we have enough issues (threshold: 10)
			if autoFixableCount >= validationAutoFixBatchThreshold {
				if err := handleAutoFixBatching(vpc.Ctx, vpc.Cmd, projectRoot, results); err != nil {
					logging.Fluent(vpc.Logger).Warn(validationWarnBatchAutoFixFailed).WithError(err).Log()
				} else {
					logging.Fluent(vpc.Logger).Info(validationInfoAutoFixSubmitted).
						AutoFixableCount(autoFixableCount).
						Log()
				}
			}
		}
		results = filterResultsByTierIfNeeded(vpc.Cmd, results)
		return true, outputResults(vpc.Cmd, vpc.Ctx, results, nil, &staleCASResult, runIssues)
	}

	results = filterResultsByTierIfNeeded(vpc.Cmd, results)
	return true, outputResults(vpc.Cmd, vpc.Ctx, results, nil, nil, runIssues)
}

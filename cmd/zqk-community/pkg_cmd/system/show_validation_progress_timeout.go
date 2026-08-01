package system

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// handleValidationTimeout handles timeout during validation
func handleValidationTimeout(vpc *ValidationProgressContext, ctxTimeout context.Context) error {
	if !vpc.SuppressProgress && vpc.Cmd != nil {
		fmt.Fprintf(vpc.Cmd.ErrOrStderr(), "\nValidation timeout after %v\n", vpc.Timeout)
	}
	vpc.Metrics.RecordValidation(time.Since(vpc.ValidationStart))

	if err := vpc.Validator.Stop(); err != nil {
		logging.Fluent(vpc.Logger).Warn("Failed to stop validator on timeout").WithError(err).Log()
	}

	completedCopy := vpc.copyCompletedObjectIDs()
	failedCopy := vpc.copyFailedObjectIDs()

	results := collectResultsForTimeout(vpc, completedCopy, failedCopy)
	if len(results) > 0 {
		results = filterResultsByTierIfNeeded(vpc.Cmd, results)
		if err := outputResults(vpc.Cmd, vpc.Ctx, results, nil, nil, nil); err != nil {
			logging.Fluent(vpc.Logger).Warn("Failed to output results on timeout").WithError(err).Log()
		}
	}

	vpc.Mu.RLock()
	timeoutCompleted := vpc.Completed
	timeoutFailed := vpc.Failed
	queueSize := 0
	_, _, _, queueSize = vpc.Validator.GetValidationStats()
	vpc.Mu.RUnlock()

	// Emit timeout event via coordinator
	if vpc.ProjectRoot != emptyValue && vpc.StorageProvider != nil {
		profile := systemProfileHuman // Default
		if vpc.Ctx != nil && vpc.Ctx.Profile != emptyValue {
			profile = vpc.Ctx.Profile
		}
		emitCheckTimeoutEvent(
			pkgctx.NewSystemContext(),
			vpc.ProjectRoot,
			vpc.StorageProvider,
			vpc.OperationID,
			timeoutCompleted,
			vpc.TotalTasks,
			timeoutCompleted,
			timeoutFailed,
			queueSize,
			vpc.Timeout,
			profile,
		)
		// Also emit high-level operation error via OperationCallback
		// (mirrors the detailed timeout event above).
		opProfile := profile
		opCallback := coordination.NewCoordinatorOperationCallback(
			pkgctx.NewSystemContext(),
			vpc.ProjectRoot,
			vpc.StorageProvider,
			"system_check",
			opProfile,
		)
		opErr := errfmt.Errorf("validation timeout: %d/%d completed, %d failed", timeoutCompleted, vpc.TotalTasks, timeoutFailed)
		opCallback.OnError(vpc.OperationID, opErr)
		return opErr
	}

	return errfmt.Errorf("validation timeout: %d/%d completed, %d failed", timeoutCompleted, vpc.TotalTasks, timeoutFailed)
}

// collectResultsForTimeout collects results when timeout occurs
func collectResultsForTimeout(vpc *ValidationProgressContext, completedCopy map[string]bool, failedCopy map[string]string) []CheckResult {
	allStates := vpc.Validator.GetAllCachedStates()
	results := make([]CheckResult, 0, len(allStates))

	for _, state := range allStates {
		if completedCopy[state.ObjectID] {
			result := convertValidationStateToCheckResult(state)
			if errMsg, failed := failedCopy[state.ObjectID]; failed {
				if errMsg == "" {
					errMsg = "Validation failed - see logs for details"
				}
				result.Issues = append(result.Issues, Issue{
					Tier:     1,
					Category: "validation_error",
					Message:  errMsg,
				})
			}
			results = append(results, result)
			for _, issue := range result.Issues {
				vpc.Metrics.RecordTierIssue(issue.Tier)
			}
		}
	}

	return results
}

// handleFallbackTimeout handles fallback timeout when queue is empty
func handleFallbackTimeout(vpc *ValidationProgressContext) (bool, error) {
	if vpc.QueueEmptySince == nil {
		return false, nil
	}

	emptyDuration := time.Since(*vpc.QueueEmptySince)
	allStates := vpc.Validator.GetAllCachedStates()
	cachedStateMap := make(map[string]bool, len(allStates))
	for _, state := range allStates {
		cachedStateMap[state.ObjectID] = true
	}

	vpc.Mu.RLock()
	progressReceivedCopy := make(map[string]bool, len(vpc.CompletedObjectIDs))
	for id := range vpc.CompletedObjectIDs {
		progressReceivedCopy[id] = true
	}
	vpc.Mu.RUnlock()

	cachedEnqueuedCount := 0
	for objectID := range vpc.EnqueuedObjectIDs {
		hasProgress := progressReceivedCopy[objectID]
		if !hasProgress && cachedStateMap[objectID] {
			cachedEnqueuedCount++
		}
	}

	vpc.Mu.RLock()
	progressReceivedCount := len(vpc.CompletedObjectIDs)
	currentCompleted := vpc.Completed
	currentFailed := vpc.Failed
	vpc.Mu.RUnlock()

	allObjectsAccountedFor := (progressReceivedCount + cachedEnqueuedCount) >= len(vpc.EnqueuedObjectIDs)

	// Calculate how many objects are still unaccounted for
	missingCount := len(vpc.EnqueuedObjectIDs) - (progressReceivedCount + cachedEnqueuedCount)
	accountedPercent := float64(progressReceivedCount+cachedEnqueuedCount) / float64(len(vpc.EnqueuedObjectIDs)) * 100

	timeout := calculateFallbackTimeout(vpc, cachedEnqueuedCount)

	// CRITICAL: Only use fallback timeout if most objects are accounted for (>95%)
	// If queue is empty but most objects aren't validated yet, validation workers are still processing
	// Don't declare completion prematurely - wait for workers to finish
	if emptyDuration > timeout && (currentCompleted+currentFailed) > 0 {
		// If less than 95% of objects are accounted for, don't proceed with completion
		// This prevents premature completion when queue is empty but validation is still in progress
		if accountedPercent < 95.0 {
			// Check if workers are making progress - if no progress for extended period, they may be stuck
			// Use validator's internal completed count as a proxy for actual work being done
			// If validator reports many completed but we haven't received progress, workers may be stuck
			validatorCompleted := currentCompleted + currentFailed
			progressGap := validatorCompleted - progressReceivedCount

			// If validator reports significantly more completed than progress received, workers may be stuck
			// But allow some gap for dropped progress updates (up to 10% of total)
			maxAllowedGap := len(vpc.EnqueuedObjectIDs) / 10
			if progressGap > maxAllowedGap && emptyDuration > 30*time.Second {
				logging.Fluent(vpc.Logger).Warn("Queue empty but workers appear stuck - large gap between validator completed and progress received").
					ValidatorCompleted(validatorCompleted).
					ProgressReceived(progressReceivedCount).
					ProgressGap(progressGap).
					MaxAllowedGap(maxAllowedGap).
					EmptyDuration(emptyDuration.String()).
					AccountedPercent(fmt.Sprintf("%.1f%%", accountedPercent)).
					Log()
				// Still don't complete - wait for actual validation to finish or timeout
			} else {
				logging.Fluent(vpc.Logger).Debug("Queue empty but most objects not yet validated - waiting for workers").
					ProgressReceived(progressReceivedCount).
					CachedCount(cachedEnqueuedCount).
					ValidatorCompleted(validatorCompleted).
					TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
					MissingCount(missingCount).
					AccountedPercent(fmt.Sprintf("%.1f%%", accountedPercent)).
					EmptyDuration(emptyDuration.String()).
					Log()
			}
			return false, nil
		}
		// Wait for validation goroutines to complete before declaring completion
		validationTimeout := 5 * time.Second
		if !vpc.Validator.WaitForValidationCompletion(validationTimeout) {
			logging.Fluent(vpc.Logger).Debug("Validation goroutines still running after timeout in fallback, proceeding anyway").Log()
		}
		if !allObjectsAccountedFor {
			logging.Fluent(vpc.Logger).Warn("Queue empty for extended period but object count mismatch - proceeding with completion anyway (prevents hang)").
				EmptyDuration(emptyDuration.String()).
				ProgressReceived(progressReceivedCount).
				CachedCount(cachedEnqueuedCount).
				TotalEnqueued(len(vpc.EnqueuedObjectIDs)).
				ExpectedTotal(progressReceivedCount + cachedEnqueuedCount).
				Completed(currentCompleted).
				FailedOps(currentFailed).
				Log()
		} else {
			logging.Fluent(vpc.Logger).Warn("Queue empty for extended period with all objects accounted for, stopping validator (fallback timeout)").
				EmptyDuration(emptyDuration.String()).
				Completed(currentCompleted).
				FailedOps(currentFailed).
				TotalTasks(vpc.TotalTasks).
				Log()
		}

		if err := vpc.Validator.Stop(); err != nil {
			logging.Fluent(vpc.Logger).Warn("Failed to stop validator on fallback timeout").WithError(err).Log()
		}

		select {
		case <-vpc.DrainDone:
		case <-time.After(2 * time.Second):
		}

		completedCopy := vpc.copyCompletedObjectIDs()
		failedCopy := vpc.copyFailedObjectIDs()
		results := collectResultsForFallbackTimeout(vpc, completedCopy, failedCopy)
		results = filterResultsByTierIfNeeded(vpc.Cmd, results)
		return true, outputResults(vpc.Cmd, vpc.Ctx, results, nil, nil, nil)
	}

	return false, nil
}

// calculateFallbackTimeout calculates fallback timeout based on cached count
// Conservative timeout: queue empty doesn't mean validation is done - workers may still be processing
func calculateFallbackTimeout(vpc *ValidationProgressContext, cachedEnqueuedCount int) time.Duration {
	// Base timeout: give workers time to finish processing dequeued tasks
	// Increased from 2s to 10s to allow workers to complete in-flight validation
	timeout := 10 * time.Second

	// If most objects are cached (>50%), they're likely already done, use shorter timeout
	if len(vpc.EnqueuedObjectIDs) > 0 && cachedEnqueuedCount > len(vpc.EnqueuedObjectIDs)/2 {
		// Many objects are cached (likely all done, just haven't sent progress) - shorter wait
		timeout = 5 * time.Second
	}

	// For very large batches, give more time for workers to finish
	if len(vpc.EnqueuedObjectIDs) > 10000 {
		timeout = 15 * time.Second
	}

	return timeout
}

// collectResultsForFallbackTimeout collects results for fallback timeout
func collectResultsForFallbackTimeout(vpc *ValidationProgressContext, completedCopy map[string]bool, failedCopy map[string]string) []CheckResult {
	allStates := vpc.Validator.GetAllCachedStates()
	results := make([]CheckResult, 0, len(allStates))

	for _, state := range allStates {
		if completedCopy[state.ObjectID] {
			result := convertValidationStateToCheckResult(state)
			if errMsg, failed := failedCopy[state.ObjectID]; failed {
				if errMsg == "" {
					errMsg = "Validation failed - see logs for details"
				}
				result.Issues = append(result.Issues, Issue{
					Tier:     1,
					Category: "validation_error",
					Message:  errMsg,
				})
			}
			results = append(results, result)
			for _, issue := range result.Issues {
				vpc.Metrics.RecordTierIssue(issue.Tier)
			}
		}
	}

	return results
}

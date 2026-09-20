package system

import (
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/spf13/cobra"
)

// AutofixProcessPendingJobID is the well-known job that processes all AUTOFIX-*.json in .zqk/autofix/.
// Triggered when batches are submitted so autofix files don't build up.
const AutofixProcessPendingJobID = "SCH-autofix-process-pending"

// handleAutoFixBatching collects auto-fixable issues and submits them to scheduler
func handleAutoFixBatching(ctx *cli.Context, cmd *cobra.Command, projectRoot string, results []CheckResult) error {
	logger := logging.GetLoggerFromProfile(ctx.Profile)

	// Check if scheduler batching is enabled (default: true for large batches)
	useScheduler, _ := cmd.Flags().GetBool("auto-fix-scheduler")
	if !useScheduler {
		// Check if we should use scheduler based on issue count (same predicate as CollectAutoFixableIssues)
		autoFixableCount := 0
		for _, result := range results {
			for _, issue := range result.Issues {
				if IsIssueFixableForBatch(issue) {
					autoFixableCount++
				}
			}
		}

		// Use scheduler if we have more than 50 fixable issues
		if autoFixableCount < 50 {
			return errfmt.Errorf("too few issues for batching, using synchronous mode")
		}
	}

	// Create batcher
	batcher := NewAutoFixSchedulerBatcher(projectRoot, logger)

	// Collect all auto-fixable issues
	batchIssues := batcher.CollectAutoFixableIssues(results)

	if len(batchIssues) == 0 {
		logging.Fluent(logger).Info("No auto-fixable issues found, skipping batching").Log()
		return nil
	}

	logging.Fluent(logger).Info("Collecting auto-fixable issues for batching").
		Int("total_issues", len(batchIssues)).
		Log()

	// Create batches
	batches := batcher.CreateBatches(batchIssues)

	logging.Fluent(logger).Info("Created auto-fix batches").
		Int("batch_count", len(batches)).
		Int("total_issues", len(batchIssues)).
		Log()

	// Submit batches to scheduler
	jobIDs, err := batcher.SubmitAllBatches(ctx, batches)
	if err != nil {
		return errfmt.Newf("failed to submit batches to scheduler").Wrap(err)
	}

	// Trigger the "process pending" job so AUTOFIX-*.json are applied soon (and don't build up).
	// Per-batch jobs are already enqueued by SubmitBatchToScheduler; this ensures any stragglers are processed.
	if projectRoot != emptyValue {
		triggerQueue := schedulerpkg.NewJobTriggerQueue(projectRoot)
		if enqErr := triggerQueue.EnqueueTriggerRequest(AutofixProcessPendingJobID); enqErr != nil {
			logging.Fluent(logger).Warn("Failed to enqueue trigger for process-pending job (batches still submitted)").
				String("job_id", AutofixProcessPendingJobID).
				WithError(enqErr).
				Log()
		} else {
			logging.Fluent(logger).Info("Triggered process-pending job so autofixes are applied").
				String("job_id", AutofixProcessPendingJobID).
				Log()
		}
	}

	// Show first 3 job IDs for logging
	jobIDPreview := jobIDs
	if len(jobIDs) > 3 {
		jobIDPreview = jobIDs[:3]
	}
	logging.Fluent(logger).Info("Auto-fix batches submitted to scheduler").
		Int("batch_count", len(batches)).
		Int("job_count", len(jobIDs)).
		String("job_ids", fmt.Sprintf("%v", jobIDPreview)).
		Log()

	// Inform user: apply the autofixes (scheduler will process; no manual step needed)
	out := cli.CommandOutputWriter(cmd, nil)
	_, _ = fmt.Fprintf(out, "\n✅ Apply the autofixes: %d batch(es) submitted (%d issues total).\n", len(batches), len(batchIssues))
	_, _ = fmt.Fprintf(out, "   Scheduler will process shortly (triggered). No manual step needed.\n")
	_, _ = fmt.Fprintf(out, "   Job IDs: %s", jobIDs[0])
	if len(jobIDs) > 1 {
		_, _ = fmt.Fprintf(out, " and %d more", len(jobIDs)-1)
	}
	_, _ = fmt.Fprintf(out, "\n")
	_, _ = fmt.Fprintf(out, "   Monitor: zqk scheduler activity\n")
	_, _ = fmt.Fprintf(out, "   History: zqk scheduler history\n\n")

	return nil
}

// handleSynchronousAutoFix handles auto-fix synchronously (fallback mode)
func handleSynchronousAutoFix(ctx *cli.Context, cmd *cobra.Command, projectRoot string, results []CheckResult) []CheckResult {
	// This is the existing synchronous auto-fix logic
	// It's called as a fallback if batching fails or is disabled
	// The actual auto-fix happens in check_object_helpers.go during check
	return results
}

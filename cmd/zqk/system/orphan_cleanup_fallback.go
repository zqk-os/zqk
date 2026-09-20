package system

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
)

// NewOrphanCleanupFallbackCmd creates a command to process orphan cleanup queue as fallback
// This is intended to be run as a scheduled job to ensure cleanup happens even if worker fails
func NewOrphanCleanupFallbackCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Process orphan cleanup queue (fallback for when worker is idle)",
		"Processes the CAS orphan cleanup queue if the background worker is not running.",
		"",
		"This command is intended to be run as a scheduled job (e.g., every 5 minutes) to ensure",
		"orphaned hash files are cleaned up even if the background worker fails or crashes.",
		"",
		"The command:",
		"  - Checks if the cleanup queue has items",
		"  - Checks if the background worker is running",
		"  - If worker is idle and queue has items, processes a batch",
		"  - Wakes up the worker to handle remaining items",
		"",
		"This provides a fallback mechanism for orphan cleanup reliability.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemOrphanCleanupFallbackCommandBuilder(), &cobra.Command{
		Use:  "orphan-cleanup-fallback",
		RunE: runOrphanCleanupFallback,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	return cmd
}

func runOrphanCleanupFallback(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	// Get project root
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	// Get cleanup queue
	cleanupQueue := caspkg.GetGlobalOrphanCleanupQueue()

	// Configure queue if needed (for audit events)
	queueSize := cleanupQueue.QueueSize()
	isRunning := cleanupQueue.IsWorkerRunning()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("Orphan cleanup queue status").
		Int("queue_size", queueSize).
		Bool("worker_running", isRunning).
		Log()

	if queueSize == 0 {
		logging.Fluent(logger).Info("Queue is empty - nothing to process").Log()
		return nil
	}

	if isRunning {
		logging.Fluent(logger).Info("Worker is running - it will process the queue").Log()
		return nil
	}

	// Worker is idle but queue has items - process a batch as fallback
	logging.Fluent(logger).Warn("Worker is idle but queue has items - processing batch as fallback").Log()

	processed, err := cleanupQueue.ProcessQueueIfIdle(ctx)
	if err != nil {
		return errfmt.Newf("failed to process queue").Wrap(err)
	}

	logging.Fluent(logger).Info("Processed items from queue; worker woken for remaining").
		Int("processed", processed).
		Log()

	return nil
}

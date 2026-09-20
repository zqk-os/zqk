package system

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
)

// NewRetentionToleranceCmd creates a command to run retention tolerance (archive and cleanup) on demand.
// Config is loaded from .zqk/specs/configs/retention_tolerance.yaml.
func NewRetentionToleranceCmd() *cobra.Command {
	var kinds []string

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRetentionToleranceCommandBuilder(), &cobra.Command{
		Use:   "retention-tolerance",
		Short: "Run retention tolerance (archive and cleanup by age and max_count)",
		Long: paths.RewriteCanonicalCLIInvocations(`Applies per-kind archive and cleanup from retention_tolerance.yaml: archive old objects, delete by age, enforce max_count.

Use --kind to process specific kinds (can be repeated). Without --kind, processes all configured kinds.

Examples:
  # Process all configured kinds
  zqk system retention-tolerance

  # Process only mcp_session (high-frequency cleanup)
  zqk system retention-tolerance --kind mcp_session

  # Process multiple specific kinds
  zqk system retention-tolerance --kind audit_event --kind mcp_session

  # One-off aggressive cleanup (stop scheduler first): large batches, no job timeout
  zqk system retention-tolerance --kind audit_event --batch-size 5000 --max-batches 20 --bulk-delete-workers 32

  # Manual reset style catch-up: no artificial per-run batch cap (still bounded by process limits)
  zqk system retention-tolerance --kind audit_event --batch-size 5000 --max-batches -1 --bulk-delete-workers 32

Schedule via scheduler job (job_type: retention_tolerance) with KINDS env var for kind filtering.
BATCH_SIZE, MAX_BATCHES, BULK_DELETE_WORKERS can also be set via environment (overridden by flags).`),
		Args: cobra.NoArgs,
	})
	cli.BindAsyncProgress(cmd, func(c *cobra.Command, args []string) error {
		return runRetentionToleranceWithKinds(c, kinds, batchSize, maxBatches, bulkDeleteWorkers)
	})
	cmd.Flags().StringArrayVar(&kinds, "kind", nil, "Specific kind(s) to process (can be repeated)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 0, "Max objects per batch (0 = use default or BATCH_SIZE env; use 5000+ for aggressive one-off)")
	cmd.Flags().IntVar(&maxBatches, "max-batches", 0, "Max batches per kind per phase (0 = default or MAX_BATCHES env; -1 = unlimited batches for this run)")
	cmd.Flags().IntVar(&bulkDeleteWorkers, "bulk-delete-workers", 0, "Parallel delete workers (0 = use default or BULK_DELETE_WORKERS env; max 64)")
	cli.AddCommonFlags(cmd)
	return cmd
}

var (
	batchSize         int
	maxBatches        int
	bulkDeleteWorkers int
)

func runRetentionToleranceWithKinds(cmd *cobra.Command, kinds []string, batchSizeFlag, maxBatchesFlag, bulkWorkersFlag int) error {
	// Read flags from the command so we use actual parsed values (package vars may not be set in all invocation paths)
	if b, err := cmd.Flags().GetInt("batch-size"); err == nil {
		batchSizeFlag = b
	}
	if m, err := cmd.Flags().GetInt("max-batches"); err == nil {
		maxBatchesFlag = m
	}
	if w, err := cmd.Flags().GetInt("bulk-delete-workers"); err == nil {
		bulkWorkersFlag = w
	}

	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}
	profile := profileOrDefault(ctx.Profile, systemProfileHuman)
	logger := logging.GetLoggerFromProfile(profile)

	logging.Fluent(logger).Info("Initializing storage...").Log()
	storageProvider, err := getStorageProvider(cmd, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to initialize storage").Wrap(err)
	}
	handler := scheduler.NewRetentionToleranceHandler(storageProvider, projectRoot)
	// Progress via logger and async coordinator (AGENT_GUIDELINES: logger from context; async pattern via validation progress).
	validationProgress := pkgctx.GetValidationProgress(cmd.Context())
	handler.SetProgressFunc(func(msg string) {
		logging.Fluent(logger).Info(msg).Log()
		if validationProgress != nil {
			validationProgress("progress", msg)
		}
	})

	// Acquire project-level singleton lock so only one retention-tolerance runs at a time (CLI or daemon).
	// Prevents parallel runs from degrading performance (storage/lock contention).
	jobLock, lockErr := scheduler.NewJobLock(scheduler.RetentionToleranceSingletonLockID, projectRoot)
	if lockErr != nil {
		return errfmt.Newf("retention-tolerance lock setup").Wrap(lockErr)
	}
	acquired, tryErr := jobLock.TryAcquire()
	if tryErr != nil {
		return errfmt.Newf("retention-tolerance lock check").Wrap(tryErr)
	}
	if !acquired {
		return errfmt.Errorf("retention-tolerance is already running (daemon maintenance or another process); wait for it to finish or stop the other run")
	}
	defer func() { _ = jobLock.Release() }()

	// Build job: KINDS and optional BATCH_SIZE, MAX_BATCHES, BULK_DELETE_WORKERS (flags override env)
	job := &scheduler.ScheduledJob{
		ID:                   "cli-retention-tolerance-" + time.Now().Format("20060102150405"),
		JobType:              "retention_tolerance",
		EnvironmentVariables: make(map[string]string),
	}
	if len(kinds) > 0 {
		job.EnvironmentVariables[scheduler.EnvKeyKinds] = strings.Join(kinds, ",")
	}
	if batchSizeFlag > 0 {
		job.EnvironmentVariables[scheduler.EnvKeyBatchSize] = strconv.Itoa(batchSizeFlag)
	} else if v := os.Getenv(scheduler.EnvKeyBatchSize); v != emptyValue {
		job.EnvironmentVariables[scheduler.EnvKeyBatchSize] = v
	}
	if maxBatchesFlag != 0 {
		job.EnvironmentVariables[scheduler.EnvKeyMaxBatches] = strconv.Itoa(maxBatchesFlag)
	} else if v := os.Getenv(scheduler.EnvKeyMaxBatches); v != emptyValue {
		job.EnvironmentVariables[scheduler.EnvKeyMaxBatches] = v
	}
	if bulkWorkersFlag > 0 {
		job.EnvironmentVariables[scheduler.EnvKeyBulkDeleteWorkers] = strconv.Itoa(bulkWorkersFlag)
	} else if v := os.Getenv(scheduler.EnvKeyBulkDeleteWorkers); v != emptyValue {
		job.EnvironmentVariables[scheduler.EnvKeyBulkDeleteWorkers] = v
	}

	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}
	// Aggressive one-off: use a long-lived context so the timeout hook does not cancel us after ~30s.
	if batchSizeFlag >= 5000 && (maxBatchesFlag >= 20 || maxBatchesFlag == -1) {
		logging.Fluent(logger).Info("Using 2h context for aggressive retention (batch-size >= 5000, max-batches >= 20 or unlimited)").Log()
		aggCtx, aggCancel := context.WithTimeout(context.Background(), 2*time.Hour) // Background: request-or-shutdown derived
		defer aggCancel()
		runCtx = aggCtx
	}
	if err := handler.Execute(runCtx, job); err != nil {
		return errfmt.Newf("retention tolerance failed").Wrap(err)
	}
	logging.Fluent(logger).Info("Done.").Log()
	return nil
}

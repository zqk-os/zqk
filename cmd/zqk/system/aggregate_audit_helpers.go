package system

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"runtime/pprof"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// AggregateAuditContext groups state for aggregate audit execution
type AggregateAuditContext struct {
	Cmd             *cobra.Command
	Ctx             *cli.Context
	ProjectRoot     string
	StorageProvider storage.ObjectStorageProvider
	Service         *storage.AuditAggregationService
	WindowStart     time.Time
	WindowEnd       time.Time
	Logger          logging.Logger
	SecCtx          *pkgctx.SecurityContext
	StorageCtx      *pkgctx.StorageContext
	Config          *storage.OperationConfig
}

// initializeAggregateAuditContext sets up the aggregate audit context
func initializeAggregateAuditContext(cmd *cobra.Command) (*AggregateAuditContext, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return nil, errfmt.Errorf("failed to get context")
	}

	projectRoot, storageProvider, service, logger, err := initAuditAggregationSetup(cmd)
	if err != nil {
		return nil, err
	}

	windowStart, windowEnd, err := parseTimeWindow(cmd)
	if err != nil {
		return nil, errfmt.Newf("failed to parse time window").Wrap(err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	config := storage.DefaultOperationConfig()
	// Default 30m so long runs (e.g. large backlogs) complete without requiring --timeout
	config.Timeout = 30 * time.Minute
	config.ProgressCallback = func(msg string) {
		logging.Fluent(logger).Info(msg).Log()
	}

	return &AggregateAuditContext{
		Cmd:             cmd,
		Ctx:             ctx,
		ProjectRoot:     projectRoot,
		StorageProvider: storageProvider,
		Service:         service,
		WindowStart:     windowStart,
		WindowEnd:       windowEnd,
		Logger:          logger,
		SecCtx:          secCtx,
		StorageCtx:      storageCtx,
		Config:          config,
	}, nil
}

// setupCPUProfilingForAggregate sets up CPU profiling if requested
func setupCPUProfilingForAggregate(cmd *cobra.Command) (func(), error) {
	cpuProfile, err := cmd.Flags().GetString("cpu-profile")
	if err != nil {
		cpuProfile = ""
	}

	if cpuProfile == emptyValue {
		return func() {}, nil
	}

	f, err := fileutil.Create(cpuProfile)
	if err != nil {
		return nil, errfmt.Newf("failed to create CPU profile").Wrap(err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return nil, errfmt.Newf("failed to start CPU profile").Wrap(err)
	}

	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}, nil
}

// executeAggregateAuditCore executes the audit aggregation core (storage writes).
func executeAggregateAuditCore(aggCtx *AggregateAuditContext) (*storage.AuditAggregationResult, error) {
	logging.Fluent(aggCtx.Logger).Info("Aggregating audit events").
		String("start", aggCtx.WindowStart.Format(time.RFC3339)).
		String("end", aggCtx.WindowEnd.Format(time.RFC3339)).
		Log()

	logging.Fluent(aggCtx.Logger).Info("Querying audit events (this may take a moment if there are many events)").Log()

	result, err := storage.ExecuteWithConfig(aggCtx.Config, func(ctx stdcontext.Context) (*storage.AuditAggregationResult, error) {
		return aggCtx.Service.AggregateAuditEvents(ctx, aggCtx.SecCtx, aggCtx.StorageCtx, aggCtx.WindowStart, aggCtx.WindowEnd)
	})

	return result, err
}

type aggregateAuditCleanupMode string

const (
	aggregateAuditCleanupNone          aggregateAuditCleanupMode = "none"
	aggregateAuditCleanupArchive       aggregateAuditCleanupMode = "archive"
	aggregateAuditCleanupDelete        aggregateAuditCleanupMode = "delete"
	aggregateAuditCleanupMetricArchive aggregateAuditCleanupMode = "metric_archive"
	aggregateAuditCleanupMetricDelete  aggregateAuditCleanupMode = "metric_delete"
)

type aggregateAuditCleanupDecision struct {
	Mode     aggregateAuditCleanupMode
	MetricID string
}

func parseAggregateAuditCleanupDecisionFromFlags(cmd *cobra.Command) (aggregateAuditCleanupDecision, error) {
	archive, _ := cmd.Flags().GetBool("archive")
	shouldDelete, _ := cmd.Flags().GetBool("delete")
	cleanupMetricID, _ := cmd.Flags().GetString("cleanup-metric")

	if cleanupMetricID != emptyValue {
		if !archive && !shouldDelete {
			return aggregateAuditCleanupDecision{}, errfmt.Errorf("cleanup-metric requires --archive or --delete")
		}
		if shouldDelete {
			return aggregateAuditCleanupDecision{Mode: aggregateAuditCleanupMetricDelete, MetricID: cleanupMetricID}, nil
		}
		return aggregateAuditCleanupDecision{Mode: aggregateAuditCleanupMetricArchive, MetricID: cleanupMetricID}, nil
	}

	if shouldDelete {
		return aggregateAuditCleanupDecision{Mode: aggregateAuditCleanupDelete}, nil
	}
	if archive {
		return aggregateAuditCleanupDecision{Mode: aggregateAuditCleanupArchive}, nil
	}
	return aggregateAuditCleanupDecision{Mode: aggregateAuditCleanupNone}, nil
}

func executeAggregateAuditCleanupCore(
	aggCtx *AggregateAuditContext,
	result *storage.AuditAggregationResult,
	decision aggregateAuditCleanupDecision,
) error {
	switch decision.Mode {
	case aggregateAuditCleanupNone:
		return outputNoCleanupMessage(aggCtx.Cmd)
	case aggregateAuditCleanupDelete:
		return handleDeleteCleanup(aggCtx, result)
	case aggregateAuditCleanupArchive:
		return handleArchiveCleanup(aggCtx, result)
	case aggregateAuditCleanupMetricArchive:
		return cleanupEventsFromMetric(aggCtx.Cmd, decision.MetricID, true, false)
	case aggregateAuditCleanupMetricDelete:
		return cleanupEventsFromMetric(aggCtx.Cmd, decision.MetricID, false, true)
	default:
		return errfmt.Errorf("unknown cleanup mode: %s", decision.Mode)
	}
}

// outputAggregationResults outputs the aggregation results
func outputAggregationResults(cmd *cobra.Command, result *storage.AuditAggregationResult) error {
	var buf bytes.Buffer
	buf.WriteString("\n✅ Aggregation complete!\n")
	fmt.Fprintf(&buf, "  - Events processed: %d\n", result.EventCount)
	fmt.Fprintf(&buf, "  - Metrics created: %d\n", result.MetricsCreated)
	if result.MetricID != emptyValue {
		fmt.Fprintf(&buf, "  - Metric ID: %s\n", result.MetricID)
	}
	fmt.Fprintf(&buf, "  - Events updated: %d\n", result.EventsUpdated)
	if result.EventCount == 0 {
		buf.WriteString("\nℹ️  No events in window. Audit event counts only drop when you pass --delete (or --archive).\n")
		buf.WriteString("   Run from project root; use -v for debug (e.g. CAS fallback). If using graph storage, ensure audit events are in the graph.\n")
	}
	return cli.WriteOutput(cmd, buf.Bytes())
}

// handleCleanup handles cleanup operations (delete/archive)
func handleCleanup(aggCtx *AggregateAuditContext, result *storage.AuditAggregationResult) error {
	archive, _ := aggCtx.Cmd.Flags().GetBool("archive")
	shouldDelete, _ := aggCtx.Cmd.Flags().GetBool("delete")
	cleanupMetricID, _ := aggCtx.Cmd.Flags().GetString("cleanup-metric")

	if cleanupMetricID != emptyValue {
		return cleanupEventsFromMetric(aggCtx.Cmd, cleanupMetricID, archive, shouldDelete)
	}

	if shouldDelete {
		return handleDeleteCleanup(aggCtx, result)
	}

	if archive {
		return handleArchiveCleanup(aggCtx, result)
	}

	return outputNoCleanupMessage(aggCtx.Cmd)
}

// handleDeleteCleanup handles deletion cleanup
func handleDeleteCleanup(aggCtx *AggregateAuditContext, result *storage.AuditAggregationResult) error {
	var buf bytes.Buffer
	buf.WriteString("\n⚠️  Deleting processed events...\n")
	fmt.Fprintf(&buf, "  - Processing %d event ID(s) (may be expanded from ranges)\n", len(result.EventsProcessed))
	if err := cli.WriteOutput(aggCtx.Cmd, buf.Bytes()); err != nil {
		return err
	}

	cliCtx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	deletedCount, err := aggCtx.Service.CleanupAggregatedEvents(
		cliCtx, aggCtx.SecCtx, result.EventsProcessed, false)
	if err != nil {
		return errfmt.Newf("failed to delete events").Wrap(err)
	}

	buf.Reset()
	if deletedCount == 0 {
		buf.WriteString("  ⚠️  Warning: 0 events deleted. Events may not exist or may already be deleted.\n")
		buf.WriteString("  ℹ️  Note: Check if events exist and are in 'aggregated' status.\n")
	} else {
		fmt.Fprintf(&buf, "  ✅ Events deleted: %d\n", deletedCount)
		handleCacheInvalidation(aggCtx, result, &buf)
	}

	return cli.WriteOutput(aggCtx.Cmd, buf.Bytes())
}

// handleArchiveCleanup handles archive cleanup
func handleArchiveCleanup(aggCtx *AggregateAuditContext, result *storage.AuditAggregationResult) error {
	var buf bytes.Buffer
	buf.WriteString("\n📦 Archiving processed events...\n")
	if err := cli.WriteOutput(aggCtx.Cmd, buf.Bytes()); err != nil {
		return err
	}

	cliCtx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	archivedCount, err := aggCtx.Service.CleanupAggregatedEvents(
		cliCtx, aggCtx.SecCtx, result.EventsProcessed, true)
	if err != nil {
		return errfmt.Newf("failed to archive events").Wrap(err)
	}

	buf.Reset()
	fmt.Fprintf(&buf, "  - Events archived: %d\n", archivedCount)
	return cli.WriteOutput(aggCtx.Cmd, buf.Bytes())
}

func invalidateBulkEventsCache(cmd *cobra.Command, projectRoot, reason string, eventIDs []string, buf *bytes.Buffer) {
	proc, procErr := cli.NewProcessor(cmd)
	if procErr != nil {
		return
	}

	cacheCtx := pkgctx.NewCacheInvalidationContext(
		eventIDs,
		projectRoot,
		reason,
	)
	cacheInvalidated, cacheErr := proc.InvalidateCache(cacheCtx)
	if cacheErr != nil {
		//nolint:gocritic // preferFprint: POL-CODE-007 buffer via WriteString(Sprintf)
		fmt.Fprintf(buf, "  ⚠️  Warning: Failed to invalidate cache: %v\n", cacheErr)
	} else if cacheInvalidated > 0 {
		//nolint:gocritic // preferFprint: POL-CODE-007 buffer via WriteString(Sprintf)
		fmt.Fprintf(buf, "  ✅ Cache entries invalidated: %d\n", cacheInvalidated)
	}
}

// handleCacheInvalidation handles cache invalidation after deletion
func handleCacheInvalidation(aggCtx *AggregateAuditContext, result *storage.AuditAggregationResult, buf *bytes.Buffer) {
	expandedIDs := storage.ExpandIDRanges(result.EventsProcessed)
	invalidateBulkEventsCache(aggCtx.Cmd, aggCtx.ProjectRoot, "Bulk deletion of aggregated audit events", expandedIDs, buf)
}

// outputNoCleanupMessage outputs message when no cleanup is performed
func outputNoCleanupMessage(cmd *cobra.Command) error {
	var buf bytes.Buffer
	buf.WriteString("\nℹ️  Events marked as aggregated but not archived/deleted.\n")
	buf.WriteString("   Use --archive or --delete to clean up processed events.\n")
	return cli.WriteOutput(cmd, buf.Bytes())
}

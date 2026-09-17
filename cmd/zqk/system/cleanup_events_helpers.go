package system

import (
	"bytes"
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// CleanupEventsContext groups state for cleanup events from metric
type CleanupEventsContext struct {
	Cmd             *cobra.Command
	MetricID        string
	Archive         bool
	ShouldDelete    bool
	ProjectRoot     string
	StorageProvider storage.ObjectStorageProvider
	Service         *storage.AuditAggregationService
	Logger          logging.Logger
	SecCtx          *pkgctx.SecurityContext
}

// initializeCleanupEventsContext sets up the cleanup events context
func initializeCleanupEventsContext(cmd *cobra.Command, metricID string, archive, shouldDelete bool) (*CleanupEventsContext, error) {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not found")
	}

	storageProvider, err := getStorageProvider(cmd, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to initialize storage").Wrap(err)
	}

	service := storage.NewAuditAggregationService(storageProvider)

	cliCtx := cli.GetContext(cmd)
	profile := systemProfileSystem
	if cliCtx != nil && cliCtx.Profile != emptyValue {
		profile = cliCtx.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)

	secCtx := pkgctx.NewSystemSecurityContext()

	return &CleanupEventsContext{
		Cmd:             cmd,
		MetricID:        metricID,
		Archive:         archive,
		ShouldDelete:    shouldDelete,
		ProjectRoot:     projectRoot,
		StorageProvider: storageProvider,
		Service:         service,
		Logger:          logger,
		SecCtx:          secCtx,
	}, nil
}

// loadMetricAndExtractEventIDs loads the metric and extracts event IDs
func loadMetricAndExtractEventIDs(cleanupCtx *CleanupEventsContext) ([]string, error) {
	metricObj, err := cleanupCtx.StorageProvider.Read(pkgctx.NewSystemContext(), cleanupCtx.SecCtx, cleanupCtx.MetricID)
	if err != nil {
		return nil, errfmt.Errorf("failed to get aggregation metric %s: %w", cleanupCtx.MetricID, err)
	}

	eventIDRanges, ok := metricObj[objects.FieldKeyAggregatedEventIDs].([]any)
	if !ok {
		if strArray, ok := metricObj[objects.FieldKeyAggregatedEventIDs].([]string); ok {
			eventIDRanges = make([]any, len(strArray))
			for i, s := range strArray {
				eventIDRanges[i] = s
			}
		} else {
			return nil, errfmt.Errorf("aggregated_event_ids not found or invalid in metric %s", cleanupCtx.MetricID)
		}
	}

	var idRanges []string
	for _, r := range eventIDRanges {
		if str, ok := r.(string); ok {
			idRanges = append(idRanges, str)
		}
	}

	eventIDs := storage.ExpandIDRanges(idRanges)
	logging.Fluent(cleanupCtx.Logger).Info("Found event IDs in metric").
		Count(len(eventIDs)).
		MetricID(cleanupCtx.MetricID).
		Log()

	return eventIDs, nil
}

// handleDeleteCleanupFromMetric handles deletion cleanup from metric
func handleDeleteCleanupFromMetric(cleanupCtx *CleanupEventsContext, eventIDs []string) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "\n⚠️  Deleting %d events from metric %s...\n", len(eventIDs), cleanupCtx.MetricID)
	if err := cli.WriteOutput(cleanupCtx.Cmd, buf.Bytes()); err != nil {
		return err
	}

	cliCtx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	deletedCount, err := cleanupCtx.Service.CleanupAggregatedEvents(cliCtx, cleanupCtx.SecCtx, eventIDs, false)
	if err != nil {
		return errfmt.Newf("failed to delete events").Wrap(err)
	}

	buf.Reset()
	if deletedCount == 0 {
		buf.WriteString("  ⚠️  Warning: 0 events deleted. Events may not exist or may already be deleted.\n")
	} else {
		fmt.Fprintf(&buf, "  ✅ Events deleted: %d\n", deletedCount)
		handleCacheInvalidationForDelete(cleanupCtx, eventIDs, &buf)
	}

	return cli.WriteOutput(cleanupCtx.Cmd, buf.Bytes())
}

// handleArchiveCleanupFromMetric handles archive cleanup from metric
func handleArchiveCleanupFromMetric(cleanupCtx *CleanupEventsContext, eventIDs []string) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "\n📦 Archiving %d events from metric %s...\n", len(eventIDs), cleanupCtx.MetricID)
	if err := cli.WriteOutput(cleanupCtx.Cmd, buf.Bytes()); err != nil {
		return err
	}

	cliCtx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	archivedCount, err := cleanupCtx.Service.CleanupAggregatedEvents(cliCtx, cleanupCtx.SecCtx, eventIDs, true)
	if err != nil {
		return errfmt.Newf("failed to archive events").Wrap(err)
	}

	buf.Reset()
	if archivedCount == 0 {
		buf.WriteString("  ⚠️  Warning: 0 events archived. Events may not exist or may already be archived.\n")
		buf.WriteString("  ℹ️  Note: Events may need to be deleted directly from filesystem if BulkUpdate fails.\n")
	} else {
		fmt.Fprintf(&buf, "  ✅ Events archived: %d\n", archivedCount)
	}

	return cli.WriteOutput(cleanupCtx.Cmd, buf.Bytes())
}

// handleCacheInvalidationForDelete handles cache invalidation after deletion
func handleCacheInvalidationForDelete(cleanupCtx *CleanupEventsContext, eventIDs []string, buf *bytes.Buffer) {
	proc, procErr := cli.NewProcessor(cleanupCtx.Cmd)
	if procErr != nil {
		return
	}

	cacheCtx := pkgctx.NewCacheInvalidationContext(
		eventIDs,
		cleanupCtx.ProjectRoot,
		fmt.Sprintf("Bulk deletion of events from metric %s", cleanupCtx.MetricID),
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

package scheduler

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type schedulerQueryBaseContext struct {
	ProjectRoot     string
	StorageProvider storagepkg.ObjectStorageProvider
	SecCtx          *pkgctx.SecurityContext
	StorageCtx      *pkgctx.StorageContext
}

func initSchedulerQueryBaseContext(ctx *cli.Context, timeout time.Duration) (*schedulerQueryBaseContext, error) {
	if ctx == nil {
		return nil, errfmt.Errorf("failed to get CLI context")
	}
	var projectRoot string
	if strings.TrimSpace(ctx.ProjectRoot) != emptyValue {
		if abs, err := filepath.Abs(ctx.ProjectRoot); err == nil {
			projectRoot = abs
		} else {
			projectRoot = filepath.Clean(ctx.ProjectRoot)
		}
	}
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not found")
	}

	cmdCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), timeout)
	defer cancel()

	storageProvider, err := storagepkg.GetGlobalStorageProviderCache().GetOrCreate(cmdCtx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to get storage").Wrap(err)
	}

	return &schedulerQueryBaseContext{
		ProjectRoot:     projectRoot,
		StorageProvider: storageProvider,
		SecCtx:          pkgctx.NewSystemSecurityContext(),
		StorageCtx:      ctx.GetStorageContext(),
	}, nil
}

func queryCachedEventIDs(projectRoot string, start, end time.Time, limit int) (bool, []string) {
	cache := storagepkg.GetGlobalHighVolumeEventCache()
	if !cache.IsPopulatedForProject(projectRoot) {
		return false, nil
	}
	ids := cache.QueryByTimeWindow(start, end, limit)
	return len(ids) > 0, ids
}

// OutputJobHistoryData handles output of job history data using coordinator
// This is a shared component that centralizes format handling and coordinator integration
func OutputJobHistoryData(
	ctx context.Context,
	cmd *cobra.Command,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	statsList []*jobStats,
	jobIDFilter string,
	profile string,
) error {
	// Emit coordinator event for history query
	operationID := fmt.Sprintf("scheduler_history_%d", time.Now().UnixNano())
	emitSchedulerHistoryEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		operationID,
		len(statsList),
		jobIDFilter,
	)

	if cli.GetContext(cmd) == nil {
		return errfmt.Errorf("failed to get context")
	}

	data := map[string]any{
		"jobs":  statsList,
		"count": len(statsList),
	}

	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		err := cli.FormatOutput(cmd, data)
		if err != nil {
			emitSchedulerOutputErrorViaCoordinator(ctx, projectRoot, storageProvider, string(format), err, profile)
			return err
		}
		return nil
	default:
		return outputHistoryTableViaCoordinator(ctx, cmd, projectRoot, storageProvider, statsList, profile)
	}
}

// outputHistoryTableViaCoordinator outputs job history as table using coordinator
func outputHistoryTableViaCoordinator(
	_ context.Context,
	cmd *cobra.Command,
	_ string,
	_ storagepkg.ObjectStorageProvider,
	statsList []*jobStats,
	_ string,
) error {
	// Delegate to existing table output function
	return outputHistoryTable(statsList, cmd)
}

// emitSchedulerOutputErrorViaCoordinator emits error events for output formatting errors
func emitSchedulerOutputErrorViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storagepkg.ObjectStorageProvider,
	format string, // "json", "yaml"
	err error,
	profile string,
) {
	ctx, coordinator, ok := initSchedulerCoordination(ctx, projectRoot, profile)
	if !ok {
		return
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: fmt.Sprintf("scheduler_history_output_%s", format)},
		{Key: "format", Value: format},
		{Key: "error", Value: err.Error()},
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // Output errors don't create audit events
		MetricsData:   nil,
	}

	// Create operation ID
	operationID := fmt.Sprintf("scheduler_history_output_%s_%d", format, time.Now().UnixNano())

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, "scheduler_history_output", "error").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false). // Logging only
		WithError(err)

	emitCoordinatorEventAsync(ctx, coordinator, eventCtx)
}

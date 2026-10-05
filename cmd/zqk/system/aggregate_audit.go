package system

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

// NewAggregateAuditCmd creates a command to aggregate audit events
func NewAggregateAuditCmd() *cobra.Command {
	longHelp := fmt.Sprintf(`Aggregate audit events within a time window into metrics for efficient storage.

This command:
  - Queries audit events in the specified time window
  - Aggregates them into audit_aggregation_metric objects
  - Compresses event IDs into ranges for space efficiency
  - Marks events as aggregated
  - Optionally archives or deletes processed events

Examples:
  # Aggregate events from last 24 hours
  %s system aggregate-audit --window 24h

  # Aggregate events from specific date range
  %s system aggregate-audit --start "2025-12-01T00:00:00Z" --end "2025-12-02T00:00:00Z"

  # Aggregate and archive processed events
  %s system aggregate-audit --window 24h --archive

  # Aggregate and delete processed events (use with caution)
  %s system aggregate-audit --window 24h --delete

Note: --window 7d only includes events from the last 7 days. For large backlogs of older
events, use a longer window (e.g. --window 30d or --window 90d) or explicit --start/--end.`, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAggregateAuditCommandBuilder(), &cobra.Command{
		Use:   "aggregate-audit",
		Short: "Aggregate audit events into metrics",
		Long:  longHelp,
		Args:  cobra.NoArgs,
		RunE:  runAggregateAudit,
	})

	addAggregationWindowFlags(cmd)
	cmd.Flags().Bool("archive", false, "Archive processed events after aggregation")
	cmd.Flags().Bool("delete", false, "Delete processed events after aggregation (use with caution)")
	cmd.Flags().String("cleanup-metric", "", "Clean up events from a specific aggregation metric ID (e.g., AAM-001)")
	cmd.Flags().String("cpu-profile", "", "Write CPU profile to file (for performance analysis)")

	cli.AddCommonFlags(cmd)
	return cmd
}

func addAggregationWindowFlags(cmd *cobra.Command) {
	cmd.Flags().String("window", "24h", "Time window for aggregation (e.g., 24h, 7d, 1w)")
	cmd.Flags().String("start", "", "Start time for aggregation window (ISO 8601 format)")
	cmd.Flags().String("end", "", "End time for aggregation window (ISO 8601 format, defaults to now)")
}

func runAggregateAudit(cmd *cobra.Command, args []string) error {
	return RunAggregateAuditViaPipeline(cmd, args)
}

func parseTimeWindow(cmd *cobra.Command) (start, end time.Time, err error) {
	return parseTimeWindowWithNow(cmd, time.Now().UTC())
}

func parseTimeWindowWithNow(cmd *cobra.Command, nowUTC time.Time) (start, end time.Time, err error) {
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	startStr, _ := cmd.Flags().GetString("start")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	endStr, _ := cmd.Flags().GetString("end")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	windowStr, _ := cmd.Flags().GetString("window")
	if windowStr == emptyValue {
		windowStr = "24h" // Default
	}

	var windowStart, windowEnd time.Time

	if startStr != emptyValue {
		// Explicit start time provided
		windowStart, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return time.Time{}, time.Time{}, errfmt.Newf("invalid start time format").Wrap(err)
		}

		if endStr != emptyValue {
			windowEnd, err = time.Parse(time.RFC3339, endStr)
			if err != nil {
				return time.Time{}, time.Time{}, errfmt.Newf("invalid end time format").Wrap(err)
			}
		} else {
			windowEnd = nowUTC
		}
	} else {
		// Use window duration
		duration, err := parseDuration(windowStr)
		if err != nil {
			return time.Time{}, time.Time{}, errfmt.Newf("invalid window format").Wrap(err)
		}

		windowEnd = nowUTC
		windowStart = windowEnd.Add(-duration)
	}

	return windowStart, windowEnd, nil
}

func parseDuration(s string) (time.Duration, error) {
	return storage.ParseWindowSize(s)
}

// cleanupEventsFromMetric cleans up events from a specific aggregation metric
func cleanupEventsFromMetric(cmd *cobra.Command, metricID string, archive, shouldDelete bool) error {
	cleanupCtx, err := initializeCleanupEventsContext(cmd, metricID, archive, shouldDelete)
	if err != nil {
		return err
	}

	eventIDs, err := loadMetricAndExtractEventIDs(cleanupCtx)
	if err != nil {
		return err
	}

	if shouldDelete {
		return handleDeleteCleanupFromMetric(cleanupCtx, eventIDs)
	}

	if archive {
		return handleArchiveCleanupFromMetric(cleanupCtx, eventIDs)
	}

	return errfmt.Errorf("must specify --archive or --delete when using --cleanup-metric")
}

// getStorageProvider returns the appropriate storage provider (file or graph) via StorageFactory
func getStorageProvider(cmd *cobra.Command, projectRoot string) (storage.ObjectStorageProvider, error) {
	if cmd != nil && cmd.Context() != nil {
		if p := cli.GetStorageProvider(cmd.Context()); p != nil {
			return p.(storage.ObjectStorageProvider), nil
		}
	}

	ctx := pkgctx.NewSystemContext()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}

	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to initialize storage factory").Wrap(err)
	}
	return factory.GetStorage(), nil
}

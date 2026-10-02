package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// NewFileLockMetricsCmd creates a command group for file lock metrics
func NewFileLockMetricsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Manage file lock metrics (view buffer, flush)",
		"Manage file lock metrics that are accumulated in memory.",
		"",
		"File lock metrics track lock acquisition times, contention rates, wait times, and system busyness.",
		"Metrics are accumulated in memory and can be collected/flushed to create metric objects.",
	).
		AddExample("View current buffered metrics", "%s system metrics file-lock view").
		AddExample("Flush metrics to create a metric object", "%s system metrics file-lock flush").
		AddExample("Flush with custom time window", "%s system metrics file-lock flush --window 1h").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemFileLockCommandBuilder(), &cobra.Command{
		Use: "file-lock",
	})

	cmd.AddCommand(NewFileLockMetricsViewCmd())
	cmd.AddCommand(NewFileLockMetricsFlushCmd())

	return cli.FinalizeBareCommand(cmd, helpBuilder)
}

// NewFileLockMetricsViewCmd creates a command to view buffered metrics
func NewFileLockMetricsViewCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"View current buffered file lock metrics",
		"View the current state of file lock metrics accumulated in memory.",
		"",
		"This shows metrics that have been recorded but not yet collected/flushed.",
		"Metrics are accumulated atomically as file lock operations occur.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemViewCommandBuilder(), &cobra.Command{
		Use:  "view",
		RunE: runFileLockMetricsView,
	})

	return cli.FinalizeCommand(cmd, helpBuilder)
}

// NewFileLockMetricsFlushCmd creates a command to flush metrics
func NewFileLockMetricsFlushCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Flush buffered metrics to create a metric object",
		"Flush buffered file lock metrics by creating a file_lock_metric object and resetting counters.",
		"",
		"This collects all accumulated metrics, creates a metric object, and resets the counters",
		"for the next collection period.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemFlushCommandBuilder(), &cobra.Command{
		Use:  "flush",
		RunE: runFileLockMetricsFlush,
	})

	cmd.Flags().String("window", "", "Time window for metrics (e.g., '1h', '6h', '24h'). Defaults to since last flush or 1 hour.")
	return cli.FinalizeCommand(cmd, helpBuilder)
}

func runFileLockMetricsView(cmd *cobra.Command, args []string) error {
	if cli.GetContext(cmd) == nil {
		return errfmt.Errorf("failed to get context")
	}

	// Get current metrics snapshot
	metrics := storagepkg.GetFileLockMetrics()
	snapshot := metrics.GetSnapshot()

	// Calculate derived metrics
	avgAcquisitionTime := snapshot.AverageAcquisitionTime()
	avgWaitTime := snapshot.AverageWaitTime()
	contentionRate := snapshot.ContentionRate()
	successRate := snapshot.SuccessRate()

	// Build output data
	outputData := map[string]any{
		objects.FieldKeyTotalAcquisitions:    snapshot.TotalAcquisitions,
		objects.FieldKeyTotalFailures:        snapshot.TotalFailures,
		objects.FieldKeyTotalTimeouts:        snapshot.TotalTimeouts,
		objects.FieldKeyTotalContention:      snapshot.TotalContention,
		"current_holders":                    snapshot.CurrentHolders,
		objects.FieldKeyPeakContention:       snapshot.PeakContention,
		objects.FieldKeyAvgAcquisitionTimeMs: float64(avgAcquisitionTime.Nanoseconds()) / 1e6,
		objects.FieldKeyAvgWaitTimeMs:        float64(avgWaitTime.Nanoseconds()) / 1e6,
		objects.FieldKeyMaxAcquisitionTimeMs: float64(snapshot.MaxAcquisitionTime.Nanoseconds()) / 1e6,
		objects.FieldKeyMaxWaitTimeMs:        float64(snapshot.MaxWaitTime.Nanoseconds()) / 1e6,
		objects.FieldKeyContentionRate:       contentionRate * 100,
		objects.FieldKeySuccessRate:          successRate * 100,
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, outputData)
	default:
		var buf strings.Builder
		fmt.Fprintln(&buf, "File Lock Metrics (Buffered in Memory)")
		fmt.Fprintln(&buf, "======================================")
		fmt.Fprintf(&buf, "Total Acquisitions:     %d\n", snapshot.TotalAcquisitions)
		fmt.Fprintf(&buf, "Total Failures:         %d\n", snapshot.TotalFailures)
		fmt.Fprintf(&buf, "Total Timeouts:         %d\n", snapshot.TotalTimeouts)
		fmt.Fprintf(&buf, "Total Contention:       %d\n", snapshot.TotalContention)
		fmt.Fprintf(&buf, "Current Holders:        %d\n", snapshot.CurrentHolders)
		fmt.Fprintf(&buf, "Peak Contention:        %d\n", snapshot.PeakContention)
		fmt.Fprintf(&buf, "\nPerformance:\n")
		fmt.Fprintf(&buf, "  Avg Acquisition Time: %.2f ms\n", float64(avgAcquisitionTime.Nanoseconds())/1e6)
		fmt.Fprintf(&buf, "  Avg Wait Time:        %.2f ms\n", float64(avgWaitTime.Nanoseconds())/1e6)
		fmt.Fprintf(&buf, "  Max Acquisition Time: %.2f ms\n", float64(snapshot.MaxAcquisitionTime.Nanoseconds())/1e6)
		fmt.Fprintf(&buf, "  Max Wait Time:        %.2f ms\n", float64(snapshot.MaxWaitTime.Nanoseconds())/1e6)
		fmt.Fprintf(&buf, "\nRates:\n")
		fmt.Fprintf(&buf, "  Contention Rate:      %.2f%%\n", contentionRate*100)
		fmt.Fprintf(&buf, "  Success Rate:         %.2f%%\n", successRate*100)
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}
}

func runFileLockMetricsFlush(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	// Get window flag
	windowStr, _ := cmd.Flags().GetString("window") //nolint:errcheck // Flag parsing errors are non-critical
	var windowDuration time.Duration
	if windowStr != emptyValue {
		var err error
		windowDuration, err = time.ParseDuration(windowStr)
		if err != nil {
			return errfmt.Newf("invalid window duration").Wrap(err)
		}
	} else {
		// Default to 1 hour
		windowDuration = 1 * time.Hour
	}

	// Create storage provider
	systemCtx := pkgctx.NewSystemContext()
	storageFactory, err := storagepkg.NewStorageFactory(systemCtx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create collector
	collector := storagepkg.NewFileLockMetricsCollector(storageProvider)

	// Calculate time window
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.Add(-windowDuration)

	// Flush metrics (collects and resets)
	secCtx := pkgctx.NewSystemSecurityContext()
	metricID, err := collector.CollectAndReset(systemCtx, secCtx, windowStart, windowEnd)
	if err != nil {
		return errfmt.Newf("failed to flush metrics").Wrap(err)
	}

	result := map[string]any{
		"metric_id":                 metricID,
		objects.FieldKeyWindowStart: windowStart.Format(time.RFC3339),
		objects.FieldKeyWindowEnd:   windowEnd.Format(time.RFC3339),
		"window_duration":           windowDuration.String(),
		objects.FieldKeyStatus:      objects.ObjectStatusFlushed,
	}
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		var buf strings.Builder
		fmt.Fprintf(&buf, "✅ Flushed file lock metrics\n")
		fmt.Fprintf(&buf, "   Metric ID: %s\n", metricID)
		fmt.Fprintf(&buf, "   Window: %s to %s (%s)\n",
			windowStart.Format(time.RFC3339),
			windowEnd.Format(time.RFC3339),
			windowDuration.String())
		cliCmd := paths.CLICommandName
		if cliCmd == emptyValue {
			cliCmd = paths.CLICommandNameDefault
		}
		fmt.Fprintf(&buf, "\n💡 View the metric with:\n")
		fmt.Fprintf(&buf, "   %s internal get file_lock_metric %s\n", cliCmd, metricID)
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}
}

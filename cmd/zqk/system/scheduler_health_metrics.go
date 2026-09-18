package system

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqktime"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
)

// NewSchedulerHealthMetricsCmd creates a command group for scheduler health metrics
func NewSchedulerHealthMetricsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Manage scheduler health metrics",
		"Manage scheduler health metrics that track scheduler daemon reliability and self-healing.",
		"",
		"Scheduler health metrics track:",
		"  - Health check runs",
		"  - Missed job triggers",
		"  - Automatic job recoveries",
		"  - Cron scheduler restarts",
		"  - Health check duration",
	).
		AddExample("Create a test health metric for inspection", "%s system metrics scheduler-health test").
		AddExample("View existing health metrics", "%s internal list scheduler_health_metric").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSchedulerHealthCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "scheduler-health",
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(NewSchedulerHealthMetricsTestCmd())

	return cmd
}

// NewSchedulerHealthMetricsTestCmd creates a command to generate a test health metric
func NewSchedulerHealthMetricsTestCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Create a dummy scheduler health metric for inspection",
		"Create a test/dummy scheduler health metric object to inspect the structure.",
		"",
		"This command creates a sample health metric with example values to help you",
		"understand the metric structure and fields.",
	).
		ExcludeCommonFlags()

	testCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemTestCommandBuilder(), &cobra.Command{
		Use: "test",
	})
	cli.BindAsyncProgress(testCmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		if ctx == nil {
			return errfmt.Errorf("failed to get context")
		}
		return createTestSchedulerHealthMetric(ctx, cmd)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(testCmd)

	// Add common flags
	cli.AddCommonFlags(testCmd)

	return testCmd
}

// createTestSchedulerHealthMetric creates a dummy health metric for inspection
func createTestSchedulerHealthMetric(_ *cli.Context, cmd *cobra.Command) error {
	now := time.Now().UTC()
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	// Generate metric ID - scheduler_health_metric uses SHM- prefix
	metricID := fmt.Sprintf("SHM-TEST-%d", now.Unix())

	// Create test metric object using scheduler_health_metric spec
	metricData := map[string]any{
		objects.FieldKeyID:              metricID,
		objects.FieldKeyKind:            objects.KindSchedulerHealthMetric,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyCreatedAt:       zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:       "system",
		objects.FieldKeyUpdatedAt:       zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:       "system",
		objects.FieldKeyMetricType:      "system", // Required enum: command, performance, system, application, custom
		objects.FieldKeySource:          "scheduler",
		objects.FieldKeyCollectionCount: 1, // Required: non-negative integer
		objects.FieldKeyFirstSeen:       zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyLastSeen:        zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyTags:            []string{"scheduler", "health", "monitoring", "test"},
		objects.FieldKeyTitle:           "Test Scheduler Health Metric",
		objects.FieldKeyDescription:     "Dummy health metric created for inspection purposes",
		// Health-specific fields (from scheduler_health_metric spec)
		objects.FieldKeyHealthChecks:           1,
		objects.FieldKeyMissedTriggers:         3,
		objects.FieldKeyRecoveredJobs:          2,
		objects.FieldKeyCronRestarts:           0,
		objects.FieldKeyHealthCheckDurationMs:  125.5,
		objects.FieldKeyMeasurementWindowStart: zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyMeasurementWindowEnd:   zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, metricData)
	}

	// Default / table: human-oriented walkthrough with an indented JSON sample.
	jsonBytes, err := json.MarshalIndent(metricData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal metric data").Wrap(err)
	}

	var b strings.Builder
	fmt.Fprintln(&b, "📋 Scheduler Health Metric Structure (using scheduler_health_metric spec):")
	fmt.Fprintln(&b, string(jsonBytes))
	fmt.Fprintf(&b, "\n💡 Key points:\n")
	fmt.Fprintln(&b, "   - Uses 'scheduler_health_metric' kind (extends base_metric)")
	fmt.Fprintln(&b, "   - Required fields: metric_type, collection_count, first_seen, last_seen")
	fmt.Fprintln(&b, "   - Health-specific fields: health_checks, missed_triggers, recovered_jobs, cron_restarts, health_check_duration_ms")
	fmt.Fprintln(&b, "   - ID format: SHM-XXX (scheduler_health_metric prefix)")
	fmt.Fprintln(&b, "\n💡 Spec location:")
	fmt.Fprintln(&b, "   "+filepath.Join(paths.ProcessInternalObjectSpecsDir, "scheduler_health_metric.yaml"))
	fmt.Fprintln(&b, "\n💡 To view existing health metrics:")
	fmt.Fprintf(&b, "   %s internal list scheduler_health_metric\n", cliCmd)

	return cli.WriteOutput(cmd, []byte(b.String()))
}

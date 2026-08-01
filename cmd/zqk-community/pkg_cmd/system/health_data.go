package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/healthcheck"
	_ "github.com/lanceman/zqk/pkg/healthcheck/monitors" // register object_volume and stream_volume
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/spf13/cobra"
)

const (
	healthMonitorStatusSkip     = "skip"
	healthMonitorIDObjectVolume = "object_volume"
	healthMonitorIDStreamVolume = "stream_volume"
	systemHealthLatestFile      = "tier1-latest.json"
	streamSummaryDir            = "stream_summary"
	testBundleHealthSummaryFile = "test_bundle_health.json"
)

// HealthDataOutput is the aggregated system-health data for dashboard consumption.
// Use --format json or --format yaml; default table is a short summary.
type HealthDataOutput struct {
	Quarantine       *QuarantineReportData `json:"quarantine,omitempty" yaml:"quarantine,omitempty"`
	CheckSummary     *CheckSummaryData     `json:"check_summary,omitempty" yaml:"check_summary,omitempty"`
	SchedulerMetrics *SchedulerMetricsData `json:"scheduler_metrics,omitempty" yaml:"scheduler_metrics,omitempty"`
	VolumeRates      *VolumeRatesData      `json:"volume_rates,omitempty" yaml:"volume_rates,omitempty"`
	// TestBundleHealth is populated when .zqk/stream_summary/test_bundle_health.json exists
	// (see scheduler.WriteTestBundleHealthStreamSummary, REQ-DATASTREAM-001 pilot).
	TestBundleHealth *scheduler.TestBundleHealthStreamSummary `json:"test_bundle_health,omitempty" yaml:"test_bundle_health,omitempty"`
	// RetentionAdvisory compares internal object counts to retention_tolerance.yaml (same logic as
	// zqk system retention-status / check retention_drift reminder). Populated each run when config loads.
	RetentionAdvisory *RetentionAdvisoryData `json:"retention_advisory,omitempty" yaml:"retention_advisory,omitempty"`
}

// RetentionAdvisoryData is a compact retention vs target signal for dashboards (see RETENTION_TARGET_VISIBILITY_GAP.md).
type RetentionAdvisoryData struct {
	OverTarget      bool   `json:"over_target" yaml:"over_target"`
	Message         string `json:"message,omitempty" yaml:"message,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty" yaml:"suggested_action,omitempty"`
}

// VolumeRatesData holds object and stream volume monitor results (count + rate per hour).
// Populated by running object_volume and stream_volume health monitors.
type VolumeRatesData struct {
	ObjectVolume *healthcheck.Result `json:"object_volume,omitempty" yaml:"object_volume,omitempty"`
	StreamVolume *healthcheck.Result `json:"stream_volume,omitempty" yaml:"stream_volume,omitempty"`
}

// SchedulerMetricsData is a subset of scheduler metrics for dashboard/reporting.
// Populated when the scheduler daemon is running and has a metrics collector.
type SchedulerMetricsData struct {
	PoolCreationDeclinedCount int64 `json:"pool_creation_declined_count" yaml:"pool_creation_declined_count"`
	// DispatchPressureDropped counts attempts where the job did not start (dispatch wait budget exhausted).
	DispatchPressureDropped scheduler.DispatchPressureMetrics `json:"dispatch_pressure_dropped" yaml:"dispatch_pressure_dropped"`
}

// CheckSummaryData is a minimal summary from the last check run (e.g. from .zqk/system-health/tier1-latest.json).
// Populated when that file exists; otherwise omitted.
type CheckSummaryData struct {
	Source          string `json:"source,omitempty" yaml:"source,omitempty"` // e.g. "tier1-latest.json"
	TotalObjects    int    `json:"total_objects,omitempty" yaml:"total_objects,omitempty"`
	Blocking        int    `json:"blocking,omitempty" yaml:"blocking,omitempty"`
	Warnings        int    `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Informational   int    `json:"informational,omitempty" yaml:"informational,omitempty"`
	Recommendations int    `json:"recommendations,omitempty" yaml:"recommendations,omitempty"`
}

// NewHealthDataCmd creates a command that outputs aggregated system-health data for dashboards.
// Follows existing patterns (status, quarantine-report): format-driven output, data-oriented JSON/YAML.
func NewHealthDataCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Output aggregated system-health data for dashboard",
		"Outputs quarantine and other system-health data in one payload. Use --format json or --format yaml for dashboard/consumer use; default is a short table summary.",
		"",
		"Additional sections (e.g. check summary) may be added in future.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemHealthDataCommandBuilder(), &cobra.Command{
		Use:   "health-data",
		Short: "Output aggregated system-health data for dashboard",
		RunE:  runHealthData,
	})
	helpBuilder.ApplyToCommand(cmd)
	return cmd
}

func runHealthData(cmd *cobra.Command, _ []string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("not in a ZQK project (no .zqk found)")
	}
	data := HealthDataOutput{}
	quarantine, err := BuildQuarantineReportData(projectRoot)
	if err != nil {
		return err
	}
	data.Quarantine = quarantine
	data.CheckSummary = loadCheckSummaryFromSystemHealth(projectRoot)
	data.SchedulerMetrics = loadSchedulerMetricsForHealthData()
	data.VolumeRates = loadVolumeRatesForHealthData(cmd.Context(), projectRoot)
	data.TestBundleHealth = loadTestBundleHealthStreamSummary(projectRoot)
	vis, msg, action := GetRetentionDriftReminder(cmd.Context(), projectRoot)
	data.RetentionAdvisory = &RetentionAdvisoryData{
		OverTarget:      vis,
		Message:         msg,
		SuggestedAction: action,
	}

	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, data)
	default:
		return writeHealthDataTable(cmd, &data)
	}
}

func writeHealthDataTable(cmd *cobra.Command, data *HealthDataOutput) error {
	var buf []byte
	when.When(func() bool { return data.Quarantine != nil }).Then(func() {
		buf = append(buf, fmt.Sprintf("Quarantine: %s\n", data.Quarantine.QuarantineRoot)...)
		buf = append(buf, fmt.Sprintf("  Total files: %d\n", data.Quarantine.TotalFiles)...)
		for _, s := range data.Quarantine.Subfolders {
			buf = append(buf, fmt.Sprintf("  %s: %d (%s)\n", s.Name, s.Count, s.Source)...)
		}
	}).OrElse(func() {
		buf = append(buf, "Quarantine: (none)\n"...)
	}).Run()
	if data.CheckSummary != nil {
		buf = append(buf, fmt.Sprintf("Check summary: %s (blocking=%d warnings=%d informational=%d)\n",
			data.CheckSummary.Source, data.CheckSummary.Blocking, data.CheckSummary.Warnings, data.CheckSummary.Informational)...)
	}
	if data.SchedulerMetrics != nil {
		dp := data.SchedulerMetrics.DispatchPressureDropped
		buf = append(buf, fmt.Sprintf("Scheduler metrics: pool_creation_declined_count=%d dispatch_pressure_dropped_total=%d (cron=%d recovery=%d trigger_submit=%d immediate_submit=%d other=%d)\n",
			data.SchedulerMetrics.PoolCreationDeclinedCount,
			dp.Total, dp.Cron, dp.MissedJobRecovery, dp.TriggerJobSubmit, dp.ImmediateJobSubmit, dp.Other)...)
	}
	if data.VolumeRates != nil {
		buf = append(buf, "Volume rates:\n"...)
		if data.VolumeRates.ObjectVolume != nil {
			buf = append(buf, fmt.Sprintf("  object_volume: %s\n", data.VolumeRates.ObjectVolume.Summary)...)
		}
		if data.VolumeRates.StreamVolume != nil {
			buf = append(buf, fmt.Sprintf("  stream_volume: %s\n", data.VolumeRates.StreamVolume.Summary)...)
		}
	}
	if data.TestBundleHealth != nil {
		tbh := data.TestBundleHealth
		buf = append(buf, fmt.Sprintf("Test-bundle health stream: alias=%s lines=%d bytes=%d updated=%s\n",
			tbh.PathAlias, tbh.LineCount, tbh.ByteSize, tbh.UpdatedAtRFC3339)...)
	}
	if data.RetentionAdvisory != nil {
		if data.RetentionAdvisory.OverTarget {
			buf = append(buf, fmt.Sprintf("Retention: OVER TARGET — %s\n", data.RetentionAdvisory.Message)...)
			if data.RetentionAdvisory.SuggestedAction != "" {
				buf = append(buf, fmt.Sprintf("  %s\n", data.RetentionAdvisory.SuggestedAction)...)
			}
		} else {
			buf = append(buf, "Retention: at or under target (per retention_tolerance vs internal count)\n"...)
		}
	}
	return cli.WriteOutput(cmd, buf)
}

// loadVolumeRatesForHealthData runs object_volume and stream_volume monitors and returns their results.
// Best-effort: if the registry or a monitor fails, that result is omitted.
func loadVolumeRatesForHealthData(ctx context.Context, projectRoot string) *VolumeRatesData {
	if projectRoot == emptyValue {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if reg, ok := healthcheck.DefaultRegistry.(*healthcheck.DefaultRegistryImpl); ok {
		_ = reg.BindProjectRoot(projectRoot)
	}
	out := &VolumeRatesData{}
	if res, err := healthcheck.DefaultRegistry.Run(ctx, projectRoot, healthMonitorIDObjectVolume); err == nil && res != nil && res.Status != healthMonitorStatusSkip {
		out.ObjectVolume = res
	}
	if res, err := healthcheck.DefaultRegistry.Run(ctx, projectRoot, healthMonitorIDStreamVolume); err == nil && res != nil && res.Status != healthMonitorStatusSkip {
		out.StreamVolume = res
	}
	if out.ObjectVolume == nil && out.StreamVolume == nil {
		return nil
	}
	return out
}

// loadSchedulerMetricsForHealthData returns scheduler metrics when the daemon is running; otherwise nil.
func loadSchedulerMetricsForHealthData() *SchedulerMetricsData {
	sched := scheduler.GetGlobalScheduler()
	if sched == nil {
		return nil
	}
	snap := sched.GetMetricsSnapshot()
	return &SchedulerMetricsData{
		PoolCreationDeclinedCount: snap.Pool.CreationDeclined,
		DispatchPressureDropped:   snap.DispatchPressure,
	}
}

// loadCheckSummaryFromSystemHealth reads .zqk/system-health/tier1-latest.json if present and returns a minimal summary; otherwise nil.
func loadCheckSummaryFromSystemHealth(projectRoot string) *CheckSummaryData {
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, systemHealthLatestFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload struct {
		Summary struct {
			TotalObjects    int `json:"total_objects"`
			Blocking        int `json:"blocking_issues"`
			Warnings        int `json:"warnings"`
			Informational   int `json:"informational"`
			Recommendations int `json:"recommendations"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return &CheckSummaryData{
		Source:          systemHealthLatestFile,
		TotalObjects:    payload.Summary.TotalObjects,
		Blocking:        payload.Summary.Blocking,
		Warnings:        payload.Summary.Warnings,
		Informational:   payload.Summary.Informational,
		Recommendations: payload.Summary.Recommendations,
	}
}

// loadTestBundleHealthStreamSummary reads .zqk/stream_summary/test_bundle_health.json when present.
func loadTestBundleHealthStreamSummary(projectRoot string) *scheduler.TestBundleHealthStreamSummary {
	if projectRoot == emptyValue {
		return nil
	}
	path := filepath.Join(projectRoot, paths.ProjectDataDir, streamSummaryDir, testBundleHealthSummaryFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var row scheduler.TestBundleHealthStreamSummary
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil
	}
	if row.PathAlias == emptyValue && row.LogicalPath == emptyValue {
		return nil
	}
	return &row
}

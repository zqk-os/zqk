package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// NewHealthCheckCmd creates the health-check command
// This command runs externally (not inside the scheduler daemon) to check if the daemon is alive
// It can be run via cron or other external monitoring tools
func NewHealthCheckCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Check scheduler daemon health (external health check)",
		"Check if the scheduler daemon is running and healthy.",
		"",
		"This command runs externally (not inside the scheduler daemon) and can be used",
		"by monitoring systems, cron jobs, or other external tools to detect if the",
		"scheduler daemon is down.",
		"",
		"Exit codes:",
		"  0 - Scheduler is healthy (running and responsive)",
		"  1 - Scheduler is unhealthy (down but should be running, or keep-alive is stale)",
		"  2 - Error checking scheduler status",
		"",
		"This command checks:",
		"  - PID file exists and process is running",
		"  - Keep-alive file is fresh (updated within last 2 minutes)",
		"  - If enabled timer jobs exist, daemon must be running",
	).
		ExcludeCommonFlags()

	healthCheckCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerHealthCheckCommandBuilder(), &cobra.Command{
		Use: "health-check",
	})
	cli.BindAsyncProgress(healthCheckCmd, runHealthCheck)

	// Apply help builder to command
	helpBuilder.ApplyToCommand(healthCheckCmd)

	// Add common flags
	cli.AddCommonFlags(healthCheckCmd)

	return healthCheckCmd
}

func runHealthCheck(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	isAlive, lastKeepAlive, err := scheduler.IsSchedulerAlive(projectRoot)
	if err != nil {
		logging.Fluent(logger).Error("Failed to check scheduler status", err).Log()
		os.Exit(2)
	}

	storageProvider, err := cli.NewStorageProviderFromFactory(pkgctx.NewSystemContext(), projectRoot)
	if err != nil {
		logging.Fluent(logger).Error("Failed to create storage factory", err).Log()
		os.Exit(2)
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	filters := map[string]any{
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyTriggerType:        schedulerTriggerTimer,
		objects.FieldKeyScheduleExpression: map[string]any{"$ne": ""},
	}

	jobResult, err := storageProvider.List(pkgctx.NewSystemContext(), secCtx, storageCtx, storagepkg.ListFilter{
		Kind:    schedulerKindJob,
		Filters: filters,
	})
	if err != nil {
		logging.Fluent(logger).Error("Failed to query scheduler jobs", err).Log()
		os.Exit(2)
	}

	hasEnabledTimerJobs := len(jobResult.Objects) > 0
	now := time.Now().UTC()

	if !isAlive {
		if hasEnabledTimerJobs {
			recordExternalHealthMetric(storageProvider, secCtx, now, 1, len(jobResult.Objects), 0, 0, 0)
			writeDaemonUnhealthyFile(projectRoot, "daemon_not_running", len(jobResult.Objects), "Scheduler daemon is not running but has enabled timer jobs")
			logging.Fluent(logger).Error("CRITICAL: Scheduler daemon is not running but has enabled timer jobs", nil).
				Int("enabled_timer_jobs", len(jobResult.Objects)).
				Log()
			if !lastKeepAlive.IsZero() {
				age := time.Since(lastKeepAlive)
				logging.Fluent(logger).Warn("Last keep-alive").
					String("last_keep_alive", lastKeepAlive.Format(time.RFC3339)).
					String("age", age.Round(time.Second).String()).
					Log()
			}
			logging.Fluent(logger).Info(paths.RewriteCanonicalCLIInvocations("Start the daemon with: zqk scheduler start")).Log()
			os.Exit(1)
		}
		removeDaemonUnhealthyFile(projectRoot)
		logging.Fluent(logger).Info("OK: Scheduler daemon is not running (no enabled timer jobs)").Log()
		os.Exit(0)
	}

	if !lastKeepAlive.IsZero() {
		age := time.Since(lastKeepAlive)
		if age > scheduler.DefaultKeepAliveTimeout {
			recordExternalHealthMetric(storageProvider, secCtx, now, 1, len(jobResult.Objects), 0, 0, 0)
			writeDaemonUnhealthyFile(projectRoot, "keep_alive_stale", len(jobResult.Objects), "Scheduler keep-alive is stale; daemon may be hung or dead")
			logging.Fluent(logger).Warn("Scheduler keep-alive is stale").
				String("age", age.Round(time.Second).String()).
				String("max", scheduler.DefaultKeepAliveTimeout.String()).
				String("last_keep_alive", lastKeepAlive.Format(time.RFC3339)).
				Log()
			os.Exit(1)
		}
	}

	removeDaemonUnhealthyFile(projectRoot)
	logging.Fluent(logger).Info("OK: Scheduler daemon is healthy").Log()
	if !lastKeepAlive.IsZero() {
		age := time.Since(lastKeepAlive)
		logging.Fluent(logger).Info("Last keep-alive").
			String("last_keep_alive", lastKeepAlive.Format(time.RFC3339)).
			String("age", age.Round(time.Second).String()).
			Log()
	}
	if hasEnabledTimerJobs {
		logging.Fluent(logger).Info("Enabled timer jobs").
			Int("count", len(jobResult.Objects)).
			Log()
	}
	os.Exit(0)
	return nil
}

// daemonUnhealthyPayload is the JSON written to .zqk/scheduler/daemon-unhealthy.json when health-check finds the daemon down or stale.
type daemonUnhealthyPayload struct {
	Reason             string `json:"reason"`
	CheckedAt          string `json:"checked_at"`
	EnabledTimerJobs   int    `json:"enabled_timer_jobs"`
	Message            string `json:"message"`
	RemediationCommand string `json:"remediation_command"`
}

func daemonUnhealthyPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerDaemonUnhealthyFile)
}

func writeDaemonUnhealthyFile(projectRoot, reason string, enabledTimerJobs int, message string) {
	path := daemonUnhealthyPath(projectRoot)
	dir := filepath.Dir(path)
	if err := fileutil.MkdirAll(dir, paths.DirPerm750); err != nil {
		return
	}
	payload := daemonUnhealthyPayload{
		Reason:             reason,
		CheckedAt:          zqktime.NowRFC3339UTC(),
		EnabledTimerJobs:   enabledTimerJobs,
		Message:            message,
		RemediationCommand: paths.CLIUsage("scheduler", "start"),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = fileutil.WriteFile(path, data, paths.FilePerm600)
}

func removeDaemonUnhealthyFile(projectRoot string) {
	_ = fileutil.Remove(daemonUnhealthyPath(projectRoot))
}

// recordExternalHealthMetric records a scheduler health metric from outside the daemon
// This is used by the external health check command to record critical issues
func recordExternalHealthMetric(storageProvider storagepkg.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, timestamp time.Time, healthChecks, missedTriggers, recoveredJobs, cronRestarts int, healthCheckDuration time.Duration) {
	// Only record if there are non-zero values (to avoid noise)
	if healthChecks == 0 && missedTriggers == 0 && recoveredJobs == 0 && cronRestarts == 0 {
		return
	}

	// Determine actor
	actor := secCtx.AccountID
	if actor == emptyValue {
		actor = "system"
	}

	// Generate metric ID - scheduler_health_metric uses SHM- prefix
	metricID := fmt.Sprintf("SHM-%d", timestamp.Unix())

	// Create metric object using scheduler_health_metric spec
	metricData := map[string]any{
		objects.FieldKeyID:              metricID,
		objects.FieldKeyKind:            objects.KindSchedulerHealthMetric,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyCreatedAt:       timestamp.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:       actor,
		objects.FieldKeyUpdatedAt:       timestamp.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:       actor,
		objects.FieldKeyMetricType:      "system", // Required: enum value
		objects.FieldKeySource:          "scheduler_external_health_check",
		objects.FieldKeyCollectionCount: 1,                              // Required: non-negative integer
		objects.FieldKeyFirstSeen:       timestamp.Format(time.RFC3339), // Required: ISO 8601 timestamp
		objects.FieldKeyLastSeen:        timestamp.Format(time.RFC3339), // Required: ISO 8601 timestamp
		objects.FieldKeyTags:            []string{"scheduler", "health", "monitoring", "external"},
		// Health-specific fields (from scheduler_health_metric spec)
		objects.FieldKeyHealthChecks:           healthChecks,
		objects.FieldKeyMissedTriggers:         missedTriggers,
		objects.FieldKeyRecoveredJobs:          recoveredJobs,
		objects.FieldKeyCronRestarts:           cronRestarts,
		objects.FieldKeyHealthCheckDurationMs:  float64(healthCheckDuration.Milliseconds()),
		objects.FieldKeyMeasurementWindowStart: timestamp.Format(time.RFC3339),
		objects.FieldKeyMeasurementWindowEnd:   timestamp.Format(time.RFC3339),
	}

	// Try to create the metric object (best effort - don't fail if it errors)
	ctx := pkgctx.NewSystemContext()
	if err := storageProvider.Create(ctx, secCtx, metricData); err != nil {
		// Silently fail - metrics are best effort and we don't want to break the health check
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Failed to record external health metric", err).Log()
	}
}

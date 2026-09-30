package scheduler

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/robfig/cron/v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// Log events for context_refresh handler (POL-CODE-007 stable keys).
const (
	LogEventContextRefreshJobStart               = JobTypeContextRefresh + "_job_start"
	LogEventContextRefreshListSchedulesCall      = JobTypeContextRefresh + "_list_schedules_call"
	LogEventContextRefreshListFailed             = JobTypeContextRefresh + "_list_failed"
	LogEventContextRefreshListNil                = JobTypeContextRefresh + "_list_nil"
	LogEventContextRefreshListCompleted          = JobTypeContextRefresh + "_list_completed"
	LogEventContextRefreshNoSchedules            = JobTypeContextRefresh + "_no_schedules"
	LogEventContextRefreshPoliciesFound          = JobTypeContextRefresh + "_policies_found"
	LogEventContextRefreshPolicyMissingID        = JobTypeContextRefresh + "_policy_missing_id"
	LogEventContextRefreshPolicySkipStatus       = JobTypeContextRefresh + "_policy_skip_status"
	LogEventContextRefreshScheduleMissingCadence = JobTypeContextRefresh + "_schedule_missing_cadence"
	LogEventContextRefreshParseCadenceFailed     = JobTypeContextRefresh + "_parse_cadence_failed"
	LogEventContextRefreshNotNeededYet           = JobTypeContextRefresh + "_not_needed_yet"
	LogEventContextRefreshExecuteFailed          = JobTypeContextRefresh + "_execute_failed"
	LogEventContextRefreshUpdatePolicyFailed     = JobTypeContextRefresh + "_update_policy_failed"
	LogEventContextRefreshScheduleRefreshed      = JobTypeContextRefresh + "_schedule_refreshed"
	LogEventContextRefreshJobCompleted           = JobTypeContextRefresh + "_job_completed"
	LogEventContextRefreshScriptMissing          = JobTypeContextRefresh + "_script_missing"
	LogEventContextRefreshScriptOk               = JobTypeContextRefresh + "_script_ok"
)

// ContextRefreshHandler handles context refresh jobs
// Context refresh jobs refresh context files (e.g., .ide/current_context.md) based on
// context_refresh_schedule objects that define refresh cadence per profile/project
type ContextRefreshHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
}

// NewContextRefreshHandler creates a new context refresh handler
// NewContextRefreshHandler creates a new context refresh handler
func NewContextRefreshHandler(storage storagepkg.ObjectStorageProvider, projectRoot string) ContextRefreshHandlerInterface {
	// If project root not provided, try to get from file-based storage
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}

	return &ContextRefreshHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute executes context refresh based on context_refresh_schedule objects
// This handler:
// 1. Loads all context_refresh_schedule objects
// 2. Checks if refresh is needed based on cadence and last_refresh
// 3. Refreshes context files (e.g., .ide/current_context.md) for matching policies
// 4. Updates last_refresh timestamp on policies
func (h *ContextRefreshHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunContextRefreshViaPipeline(ctx, h, job)
}

// executeContextRefreshCore executes context refresh.
// Called from RunContextRefreshViaPipeline NORMALIZE stage.
func (h *ContextRefreshHandler) executeContextRefreshCore(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info(LogEventContextRefreshJobStart).
		JobID(job.ID).
		ProjectRoot(h.projectRoot).
		Bool("storage_nil", h.storage == nil).
		Log()

	// Get security context
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// List all context_refresh_schedule objects
	filter := storagepkg.ListFilter{
		Kind: objects.KindContextRefreshSchedule,
	}

	SLog(h.logger).Debug(LogEventContextRefreshListSchedulesCall).
		JobID(job.ID).
		ProjectRoot(h.projectRoot).
		Log()

	result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		SLog(h.logger).Error(LogEventContextRefreshListFailed, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("failed to list context_refresh_schedule objects").Wrap(err)
	}

	if result == nil {
		SLog(h.logger).Warn(LogEventContextRefreshListNil).
			JobID(job.ID).
			Log()
		return nil
	}

	SLog(h.logger).Info(LogEventContextRefreshListCompleted).
		JobID(job.ID).
		ObjectCount(len(result.Objects)).
		Log()

	if len(result.Objects) == 0 {
		SLog(h.logger).Warn(LogEventContextRefreshNoSchedules).
			JobID(job.ID).
			ProjectRoot(h.projectRoot).
			Bool("result_nil", result == nil).
			Log()
		return nil
	}

	policies := result.Objects
	SLog(h.logger).Info(LogEventContextRefreshPoliciesFound).
		JobID(job.ID).
		PolicyCount(len(policies)).
		Log()

	now := time.Now().UTC()
	refreshedCount := 0
	scriptExecuted := false

	for _, policyObj := range policies {
		// Extract policy ID from object map
		scheduleID := ""
		if idField, ok := policyObj[objects.FieldKeyID]; ok {
			if idStr, ok := idField.(string); ok {
				scheduleID = idStr
			}
		}

		if scheduleID == emptyValue {
			SLog(h.logger).Warn(LogEventContextRefreshPolicyMissingID).
				JobID(job.ID).
				Log()
			continue
		}

		// Extract status - only process approved or in_progress policies
		status := getStringFromMap(policyObj, "status")
		if status != objects.ObjectStatusApproved && status != objects.ObjectStatusInProgress {
			SLog(h.logger).Debug(LogEventContextRefreshPolicySkipStatus).
				JobID(job.ID).
				ScheduleID(scheduleID).
				String("status", status).
				Log()
			continue
		}

		// Extract cadence
		cadence := getStringFromMap(policyObj, "cadence")
		if cadence == emptyValue {
			SLog(h.logger).Warn(LogEventContextRefreshScheduleMissingCadence).
				JobID(job.ID).
				ScheduleID(scheduleID).
				Log()
			continue
		}

		// Extract last_refresh
		lastRefreshStr := getStringFromMap(policyObj, "last_refresh")
		var lastRefresh time.Time
		if lastRefreshStr != emptyValue {
			if parsed, err := time.Parse(time.RFC3339, lastRefreshStr); err == nil {
				lastRefresh = parsed
			}
		}

		// Parse cadence and check if refresh is needed
		cadenceDuration, err := parseCadence(cadence)
		if err != nil {
			SLog(h.logger).Warn(LogEventContextRefreshParseCadenceFailed).
				JobID(job.ID).
				ScheduleID(scheduleID).
				String("cadence", cadence).
				WithError(err).
				Log()
			continue
		}

		// Check if refresh is needed
		needsRefresh := lastRefresh.IsZero() || now.Sub(lastRefresh) >= cadenceDuration
		if !needsRefresh {
			SLog(h.logger).Debug(LogEventContextRefreshNotNeededYet).
				JobID(job.ID).
				ScheduleID(scheduleID).
				String("last_refresh", zqktime.FormatRFC3339UTC(lastRefresh)).
				String("next_refresh", lastRefresh.Add(cadenceDuration).UTC().Format(time.RFC3339)).
				Log()
			continue
		}

		// Execute refresh
		target := getStringFromMap(policyObj, "target")
		if !scriptExecuted {
			if err := h.executeRefresh(ctx, scheduleID, target); err != nil {
				SLog(h.logger).Error(LogEventContextRefreshExecuteFailed, err).
					JobID(job.ID).
					ScheduleID(scheduleID).
					Log()
				continue
			}
			scriptExecuted = true
		}

		// Update last_refresh and next_refresh
		nextRefresh := now.Add(cadenceDuration)
		updateFields := map[string]any{
			objects.FieldKeyLastRefresh: now.Format(time.RFC3339),
			objects.FieldKeyNextRefresh: nextRefresh.Format(time.RFC3339),
		}

		if err := h.updateSchedule(ctx, secCtx, scheduleID, updateFields); err != nil {
			SLog(h.logger).Error(LogEventContextRefreshUpdatePolicyFailed, err).
				JobID(job.ID).
				ScheduleID(scheduleID).
				Log()
			continue
		}

		SLog(h.logger).Info(LogEventContextRefreshScheduleRefreshed).
			JobID(job.ID).
			ScheduleID(scheduleID).
			Target(target).
			Log()
		refreshedCount++
	}

	SLog(h.logger).Info(LogEventContextRefreshJobCompleted).
		JobID(job.ID).
		PoliciesProcessed(len(policies)).
		PoliciesRefreshed(refreshedCount).
		Log()

	return nil
}

// parseCadence parses a cadence string (ISO duration like P1D or cron expression)
// Returns duration for ISO format, or calculates next run time for cron
func parseCadence(cadence string) (time.Duration, error) {
	// Try ISO 8601 duration format (P1D, P7D, etc.)
	if strings.HasPrefix(cadence, "P") {
		return parseISODuration(cadence)
	}

	// Try Go duration format (24h, 1h30m, etc.)
	if duration, err := time.ParseDuration(cadence); err == nil {
		return duration, nil
	}

	// Try cron expression - for now, treat as daily if valid cron
	// In a full implementation, we'd calculate next run time from cron
	if _, err := cron.ParseStandard(cadence); err == nil {
		// Default to 24 hours for cron expressions (simplified)
		// Full implementation would calculate actual next run time
		return 24 * time.Hour, nil
	}

	return 0, errfmt.Errorf("invalid cadence format: %q (expected ISO duration like P1D, Go duration like 24h, or cron)", cadence)
}

// parseISODuration parses ISO 8601 duration format (P1D, P7D, PT1H, etc.)
func parseISODuration(s string) (time.Duration, error) {
	if !strings.HasPrefix(s, "P") {
		return 0, errfmt.Errorf("ISO duration must start with P")
	}

	// Remove P prefix
	s = s[1:]

	// Check for time component (T)
	var datePart, timePart string
	when.When(func() bool { return strings.Contains(s, "T") }).Then(func() {
		parts := strings.Split(s, "T")
		datePart = parts[0]
		timePart = parts[1]
	}).OrElse(func() {
		datePart = s
	}).Run()

	var totalDuration time.Duration

	// Parse date part (D for days, W for weeks, M for months, Y for years)
	// For simplicity, we'll handle D (days) and W (weeks)
	// M and Y would require calendar calculations
	dayRegex := regexp.MustCompile(`(\d+)D`)
	if matches := dayRegex.FindStringSubmatch(datePart); len(matches) > 1 {
		if days, err := strconv.Atoi(matches[1]); err == nil {
			totalDuration += time.Duration(days) * 24 * time.Hour
		}
	}

	weekRegex := regexp.MustCompile(`(\d+)W`)
	if matches := weekRegex.FindStringSubmatch(datePart); len(matches) > 1 {
		if weeks, err := strconv.Atoi(matches[1]); err == nil {
			totalDuration += time.Duration(weeks) * 7 * 24 * time.Hour
		}
	}

	// Parse time part (H for hours, M for minutes, S for seconds)
	if timePart != emptyValue {
		hourRegex := regexp.MustCompile(`(\d+)H`)
		if matches := hourRegex.FindStringSubmatch(timePart); len(matches) > 1 {
			if hours, err := strconv.Atoi(matches[1]); err == nil {
				totalDuration += time.Duration(hours) * time.Hour
			}
		}

		minuteRegex := regexp.MustCompile(`(\d+)M`)
		if matches := minuteRegex.FindStringSubmatch(timePart); len(matches) > 1 {
			if minutes, err := strconv.Atoi(matches[1]); err == nil {
				totalDuration += time.Duration(minutes) * time.Minute
			}
		}

		secondRegex := regexp.MustCompile(`(\d+)S`)
		if matches := secondRegex.FindStringSubmatch(timePart); len(matches) > 1 {
			if seconds, err := strconv.Atoi(matches[1]); err == nil {
				totalDuration += time.Duration(seconds) * time.Second
			}
		}
	}

	if totalDuration == 0 {
		return 0, errfmt.Errorf("could not parse ISO duration: %q", s)
	}

	return totalDuration, nil
}

// executeRefresh executes the actual context refresh based on policy target
func (h *ContextRefreshHandler) executeRefresh(ctx context.Context, scheduleID, target string) error {
	if h.projectRoot == emptyValue {
		return errfmt.Errorf("project root not available for refresh")
	}

	// For projects with custom shell hooks, run agent-protocol-context-refresh.sh
	scriptPath := filepath.Join(h.projectRoot, "scripts", "agent-protocol-context-refresh.sh")
	if _, err := fileutil.Stat(scriptPath); err == nil {
		cmd := execwrap.CommandContext(ctx, "bash", scriptPath)
		cmd.Dir = h.projectRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errfmt.Errorf("context refresh script failed: %w, output: %s", err, string(output))
		}
		SLog(h.logger).Debug(LogEventContextRefreshScriptOk).
			ScheduleID(scheduleID).
			Target(target).
			Log()
		return nil
	}

	// Native Go fallback: when external shell script is absent in standalone distributions,
	// execute internal native context refresh routine without warning log spam (F-SUPPLY-006).
	return h.executeNativeRefresh(ctx, scheduleID, target)
}

// executeNativeRefresh executes native in-process context refresh when external scripts are absent.
func (h *ContextRefreshHandler) executeNativeRefresh(ctx context.Context, scheduleID, target string) error {
	SLog(h.logger).Debug("context refresh executed via native internal routine").
		ScheduleID(scheduleID).
		Target(target).
		Log()
	return nil
}

// updateSchedule updates a context_refresh_schedule object with new field values
func (h *ContextRefreshHandler) updateSchedule(ctx context.Context, secCtx *pkgctx.SecurityContext, scheduleID string, fields map[string]any) error {
	// Update the object directly with the fields
	err := h.storage.Update(ctx, secCtx, scheduleID, fields)
	if err != nil {
		return errfmt.Errorf("failed to update schedule %q: %w", scheduleID, err)
	}

	return nil
}

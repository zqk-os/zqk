package scheduler

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// envelope tick follow-up dispatch (operational envelope tokens → scheduler job_type → TriggerJob).

const (
	// EnvKeyEnvelopeTickDispatchMode is read from the data_cell_envelope_tick scheduler_job environment:
	// "off" (default) — no follow-up triggers; "shadow" — log what would trigger; "on" — trigger allowlisted jobs.
	EnvKeyEnvelopeTickDispatchMode = "ENVELOPE_TICK_DISPATCH_MODE"
	// EnvKeyEnvelopeTickDispatchExpand adds heavier job_types when "true"/"1"/"yes"/"on":
	// object_validation and metrics_collection (still subject to hard-deny for recursive/heavy types).
	EnvKeyEnvelopeTickDispatchExpand = "ENVELOPE_TICK_DISPATCH_EXPAND"
	// EnvKeyEnvelopeTickDispatchAllowlistExtra is comma-separated scheduler job_type strings added to the effective
	// allowlist on top of core (and expanded types when EXPAND is on). Still blocked by envelopeTickDispatchHardDeny
	// and by ENVELOPE_TICK_DISPATCH_DENY_EXTRA.
	EnvKeyEnvelopeTickDispatchAllowlistExtra = "ENVELOPE_TICK_DISPATCH_ALLOWLIST_EXTRA"
	// EnvKeyEnvelopeTickDispatchDenyExtra is comma-separated job_types blocked from dispatch for this job (after hard deny).
	// Deny-extra wins over allowlist / allowlist-extra (operator tightening without code deploy).
	EnvKeyEnvelopeTickDispatchDenyExtra = "ENVELOPE_TICK_DISPATCH_DENY_EXTRA"
	// EnvKeyEnvelopeTickDispatchMaxTriggers caps follow-up shadow/on dispatches per envelope tick (after filtering to a target job).
	// Empty or unset means unlimited; non-positive values mean unlimited; invalid strings log once and behave as unlimited.
	EnvKeyEnvelopeTickDispatchMaxTriggers = "ENVELOPE_TICK_DISPATCH_MAX_TRIGGERS"
	// EnvKeyEnvelopeTickDispatchSkipIfTargetBusy: when true (default), skip follow-up if the target scheduler_job
	// is already running in this process (conflict manager). Set to false to attempt follow-up regardless.
	EnvKeyEnvelopeTickDispatchSkipIfTargetBusy = "ENVELOPE_TICK_DISPATCH_SKIP_IF_TARGET_BUSY"
	// EnvKeyEnvelopeTickDispatchCheckTriggerQueue: when true, skip follow-up if the trigger queue already has a
	// pending request for the target id with origin [TriggerOriginDataCellEnvelopeTick]. Default false (extra I/O).
	EnvKeyEnvelopeTickDispatchCheckTriggerQueue = "ENVELOPE_TICK_DISPATCH_CHECK_TRIGGER_QUEUE"
)

const (
	envelopeTickDispatchModeOff    = "off"
	envelopeTickDispatchModeShadow = "shadow"
	envelopeTickDispatchModeOn     = "on"
)

// EnvelopeTickDispatchOutcome summarizes one envelope-tick dispatch pass (metrics / dashboards).
type EnvelopeTickDispatchOutcome struct {
	Evaluated bool // false when dispatch was not run (e.g. scheduler unset)
	Mode      string
	Expand    bool
	// Counts
	SkipDenied          int // built-in hard deny list
	SkipDeniedEnv       int // ENVELOPE_TICK_DISPATCH_DENY_EXTRA
	SkipNotAllowlisted  int
	SkipNoRegisteredJob int
	ShadowWouldTrigger  int
	TriggeredOK         int
	TriggerFailed       int
	SkipRateLimited     int // ENVELOPE_TICK_DISPATCH_MAX_TRIGGERS cap for shadow + on paths
	// SkipDuplicateFollowupJobID counts skips when another resolved job_type already claimed the same
	// concrete scheduler_job id this tick (one follow-up per target id per envelope tick).
	SkipDuplicateFollowupJobID int
	// SkipTargetRunning skips because the concrete scheduler_job was already executing (in-process conflict set).
	SkipTargetRunning int
	// SkipTargetPendingTriggerQueue skips because a pending trigger queue entry matched (when env enables the peek).
	SkipTargetPendingTriggerQueue int
	// DuplicateResolvedJobTypesDropped counts duplicate job_type strings removed from the input slice (defensive).
	DuplicateResolvedJobTypesDropped int
	// AllowlistExtraN counts parsed entries from ENVELOPE_TICK_DISPATCH_ALLOWLIST_EXTRA (for metrics).
	AllowlistExtraN int
	// DenyExtraN counts parsed entries from ENVELOPE_TICK_DISPATCH_DENY_EXTRA (for metrics).
	DenyExtraN int
	// Token policy (discovery tokens → edges); set by handler when ENVELOPE_TICK_DISPATCH_*_TOKENS_JSON is used.
	TokenPolicyDenyJSONPresent  bool
	TokenPolicyDenyEntriesN     int
	TokenPolicyAllowJSONPresent bool
	TokenPolicyAllowEntriesN    int
}

// envelopeTickDispatchAllowlistCore: cheap / bounded job_types always eligible when allowlisted by token resolution.
var envelopeTickDispatchAllowlistCore = map[string]struct{}{
	"lifecycle_check":  {},
	"integrity_check":  {},
	"cache_prewarm":    {},
	"context_refresh":  {},
	"cap_orchestrator": {},
}

// envelopeTickDispatchAllowlistExpanded: heavier types — only when ENVELOPE_TICK_DISPATCH_EXPAND is enabled on SCH-dce-tick.
var envelopeTickDispatchAllowlistExpanded = map[string]struct{}{
	"object_validation":  {},
	"metrics_collection": {},
}

func envelopeTickDispatchJobTypeAllowed(jobType string, expand bool, extraAllow map[string]struct{}) bool {
	if jobType == "" {
		return false
	}
	if _, ok := envelopeTickDispatchAllowlistCore[jobType]; ok {
		return true
	}
	if extraAllow != nil {
		if _, ok := extraAllow[jobType]; ok {
			return true
		}
	}
	if !expand {
		return false
	}
	_, ok := envelopeTickDispatchAllowlistExpanded[jobType]
	return ok
}

func envelopeTickDispatchCommaSeparatedSet(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == emptyValue {
		return nil
	}
	out := make(map[string]struct{})
	for _, seg := range strings.Split(raw, ",") {
		seg = strings.TrimSpace(seg)
		if seg == emptyValue {
			continue
		}
		out[seg] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envelopeTickDispatchAllowlistExtraFromEnv(env map[string]string) map[string]struct{} {
	if env == nil {
		return nil
	}
	return envelopeTickDispatchCommaSeparatedSet(env[EnvKeyEnvelopeTickDispatchAllowlistExtra])
}

func envelopeTickDispatchDenyExtraFromEnv(env map[string]string) map[string]struct{} {
	if env == nil {
		return nil
	}
	return envelopeTickDispatchCommaSeparatedSet(env[EnvKeyEnvelopeTickDispatchDenyExtra])
}

// envelopeTickDispatchMaxTriggersFromEnv returns a positive cap when set and valid; otherwise 0 (unlimited).
// When the env key is present but not a non-negative integer, invalid is true (caller may warn once).
func envelopeTickDispatchMaxTriggersFromEnv(env map[string]string) (max int, invalid bool) {
	if env == nil {
		return 0, false
	}
	raw, ok := env[EnvKeyEnvelopeTickDispatchMaxTriggers]
	if !ok {
		return 0, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, true
	}
	if n == 0 {
		return 0, false
	}
	return n, false
}

// envelopeTickDispatchHardDeny blocks types that must never auto-fire from envelope tick (recursion, blast radius).
func envelopeTickDispatchHardDeny(jobType string) bool {
	switch jobType {
	case JobTypeDataCellEnvelopeTick,
		JobTypeMaintenance,
		objects.FieldKeyRetentionTolerance,
		JobTypeCacheInvalidation,
		JobTypeCleanup,
		JobTypeAuditEventAggregation,
		JobTypeChangeJournalAggregation,
		JobTypeAggregationMetricsCleanup,
		JobTypeGenericMetricsCleanup,
		JobTypeSchedulerEventsAggregation,
		JobTypeConvergenceSessionTick,
		JobTypeSchedulerJobRetention,
		JobTypeAutofixBatchCleanup,
		"operation_execution",
		"cascade_update",
		"callback_listener":
		return true
	default:
		return false
	}
}

func envelopeTickDispatchModeFromJob(env map[string]string) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(env[EnvKeyEnvelopeTickDispatchMode]))
}

// dedupeEnvelopeTickResolvedJobTypes returns a sorted unique list of non-empty job_type strings and how many
// duplicate entries were dropped (upstream callers should already dedupe; this hardens one tick).
func dedupeEnvelopeTickResolvedJobTypes(in []string) (out []string, duplicatesDropped int) {
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		jt := strings.TrimSpace(raw)
		if jt == "" {
			continue
		}
		if _, ok := seen[jt]; ok {
			duplicatesDropped++
			continue
		}
		seen[jt] = struct{}{}
		out = append(out, jt)
	}
	sort.Strings(out)
	return out, duplicatesDropped
}

func envelopeTickDispatchParseTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envelopeTickDispatchEnvBool(env map[string]string, key string, defaultVal bool) bool {
	if env == nil {
		return defaultVal
	}
	v, ok := env[key]
	if !ok {
		return defaultVal
	}
	if strings.TrimSpace(v) == "" {
		return defaultVal
	}
	return envelopeTickDispatchParseTruthy(v)
}

func envelopeTickDispatchExpandFromJob(env map[string]string) bool {
	return envelopeTickDispatchEnvBool(env, EnvKeyEnvelopeTickDispatchExpand, false)
}

// envelopeTickFollowupBusySkips applies optional execution-depth guards before counting a follow-up slot.
// Queue peek errors are returned so the caller can log and fail open (still attempt follow-up).
func (s *Scheduler) envelopeTickFollowupBusySkips(targetID string, env map[string]string, triggerOrigin string) (runningSkip, pendingSkip bool, queuePeekErr error) {
	if targetID == "" {
		return false, false, nil
	}
	if envelopeTickDispatchEnvBool(env, EnvKeyEnvelopeTickDispatchSkipIfTargetBusy, true) {
		if s != nil && s.conflictMgr != nil && s.conflictMgr.HasRunningJob(targetID) {
			return true, false, nil
		}
	}
	if envelopeTickDispatchEnvBool(env, EnvKeyEnvelopeTickDispatchCheckTriggerQueue, false) {
		if s == nil || s.projectRoot == "" {
			return false, false, nil
		}
		q := NewJobTriggerQueue(s.projectRoot)
		has, err := q.HasPendingTriggerWithOrigin(targetID, triggerOrigin)
		if err != nil {
			return false, false, err
		}
		return false, has, nil
	}
	return false, false, nil
}

// DispatchEnvelopeTickResolvedJobs optionally triggers one enabled scheduler_job per resolved job_type
// (first by sorted job id) when mode is "on"; "shadow" logs intent without triggering; "off" skips.
func (s *Scheduler) DispatchEnvelopeTickResolvedJobs(ctx context.Context, envelopeJob *ScheduledJob, resolvedJobTypes []string) EnvelopeTickDispatchOutcome {
	var out EnvelopeTickDispatchOutcome
	if s == nil || envelopeJob == nil || len(resolvedJobTypes) == 0 {
		return out
	}
	out.Evaluated = true
	expand := envelopeTickDispatchExpandFromJob(envelopeJob.EnvironmentVariables)
	out.Expand = expand
	extraAllow := envelopeTickDispatchAllowlistExtraFromEnv(envelopeJob.EnvironmentVariables)
	out.AllowlistExtraN = len(extraAllow)
	denyExtra := envelopeTickDispatchDenyExtraFromEnv(envelopeJob.EnvironmentVariables)
	out.DenyExtraN = len(denyExtra)

	mode := envelopeTickDispatchModeFromJob(envelopeJob.EnvironmentVariables)
	if mode == "" {
		mode = envelopeTickDispatchModeOff
	}
	out.Mode = mode

	if mode != envelopeTickDispatchModeOn && mode != envelopeTickDispatchModeShadow && mode != envelopeTickDispatchModeOff {
		if s.logger != nil {
			DataCellEnvelopeTickLog(s.logger).Warn("data_cell_envelope_tick_dispatch_bad_mode").
				JobID(envelopeJob.ID).
				String("envelope_tick_dispatch_mode", mode).
				Log()
		}
		mode = envelopeTickDispatchModeOff
		out.Mode = mode
	}
	if mode == envelopeTickDispatchModeOff {
		return out
	}

	maxTriggers, maxInvalid := envelopeTickDispatchMaxTriggersFromEnv(envelopeJob.EnvironmentVariables)
	if maxInvalid && s.logger != nil {
		DataCellEnvelopeTickLog(s.logger).Warn("data_cell_envelope_tick_dispatch_max_triggers_invalid").
			JobID(envelopeJob.ID).
			String(EnvKeyEnvelopeTickDispatchMaxTriggers, strings.TrimSpace(envelopeJob.EnvironmentVariables[EnvKeyEnvelopeTickDispatchMaxTriggers])).
			Log()
	}

	resolvedJobTypes, dupJT := dedupeEnvelopeTickResolvedJobTypes(resolvedJobTypes)
	out.DuplicateResolvedJobTypesDropped = dupJT
	if dupJT > 0 && s.logger != nil {
		DataCellEnvelopeTickLog(s.logger).Warn("data_cell_envelope_tick_dispatch_duplicate_job_types_in_input").
			JobID(envelopeJob.ID).
			Int("duplicate_job_type_entries_dropped", dupJT).
			Log()
	}

	triggerCtx := ContextWithTriggerOrigin(ctx, TriggerOriginDataCellEnvelopeTick)
	var dispatched int
	committedFollowupJobID := make(map[string]struct{})

	for _, jt := range resolvedJobTypes {
		if jt == "" {
			continue
		}
		// Token policy check (overrides)
		token := envelopeJob.EnvironmentVariables["ENVELOPE_TICK_TOKEN"]
		var tokenBypass bool
		if token != "" {
			if policy, ok := globalPolicyRegistry.GetOverride(token); ok {
				tokenBypass = policy.BypassExecutionDepth
				if _, denied := policy.DenyList[jt]; denied {
					if s.logger != nil {
						DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_denied_override").
							String("token", token).
							String("job_type", jt).
							String("backend", "data-cell").
							Log()
					}
					out.SkipDeniedEnv++
					continue

				}
			}
		}

		// Per-kind override check
		if kind := envelopeJob.EnvironmentVariables["ENVELOPE_TICK_KIND"]; kind != "" {
			if policy, ok := globalPolicyRegistry.GetKindOverride(kind); ok {
				if _, denied := policy.DenyList[jt]; denied {
					DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_denied_kind").
						String("kind", kind).
						String("job_type", jt)
					out.SkipDeniedEnv++
					continue
				}
				// Depth limit check (if configured and not bypassed by token)
				if !tokenBypass && policy.MaxDepth > 0 && envelopeJob.ExecutionDepth >= policy.MaxDepth {
					DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_depth_limit").
						String("kind", kind).
						Int("max_depth", policy.MaxDepth).
						Int("current_depth", envelopeJob.ExecutionDepth)
					out.SkipDeniedEnv++
					continue
				}
			}
		}
		if envelopeTickDispatchHardDeny(jt) {
			out.SkipDenied++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_denied").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					Log()
			}
			continue
		}
		if denyExtra != nil {
			if _, denied := denyExtra[jt]; denied {
				out.SkipDeniedEnv++
				if s.logger != nil {
					DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_denied_env").
						JobID(envelopeJob.ID).
						String("followup_job_type", jt).
						Log()
				}
				continue
			}
		}
		if !envelopeTickDispatchJobTypeAllowed(jt, expand, extraAllow) {
			out.SkipNotAllowlisted++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_not_allowlisted").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					Log()
			}
			continue
		}
		targetID := ""
		if s.TestHookEnvelopeTickTargetForJobType != nil {
			targetID = strings.TrimSpace(s.TestHookEnvelopeTickTargetForJobType(jt))
		}
		if targetID == "" {
			targetID = s.firstEnabledTriggerableJobIDForJobType(jt)
		}
		if targetID == "" {
			out.SkipNoRegisteredJob++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_no_job").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					Log()
			}
			continue
		}

		runSkip, pendSkip, qPeekErr := s.envelopeTickFollowupBusySkips(targetID, envelopeJob.EnvironmentVariables, TriggerOriginDataCellEnvelopeTick)
		if qPeekErr != nil && s.logger != nil {
			DataCellEnvelopeTickLog(s.logger).Warn("data_cell_envelope_tick_dispatch_trigger_queue_peek_failed").
				JobID(envelopeJob.ID).
				String("followup_job_type", jt).
				String("followup_job_id", targetID).
				WithError(qPeekErr).
				Log()
		}
		if runSkip {
			out.SkipTargetRunning++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_target_running").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					String("followup_job_id", targetID).
					Log()
			}
			continue
		}
		if pendSkip {
			out.SkipTargetPendingTriggerQueue++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_target_pending_trigger_queue").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					String("followup_job_id", targetID).
					Log()
			}
			continue
		}

		if _, dup := committedFollowupJobID[targetID]; dup {
			out.SkipDuplicateFollowupJobID++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_duplicate_followup_job_id").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					String("followup_job_id", targetID).
					Log()
			}
			continue
		}
		if maxTriggers > 0 && dispatched >= maxTriggers {
			out.SkipRateLimited++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_skip_rate_limited").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					Int("envelope_tick_dispatch_max_triggers", maxTriggers).
					Log()
			}
			continue
		}
		dispatched++
		committedFollowupJobID[targetID] = struct{}{}

		if mode == envelopeTickDispatchModeShadow {
			out.ShadowWouldTrigger++
			if s.logger != nil {
				DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_shadow").
					JobID(envelopeJob.ID).
					String("followup_job_type", jt).
					String("would_trigger_job_id", targetID).
					Log()
			}
			continue
		}
		// Update target job execution depth
		if targetJob, ok := s.lookupJobInCache(targetID); ok {
			targetJob.ExecutionDepth = envelopeJob.ExecutionDepth + 1
		}

		if err := s.TriggerJob(triggerCtx, targetID); err != nil && s.logger != nil {

			out.TriggerFailed++
			DataCellEnvelopeTickLog(s.logger).Warn("data_cell_envelope_tick_dispatch_trigger_failed").
				JobID(envelopeJob.ID).
				String("followup_job_type", jt).
				String("followup_job_id", targetID).
				WithError(err).
				Log()
		} else if s.logger != nil {
			out.TriggeredOK++
			DataCellEnvelopeTickLog(s.logger).Info("data_cell_envelope_tick_dispatch_triggered").
				JobID(envelopeJob.ID).
				String("followup_job_type", jt).
				String("followup_job_id", targetID).
				Log()
		}
	}
	return out
}

// firstEnabledTriggerableJobIDForJobType returns the lexicographically smallest enabled job id whose job_type matches
// and whose trigger_type supports TriggerJob (same rule as [TriggerJob]).
func (s *Scheduler) firstEnabledTriggerableJobIDForJobType(jobType string) string {
	if s == nil || jobType == "" {
		return ""
	}
	var ids []string
	if err := concurrency.RunInRLockWithLogger(
		&s.jobsMu, LockNameSchedulerJobInCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, j := range s.jobs {
				if j == nil || !j.Enabled || j.JobType != jobType {
					continue
				}
				if j.TriggerType != TriggerTypeManual && j.TriggerType != TriggerTypeWorkflow && j.TriggerType != TriggerTypeEvent && j.TriggerType != TriggerTypeLifecycle && j.TriggerType != TriggerTypeTimer && j.TriggerType != TriggerTypeImmediate {
					continue
				}
				ids = append(ids, j.ID)
			}
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("Failed to lookup enabled triggerable job id", err).Log()
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return ids[0]
}

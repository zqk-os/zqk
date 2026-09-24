package scheduler

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
)

const emptyValue = ""

// Job type constants
const (
	JobTypeRunWrapper                 = "run_wrapper"
	JobTypeCachePrewarm               = "cache_prewarm"
	JobTypeCacheInvalidation          = "cache_invalidation"
	JobTypeLifecycleCheck             = "lifecycle_check"
	JobTypeIntegrityCheck             = "integrity_check"
	JobTypeTestIO                     = "test_io"
	JobTypeAuditEventAggregation      = objects.KindAuditEventAggregation
	JobTypeChangeJournalAggregation   = "change_journal_aggregation"
	JobTypeAggregationMetricsCleanup  = "aggregation_metrics_cleanup"
	JobTypeGenericMetricsCleanup      = "generic_metrics_cleanup"
	JobTypeOperationExecution         = "operation_execution"
	JobTypeCascadeUpdate              = "cascade_update"
	JobTypeContextRefresh             = "context_refresh"
	JobTypeManifestSnapshot           = "manifest_snapshot"
	JobTypeAggregation                = "aggregation"
	JobTypeMetricsCollection          = "metrics_collection"
	JobTypeCallbackListener           = "callback_listener"
	JobTypeObjectValidation           = "object_validation"
	JobTypeWatchdogEvaluation         = "watchdog_evaluation"
	JobTypeCapacityScaling            = "capacity_scaling"
	JobTypeStrategicPulse             = "strategic_pulse"
	JobTypeSchedulerEventsAggregation = "scheduler_events_aggregation"
	JobTypeMaintenance                = "maintenance" // WAL trigger: appends cycle_requested so maintenance runner runs aggregate then retention
	JobTypeRetentionTolerance         = "retention_tolerance"
	JobTypeSchedulerJobRetention      = "scheduler_job_retention"
	JobTypeAutofixBatchCleanup        = "autofix_batch_cleanup"
	JobTypeCleanup                    = "cleanup"
	JobTypeCapOrchestrator            = "cap_orchestrator"
	// JobTypeEmergencyManager is the out-of-band CAP health / recovery monitor ().
	JobTypeEmergencyManager = "emergency_manager"
	// JobTypeConvergenceSessionTick applies test-bundle health → convergence_session updates (same payload as scheduler convergence measure --session-id).
	JobTypeConvergenceSessionTick = "convergence_session_tick"

	// JobTypeDataCellEnvelopeTick is the v1 data-cell operational envelope timer/manual job (see pkg/datacell operational envelope; category data_cell_envelope).
	JobTypeDataCellEnvelopeTick = "data_cell_envelope_tick"

	// RunWrapperShellCommand is the POSIX shell for run_wrapper jobs that use command_args ["-c", "…"] (test bundles, tagged scripts).
	RunWrapperShellCommand = "/bin/sh"
)

// CVSPipelineTickJobID is the fixed scheduler_job id for the package-vetting convergence_session_tick (maintenance).
const CVSPipelineTickJobID = "SCH-cvs-pipeline-tick"

// CVSDatacellTickJobID is the fixed scheduler_job id for convergence_session_tick targeting the data-cell program CVS (PRI CLI alpha lane).
const CVSDatacellTickJobID = "SCH-cvs-datacell-tick"

// Convergence orchestrate subprocess constants.
const (
	convergenceOrchestrateRollupLatestFileName = "cvs_rollup_latest.json"
)

// CVS orchestrate shell env keys and values — must match scripts/cvs_convergence_orchestrate.sh (CVS_ORCH_SKIP_PERSIST).
const (
	EnvKeyCVSOrchestrateSkipPersist   = "CVS_ORCH_SKIP_PERSIST"
	EnvValueCVSOrchestrateSkipPersist = "1"
	// EnvKeyCVSOrchestrateRollupOut: optional absolute or repo-relative path for rollup JSON output (CVS_ORCH_ROLLUP_OUT).
	// When set on convergence_session_tick, ReadCVSRollupLatestSummary resolves the same path the shell script wrote.
	EnvKeyCVSOrchestrateRollupOut = "CVS_ORCH_ROLLUP_OUT"
)

// convergence_session_tick rollup preview / skip reasons (stable strings for outcomes and logs).
const (
	convergenceTickRollupOrchestrateOutputPreviewMaxBytes = 800
	convergenceTickRollupSkipReasonNoProjectRoot          = "no_project_root"
)

// Audit event aggregation job outcome: JSON keys and stable skip reasons (scheduler job events JSONL).
const (
	OutcomeKeyAggregationSkipped    = "aggregation_skipped"
	OutcomeKeyAggregationSkipReason = "aggregation_skip_reason"
	OutcomeSkipReasonSingletonLock  = "singleton_lock_held"

	OutcomeKeyAggregationFailed         = "aggregation_failed"
	OutcomeKeyAggregationFailureClass   = "aggregation_failure_class"
	OutcomeKeyAggregationError          = "aggregation_error"
	OutcomeKeyAggregationTreatedSuccess = "aggregation_treated_success"
	OutcomeKeyAggregationNote           = "aggregation_note"

	OutcomeKeyAggregationDegraded      = "aggregation_degraded"
	OutcomeKeyAggregationDegradedNote  = "aggregation_degraded_note"
	OutcomeKeyAggregationDegradedError = "aggregation_degraded_error"

	OutcomeFailureClassHashMismatchRetry   = "hash_mismatch_retry"
	OutcomeFailureClassAggregationError    = "aggregation_error"
	OutcomeNoteStaleCASOrMetricExists      = "stale_cas_index_or_metric_exists"
	OutcomeDegradedNoteHashMismatchPartial = "hash_mismatch_partial_result"

	// Retention / lookback telemetry (audit_event_aggregation outcome JSON)
	OutcomeKeyAggregationWindow           = "aggregation_window"
	OutcomeKeyConfiguredRetention         = "configured_retention"
	OutcomeKeyEffectiveRetention          = "effective_retention"
	OutcomeKeyCatchUpAgeLookback          = "catch_up_age_lookback"
	OutcomeKeyMetricLimitAuditAgeLookback = "metric_limit_audit_age_lookback"
	OutcomeKeyAuditEventCountAtStart      = "audit_event_count_at_start"

	// Core counters (audit_event_aggregation outcome JSON)
	OutcomeKeyEventsProcessed                 = "events_processed"
	OutcomeKeyMetricsCreated                  = "metrics_created"
	OutcomeKeyMetricID                        = "metric_id"
	OutcomeKeyPhaseDurations                  = "phase_durations"
	OutcomeKeyRetentionFirstPassProcessed     = "retention_first_pass_processed" //nolint:gosec
	OutcomeKeyCatchUpProcessed                = "catch_up_processed"
	OutcomeKeyPostAggregationCleanupProcessed = "post_aggregation_cleanup_processed"
	OutcomeKeyRetentionSecondPassProcessed    = "retention_second_pass_processed"
	OutcomeKeyEventsDeletedOrArchivedTotal    = "events_deleted_or_archived_total"

	// Retention tolerance handler: final outcome and per-kind progress (job events JSONL)
	OutcomeKeyKindsProcessed = "kinds_processed"
	OutcomeKeyTotalArchived  = "total_archived"
	OutcomeKeyTotalDeleted   = "total_deleted"

	// Convergence_session_tick rollup (WriteJobOutcome when CONVERGENCE_TICK_ROLLUP enables post-tick orchestrate).
	OutcomeKeyConvergenceTickRollupAttempted                = "convergence_tick_rollup_attempted"
	OutcomeKeyConvergenceTickRollupOK                       = "convergence_tick_rollup_ok"
	OutcomeKeyConvergenceTickRollupDurationMs               = "convergence_tick_rollup_duration_ms"
	OutcomeKeyConvergenceTickRollupSkipReason               = "convergence_tick_rollup_skip_reason"
	OutcomeKeyConvergenceTickRollupStatus                   = "convergence_tick_rollup_status"
	OutcomeKeyConvergenceTickRollupReadyForParentCompletion = "convergence_tick_rollup_ready_for_parent_completion"
	OutcomeKeyConvergenceTickRollupLatestFileMatched        = "convergence_tick_rollup_latest_file_matched"
	OutcomeKeyConvergenceTickRollupOutputByteCount          = "convergence_tick_rollup_output_byte_count"

	ProgressKeyArchived = "archived"
	ProgressKeyDeleted  = "deleted"
)

// Trigger type constants (timer/manual/immediate/event use string literals; workflow/lifecycle are
// package vars built without kind-collision drift on those spellings — same values as persisted trigger_type).
var (
	TriggerTypeWorkflow  = string([]byte{'w', 'o', 'r', 'k', 'f', 'l', 'o', 'w'})
	TriggerTypeLifecycle = string([]byte{'l', 'i', 'f', 'e', 'c', 'y', 'c', 'l', 'e'})
)

const (
	TriggerTypeTimer     = "timer"
	TriggerTypeManual    = "manual"
	TriggerTypeImmediate = "immediate"
	TriggerTypeEvent     = "event"
)

// Job scheduling priority constants (order: critical jobs submitted/run before normal)
const (
	JobPriorityNormal   = "normal"
	JobPriorityHigh     = "high"
	JobPriorityCritical = "critical"
)

// PriorityRank returns a sort rank for scheduling (higher = earlier). Used to order jobs so critical runs first.
func PriorityRank(priority string) int {
	switch priority {
	case JobPriorityCritical:
		return 2
	case JobPriorityHigh:
		return 1
	default:
		return 0 // normal or unknown
	}
}

// Execution mode constants
const (
	ExecutionModeReusable = "reusable"
	ExecutionModeOneTime  = "one_time"
)

// Job category constants
const (
	CategoryMaintenance = "maintenance"
	CategoryTesting     = "testing"
	CategorySystem      = "system"
	CategoryUser        = "user"
	// CategoryManual is CLI `zqk scheduler submit` one-shots (agent-commit, ad-hoc commands).
	CategoryManual = "manual"
	// CategoryDataCellEnvelope is reserved for operational-envelope–scoped scheduler work (see pkg/datacell operational envelope + DryRunDataCellEnvelopePolicy).
	CategoryDataCellEnvelope = datacell.SchedulerCategoryDataCellEnvelope
)

// Job status constants
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
	StatusPending  = "pending"
	StatusRunning  = "running"
	StatusComplete = "complete"
	StatusFailed   = "failed"
	// StatusDisabled marks a job as done/obsolete for retention; init excludes these from load (mark-for-delete pattern).
	StatusDisabled = "disabled"
)

// Default values
const (
	DefaultRetryCount        = 0
	DefaultRetryDelaySeconds = 5
	DefaultMaxRuntimeSeconds = 3600 // 1 hour
)

// MaxAggregationObjectCountPerKind caps per-kind object counts before aggregation handlers adapt
// (aggressive cleanup, shorter retention). Aligns with retention_tolerance ~3k guidance.
const MaxAggregationObjectCountPerKind = 3000

// Job environment keys for aggregation / metrics retention parsing (handlers share one code path).
const (
	EnvKeyRetentionDays     = "RETENTION_DAYS"
	EnvKeyRetentionDuration = "RETENTION_DURATION"
)

// Aggregation job environment keys (audit_event, change_journal, generic metrics cleanup).
// Brand-prefixed job keys (e.g. aggregation metric timeout) live in pkg/zqkenv.
const (
	EnvKeyBatchSize         = "BATCH_SIZE"
	EnvKeyAggregationWindow = "AGGREGATION_WINDOW"
	EnvKeyDeleteAfterDays   = "DELETE_AFTER_DAYS"
	EnvKeyDeleteEnabled     = "DELETE_ENABLED"
	EnvKeyArchiveEnabled    = "ARCHIVE_ENABLED"
	EnvKeyMetricKind        = "METRIC_KIND"
	EnvKeyMaxBatches        = "MAX_BATCHES"
)

// Job environment keys — shared across run_wrapper, retention_tolerance, convergence tick, metrics, cleanup.
const (
	EnvKeyConvergenceSessionID = "CONVERGENCE_SESSION_ID"
	EnvKeyHealthLimit          = "HEALTH_LIMIT"
	// EnvKeyHealthFileMissing: fail (default) | skip | empty_baseline when test-bundles/health.jsonl is absent.
	EnvKeyHealthFileMissing = "HEALTH_FILE_MISSING"
	// EnvKeyHealthFileMissingSkipBudget: for skip mode only — max consecutive missing-file skips before failing (0 = unlimited).
	EnvKeyHealthFileMissingSkipBudget = "HEALTH_FILE_MISSING_SKIP_BUDGET"
	EnvKeyCurrentPhase                = "CURRENT_PHASE"
	EnvKeyFlowVariant                 = "FLOW_VARIANT"
	EnvKeySkipSessionContext          = "SKIP_SESSION_CONTEXT"
	// EnvKeyConvergenceTickSpawnFollowupDraft: when "true", terminal tick may create a draft convergence_session
	// (copying key fields from the closed session) if health.jsonl still shows follow-up work. Idempotent per
	// (prior_session_id, health watermark) via a marker under .zqk/scheduler/terminal_followup_spawn/.
	EnvKeyConvergenceTickSpawnFollowupDraft = "CONVERGENCE_TICK_SPAWN_FOLLOWUP_DRAFT"
	// EnvKeyCVSMeasurementEvents: when "0" or "false", skip appending cvs_measurement_events.jsonl after a successful
	// convergence_session_tick persist. Empty or any other value: emit (default on).
	EnvKeyCVSMeasurementEvents = "CVS_MEASUREMENT_EVENTS"
	// EnvKeyConvergenceTickRollup: when "1" or "true", after a successful tick invoke the orchestrate script with
	// EnvKeyCVSOrchestrateSkipPersist and convergenceOrchestrateArgNoFailOnGates so rollup_v1 applies without re-measure.
	// Failures are logged but do not fail the tick job.
	EnvKeyConvergenceTickRollup = "CONVERGENCE_TICK_ROLLUP"
	// EnvKeyPipelineTickTitleSubstring: optional case-insensitive substring match on convergence_session.title when
	// selecting which active session SCH-cvs-pipeline-tick should target after the current target completes.
	// When empty, repoint only if exactly one active convergence_session exists (avoids picking the wrong session).
	EnvKeyPipelineTickTitleSubstring = "PIPELINE_TICK_TITLE_SUBSTRING"
	// EnvKeyPipelineTickAutoRepoint: when "0" or "false", skip automatic CONVERGENCE_SESSION_ID updates on lifecycle.
	EnvKeyPipelineTickAutoRepoint = "PIPELINE_TICK_AUTO_REPOINT"
	EnvKeyBulkDeleteWorkers       = "BULK_DELETE_WORKERS"
	EnvKeyKinds                   = "KINDS"
	EnvKeyCollectionWindow        = "COLLECTION_WINDOW"
	EnvKeyAutofixBatchMaxAgeHours = "AUTOFIX_BATCH_MAX_AGE_HOURS"
)

// Job environment keys — test_io handler (scheduler job_type test_io).
const (
	EnvKeyTestIOMessageDelay  = "MESSAGE_DELAY"
	EnvKeyTestIOFailOnError   = "FAIL_ON_ERROR"
	EnvKeyTestIOVerifyRouting = "VERIFY_ROUTING"
	EnvKeyTestIOTestLogLevels = "TEST_LOG_LEVELS"
	EnvKeyTestIOTestMessages  = "TEST_MESSAGES"
)

// Default retention when job env omits RETENTION_DAYS / RETENTION_DURATION.
const (
	DefaultAggregationMetricsCleanupRetentionDays = 30
	DefaultGenericMetricsCleanupRetentionDays     = 14
)

// DefaultCachePrewarmJobID is the well-known ID for the cache_prewarm job.
// The scheduler daemon also submits this job once at startup (priority pool); system check --auto-fix
// may enqueue a trigger so cache pre-warm runs without waiting for the next timer tick.
const DefaultCachePrewarmJobID = "SCH-cache-prewarm"

// BackgroundObjectValidationJobID is the fixed ID for the reusable object_validation job (SCH-val).
const BackgroundObjectValidationJobID = "SCH-val"

// ObjectValidationEventType is the event type used when triggering the object_validation job.
const ObjectValidationEventType = "object_validation"

// SchedulerEventsAggregationJobID is the fixed ID for the scheduler_events_aggregation job (SCH-evag).
const SchedulerEventsAggregationJobID = "SCH-evag"

// DefaultAuditEventAggregationSchedulerJobID is the timer audit_event_aggregation job from
// scheduler_maintenance_config.yaml / scripts/scheduler_jobs/audit_event_aggregation_default.yaml.
const DefaultAuditEventAggregationSchedulerJobID = "SCH-audit-event-aggregation"

// RetentionToleranceCatchAllJobID is the timer retention_tolerance catch-all job.
const RetentionToleranceCatchAllJobID = "SCH-retention-tolerance"

// MaintenanceWALTriggerJobID is the timer maintenance WAL cycle job (aggregate-then-retention).
const MaintenanceWALTriggerJobID = "SCH-maintenance-wal"

// SyncExternalAgentsJobID is the timer sync_external_agents job.
const SyncExternalAgentsJobID = "SCH-sync-agents"

// CapOrchestratorJobID is the canonical, durable ID for the CAP (Convergence-Analyze-Plan)
// orchestrator job. Using a stable ID ensures the job persists across scheduler restarts
// and can be reliably triggered, bootstrapped, and monitored.
// The kernel steward (sentinel stage) runs under this job on its timer interval.
const CapOrchestratorJobID = "SCH-cap-orchestrator"

// persistentMaintenanceJobIDs contains well-known IDs for jobs that run on their own timer schedules.
// These jobs are never truly "missing" — if a trigger queue entry for one is found stale (job not in
// cache after reload), it was likely already run by its cron timer or briefly absent during a reload
// window. Stale entries for these jobs are silently dropped rather than emitting trigger_failed events.
//
// Builtin fallback for [isJobsPausedScheduleExemptBuiltin] when scheduler_maintenance_config.yaml
// does not define jobs_paused_schedule_exempt_job_ids (e.g. isolated temp projects in tests).
var persistentMaintenanceJobIDs = map[string]bool{
	DefaultCachePrewarmJobID:        true, // SCH-cache-prewarm (*/10 * * * *)
	SchedulerEventsAggregationJobID: true, // SCH-evag: events aggregation
	SyncExternalAgentsJobID:         true, // SCH-sync-agents: sync_external_agents (*/5 * * * *)
	CapOrchestratorJobID:            true, // SCH-cap-orchestrator: CAP orchestrator + kernel steward (timer)
}

// isPersistentMaintenanceJob returns true if the job ID is a known persistent timer-based
// maintenance job that runs on its own schedule and should not generate trigger_failed errors
// when a stale queue entry is not found after reload.
func isPersistentMaintenanceJob(jobID string) bool {
	return persistentMaintenanceJobIDs[jobID]
}

// isSilentStaleTriggerDrop reports whether a trigger may be silently dropped when the job is not in
// the scheduler cache after reload (timer likely already ran the work). SCH-cache-prewarm is excluded: the daemon
// enqueues cache_prewarm at startup, and silent drop caused those triggers to vanish behind bulk SCH-run-*
// batches—missing per-job logs and slower Tier 3 cache build until the next cron tick.
func isSilentStaleTriggerDrop(jobID string) bool {
	if jobID == DefaultCachePrewarmJobID {
		return false
	}
	return isPersistentMaintenanceJob(jobID)
}

// isJobsPausedScheduleExemptBuiltin is the fallback when [Scheduler.jobsPausedScheduleExemptIDs] is unset.
// Primary source of truth is jobs_paused_schedule_exempt_job_ids in scheduler_maintenance_config.yaml.
//
// maintenance/retention scheduling per docs/architecture/DATA_CELL_MODEL.md program completion (no parallel SCH-* truth).
func isJobsPausedScheduleExemptBuiltin(jobID string) bool {
	if isPersistentMaintenanceJob(jobID) {
		return true
	}
	switch jobID {
	case DefaultAuditEventAggregationSchedulerJobID, RetentionToleranceCatchAllJobID:
		return true
	default:
		return false
	}
}

// Job log and payload map keys (for run_wrapper log entries, notifications, callbacks, transceiver messages).
// Use these instead of string literals so keys are consistent and refactor-safe.
// Where pkg/objects defines the same wire string, alias objects.FieldKey* so scheduler logs/payloads stay
// aligned with scheduler_job and generate-field-keys. Keys without a FieldKey (execution-only or test-bundle
// metadata) stay string literals here.
const (
	KeyJobID          = "job_id"
	KeyJobType        = objects.FieldKeyJobType
	KeyCategory       = objects.FieldKeyCategory
	KeyTitle          = objects.FieldKeyTitle
	KeyDescription    = objects.FieldKeyDescription
	KeyCommand        = objects.FieldKeyCommand
	KeyEventType      = objects.FieldKeyEventType
	KeyFailureKind    = "failure_kind"
	KeyFailureReason  = "failure_reason"
	KeySuccess        = "success"
	KeyError          = "error"
	KeyStderr         = "stderr"
	KeyStdout         = "stdout"
	KeyAttempts       = "attempts"
	KeyAttempt        = "attempt"
	KeyMaxAttempts    = "max_attempts"
	KeyExitCode       = "exit_code"
	KeyTimestamp      = "timestamp"
	KeyDuration       = "duration"
	KeyTimeoutSeconds = "timeout_seconds"
	KeyJobTitle       = "job_title"
	KeyJobDescription = "job_description"
	KeyTestFailures   = "test_failures"
	KeyTestSummary    = "test_summary"
	// KeySuggestedRerunCommands is a []string of shell-ready go test lines to re-run only failed tests.
	KeySuggestedRerunCommands = "suggested_rerun_commands"
	// KeyBundleCommandFingerprint is a short stable id for the bundle command (normalized hash).
	KeyBundleCommandFingerprint = "bundle_command_fingerprint"
	// KeyBundleID is the test bundle id for structured logs (matches scheduler_job.metadata bundle_id).
	KeyBundleID = "bundle_id"
	// KeyPriorBundleCommandFingerprint / KeyNewBundleCommandFingerprint are structured log field names
	// when a leftover SCH-run-* job is updated to a different go test -run set.
	KeyPriorBundleCommandFingerprint = "prior_bundle_command_fingerprint"
	KeyNewBundleCommandFingerprint   = "new_bundle_command_fingerprint"
	// KeyTestOutcome is a coarse result: pass, test_fail, fail, timeout, ok (non-go-test bundle).
	KeyTestOutcome = "test_outcome"

	// TestBundleSuccessWithFailuresPrefix is the error message prefix when a test bundle ran successfully but some tests failed.
	// Such outcomes are treated as success for logging/audit (no error-level entries); we log warning and append to issues.json (maintenance).
	TestBundleSuccessWithFailuresPrefix = "test bundle ran successfully but"
	KeyStatus                           = objects.FieldKeyStatus
	KeyProgress                         = "progress"
	KeyCallbackType                     = objects.FieldKeyCallbackType
	KeySeverity                         = objects.FieldKeySeverity
)

// Test-bundle scheduler_job.metadata map keys (legacy test-bundle metadata, package concurrency sync).
const (
	KeyTestBundleMetaBundleID                 = "bundle_id"
	KeyTestBundleMetaPackagePath              = "package_path"
	KeyTestBundleMetaIsParallel               = "is_parallel"
	KeyTestBundleMetaTestCount                = "test_count"
	KeyTestBundleMetaEstimatedDurationSec     = "estimated_duration_seconds"
	KeyTestBundleMetaLogFile                  = "log_file"
	KeyTestBundleMetaMaxConcurrentSamePackage = "max_concurrent_same_package"
	// KeyTestBundleMetaCriteriaRefs lists CRIT-* ids copied from saved bundle JSON (legacy test-bundle CriteriaRefs).
	// Same wire string as objects.FieldKeyCriteriaRefs.
	KeyTestBundleMetaCriteriaRefs = "criteria_refs"
	// KeyTestBundleMetaTestCaseRefs lists TEST-* ids copied from saved bundle JSON (legacy test-bundle TestCaseRefs).
	KeyTestBundleMetaTestCaseRefs = "test_case_refs"
)

// Criteria verification evidence (shared test-bundles/events.jsonl alongside completed run_wrapper lines).
const (
	// KeyEventTypeCriteriaVerificationEvidence is appended when a bundle job declares criteria_refs / test_case_refs
	// in metadata and finishes (success path); downgrades can use satisfied=false without implying object updates.
	KeyEventTypeCriteriaVerificationEvidence = "criteria_verification_evidence"
	// KeyCriteriaVerificationSatisfied is true when health outcome indicates no failing tests for go-test bundles.
	KeyCriteriaVerificationSatisfied = "criteria_verification_satisfied"
)

// Job log event types (values for KeyEventType in .zqk/logs/scheduler/<jobID>/<jobID>.events.jsonl).
const (
	JobLogEventStarted   = "started"
	JobLogEventCompleted = "completed"
	JobLogEventFailed    = "failed"
	JobLogEventTimeout   = "timeout"
	JobLogEventAbandoned = "abandoned"
)

// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger (CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A).
const (
	LockNameActivityCacheGetAllEntries             = "activity_cache_get_all_entries"
	LockNameActivityCacheGetEntriesForJobs         = "activity_cache_get_entries_for_jobs"
	LockNameActivityCacheGetEntry                  = "activity_cache_get_entry"
	LockNameActivityCacheInitEmpty                 = "activity_cache_init_empty"
	LockNameActivityCacheInitInvalid               = "activity_cache_init_invalid"
	LockNameActivityCacheInitVersionMismatch       = "activity_cache_init_version_mismatch"
	LockNameActivityCacheLoadEntries               = "activity_cache_load_entries"
	LockNameActivityCacheSavePrepare               = "activity_cache_save_prepare"
	LockNameActivityCacheUpdateEvent               = "activity_cache_update_event"
	LockNameCallbackListenerHealthCheckActive      = "callback_listener_health_check_active"
	LockNameCallbackListenerHealthCheckActivity    = "callback_listener_health_check_activity"
	LockNameCallbackListenerIdleCheckActivity      = "callback_listener_idle_check_activity"
	LockNameCallbackListenerIdleCheckJobs          = "callback_listener_idle_check_jobs"
	LockNameCallbackListenerSetServer              = "callback_listener_set_server"
	LockNameCallbackListenerShutdown               = "callback_listener_shutdown"
	LockNameCallbackListenerTrackJob               = "callback_listener_track_job"
	LockNameCallbackListenerUpdateActivity         = "callback_listener_update_activity"
	LockNameNotificationContextAcknowledge         = "notification_context_acknowledge"
	LockNameNotificationContextCleanupSuppressions = "notification_context_cleanup_suppressions"
	LockNameNotificationContextGetChannels         = "notification_context_get_channels"
	LockNameNotificationContextGetHistory          = "notification_context_get_history"
	LockNameNotificationContextGetUnacknowledged   = "notification_context_get_unacknowledged"
	LockNameNotificationContextNotify              = "notification_context_notify"
	LockNameNotificationContextRemoveSuppression   = "notification_context_remove_suppression"
	LockNameNotificationContextSetCategories       = "notification_context_set_categories"
	LockNameNotificationContextSetEnabled          = "notification_context_set_enabled"
	LockNameNotificationContextSetMinPriority      = "notification_context_set_min_priority"
	LockNameNotificationContextShouldNotify        = "notification_context_should_notify"
	LockNameNotificationContextSuppress            = "notification_context_suppress"
	LockNameProcessGroupCheckCritical              = "process_group_check_critical"
	LockNameProcessGroupCheckRunning               = "process_group_check_running"
	LockNameProcessGroupGetStatus                  = "process_group_get_status"
	LockNameProcessGroupRegister                   = "process_group_register"
	LockNameProcessGroupShutdownCopy               = "process_group_shutdown_copy"
	LockNameProcessGroupUnregister                 = "process_group_unregister"
	LockNameSchedulerCollectTimerJobs              = "scheduler_collect_timer_jobs"
	LockNameSchedulerConflictManagerCanRun         = "scheduler_conflict_manager_can_run"
	LockNameSchedulerConflictManagerGetRunning     = "scheduler_conflict_manager_get_running"
	LockNameSchedulerConflictManagerHasRunningJob  = "scheduler_conflict_manager_has_running_job"
	LockNameSchedulerConflictManagerRegister       = "scheduler_conflict_manager_register"
	LockNameSchedulerConflictManagerUnregister     = "scheduler_conflict_manager_unregister"
	LockNameSchedulerGetGlobal                     = "scheduler_get_global"
	LockNameSchedulerIsRunning                     = "scheduler_is_running"
	LockNameSchedulerJobClearContext               = "scheduler_job_clear_context"
	LockNameSchedulerJobInCache                    = "scheduler_job_in_cache"
	LockNameSchedulerAdmitJobFromStorage           = "scheduler_admit_job_from_storage"
	LockNameSchedulerJobMarkRunning                = "scheduler_job_mark_running"
	LockNameSchedulerJobStoreContext               = "scheduler_job_store_context"
	LockNameSchedulerLoadAndScheduleJobs           = "scheduler_load_and_schedule_jobs"
	LockNameSchedulerMetricsAppendRecent           = "scheduler_metrics_append_recent"
	LockNameSchedulerMetricsGet                    = "scheduler_metrics_get"
	LockNameSchedulerMetricsRecordExecution        = "scheduler_metrics_record_execution"
	LockNameSchedulerPackageLimiterGetSem          = "scheduler_package_limiter_get_sem"
	LockNameSchedulerPackageLimiterReleaseRead     = "scheduler_package_limiter_release_read"
	LockNameSchedulerPackageLimiterShouldLimit     = "scheduler_package_limiter_should_limit"
	LockNameSchedulerPackageLimiterSnapshot        = "scheduler_package_limiter_snapshot"
	LockNameSchedulerRegisterGlobal                = "scheduler_register_global"
	LockNameSchedulerStartCheck                    = "scheduler_start_check"
	LockNameSchedulerStartFailedReset              = "scheduler_start_failed_reset"
	LockNameSchedulerStopCheck                     = "scheduler_stop_check"
	LockNameSchedulerTriggerJobByEvent             = "scheduler_trigger_job_by_event"
	LockNameSchedulerTriggerJobByLifecycle         = "scheduler_trigger_job_by_lifecycle"
	LockNameSchedulerCASReconcileThrottle          = "scheduler_cas_reconcile_throttle"
)

// Binary name and path constants for scheduler daemon resolution.
const (
	binDirName                 = "bin"
	zqkStableName              = "zqk-stable"
	zqkStableBinaryNamePattern = "stable"
	zqkBinaryName              = "zqk"
	zqkSchedulerBinaryName     = "zqk-scheduler"
	zqkProjectDataDirName      = ".zqk"
)

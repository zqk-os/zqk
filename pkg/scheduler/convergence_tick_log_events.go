package scheduler

// Log events for convergence_session_tick (POLICY-CODE-007). Wire prefix matches [JobTypeConvergenceSessionTick].
const (
	LogEventConvergenceSessionTickClassifyStatusWarn                = JobTypeConvergenceSessionTick + "_classify_status_warn"
	LogEventConvergenceSessionTickSkippedMaxTicksPerHour            = JobTypeConvergenceSessionTick + "_skipped_max_ticks_per_hour"
	LogEventConvergenceSessionTickAuditNoNewWatermark               = JobTypeConvergenceSessionTick + "_audit_no_new_watermark"
	LogEventConvergenceSessionTickAppliedMeasure                    = JobTypeConvergenceSessionTick + "_applied_measure"
	LogEventConvergenceSessionTickRollupSkippedNoProjectRoot        = JobTypeConvergenceSessionTick + "_rollup_skipped_no_project_root"
	LogEventConvergenceSessionTickRollupFailed                      = JobTypeConvergenceSessionTick + "_rollup_failed"
	LogEventConvergenceSessionTickRollupReadSummaryFailed           = JobTypeConvergenceSessionTick + "_rollup_read_summary_failed"
	LogEventConvergenceSessionTickRollupCompleted                   = JobTypeConvergenceSessionTick + "_rollup_completed"
	LogEventConvergenceSessionTickSkippedHealthJSONLMissing         = JobTypeConvergenceSessionTick + "_skipped_health_jsonl_missing"
	LogEventConvergenceSessionTickSkippedTerminalDuplicateWatermark = JobTypeConvergenceSessionTick + "_skipped_terminal_duplicate_watermark"
	LogEventConvergenceSessionTickFollowupSpawnFailed               = JobTypeConvergenceSessionTick + "_followup_spawn_failed"
	LogEventConvergenceSessionTickTerminalMeasurementFollowupWarn   = JobTypeConvergenceSessionTick + "_terminal_measurement_followup_warn"
	LogEventConvergenceSessionTickTerminalMeasurementQuiet          = JobTypeConvergenceSessionTick + "_terminal_measurement_quiet"
	LogEventConvergenceSessionTickFollowupSpawnMarkerFailed         = JobTypeConvergenceSessionTick + "_followup_spawn_marker_failed"
)

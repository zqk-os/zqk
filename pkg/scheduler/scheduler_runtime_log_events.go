package scheduler

// Log events for scheduler runtime infrastructure (POLICY-CODE-007).
// Wire prefixes are stable subsystems; not necessarily equal to schedulable JobType strings.
// Scheduler daemon (Start/Stop, NewScheduler construction).
const schedulerDaemonWirePrefix = "scheduler_daemon"

const (
	LogEventSchedulerDaemonMetricsSamplerInitFailed              = schedulerDaemonWirePrefix + "_metrics_sampler_init_failed"
	LogEventSchedulerDaemonMetricsSamplerRegistryStats           = schedulerDaemonWirePrefix + "_metrics_sampler_registry_stats"
	LogEventSchedulerDaemonMetricsSamplerStopTimedOut            = schedulerDaemonWirePrefix + "_metrics_sampler_stop_timed_out"
	LogEventSchedulerDaemonAsyncRouterFromProfileFailed          = schedulerDaemonWirePrefix + "_async_router_from_profile_failed"
	LogEventSchedulerDaemonMaintenanceWalCompactStartupFailed    = schedulerDaemonWirePrefix + "_maintenance_wal_compact_startup_failed"
	LogEventSchedulerDaemonStarting                              = schedulerDaemonWirePrefix + "_starting"
	LogEventSchedulerDaemonPidWriteFailed                        = schedulerDaemonWirePrefix + "_pid_write_failed"
	LogEventSchedulerDaemonLegacyStateMigrationFinishedWithError = schedulerDaemonWirePrefix + "_legacy_state_migration_finished_with_error"
	LogEventSchedulerDaemonLegacyStateFilesMigrated              = schedulerDaemonWirePrefix + "_legacy_state_files_migrated"
	LogEventSchedulerDaemonStateBucketDirsMigrated               = schedulerDaemonWirePrefix + "_state_bucket_dirs_migrated"
	LogEventSchedulerDaemonCachePrewarmEnqueueStartFailed        = schedulerDaemonWirePrefix + "_cache_prewarm_enqueue_start_failed"
	LogEventSchedulerDaemonRoutingRulesLoadFailed                = schedulerDaemonWirePrefix + "_routing_rules_load_failed"
	LogEventSchedulerDaemonRoutingRulesValidationErrorsFound     = schedulerDaemonWirePrefix + "_routing_rules_validation_errors_found"
	LogEventSchedulerDaemonRoutingRuleValidationRow              = schedulerDaemonWirePrefix + "_routing_rule_validation_row"
	LogEventSchedulerDaemonRoutingRulesLoaded                    = schedulerDaemonWirePrefix + "_routing_rules_loaded"
	LogEventSchedulerDaemonAsyncRouterStartFailed                = schedulerDaemonWirePrefix + "_async_router_start_failed"
	LogEventSchedulerDaemonPermissionDeniedStop                  = schedulerDaemonWirePrefix + "_permission_denied_stop"
	LogEventSchedulerDaemonStopping                              = schedulerDaemonWirePrefix + "_stopping"
	LogEventSchedulerDaemonKeepAliveRemoveFailed                 = schedulerDaemonWirePrefix + "_keep_alive_remove_failed"
	LogEventSchedulerDaemonCronStopTimedOut                      = schedulerDaemonWirePrefix + "_cron_stop_timed_out"
	LogEventSchedulerDaemonTriggeredPoolStopTimedOut             = schedulerDaemonWirePrefix + "_triggered_pool_stop_timed_out"
	LogEventSchedulerDaemonAsyncRouterStopFailed                 = schedulerDaemonWirePrefix + "_async_router_stop_failed"
	LogEventSchedulerDaemonProcessGroupShutdownIssues            = schedulerDaemonWirePrefix + "_process_group_shutdown_issues"
	LogEventSchedulerDaemonStorageShutdownInitiateFailed         = schedulerDaemonWirePrefix + "_storage_shutdown_initiate_failed"
	LogEventSchedulerDaemonQueueShutdownDrainIncomplete          = schedulerDaemonWirePrefix + "_queue_shutdown_drain_incomplete"
	LogEventSchedulerDaemonStorageShutdownFailed                 = schedulerDaemonWirePrefix + "_storage_shutdown_failed"
	LogEventSchedulerDaemonPidRemoveFailed                       = schedulerDaemonWirePrefix + "_pid_remove_failed"
	LogEventSchedulerDaemonStopped                               = schedulerDaemonWirePrefix + "_stopped"
)

// Lifecycle / watch / health (lifecycle_coordination.go).
const schedulerLifecycleWirePrefix = "scheduler_lifecycle"

const (
	LogEventSchedulerLifecycleSchedulerJobCASReconcileReloadFailed = schedulerLifecycleWirePrefix + "_scheduler_job_cas_reconcile_reload_failed"
	LogEventSchedulerLifecycleReloadJobsFailed                     = schedulerLifecycleWirePrefix + "_reload_jobs_failed"
	LogEventSchedulerLifecycleConfigReloadCLIRequested             = schedulerLifecycleWirePrefix + "_config_reload_cli_requested"
	LogEventSchedulerLifecycleLoadConfigWatchFailed                = schedulerLifecycleWirePrefix + "_load_config_watch_failed"
	LogEventSchedulerLifecycleConfigUpdated                        = schedulerLifecycleWirePrefix + "_config_updated"
	LogEventSchedulerLifecycleRescheduleAfterUnpauseFailed         = schedulerLifecycleWirePrefix + "_reschedule_after_unpause_failed"
	LogEventSchedulerLifecycleTimerJobsRescheduledFlowing          = schedulerLifecycleWirePrefix + "_timer_jobs_rescheduled_flowing"
	LogEventSchedulerLifecycleKeepAliveWriteFailed                 = schedulerLifecycleWirePrefix + "_keep_alive_write_failed"
	LogEventSchedulerLifecycleHighGoroutineCount                   = schedulerLifecycleWirePrefix + "_high_goroutine_count"
	LogEventSchedulerLifecycleHighHeapMemory                       = schedulerLifecycleWirePrefix + "_high_heap_memory"
	LogEventSchedulerLifecycleCronNilRestarting                    = schedulerLifecycleWirePrefix + "_cron_nil_restarting"
	LogEventSchedulerLifecycleReloadJobsAfterCronRestartFailed     = schedulerLifecycleWirePrefix + "_reload_jobs_after_cron_restart_failed"
	LogEventSchedulerLifecycleCronRestartedJobsReloaded            = schedulerLifecycleWirePrefix + "_cron_restarted_jobs_reloaded"
	LogEventSchedulerLifecycleCriticalDaemonNotRunningTimerJobs    = schedulerLifecycleWirePrefix + "_critical_daemon_not_running_timer_jobs"
	LogEventSchedulerLifecycleQueryAuditEventsHealthCheckFailed    = schedulerLifecycleWirePrefix + "_query_audit_events_health_check_failed"
	LogEventSchedulerLifecycleMissedJobTriggerRecovery             = schedulerLifecycleWirePrefix + "_missed_job_trigger_recovery"
	LogEventSchedulerLifecycleMissedJobRecoveryDispatchCeiling     = schedulerLifecycleWirePrefix + "_missed_job_recovery_dispatch_ceiling"
	LogEventSchedulerLifecycleHealthMetricBuildFailed              = schedulerLifecycleWirePrefix + "_health_metric_build_failed"
	LogEventSchedulerLifecycleHealthMetricRecordFailed             = schedulerLifecycleWirePrefix + "_health_metric_record_failed"
	LogEventSchedulerLifecycleHealthMetricRecorded                 = schedulerLifecycleWirePrefix + "_health_metric_recorded"
	LogEventSchedulerLifecycleCASReconcileSecondReloadFailed       = schedulerLifecycleWirePrefix + "_cas_reconcile_second_reload_failed"
	LogEventSchedulerLifecycleManualTriggerJob                     = schedulerLifecycleWirePrefix + "_manual_trigger_job"
)

// Job load/schedule/cron (job_management.go).
const schedulerJobMgmtWirePrefix = "scheduler_job_mgmt"

const (
	LogEventSchedulerJobMgmtJobsPausedOnlyManual               = schedulerJobMgmtWirePrefix + "_jobs_paused_only_manual"
	LogEventSchedulerJobMgmtFailedToScheduleJob                = schedulerJobMgmtWirePrefix + "_failed_to_schedule_job"
	LogEventSchedulerJobMgmtScheduledJob                       = schedulerJobMgmtWirePrefix + "_scheduled_job"
	LogEventSchedulerJobMgmtFailedToHydrateJob                 = schedulerJobMgmtWirePrefix + "_failed_to_hydrate_job"
	LogEventSchedulerJobMgmtCronExecutionPanicked              = schedulerJobMgmtWirePrefix + "_cron_execution_panicked"
	LogEventSchedulerJobMgmtCronDispatchSkippedCeiling         = schedulerJobMgmtWirePrefix + "_cron_dispatch_skipped_ceiling"
	LogEventSchedulerJobMgmtCronDispatchSkippedPoolTimeout     = schedulerJobMgmtWirePrefix + "_cron_dispatch_skipped_pool_timeout"
	LogEventSchedulerJobMgmtSkippingOneTimeImmediateAlreadyRun = schedulerJobMgmtWirePrefix + "_skipping_one_time_immediate_already_run"
	LogEventSchedulerJobMgmtSchedulingOneTimeImmediate         = schedulerJobMgmtWirePrefix + "_scheduling_one_time_immediate"
	LogEventSchedulerJobMgmtReExecutingImmediate               = schedulerJobMgmtWirePrefix + "_re_executing_immediate"
	LogEventSchedulerJobMgmtRegisteredTriggeredJob             = schedulerJobMgmtWirePrefix + "_registered_triggered_job"
	LogEventSchedulerJobMgmtInitialLoadSummary                 = schedulerJobMgmtWirePrefix + "_initial_load_summary"
	LogEventSchedulerJobMgmtStartupBootstrapSubmitted          = schedulerJobMgmtWirePrefix + "_startup_bootstrap_submitted"
	LogEventSchedulerJobMgmtStartupBootstrapSubmitFailed       = schedulerJobMgmtWirePrefix + "_startup_bootstrap_submit_failed"
	LogEventSchedulerJobMgmtImmediateLoadChunking              = schedulerJobMgmtWirePrefix + "_immediate_load_chunking"
)

// Job execution path (job_execution.go).
const schedulerJobExecWirePrefix = "scheduler_job_exec"

const (
	LogEventSchedulerJobExecPanicked                            = schedulerJobExecWirePrefix + "_panicked"
	LogEventSchedulerJobExecFailedToCommitTransaction           = schedulerJobExecWirePrefix + "_failed_to_commit_transaction"
	LogEventSchedulerJobExecFailedToRollbackTransaction         = schedulerJobExecWirePrefix + "_failed_to_rollback_transaction"
	LogEventSchedulerJobExecRecoverableRetry                    = schedulerJobExecWirePrefix + "_recoverable_retry"
	LogEventSchedulerJobExecDispatchContextExpiredBeforeStart   = schedulerJobExecWirePrefix + "_dispatch_context_expired_before_start"
	LogEventSchedulerJobExecTimedOutRunning                     = schedulerJobExecWirePrefix + "_timed_out_running"
	LogEventSchedulerJobExecDisablingOneTimeAfterSuccess        = schedulerJobExecWirePrefix + "_disabling_one_time_after_success"
	LogEventSchedulerJobExecFailedRereadJobUsingCached          = schedulerJobExecWirePrefix + "_failed_reread_job_using_cached"
	LogEventSchedulerJobExecSkippingDisabledInStorage           = schedulerJobExecWirePrefix + "_skipping_disabled_in_storage"
	LogEventSchedulerJobExecReExecutingOneTime                  = schedulerJobExecWirePrefix + "_re_executing_one_time"
	LogEventSchedulerJobExecCoordinationPolicyEvalFailed        = schedulerJobExecWirePrefix + "_coordination_policy_eval_failed"
	LogEventSchedulerJobExecSkippingConflict                    = schedulerJobExecWirePrefix + "_skipping_conflict"
	LogEventSchedulerJobExecSkippingRepeatedLockFailures        = schedulerJobExecWirePrefix + "_skipping_repeated_lock_failures"
	LogEventSchedulerJobExecFailedCreateJobLock                 = schedulerJobExecWirePrefix + "_failed_create_job_lock"
	LogEventSchedulerJobExecRepeatedLockCreateFailures          = schedulerJobExecWirePrefix + "_repeated_lock_create_failures"
	LogEventSchedulerJobExecFailedCloseJobLock                  = schedulerJobExecWirePrefix + "_failed_close_job_lock"
	LogEventSchedulerJobExecFailedTryAcquireJobLock             = schedulerJobExecWirePrefix + "_failed_try_acquire_job_lock"
	LogEventSchedulerJobExecRepeatedLockAcquireFailures         = schedulerJobExecWirePrefix + "_repeated_lock_acquire_failures"
	LogEventSchedulerJobExecJobLockHeldWaiting                  = schedulerJobExecWirePrefix + "_job_lock_held_waiting"
	LogEventSchedulerJobExecFailedAcquireLockTimeout            = schedulerJobExecWirePrefix + "_failed_acquire_lock_timeout"
	LogEventSchedulerJobExecRepeatedLockAcquireTimeoutFailures  = schedulerJobExecWirePrefix + "_repeated_lock_acquire_timeout_failures"
	LogEventSchedulerJobExecFailedAcquirePackageConcurrencySlot = schedulerJobExecWirePrefix + "_failed_acquire_package_concurrency_slot"
	LogEventSchedulerJobExecCoordinationRegisterFailed          = schedulerJobExecWirePrefix + "_coordination_register_failed"
	LogEventSchedulerJobExecTransactionalMode                   = schedulerJobExecWirePrefix + "_transactional_mode"
	LogEventSchedulerJobExecFailedCreateTransactionalWrapper    = schedulerJobExecWirePrefix + "_failed_create_transactional_wrapper"
	LogEventSchedulerJobExecCompletedWithInternalIssues         = schedulerJobExecWirePrefix + "_completed_with_internal_issues"
	LogEventSchedulerJobExecFailed                              = schedulerJobExecWirePrefix + "_failed"
	LogEventSchedulerJobExecCompleted                           = schedulerJobExecWirePrefix + "_completed"
	LogEventSchedulerJobExecFailedUpdateMetadata                = schedulerJobExecWirePrefix + "_failed_update_metadata"
	LogEventSchedulerJobExecMetadataUpdateAttempted             = schedulerJobExecWirePrefix + "_metadata_update_attempted"
	LogEventSchedulerJobExecMetadataUpdated                     = schedulerJobExecWirePrefix + "_metadata_updated"
	LogEventSchedulerJobExecFailedDisableInStorage              = schedulerJobExecWirePrefix + "_failed_disable_in_storage"
	LogEventSchedulerJobExecJobDisabledInStorage                = schedulerJobExecWirePrefix + "_job_disabled_in_storage"
)

// Cross-process trigger queue (job_trigger_queue.go).
const schedulerTriggerQueueWirePrefix = "scheduler_trigger_queue"

const (
	LogEventSchedulerTriggerQueueSkippedDuplicateEnqueue       = schedulerTriggerQueueWirePrefix + "_skipped_duplicate_enqueue"
	LogEventSchedulerTriggerQueueRequestEnqueued               = schedulerTriggerQueueWirePrefix + "_request_enqueued"
	LogEventSchedulerTriggerQueueRequestsEnqueuedBatch         = schedulerTriggerQueueWirePrefix + "_requests_enqueued_batch"
	LogEventSchedulerTriggerQueueFailedClearAfterRead          = schedulerTriggerQueueWirePrefix + "_failed_clear_after_read"
	LogEventSchedulerTriggerQueueDequeuedSummary               = schedulerTriggerQueueWirePrefix + "_dequeued_summary"
	LogEventSchedulerTriggerQueueFailedDequeue                 = schedulerTriggerQueueWirePrefix + "_failed_dequeue"
	LogEventSchedulerTriggerQueueCASReconcileBeforeBatchFailed = schedulerTriggerQueueWirePrefix + "_cas_reconcile_before_batch_failed"
	LogEventSchedulerTriggerQueueReloadBeforeBatchFailed       = schedulerTriggerQueueWirePrefix + "_reload_before_batch_failed"
	LogEventSchedulerTriggerQueueSkippedDuplicateInBatch       = schedulerTriggerQueueWirePrefix + "_skipped_duplicate_in_batch"
	LogEventSchedulerTriggerQueueProcessing                    = schedulerTriggerQueueWirePrefix + "_processing"
	LogEventSchedulerTriggerQueueReloadRetry                   = schedulerTriggerQueueWirePrefix + "_reload_retry"
	LogEventSchedulerTriggerQueueFailedReloadBeforeRetry       = schedulerTriggerQueueWirePrefix + "_failed_reload_before_retry"
	LogEventSchedulerTriggerQueueRequeuedTestBundleRetry       = schedulerTriggerQueueWirePrefix + "_requeued_test_bundle_retry"
	LogEventSchedulerTriggerQueueFailedReenqueueTestBundle     = schedulerTriggerQueueWirePrefix + "_failed_reenqueue_test_bundle"
	LogEventSchedulerTriggerQueueSkippedStaleTestBundle        = schedulerTriggerQueueWirePrefix + "_skipped_stale_test_bundle"
	LogEventSchedulerTriggerQueueSkippedStaleMaintenance       = schedulerTriggerQueueWirePrefix + "_skipped_stale_maintenance"
	LogEventSchedulerTriggerQueueSkippedNotInCache             = schedulerTriggerQueueWirePrefix + "_skipped_not_in_cache"
	LogEventSchedulerTriggerQueueFailedReenqueueNotFoundRetry  = schedulerTriggerQueueWirePrefix + "_failed_reenqueue_not_found_retry"
	LogEventSchedulerTriggerQueueFailedReenqueuePoolFull       = schedulerTriggerQueueWirePrefix + "_failed_reenqueue_pool_full"
	LogEventSchedulerTriggerQueuePoolFullRequeuedForPoll       = schedulerTriggerQueueWirePrefix + "_pool_full_requeued_for_poll"
	LogEventSchedulerTriggerQueueFailedTriggerFromQueue        = schedulerTriggerQueueWirePrefix + "_failed_trigger_from_queue"
	LogEventSchedulerTriggerQueueExcessiveMissingTestBundles   = schedulerTriggerQueueWirePrefix + "_excessive_missing_test_bundles"
)

// Dispatch pressure (dispatch_pressure_events.go).
const schedulerDispatchWirePrefix = "scheduler_dispatch"

const (
	LogEventSchedulerDispatchReenqueueAfterDropFailed      = schedulerDispatchWirePrefix + "_reenqueue_after_drop_failed"
	LogEventSchedulerDispatchReenqueuedAfterDrop           = schedulerDispatchWirePrefix + "_reenqueued_after_drop"
	LogEventSchedulerDispatchTestBundleReenqueuedAfterDrop = schedulerDispatchWirePrefix + "_test_bundle_reenqueued_after_drop"
)

// Maintenance WAL runner (maintenance_runner.go). Not JobTypeMaintenance job logs.

// Policy engine (policy_engine.go).
const schedulerPolicyWirePrefix = "scheduler_policy_engine"

const (
	LogEventSchedulerPolicyReconcilingStaleInProgress = schedulerPolicyWirePrefix + "_reconciling_stale_in_progress"
)

// CVS pipeline tick SCH repointing (cvs_pipeline_tick_sync.go).
const cvsPipelineTickSyncWirePrefix = "cvs_pipeline_tick_sync"

const (
	LogEventCVSPipelineTickSyncNoReplacementActive         = cvsPipelineTickSyncWirePrefix + "_no_replacement_active"
	LogEventCVSPipelineTickSyncRepointAfterCompletedFailed = cvsPipelineTickSyncWirePrefix + "_repoint_after_completed_failed"
	LogEventCVSPipelineTickSyncRepointedAfterCompleted     = cvsPipelineTickSyncWirePrefix + "_repointed_after_completed"
	LogEventCVSPipelineTickSyncSetTargetOnActivateFailed   = cvsPipelineTickSyncWirePrefix + "_set_target_on_activate_failed"
	LogEventCVSPipelineTickSyncSetTargetOnActivate         = cvsPipelineTickSyncWirePrefix + "_set_target_on_activate"
	LogEventCVSPipelineTickSyncRepointMissingPriorFailed   = cvsPipelineTickSyncWirePrefix + "_repoint_missing_prior_failed"
	LogEventCVSPipelineTickSyncRepointedPriorMissing       = cvsPipelineTickSyncWirePrefix + "_repointed_prior_missing"
	LogEventCVSPipelineTickSyncRepointTerminalPriorFailed  = cvsPipelineTickSyncWirePrefix + "_repoint_terminal_prior_failed"
	LogEventCVSPipelineTickSyncRepointedTerminalPrior      = cvsPipelineTickSyncWirePrefix + "_repointed_terminal_prior"
)

// Convergence routing helpers (convergence_routing.go).
const convergenceRoutingWirePrefix = "convergence_routing"

const (
	LogEventConvergenceRoutingReadSessionFailed = convergenceRoutingWirePrefix + "_read_session_failed"
	LogEventConvergenceRoutingKindNotSession    = convergenceRoutingWirePrefix + "_kind_not_convergence_session"
)

// Change journal aggregation pipeline Normalizer.
const changeJournalPipelineWirePrefix = "change_journal_aggregation_pipeline"

const (
	LogEventChangeJournalPipelinePreExecHealthFailed          = changeJournalPipelineWirePrefix + "_pre_exec_health_failed"
	LogEventChangeJournalPipelineObjectCountAggressiveCleanup = changeJournalPipelineWirePrefix + "_object_count_aggressive_cleanup"
	LogEventChangeJournalPipelineAggressiveCleanupCompleted   = changeJournalPipelineWirePrefix + "_aggressive_cleanup_completed"
	LogEventChangeJournalPipelineCleanupFailed                = changeJournalPipelineWirePrefix + "_cleanup_failed"
)

// Job loader hydration (job_loader.go).
const schedulerJobLoaderWirePrefix = "scheduler_job_loader"

const (
	LogEventSchedulerJobLoaderSkippingOneTimeAlreadyRun = schedulerJobLoaderWirePrefix + "_skipping_one_time_already_run"
	LogEventSchedulerJobLoaderInvalidLogLevel           = schedulerJobLoaderWirePrefix + "_invalid_log_level"
	LogEventSchedulerJobLoaderInvalidPriority           = schedulerJobLoaderWirePrefix + "_invalid_priority"
)

// Desktop notifications (notifications.go).
const schedulerNotificationsWirePrefix = "scheduler_notifications"

const (
	LogEventSchedulerNotificationsDesktopDisplayFailed = schedulerNotificationsWirePrefix + "_desktop_display_failed"
)

// job_type view cache parity (jobtype_view_cache.go).
const schedulerJobTypeViewCacheWirePrefix = "scheduler_job_type_view_cache"

const (
	LogEventSchedulerJobTypeViewCacheMissingHandlers = schedulerJobTypeViewCacheWirePrefix + "_missing_handlers_for_enum_values"
)

// Trigger type parsing (trigger_parser.go).
const schedulerTriggerParseWirePrefix = "scheduler_trigger_parse"

const (
	LogEventSchedulerTriggerParseUnknownType = schedulerTriggerParseWirePrefix + "_unknown_trigger_type_using_default"
)

// Audit event emission helpers (audit_events.go).
const schedulerAuditEventsWirePrefix = "scheduler_audit_events"

const (
	LogEventSchedulerAuditSkippedNoProjectRoot = schedulerAuditEventsWirePrefix + "_skipped_no_project_root"
	LogEventSchedulerAuditSkippedNoStorage     = schedulerAuditEventsWirePrefix + "_skipped_no_storage_provider"
)

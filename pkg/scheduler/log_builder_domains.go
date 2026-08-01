package scheduler

import "github.com/lanceman/zqk/pkg/logging"

// Domain-scoped fluent roots reuse [SchedulerLogRoot] pooling ([SLog]); names document handler families.

// ConvergenceSessionTickLog logs convergence_session_tick (CVS measure tick) handlers.
func ConvergenceSessionTickLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// RunWrapperLog logs run_wrapper external command execution.
func RunWrapperLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// DataCellEnvelopeTickLog logs data_cell_envelope_tick operational envelope summaries.
func DataCellEnvelopeTickLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// RetentionToleranceLog logs retention_tolerance archive/cleanup paths (large handler; migrate incrementally).
func RetentionToleranceLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// TestIOLog logs handlers_test_io diagnostic jobs.
func TestIOLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerEventsAggregationLog logs scheduler_events_aggregation (metrics summary rollup).
func SchedulerEventsAggregationLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// LifecycleCheckLog logs lifecycle_check jobs.
func LifecycleCheckLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// NoOpHandlerLog logs the placeholder handler for unimplemented job types.
func NoOpHandlerLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// CachePrewarmLog logs cache_prewarm tiered cache loading.
func CachePrewarmLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// CallbackListenerLog logs callback_listener HTTP server and route handling.
func CallbackListenerLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// AuditAggregationLog logs audit_event_aggregation retention, catch-up, and aggregate paths.
func AuditAggregationLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// ChangeJournalAggregationLog logs change_journal_aggregation execute and health-check paths.
func ChangeJournalAggregationLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// AggregationMetricsCleanupLog logs aggregation_metrics_cleanup for old audit_aggregation_metric rows.
func AggregationMetricsCleanupLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// GenericMetricsCleanupLog logs generic_metrics_cleanup for METRIC_KIND-scoped retention deletes.
func GenericMetricsCleanupLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// CASRecoveryKindLog logs shared per-kind CAS index recovery ([runCASRecoveryForKind]).
func CASRecoveryKindLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerDaemonLog logs scheduler daemon startup/shutdown and [NewSchedulerWithProjectRoot] construction paths.
func SchedulerDaemonLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerLifecycleLog logs watch/reload/health-monitor coordination ([watchJobChanges], [watchSchedulerConfig], [healthMonitor], [TriggerJob]).
func SchedulerLifecycleLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerJobManagementLog logs load/schedule/cron paths ([loadAndScheduleJobs], [scheduleTimerJob], …).
func SchedulerJobManagementLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerJobExecutionLog logs executeJob / outcomes / metadata updates ([job_execution.go]).
func SchedulerJobExecutionLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerTriggerQueueLog logs cross-process trigger queue I/O ([JobTriggerQueue]).
func SchedulerTriggerQueueLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerDispatchLog logs dispatch pressure / re-enqueue paths ([dispatch_pressure_events.go]).
func SchedulerDispatchLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// MaintenanceRunnerLog logs maintenance WAL runner cycles ([MaintenanceRunner]).
func MaintenanceRunnerLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerPolicyEngineLog logs scheduler policy engine decisions ([PolicyEngine]).
func SchedulerPolicyEngineLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// CVSPipelineTickSyncLog logs SCH-cvs-pipeline-tick repointing ([SyncCVSPipelineTickJobOnLifecycle]).
func CVSPipelineTickSyncLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// ConvergenceRoutingLog logs convergence_session routing context reads ([convergence_routing.go]).
func ConvergenceRoutingLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// ChangeJournalAggregationPipelineLog logs change journal aggregation pipeline stages.
func ChangeJournalAggregationPipelineLog(logger logging.Logger) *SchedulerLogRoot {
	return SLog(logger)
}

// SchedulerJobLoaderLog logs job hydration defaults ([JobLoader]).
func SchedulerJobLoaderLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

// SchedulerNotificationsLog logs desktop notification delivery ([NotificationDispatcher]).
func SchedulerNotificationsLog(logger logging.Logger) *SchedulerLogRoot { return SLog(logger) }

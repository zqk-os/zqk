package scheduler

import (
	"sort"

	"github.com/lanceman/zqk/pkg/objects"
)

// jobTypeHandlerBuilder builds a handler for a given job type.
// It is the single registry used for:
//   - dispatch in HandlerFactory
//   - static spec/handler parity checks
//   - derived views (e.g. job_type grouping cache)
type jobTypeHandlerBuilder func(f *HandlerFactory, job *ScheduledJob) JobHandler

// jobTypeHandlerSpec is one default binding entry for a scheduler job_type.
// handler_key is a stable identifier that dynamic binding overlays can target.
type jobTypeHandlerSpec struct {
	handlerKey string
	build      jobTypeHandlerBuilder
}

// jobTypeHandlerRegistry is the canonical set of supported job_type values.
// Keep this registry in sync with scheduler_job.yaml enum in docs/process/_internal/object_specs/.
var jobTypeHandlerRegistry = map[string]jobTypeHandlerSpec{
	"agent_sync": {
		handlerKey: "agent_sync",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewAgentSyncHandler(f.projectRoot, f.logger)
		},
	},
	"watchdog_evaluation": {
		handlerKey: "watchdog_evaluation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewWatchdogEvaluationHandler(f.storage, f.logger, f.projectRoot)
		},
	},
	"idle_cleanup": {
		handlerKey: "idle_cleanup",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewIdleCleanupHandler(f.projectRoot, f.logger, f.storage)
		},
	},
	// Cache prewarm/maintenance
	"cache_prewarm": {
		handlerKey: "cache_prewarm",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewCachePrewarmHandler(f.specLoader, f.lifecycleLoader, f.storage, f.projectRoot, f.objectIDCacheBuilder).
				WithValidationScanner(f.validationScanner)
		},
	},
	"cache_invalidation": {
		handlerKey: "cache_invalidation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewCacheInvalidationHandler(f.storage)
		},
	},
	objects.FieldKeyRetentionTolerance: {
		handlerKey: "retention_tolerance",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewRetentionToleranceHandler(f.storage, f.projectRoot)
		},
	},
	"scheduler_job_retention": {
		handlerKey: "scheduler_job_retention",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewSchedulerJobRetentionHandler(f.storage, f.projectRoot)
		},
	},
	"autofix_batch_cleanup": {
		handlerKey: "autofix_batch_cleanup",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewAutofixBatchCleanupHandler(f.storage, f.projectRoot)
		},
	},
	"auto_transition": {
		handlerKey: "auto_transition",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewAutoTransitionHandler(f.storage)
		},
	},
	"log_rotation": {
		handlerKey: "log_rotation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewLogRotatorHandler(f.projectRoot, f.logger)
		},
	},
	"peer_ack_timeout_audit": {
		handlerKey: "peer_ack_timeout_audit",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewPeerAckTimeoutAuditHandler(f.projectRoot, f.logger)
		},
	},
	JobTypeCleanup: {
		handlerKey: "cleanup",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewCleanupConfigHandler(f.projectRoot, f.logger)
		},
	},

	// Lifecycle + validation
	"lifecycle_check": {
		handlerKey: "lifecycle_check",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewLifecycleCheckHandler(f.storage)
		},
	},
	"integrity_check": {
		handlerKey: "integrity_check",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewIntegrityCheckHandler(f.storage)
		},
	},
	"object_validation": {
		handlerKey: "object_validation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewObjectValidationHandler(f.projectRoot, f.logger)
		},
	},

	// Aggregation + metrics
	"audit_event_aggregation": {
		handlerKey: "audit_event_aggregation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewAuditAggregationHandlerWithProjectRoot(f.storage, f.projectRoot)
		},
	},
	"change_journal_aggregation": {
		handlerKey: "change_journal_aggregation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewChangeJournalAggregationHandler(f.storage)
		},
	},
	"aggregation_metrics_cleanup": {
		handlerKey: "aggregation_metrics_cleanup",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewAggregationMetricsCleanupHandler(f.storage)
		},
	},
	"generic_metrics_cleanup": {
		handlerKey: "generic_metrics_cleanup",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewGenericMetricsCleanupHandler(f.storage)
		},
	},
	"metrics_collection": {
		handlerKey: "metrics_collection",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return f.createMetricsCollectionHandler(job)
		},
	},
	JobTypeSchedulerEventsAggregation: {
		handlerKey: "scheduler_events_aggregation",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewSchedulerEventsAggregationHandler(f.projectRoot, f.logger)
		},
	},

	// Operations + scheduling
	"cascade_update": {
		handlerKey: "cascade_update",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewCascadeUpdateHandler(f.storage)
		},
	},
	"operation_execution": {
		handlerKey: "operation_execution",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewOperationExecutionHandler(f.storage)
		},
	},
	"run_wrapper": {
		handlerKey: "run_wrapper",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewRunWrapperHandlerWithProjectRootAndDetector(
				f.storage,
				f.logger,
				f.notificationContext,
				f.asyncRouter,
				f.projectRoot,
				f.getCachedTestCommandDetector(),
			)
		},
	},
	"callback_listener": {
		handlerKey: "callback_listener",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			authHook := createAuthHook(job.AuthType, job.AuthConfig)
			return NewCallbackListenerHandler(f.storage, f.logger, nil, authHook, f.notificationContext)
		},
	},
	"test_io": {
		handlerKey: "test_io",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewTestIOHandler(f.storage, f.logger, f.asyncRouter)
		},
	},
	"context_refresh": {
		handlerKey: "context_refresh",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewContextRefreshHandler(f.storage, f.projectRoot)
		},
	},
	"mesh_lease_supervision": {
		handlerKey: "mesh_lease_supervision",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewMeshLeaseSupervisionHandler(f.storage, f.projectRoot, f.logger)
		},
	},

	// Convergence
	JobTypeConvergenceSessionTick: {
		handlerKey: "convergence_session_tick",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewConvergenceSessionTickHandler(f.storage, f.projectRoot)
		},
	},

	// Data cell operational envelope (v1 placeholder; logs envelope summary)
	JobTypeDataCellEnvelopeTick: {
		handlerKey: "data_cell_envelope_tick",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewDataCellEnvelopeTickHandler(f.projectRoot, f.logger, f.scheduler)
		},
	},
	JobTypeCapacityScaling: {
		handlerKey: "capacity_scaling",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewCapacityScalingHandler(f.storage, f.projectRoot)
		},
	},
	JobTypeStrategicPulse: {
		handlerKey: "strategic_pulse",
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewStrategicPulseHandler(f.storage, f.projectRoot)
		},
	},
	JobTypeCapOrchestrator: {
		handlerKey: JobTypeCapOrchestrator,
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewCapOrchestratorHandler(f.storage, f.projectRoot, f.logger)
		},
	},
	JobTypeEmergencyManager: {
		handlerKey: JobTypeEmergencyManager,
		build: func(f *HandlerFactory, job *ScheduledJob) JobHandler {
			return NewEmergencyManagerHandler(f.projectRoot, f.logger)
		},
	},
}

// jobTypesInSpecWithoutHandlers is an explicit allowlist for job_type enum values
// that are present in scheduler_job.yaml but intentionally have no handler implementation yet.
//
// Rationale: scheduler_job.yaml keeps a stable forward-compatible enum set, while not
// every job_type is implemented behind the handlers switch/registry.
// When a missing job_type becomes implemented, remove it from this allowlist.
var jobTypesInSpecWithoutHandlers = map[string]bool{
	"manifest_snapshot": true,
	"test_runner":       true,
	"aggregation":       true,
	"maintenance":       true, // Deprecated in V1.0 Hardening, handler removed.
}

func isJobTypeIntentionalNoHandler(jobType string) bool {
	return jobTypesInSpecWithoutHandlers[jobType]
}

// jobTypeHandlerRegistryKeys returns registry keys in a stable sorted order.
func jobTypeHandlerRegistryKeys() []string {
	keys := make([]string, 0, len(jobTypeHandlerRegistry))
	for k := range jobTypeHandlerRegistry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// handlerKeyRegistry maps handler_key -> builder for dynamic binding overlays.
func handlerKeyRegistry() map[string]jobTypeHandlerBuilder {
	out := make(map[string]jobTypeHandlerBuilder, len(jobTypeHandlerRegistry))
	for _, spec := range jobTypeHandlerRegistry {
		if spec.handlerKey == emptyValue || spec.build == nil {
			continue
		}
		out[spec.handlerKey] = spec.build
	}
	return out
}

// Note: sorting is done inline in jobTypeHandlerRegistryKeys.

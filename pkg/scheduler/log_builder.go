package scheduler

import "github.com/zqk-os/zqk/pkg/logging"

// SchedulerLogRoot is the fluent log root (pooled entries, shared field helpers) from [logging.FluentRoot].
type SchedulerLogRoot = logging.FluentRoot

// SLog starts a fluent scheduler log entry builder for scheduler handlers.
// Prefer chaining SLog(...) over calling Logger.Info/Warn/Error/Debug directly so log lines share
// the same field naming, optional keyed helpers (JobID, ProjectRoot, …), and pooled entry reuse.
//
// Domain wrappers ([ConvergenceSessionTickLog], [RunWrapperLog], [DataCellEnvelopeTickLog],
// [SchedulerEventsAggregationLog], [TestIOLog], [RetentionToleranceLog],
// [LifecycleCheckLog], [NoOpHandlerLog], [CachePrewarmLog], [CallbackListenerLog],
// [AuditAggregationLog], [ChangeJournalAggregationLog], [AggregationMetricsCleanupLog],
// [GenericMetricsCleanupLog], [CASRecoveryKindLog]) return the same
// pooled root type and help readers grep by handler family without duplicating implementations.
func SLog(logger logging.Logger) *SchedulerLogRoot {
	return logging.Fluent(logger)
}

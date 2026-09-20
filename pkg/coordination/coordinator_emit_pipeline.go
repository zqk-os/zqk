package coordination

import (
	"context"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// Pipeline kind and stage names follow docs/architecture/data-pipeline-lifecycle.md (TRIGGER = fan-out / side-effects).
// pipeline_kind = coordination.emit (general); stage = specific step for observability (logs, metrics, tracing filters).
const (
	coordinatorEmitPipelineKind = "coordination.emit"
	emptyValue                  = ""

	stageCoordinatorEmitTriggerLoggingTerminal  = "TRIGGER_logging_complete"
	stageCoordinatorEmitTriggerLoggingAsync     = "TRIGGER_logging_async"
	stageCoordinatorEmitTriggerAuditSync        = "TRIGGER_audit_callback_sync"
	stageCoordinatorEmitTriggerAuditAsync       = "TRIGGER_audit_async"
	stageCoordinatorEmitTriggerMetrics          = "TRIGGER_metrics"
	stageCoordinatorEmitTriggerOperationalSync  = "TRIGGER_operational_subscribers_sync"
	stageCoordinatorEmitTriggerOperationalAsync = "TRIGGER_operational_async"
)

// noopCoordinatorEmitMetrics avoids per-stage Info logs on the hot emit path; stage names and kind remain
// available if callers swap in a real MetricsSink (e.g. metrics or tracing).
type noopCoordinatorEmitMetrics struct{}

func (noopCoordinatorEmitMetrics) RecordStage(context.Context, string, string, time.Duration, error) {
}

func (noopCoordinatorEmitMetrics) RecordStageWithBuckets(context.Context, string, string, time.Duration, error, map[string]string) {
}

func coordinatorEmitPartitionKey(ev *EventContext) string {
	if ev == nil {
		return emptyValue
	}
	return ev.OperationType + ":" + ev.Status
}

func coordinatorEmitIdempotencyKey(ev *EventContext) string {
	if ev == nil {
		return emptyValue
	}
	if ev.OperationID != emptyValue {
		return ev.OperationID
	}
	return ev.CorrelationID
}

// buildEmitPipeline composes only the channels enabled on eventCtx; each stage is a named pipeline step.
func (c *Coordinator) buildEmitPipeline(eventCtx *EventContext) *pipeline.Pipeline {
	b := pipeline.NewBuilder(coordinatorEmitPipelineKind, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopCoordinatorEmitMetrics{}, Strategy: pipeline.NoopBucketing{}}).
		WithProfile("system")

	if eventCtx.EmitLogging {
		isTerminal := eventCtx.Status == objects.ObjectStatusCompleted || eventCtx.Status == objects.ObjectStatusError
		if isTerminal {
			b.AddStage(stageCoordinatorEmitTriggerLoggingTerminal, func(pctx *pipeline.Context, payload any) (any, error) {
				ev := payload.(*EventContext)
				runCoordinatorRouterSync(pctx.Ctx, func(ctx context.Context) {
					_ = c.emitLoggingRouter(ctx, ev) //nolint:errcheck // Best-effort; sync so completion/error appear in diagnostics.jsonl
				})
				return payload, nil
			})
		} else {
			b.AddStage(stageCoordinatorEmitTriggerLoggingAsync, func(pctx *pipeline.Context, payload any) (any, error) {
				ev := payload.(*EventContext)
				runCoordinatorRouterAsync(pctx.Ctx, "coordinator_logging_router", "coordination.emit/"+stageCoordinatorEmitTriggerLoggingAsync, func(ctx context.Context) {
					_ = c.emitLoggingRouter(ctx, ev) //nolint:errcheck // Async, best-effort
				})
				return payload, nil
			})
		}
	}

	if eventCtx.EmitAudit {
		if ar, ok := c.auditRouter.(*StorageAuditRouter); ok && ar.HasCallback() {
			b.AddStage(stageCoordinatorEmitTriggerAuditSync, func(pctx *pipeline.Context, payload any) (any, error) {
				ev := payload.(*EventContext)
				runCoordinatorRouterSync(pctx.Ctx, func(ctx context.Context) {
					_ = c.emitAuditRouter(ctx, ev) //nolint:errcheck // Best-effort
				})
				return payload, nil
			})
		} else {
			b.AddStage(stageCoordinatorEmitTriggerAuditAsync, func(pctx *pipeline.Context, payload any) (any, error) {
				ev := payload.(*EventContext)
				runCoordinatorRouterAsync(pctx.Ctx, "coordinator_audit_router", "coordination.emit/"+stageCoordinatorEmitTriggerAuditAsync, func(ctx context.Context) {
					_ = c.emitAuditRouter(ctx, ev) //nolint:errcheck // Async, best-effort
				})
				return payload, nil
			})
		}
	}

	if eventCtx.EmitMetrics && metricsrecording.Enabled() {
		b.AddStage(stageCoordinatorEmitTriggerMetrics, func(pctx *pipeline.Context, payload any) (any, error) {
			ev := payload.(*EventContext)
			runCoordinatorRouterAsync(pctx.Ctx, "coordinator_metrics_router", "coordination.emit/"+stageCoordinatorEmitTriggerMetrics, func(ctx context.Context) {
				_ = c.emitMetricsRouter(ctx, ev) //nolint:errcheck // Async, best-effort
			})
			return payload, nil
		})
	}

	if eventCtx.EmitOperational {
		if c.hasSubscribers() {
			b.AddStage(stageCoordinatorEmitTriggerOperationalSync, func(pctx *pipeline.Context, payload any) (any, error) {
				ev := payload.(*EventContext)
				c.emitOperationalEventSync(pctx.Ctx, ev)
				return payload, nil
			})
		} else {
			b.AddStage(stageCoordinatorEmitTriggerOperationalAsync, func(pctx *pipeline.Context, payload any) (any, error) {
				ev := payload.(*EventContext)
				runCoordinatorRouterAsync(pctx.Ctx, "coordinator_operational_router", "coordination.emit/"+stageCoordinatorEmitTriggerOperationalAsync, func(ctx context.Context) {
					c.emitOperationalEvent(ctx, ev)
				})
				return payload, nil
			})
		}
	}

	return b.Build()
}

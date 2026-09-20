package system

import (
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindAggregateAudit = "system_aggregate_audit"

// Outcome keys for system_aggregate_audit pipeline observability (wire shape unchanged).
const (
	aggAuditOutcomeKeyIngestInitialized  = "ingest_initialized"
	aggAuditOutcomeKeyEventsProcessed    = "events_processed"
	aggAuditOutcomeKeyMetricsCreated     = "metrics_created"
	aggAuditOutcomeKeyNormalizeCompleted = "normalize_completed"
	aggAuditOutcomeKeyCleanupMode        = "cleanup_mode"
	aggAuditOutcomeKeyDecideCompleted    = "decide_completed"
	aggAuditOutcomeKeyCleanupCompleted   = "cleanup_completed"
	aggAuditOutcomeKeyFinalizeDone       = "finalize_done"
)

type aggregateAuditPipelinePayload struct {
	aggCtx   *AggregateAuditContext
	result   *storage.AuditAggregationResult
	decision aggregateAuditCleanupDecision
}

// aggregateAuditPayloadFrom decodes the aggregate-audit pipeline payload. When requireAggCtx is true,
// aggCtx must be non-nil (NORMALIZE, COMMIT); when false, only the pointer type assertion is required (DECIDE).
func aggregateAuditPayloadFrom(payload any, requireAggCtx bool) (*aggregateAuditPipelinePayload, bool) {
	in, ok := nildecode.DecodeNonNilPayload[*aggregateAuditPipelinePayload](payload)
	if !ok {
		return nil, false
	}
	if requireAggCtx && in.aggCtx == nil {
		return nil, false
	}
	return in, true
}

// RunAggregateAuditViaPipeline wraps `aggregate-audit` in the canonical pipeline lifecycle.
func RunAggregateAuditViaPipeline(cmd *cobra.Command, _ []string) error {
	if cmd == nil {
		return errfmt.Errorf("aggregate-audit: cmd required")
	}

	stopCPUProfile, err := setupCPUProfilingForAggregate(cmd)
	if err != nil {
		return err
	}
	defer stopCPUProfile()

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	pl := pipeline.NewBuilder(pipelineKindAggregateAudit, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, _ any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			aggCtx, err := initializeAggregateAuditContext(cmd)
			if err != nil {
				return nil, err
			}
			payload := &aggregateAuditPipelinePayload{aggCtx: aggCtx}
			pctx.Outcome[objects.FieldKeyWindowStart] = aggCtx.WindowStart.Format(time.RFC3339)
			pctx.Outcome[objects.FieldKeyWindowEnd] = aggCtx.WindowEnd.Format(time.RFC3339)
			pctx.Outcome[aggAuditOutcomeKeyIngestInitialized] = true
			return payload, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := aggregateAuditPayloadFrom(payload, true)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *aggregateAuditPipelinePayload, got %T", payload)
			}
			result, err := executeAggregateAuditCore(in.aggCtx)
			if err != nil {
				return nil, err
			}
			in.result = result
			if err := outputAggregationResults(cmd, result); err != nil {
				return nil, err
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[aggAuditOutcomeKeyEventsProcessed] = result.EventCount
			pctx.Outcome[aggAuditOutcomeKeyMetricsCreated] = result.MetricsCreated
			pctx.Outcome[aggAuditOutcomeKeyNormalizeCompleted] = true
			return in, nil
		}).
		AddStage("DECIDE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := aggregateAuditPayloadFrom(payload, false)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *aggregateAuditPipelinePayload, got %T", payload)
			}
			decision, err := parseAggregateAuditCleanupDecisionFromFlags(cmd)
			if err != nil {
				return nil, err
			}
			in.decision = decision
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[aggAuditOutcomeKeyCleanupMode] = string(decision.Mode)
			pctx.Outcome[aggAuditOutcomeKeyDecideCompleted] = true
			return in, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := aggregateAuditPayloadFrom(payload, true)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *aggregateAuditPipelinePayload, got %T", payload)
			}
			if err := executeAggregateAuditCleanupCore(in.aggCtx, in.result, in.decision); err != nil {
				return nil, err
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[aggAuditOutcomeKeyCleanupCompleted] = true
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[aggAuditOutcomeKeyFinalizeDone] = true
			return payload, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: baseCtx, Outcome: make(map[string]any)}
	_, err = pl.Run(pctx, struct{}{})
	return err
}

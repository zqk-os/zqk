package scheduler

import (
	"context"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindAuditAgg = "audit_event_aggregation"

type auditAggPayload struct {
	session *auditAggregationSession
	result  *storagepkg.AuditAggregationResult
}

// RunAuditAggregationViaPipeline runs audit event aggregation through the standardized pipeline.
// The complexity previously housed in executeAuditAggregationCore has been flattened
// into observable pipeline stages here.
func RunAuditAggregationViaPipeline(
	ctx context.Context,
	h *AuditAggregationHandler,
	job *ScheduledJob,
) error {
	if h == nil || job == nil {
		return errfmt.Errorf("audit aggregation: handler and job required")
	}
	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindAuditAgg, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST_AND_CONFIG", stageAuditAggIngest).
		AddStage("PRE_CHECKS", stageAuditAggPreChecks).
		AddStage("SETUP_PHASES", stageAuditAggSetupPhases).
		AddStage("RETENTION_FIRST_PASS", stageAuditAggRetentionFirst).
		AddStage("CATCH_UP", stageAuditAggCatchUp).
		AddStage("AGGRESSIVE_CLEANUP", stageAuditAggAggressiveCleanup).
		AddStage("PROACTIVE_CLEANUP", stageAuditAggProactiveCleanup).
		AddStage("AGGREGATION", stageAuditAggAggregation).
		AddStage("POST_CLEANUP", stageAuditAggPostCleanup).
		AddStage("RETENTION_SECOND_PASS", stageAuditAggRetentionSecond).
		AddStage("FINALIZE", stageAuditAggFinalize).
		Build()

	ctx = storagepkg.WithCLIOperation(ctx) // Ensure CLI operation context for storage
	session := &auditAggregationSession{
		handler: h,
		ctx:     ctx,
		job:     job,
	}

	initial := &auditAggPayload{session: session}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}

	_, err := pl.Run(pctx, initial)
	session.cleanup() // ensures defers are handled cleanly outside the pipeline loops

	return err
}

func stageAuditAggIngest(pctx *pipeline.Context, p any) (any, error) {
	payload, ok := nildecode.DecodeNonNilPayload[*auditAggPayload](p)
	if !ok {
		return nil, errfmt.Errorf("INGEST expected *auditAggPayload")
	}

	if err := payload.session.loadConfiguration(); err != nil {
		return nil, err
	}

	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = payload.session.job.ID
	}
	return payload, nil
}

func stageAuditAggPreChecks(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	healthCheckErr, err := payload.session.runPreChecks()
	if err != nil {
		return nil, err
	}
	_ = healthCheckErr // healthCheckErr is advisory
	return payload, nil
}

func stageAuditAggSetupPhases(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	sess := payload.session
	h := sess.handler

	AuditAggregationLog(h.logger).Info(LogEventAuditAggregationStartedRun).
		JobID(sess.job.ID).
		Log()

	AuditAggregationLog(h.logger).Info(LogEventAuditAggregationAggregatingWindow).
		String("start", sess.windowStart.Format(time.RFC3339)).
		String("end", sess.windowEnd.Format(time.RFC3339)).
		String("window", sess.windowDuration.String()).
		Log()

	sess.setupPhaseContexts()
	sess.determineEffectiveRetention()
	return payload, nil
}

func stageAuditAggRetentionFirst(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	payload.session.runRetentionFirstPass()
	return payload, nil
}

func stageAuditAggCatchUp(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	payload.session.runCatchUp()
	return payload, nil
}

func stageAuditAggAggressiveCleanup(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	payload.session.runAggressiveCleanup()
	return payload, nil
}

func stageAuditAggProactiveCleanup(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	payload.session.runProactiveCleanup()
	return payload, nil
}

func stageAuditAggAggregation(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	result, err := payload.session.runAggregation()
	if err != nil {
		return nil, err
	}
	payload.result = result
	return payload, nil
}

func stageAuditAggPostCleanup(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	if payload.result != nil {
		payload.session.runPostAggregationCleanup(payload.result)
	}
	return payload, nil
}

func stageAuditAggRetentionSecond(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	payload.session.runRetentionSecondPass()
	return payload, nil
}

func stageAuditAggFinalize(pctx *pipeline.Context, p any) (any, error) {
	payload := p.(*auditAggPayload)
	if payload.result != nil {
		payload.session.finalize(payload.result)
	}

	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyNormalizeDone] = true
		pctx.Outcome[pipeline.OutcomeKeyFinalizeJobID] = payload.session.job.ID
	}
	return payload, nil
}

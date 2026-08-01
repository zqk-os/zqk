package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
)

const pipelineKindWatchdogEvaluation = "watchdog_evaluation"

type watchdogEvaluationPayload struct {
	Job *ScheduledJob
}

// RunWatchdogEvaluationViaPipeline runs watchdog evaluation through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunWatchdogEvaluationViaPipeline(ctx context.Context, h *WatchdogEvaluationHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("watchdog_evaluation: handler and job required")
	}

	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindWatchdogEvaluation, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *watchdogEvaluationPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *watchdogEvaluationPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *watchdogEvaluationPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *watchdogEvaluationPayload with Job, got %T", payload)
			}
			err := h.executeWatchdogEvaluationCore(pctx.Ctx, in.Job)
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyNormalizeDone] = true
				if err != nil {
					pctx.Outcome[pipeline.OutcomeKeyNormalizeError] = err.Error()
				}
			}
			return in, err
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyFinalizeDone] = true
			}
			return payload, nil
		}).
		Build()

	initial := &watchdogEvaluationPayload{Job: job}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}

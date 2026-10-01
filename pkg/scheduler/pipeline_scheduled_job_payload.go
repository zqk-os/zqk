package scheduler

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// decodeScheduledJobPayload decodes a pipeline stage payload that is a pointer-to-struct with a
// *ScheduledJob Job field. P must be the pointer type (e.g. *runWrapperPayload). Wrong type, nil
// payload, or nil Job yields false.
func decodeScheduledJobPayload[P any](payload any, getJob func(P) *ScheduledJob) (P, bool) {
	p, ok := payload.(P)
	if !ok {
		var z P
		return z, false
	}
	if getJob(p) == nil {
		var z P
		return z, false
	}
	return p, true
}

type defaultScheduledJobPayload struct {
	Job *ScheduledJob
}

// RunScheduledJobPipeline executes a standard 3-stage pipeline (INGEST -> NORMALIZE -> FINALIZE)
// for any scheduled job handler.
func RunScheduledJobPipeline(
	ctx context.Context,
	logger logging.Logger,
	pipelineKind string,
	job *ScheduledJob,
	coreFn func(ctx context.Context, job *ScheduledJob) error,
) error {
	if job == nil {
		return errfmt.Errorf("%s: job required", pipelineKind)
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKind, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := payload.(*defaultScheduledJobPayload)
			if !ok || in.Job == nil {
				return nil, errfmt.Errorf("INGEST expected *defaultScheduledJobPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := payload.(*defaultScheduledJobPayload)
			if !ok || in.Job == nil {
				return nil, errfmt.Errorf("NORMALIZE expected *defaultScheduledJobPayload with Job, got %T", payload)
			}
			err := coreFn(pctx.Ctx, in.Job)
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

	initial := &defaultScheduledJobPayload{Job: job}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}


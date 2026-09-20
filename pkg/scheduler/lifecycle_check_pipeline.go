// Package scheduler: pipeline-based lifecycle check.
// This wraps LifecycleCheckHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

const pipelineKindLifecycleCheck = "lifecycle_check"

type lifecycleCheckPayload struct {
	Job *ScheduledJob
}

// RunLifecycleCheckViaPipeline runs lifecycle check through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunLifecycleCheckViaPipeline(ctx context.Context, h *LifecycleCheckHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("lifecycle_check: handler and job required")
	}

	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindLifecycleCheck, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *lifecycleCheckPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *lifecycleCheckPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *lifecycleCheckPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *lifecycleCheckPayload with Job, got %T", payload)
			}
			err := h.executeLifecycleCheckCore(pctx.Ctx, in.Job)
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

	initial := &lifecycleCheckPayload{Job: job}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}

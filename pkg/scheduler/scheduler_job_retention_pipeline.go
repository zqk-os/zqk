// Package scheduler: pipeline-based scheduler job retention.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindSchedulerJobRetention = "scheduler_job_retention"

type schedulerJobRetentionPayload struct {
	Job *ScheduledJob
}

// RunSchedulerJobRetentionViaPipeline runs scheduler job retention through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunSchedulerJobRetentionViaPipeline(ctx context.Context, h *SchedulerJobRetentionHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("scheduler job retention: handler and job required")
	}
	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	pl := pipeline.NewBuilder(pipelineKindSchedulerJobRetention, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *schedulerJobRetentionPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *schedulerJobRetentionPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*schedulerJobRetentionPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *schedulerJobRetentionPayload, got %T", payload)
			}
			err := h.executeSchedulerJobRetentionCore(pctx.Ctx, in.Job)
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
	initial := &schedulerJobRetentionPayload{Job: job}
	ctx = storagepkg.WithCLIOperation(ctx)
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}

// Package scheduler: pipeline-based retention tolerance.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/pipeline"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

const pipelineKindRetentionTolerance = "retention_tolerance"

type retentionTolerancePayload struct {
	Job *ScheduledJob
}

// RunRetentionToleranceViaPipeline runs retention tolerance through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunRetentionToleranceViaPipeline(ctx context.Context, h *RetentionToleranceHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("retention tolerance: handler and job required")
	}
	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	pl := pipeline.NewBuilder(pipelineKindRetentionTolerance, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *retentionTolerancePayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *retentionTolerancePayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return payload, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*retentionTolerancePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *retentionTolerancePayload, got %T", payload)
			}
			err := h.executeRetentionToleranceCore(pctx.Ctx, in.Job)
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
	initial := &retentionTolerancePayload{Job: job}
	ctx = storagepkg.WithCLIOperation(ctx)
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}

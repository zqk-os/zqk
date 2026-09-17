// Package scheduler: pipeline-based cache invalidation.
// This wraps CacheInvalidationHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
)

const pipelineKindCacheInvalidation = "cache_invalidation"

type cacheInvalidationPayload struct {
	Job *ScheduledJob
}

// RunCacheInvalidationViaPipeline runs cache invalidation through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunCacheInvalidationViaPipeline(ctx context.Context, h *CacheInvalidationHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("cache invalidation: handler and job required")
	}

	logger := h.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindCacheInvalidation, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *cacheInvalidationPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("INGEST expected *cacheInvalidationPayload with Job, got %T", payload)
			}
			if pctx.Outcome != nil {
				pctx.Outcome[pipeline.OutcomeKeyIngestJobID] = in.Job.ID
			}
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := decodeScheduledJobPayload(payload, func(p *cacheInvalidationPayload) *ScheduledJob { return p.Job })
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *cacheInvalidationPayload with Job, got %T", payload)
			}
			err := h.executeCacheInvalidationCore(pctx.Ctx, in.Job)
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

	initial := &cacheInvalidationPayload{Job: job}
	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, initial)
	return err
}

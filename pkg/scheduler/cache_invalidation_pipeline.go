// Package scheduler: pipeline-based cache invalidation.
// This wraps CacheInvalidationHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindCacheInvalidation = "cache_invalidation"

// RunCacheInvalidationViaPipeline runs cache invalidation through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunCacheInvalidationViaPipeline(ctx context.Context, h *CacheInvalidationHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("cache invalidation: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindCacheInvalidation, job, h.executeCacheInvalidationCore)
}

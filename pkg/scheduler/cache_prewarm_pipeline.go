// Package scheduler: pipeline-based cache pre-warming.
// This wraps CachePrewarmHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindCachePrewarm = "cache_prewarm"

// RunCachePrewarmViaPipeline runs cache pre-warming through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunCachePrewarmViaPipeline(ctx context.Context, h *CachePrewarmHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("cache_prewarm: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindCachePrewarm, job, h.executeCachePrewarmCore)
}

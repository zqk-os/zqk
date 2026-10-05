// Package scheduler: pipeline-based context refresh.
// This wraps ContextRefreshHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindContextRefresh = "context_refresh"

// RunContextRefreshViaPipeline runs context refresh through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunContextRefreshViaPipeline(ctx context.Context, h *ContextRefreshHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("context_refresh: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindContextRefresh, job, h.executeContextRefreshCore)
}

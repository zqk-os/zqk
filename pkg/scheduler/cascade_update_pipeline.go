// Package scheduler: pipeline-based cascade update.
// This wraps CascadeUpdateHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindCascadeUpdate = "cascade_update"

// RunCascadeUpdateViaPipeline runs cascade update through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunCascadeUpdateViaPipeline(ctx context.Context, h *CascadeUpdateHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("cascade_update: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindCascadeUpdate, job, h.executeCascadeUpdateCore)
}

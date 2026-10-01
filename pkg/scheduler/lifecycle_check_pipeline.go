// Package scheduler: pipeline-based lifecycle check.
// This wraps LifecycleCheckHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindLifecycleCheck = "lifecycle_check"

// RunLifecycleCheckViaPipeline runs lifecycle check through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunLifecycleCheckViaPipeline(ctx context.Context, h *LifecycleCheckHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("lifecycle_check: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindLifecycleCheck, job, h.executeLifecycleCheckCore)
}


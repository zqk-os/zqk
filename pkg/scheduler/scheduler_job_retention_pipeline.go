// Package scheduler: pipeline-based scheduler job retention.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindSchedulerJobRetention = "scheduler_job_retention"

// RunSchedulerJobRetentionViaPipeline runs scheduler job retention through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunSchedulerJobRetentionViaPipeline(ctx context.Context, h *SchedulerJobRetentionHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("scheduler job retention: handler and job required")
	}
	ctx = storagepkg.WithCLIOperation(ctx)
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindSchedulerJobRetention, job, h.executeSchedulerJobRetentionCore)
}

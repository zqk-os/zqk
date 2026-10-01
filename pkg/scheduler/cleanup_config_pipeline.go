// Package scheduler: pipeline-based cleanup (config-driven).
// This wraps CleanupConfigHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindCleanupConfig = "cleanup"

// RunCleanupConfigViaPipeline runs config-driven cleanup through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunCleanupConfigViaPipeline(ctx context.Context, h *CleanupConfigHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("cleanup config: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindCleanupConfig, job, h.executeCleanupConfigCore)
}


// Package scheduler: pipeline-based retention tolerance.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindRetentionTolerance = "retention_tolerance"

// RunRetentionToleranceViaPipeline runs retention tolerance through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunRetentionToleranceViaPipeline(ctx context.Context, h *RetentionToleranceHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("retention tolerance: handler and job required")
	}
	ctx = storagepkg.WithCLIOperation(ctx)
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindRetentionTolerance, job, h.executeRetentionToleranceCore)
}


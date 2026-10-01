// Package scheduler: pipeline-based generic metrics cleanup.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindGenericMetricsCleanup = "generic_metrics_cleanup"

// RunGenericMetricsCleanupViaPipeline runs generic metrics cleanup through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunGenericMetricsCleanupViaPipeline(ctx context.Context, h *GenericMetricsCleanupHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("generic metrics cleanup: handler and job required")
	}
	ctx = storagepkg.WithCLIOperation(ctx)
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindGenericMetricsCleanup, job, h.executeGenericMetricsCleanupCore)
}


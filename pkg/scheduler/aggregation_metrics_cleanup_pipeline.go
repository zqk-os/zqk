// Package scheduler: pipeline-based aggregation metrics cleanup.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)


const pipelineKindAggregationMetricsCleanup = "aggregation_metrics_cleanup"

// RunAggregationMetricsCleanupViaPipeline runs aggregation metrics cleanup through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunAggregationMetricsCleanupViaPipeline(ctx context.Context, h *AggregationMetricsCleanupHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("aggregation metrics cleanup: handler and job required")
	}
	ctx = storagepkg.WithCLIOperation(ctx)
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindAggregationMetricsCleanup, job, h.executeAggregationMetricsCleanupCore)
}


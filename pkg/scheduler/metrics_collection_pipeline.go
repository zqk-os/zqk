// Package scheduler: pipeline-based metrics collection.
// This wraps FileLockMetricsCollectionHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindMetricsCollection = "metrics_collection"

// RunMetricsCollectionViaPipeline runs metrics collection through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunMetricsCollectionViaPipeline(ctx context.Context, h *FileLockMetricsCollectionHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("metrics_collection: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindMetricsCollection, job, h.executeMetricsCollectionCore)
}

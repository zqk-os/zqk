// Package scheduler: pipeline-based test I/O.
// This wraps TestIOHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindTestIO = "test_io"

// RunTestIOViaPipeline runs test I/O through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunTestIOViaPipeline(ctx context.Context, h *TestIOHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("test_io: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindTestIO, job, h.executeTestIOCore)
}

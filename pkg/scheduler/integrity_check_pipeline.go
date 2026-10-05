// Package scheduler: pipeline-based integrity check.
// This wraps IntegrityCheckHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindIntegrityCheck = "integrity_check"

// RunIntegrityCheckViaPipeline runs integrity checks through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunIntegrityCheckViaPipeline(ctx context.Context, h *IntegrityCheckHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("integrity_check: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindIntegrityCheck, job, h.executeIntegrityCheckCore)
}

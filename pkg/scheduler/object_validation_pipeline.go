// Package scheduler: pipeline-based object validation.
// This wraps ObjectValidationHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindObjectValidation = "object_validation"

// RunObjectValidationViaPipeline runs object validation through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunObjectValidationViaPipeline(ctx context.Context, h *ObjectValidationHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("object_validation: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindObjectValidation, job, h.executeObjectValidationCore)
}

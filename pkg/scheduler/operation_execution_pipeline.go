// Package scheduler: pipeline-based operation execution.
// This wraps OperationExecutionHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindOperationExecution = "operation_execution"

// RunOperationExecutionViaPipeline runs operation execution through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunOperationExecutionViaPipeline(ctx context.Context, h *OperationExecutionHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("operation_execution: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindOperationExecution, job, h.executeOperationExecutionCore)
}

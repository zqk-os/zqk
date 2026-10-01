// Package scheduler: pipeline-based autofix batch cleanup.
// See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.

package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindAutofixBatchCleanup = "autofix_batch_cleanup"

// RunAutofixBatchCleanupViaPipeline runs autofix batch cleanup through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunAutofixBatchCleanupViaPipeline(ctx context.Context, h *AutofixBatchCleanupHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("autofix batch cleanup: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindAutofixBatchCleanup, job, h.executeAutofixBatchCleanupCore)
}

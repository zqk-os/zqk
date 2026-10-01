// Package scheduler: pipeline-based callback listener.
// This wraps CallbackListenerHandler in a uniform pipeline lifecycle (INGEST → NORMALIZE → FINALIZE).
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindCallbackListener = "callback_listener"

// RunCallbackListenerViaPipeline runs callback listener through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunCallbackListenerViaPipeline(ctx context.Context, h *CallbackListenerHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("callback_listener: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindCallbackListener, job, h.executeCallbackListenerCore)
}

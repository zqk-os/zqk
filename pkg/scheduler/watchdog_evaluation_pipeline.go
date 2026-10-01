package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const pipelineKindWatchdogEvaluation = "watchdog_evaluation"

// RunWatchdogEvaluationViaPipeline runs watchdog evaluation through the pipeline (INGEST → NORMALIZE → FINALIZE).
func RunWatchdogEvaluationViaPipeline(ctx context.Context, h *WatchdogEvaluationHandler, job *ScheduledJob) error {
	if h == nil || job == nil {
		return errfmt.Errorf("watchdog_evaluation: handler and job required")
	}
	return RunScheduledJobPipeline(ctx, h.logger, pipelineKindWatchdogEvaluation, job, h.executeWatchdogEvaluationCore)
}


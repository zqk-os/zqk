package scheduler

import "github.com/zqk-os/zqk/pkg/objects"

const (
	schedulerKindJob              = objects.KindSchedulerJob
	schedulerKindAuditEvent       = objects.KindAuditEvent
	schedulerFileSchedulerJobYAML = "scheduler_job.yaml"

	schedulerFormatJSON        = "json"
	schedulerFormatTable       = "table"
	schedulerFormatYAML        = "yaml"
	schedulerFormatAgentPrompt = "agent-prompt"

	schedulerStateInProgress = "in_progress"
	schedulerStateStarted    = "started"
	schedulerStateDeferred   = "deferred"
	schedulerStateCompleted  = "completed"
	schedulerStateFailed     = "failed"
	schedulerStateSkipped    = "skipped"
	schedulerStateError      = "error"

	schedulerJobTypeRunWrapper = "run_wrapper"
	schedulerTriggerImmediate  = "immediate"
	schedulerTriggerTimer      = "timer"
	schedulerExecutionOneTime  = "one_time"
)

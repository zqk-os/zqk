package scheduler

// Scheduler pipeline stage labels for observability/metrics grouping.
// Execution order is determined by AddStage(...) ordering in each pipeline builder.
const (
	StageLoadAndScheduleJobs    = "LOAD_AND_SCHEDULE_JOBS"
	StageValidateDependencies   = "VALIDATE_DEPENDENCIES"
	StageInitializeStorage      = "INITIALIZE_STORAGE"
	StageConfigureScheduler     = "CONFIGURE_SCHEDULER"
	StageExecuteWithRetry       = "EXECUTE_WITH_RETRY"
	StageNormalizeTimeoutCancel = "NORMALIZE_TIMEOUT_CANCEL"
	StageReadQueue              = "READ_QUEUE"
)

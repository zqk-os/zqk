package scheduler

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// JobLoader loads and hydrates ScheduledJob objects from storage
type JobLoader struct {
	storage storagepkg.ObjectStorageProvider
	secCtx  *pkgctx.SecurityContext
	logger  logging.Logger
	metrics SchedulerMetricsCollector
}

// NewJobLoader creates a new job loader
// NewJobLoader creates a new job loader
func NewJobLoader(
	storage storagepkg.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	logger logging.Logger,
	metrics SchedulerMetricsCollector,
) JobLoaderInterface {
	return &JobLoader{
		storage: storage,
		secCtx:  secCtx,
		logger:  logger,
		metrics: metrics,
	}
}

// rawJobPriority returns the priority field from a raw job map for sorting (default: normal).
func rawJobPriority(raw map[string]any) string {
	if p, ok := raw[objects.FieldKeyPriority].(string); ok && p != emptyValue {
		if p == JobPriorityCritical || p == JobPriorityHigh || p == JobPriorityNormal {
			return p
		}
	}
	return JobPriorityNormal
}

// LoadJobs loads scheduler_job objects from storage, excluding jobs marked for deletion
// (one_time + enabled=false or status=disabled). Excluded jobs are cleaned up by the
// retention handler in the background; see TRAIT_HARNESS.md Option A and job_predicates.go.
func (jl *JobLoader) LoadJobs(ctx context.Context) ([]map[string]any, error) {
	storageCtx := &pkgctx.StorageContext{}
	filter := storagepkg.ListFilter{
		Kind: objects.KindSchedulerJob,
	}

	results, err := jl.storage.List(ctx, jl.secCtx, storageCtx, filter)
	if err != nil {
		if jl.metrics != nil {
			jl.metrics.RecordJobLoadError(err)
		}
		return nil, errfmt.Newf("failed to list scheduler jobs").Wrap(err)
	}

	// Exclude marked-for-deletion jobs so init does not load or schedule them (single pass, no extra I/O).
	out := make([]map[string]any, 0, len(results.Objects))
	for _, obj := range results.Objects {
		if !IsSchedulerJobMarkedForDeletion(obj) {
			out = append(out, obj)
		}
	}
	return out, nil
}

// RefreshEnvironmentVariablesFromRaw replaces job.EnvironmentVariables from a storage Read snapshot.
// executeJob uses this after re-reading the job so CLI updates to environment_variables take effect
// without restarting the daemon or reloading the full job cache (same gap as enabled status).
func (jl *JobLoader) RefreshEnvironmentVariablesFromRaw(job *ScheduledJob, rawJob map[string]any) {
	if jl == nil || job == nil || rawJob == nil {
		return
	}
	job.EnvironmentVariables = nil
	jl.parseEnvironmentVariables(job, rawJob)
}

// HydrateJob converts a raw job object from storage into a ScheduledJob
func (jl *JobLoader) HydrateJob(result map[string]any) (*ScheduledJob, error) {
	jobID, ok := result[objects.FieldKeyID].(string)
	if !ok {
		return nil, errfmt.Errorf("job missing id field")
	}

	// Parse basic fields (map keys: objects.FieldKey* — spec-union field_keys.go)
	jobType, _ := result[objects.FieldKeyJobType].(string)
	title, _ := result[objects.FieldKeyTitle].(string)
	description, _ := result[objects.FieldKeyDescription].(string)
	triggerType, _ := result[objects.FieldKeyTriggerType].(string)
	triggerType = ParseTriggerType(triggerType, jl.logger, jobID)

	scheduleExpr, _ := result[objects.FieldKeyScheduleExpression].(string)
	workflowRef, _ := result[objects.FieldKeyWorkflowRef].(string)
	eventFilter, _ := result[objects.FieldKeyEventFilter].(string)
	lifecycleFilter, _ := result[objects.FieldKeyLifecycleFilter].(string)

	enabled, _ := result[objects.FieldKeyEnabled].(bool)
	category, _ := result[objects.FieldKeyCategory].(string)
	if category == emptyValue {
		category = "maintenance" // Default category
	}

	executionMode, _ := result[objects.FieldKeyExecutionMode].(string)
	if executionMode == emptyValue {
		executionMode = "reusable" // Default execution mode
	}

	transactional, _ := result[objects.FieldKeyTransactional].(bool)
	// Default to false if not specified (backward compatible)

	maxRuntimeSeconds := 3600 // Default: 1 hour
	if maxRuntime, ok := result[objects.FieldKeyMaxRuntimeSeconds].(int); ok && maxRuntime > 0 {
		maxRuntimeSeconds = maxRuntime
	} else if maxRuntimeFloat, ok := result[objects.FieldKeyMaxRuntimeSeconds].(float64); ok && maxRuntimeFloat > 0 {
		maxRuntimeSeconds = int(maxRuntimeFloat)
	}

	// Validate trigger requirements
	validation := ValidateTriggerRequirements(
		triggerType, scheduleExpr, workflowRef, eventFilter, lifecycleFilter,
		jl.logger, jobID, jl.metrics,
	)
	if !validation.Valid {
		return nil, errfmt.Errorf("trigger validation failed: %s", validation.Reason)
	}

	// Skip one-time jobs that have already run
	if executionMode == "one_time" {
		if lastRunStr, ok := result[objects.FieldKeyLastRunAt].(string); ok && lastRunStr != emptyValue {
			SchedulerJobLoaderLog(jl.logger).Debug(LogEventSchedulerJobLoaderSkippingOneTimeAlreadyRun).
				JobID(jobID).
				Log()
			return nil, nil // Signal to skip
		}
	}

	// Parse timestamps
	var lastRunAt, nextRunAt *time.Time
	if lastRunStr, ok := result[objects.FieldKeyLastRunAt].(string); ok && lastRunStr != emptyValue {
		if t, err := time.Parse(time.RFC3339, lastRunStr); err == nil {
			lastRunAt = &t
		}
	}
	if nextRunStr, ok := result[objects.FieldKeyNextRunAt].(string); ok && nextRunStr != emptyValue {
		if t, err := time.Parse(time.RFC3339, nextRunStr); err == nil {
			nextRunAt = &t
		}
	}

	// Parse log_level
	logLevel, _ := result[objects.FieldKeyLogLevel].(string)
	if logLevel == emptyValue {
		logLevel = "default" // Default log level
	}
	// Validate log_level
	if logLevel != "default" && logLevel != "verbose" && logLevel != "debug" {
		SchedulerJobLoaderLog(jl.logger).Warn(LogEventSchedulerJobLoaderInvalidLogLevel).
			JobID(jobID).
			String("log_level", logLevel).
			Log()
		logLevel = "default"
	}

	priority, _ := result[objects.FieldKeyPriority].(string)
	if priority == emptyValue {
		priority = JobPriorityNormal
	}
	if priority != JobPriorityNormal && priority != JobPriorityHigh && priority != JobPriorityCritical {
		SchedulerJobLoaderLog(jl.logger).Warn(LogEventSchedulerJobLoaderInvalidPriority).
			JobID(jobID).
			String("priority", priority).
			Log()
		priority = JobPriorityNormal
	}

	allowParallel, _ := result[objects.FieldKeyAllowParallelExecution].(bool)

	// Parse job-type-specific fields
	job := &ScheduledJob{
		ID:                     jobID,
		JobType:                jobType,
		Category:               category,
		Title:                  title,
		Description:            description,
		TriggerType:            triggerType,
		ScheduleExpr:           scheduleExpr,
		WorkflowRef:            workflowRef,
		EventFilter:            eventFilter,
		LifecycleFilter:        lifecycleFilter,
		Enabled:                enabled,
		ExecutionMode:          executionMode,
		Transactional:          transactional,
		MaxRuntimeSeconds:      maxRuntimeSeconds,
		LastRunAt:              lastRunAt,
		NextRunAt:              nextRunAt,
		LogLevel:               logLevel,
		Priority:               priority,
		AllowParallelExecution: allowParallel,
	}

	// Parse job-type-specific fields
	jl.parseJobTypeFields(job, result)

	// Parse environment_variables for every job type (single code path; no special-casing).
	// All handlers that use job.EnvironmentVariables (run_wrapper, audit_event_aggregation,
	// change_journal_aggregation, retention_tolerance, test_io, file_lock_metrics, etc.)
	// receive the same treatment regardless of job_type.
	jl.parseEnvironmentVariables(job, result)

	// Record metrics
	if jl.metrics != nil {
		jl.metrics.RecordJobLoaded(jobID, jobType, triggerType)
	}

	return job, nil
}

// parseJobTypeFields parses fields specific to different job types
func (jl *JobLoader) parseJobTypeFields(job *ScheduledJob, result map[string]any) {
	switch job.JobType {
	case "run_wrapper":
		jl.parseRunWrapperFields(job, result)
	case "callback_listener":
		jl.parseCallbackListenerFields(job, result)
	case "cleanup":
		// Single track: config path is default .zqk/cleanup/config.yaml; no job fields to parse yet.
	case JobTypeConvergenceSessionTick:
		// Config: environment_variables (CONVERGENCE_SESSION_ID, HEALTH_LIMIT, optional routing overrides).
	}
}

// parseEnvironmentVariables populates job.EnvironmentVariables from result["environment_variables"].
// Called once per job in HydrateJob for all job types (run_wrapper, audit_event_aggregation,
// retention_tolerance, test_io, metrics_collection, etc.). Handlers read job.EnvironmentVariables
// as needed; no job type is special-cased here. Handles map[string]any (YAML) and map[string]string (tests).
func (jl *JobLoader) parseEnvironmentVariables(job *ScheduledJob, result map[string]any) {
	switch v := result[objects.FieldKeyEnvironmentVariables].(type) {
	case map[string]any:
		if job.EnvironmentVariables == nil {
			job.EnvironmentVariables = make(map[string]string)
		}
		for key, value := range v {
			var valueStr string
			switch v := value.(type) {
			case string:
				valueStr = v
			case int:
				valueStr = strconv.Itoa(v)
			case int64:
				valueStr = strconv.FormatInt(v, 10)
			case float64:
				valueStr = strconv.FormatInt(int64(v), 10)
			case bool:
				valueStr = strconv.FormatBool(v)
			default:
				if v != nil {
					valueStr = fmt.Sprintf("%v", v)
				}
			}
			if valueStr != emptyValue {
				job.EnvironmentVariables[key] = valueStr
			}
		}
		return
	}
}

// parseRunWrapperFields parses fields for run_wrapper job type
func (jl *JobLoader) parseRunWrapperFields(job *ScheduledJob, result map[string]any) {
	job.Command, _ = result[objects.FieldKeyCommand].(string)

	if argsInterface, ok := result[objects.FieldKeyCommandArgs].([]any); ok {
		job.CommandArgs = make([]string, 0, len(argsInterface))
		for _, arg := range argsInterface {
			if argStr, ok := arg.(string); ok {
				job.CommandArgs = append(job.CommandArgs, argStr)
			}
		}
	}

	job.WorkingDirectory, _ = result[objects.FieldKeyWorkingDirectory].(string)

	// environment_variables are parsed for all job types in parseEnvironmentVariables (HydrateJob)

	switch v := result[objects.FieldKeyRetryCount].(type) {
	case int:
		job.RetryCount = v
	case float64:
		job.RetryCount = int(v)
	}

	switch v := result[objects.FieldKeyRetryDelaySeconds].(type) {
	case int:
		job.RetryDelaySeconds = v
	case float64:
		job.RetryDelaySeconds = int(v)
	default:
		job.RetryDelaySeconds = 5 // Default
	}

	// Parse callback fields
	job.CallbackOnCompletion, _ = result[objects.FieldKeyCallbackOnCompletion].(string)
	job.CallbackOnError, _ = result[objects.FieldKeyCallbackOnError].(string)
	job.CallbackOnStatus, _ = result[objects.FieldKeyCallbackOnStatus].(string)
	job.CallbackType, _ = result[objects.FieldKeyCallbackType].(string)

	if meta, ok := result[objects.FieldKeyMetadata].(map[string]any); ok && len(meta) > 0 {
		job.Metadata = make(map[string]any, len(meta))
		maps.Copy(job.Metadata, meta)
	}

	// Auto-detect callback type if not specified
	if job.CallbackType == emptyValue {
		if job.CallbackOnCompletion != emptyValue || job.CallbackOnError != emptyValue || job.CallbackOnStatus != emptyValue {
			// Check if any callback looks like a URL
			testCallback := job.CallbackOnCompletion
			if testCallback == emptyValue {
				testCallback = job.CallbackOnError
			}
			if testCallback == emptyValue {
				testCallback = job.CallbackOnStatus
			}
			when.When(func() bool {
				return len(testCallback) >= 7 && (strings.HasPrefix(testCallback, "http://") || strings.HasPrefix(testCallback, "https://"))
			}).Then(func() { job.CallbackType = "webhook" }).OrElse(func() { job.CallbackType = "command" }).Run()
		}
	}
}

// parseCallbackListenerFields parses fields for callback_listener job type
func (jl *JobLoader) parseCallbackListenerFields(job *ScheduledJob, result map[string]any) {
	port, portOk := result[objects.FieldKeyListenerPort].(int)
	portFloat, portFloatOk := result[objects.FieldKeyListenerPort].(float64)
	when.When(func() bool { return portOk && port > 0 }).Then(func() {
		job.ListenerPort = port
	}).OrElseWhen(func() bool { return portFloatOk && portFloat > 0 }).Then(func() {
		job.ListenerPort = int(portFloat)
	}).OrElse(func() {
		job.ListenerPort = 8080 // Default
	}).Run()

	job.ListenerPath, _ = result[objects.FieldKeyListenerPath].(string)
	if job.ListenerPath == emptyValue {
		job.ListenerPath = "/callbacks" // Default
	}

	switch v := result[objects.FieldKeyIdleShutdownSeconds].(type) {
	case int:
		job.IdleShutdownSeconds = v
	case float64:
		job.IdleShutdownSeconds = int(v)
	default:
		job.IdleShutdownSeconds = 300 // Default: 5 minutes
	}

	if routes, ok := result[objects.FieldKeyRouteHandlers].(map[string]any); ok {
		job.RouteHandlers = make(map[string]string)
		for route, handler := range routes {
			if handlerStr, ok := handler.(string); ok {
				job.RouteHandlers[route] = handlerStr
			}
		}
	}

	job.AuthType, _ = result[objects.FieldKeyAuthType].(string)
	if authConfig, ok := result[objects.FieldKeyAuthConfig].(map[string]any); ok {
		job.AuthConfig = make(map[string]any)
		maps.Copy(job.AuthConfig, authConfig)
	}
}

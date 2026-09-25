package scheduler

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
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
	p := koi.GetString(raw, objects.FieldKeyPriority)
	if p == JobPriorityCritical || p == JobPriorityHigh || p == JobPriorityNormal {
		return p
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
	jobID := koi.ID(result)
	if jobID == "" {
		return nil, errfmt.Errorf("job missing id field")
	}

	// Parse basic fields (map keys: objects.FieldKey* — spec-union field_keys.go)
	jobType := koi.GetString(result, objects.FieldKeyJobType)
	title := koi.Title(result)
	description := koi.GetString(result, objects.FieldKeyDescription)
	triggerType := koi.GetString(result, objects.FieldKeyTriggerType)
	triggerType = ParseTriggerType(triggerType, jl.logger, jobID)

	scheduleExpr := koi.GetString(result, objects.FieldKeyScheduleExpression)
	workflowRef := koi.GetString(result, objects.FieldKeyWorkflowRef)
	eventFilter := koi.GetString(result, objects.FieldKeyEventFilter)
	lifecycleFilter := koi.GetString(result, objects.FieldKeyLifecycleFilter)

	enabled := koi.GetBoolOr(result, objects.FieldKeyEnabled, false)
	category := koi.GetStringOr(result, objects.FieldKeyCategory, "maintenance")
	executionMode := koi.GetStringOr(result, objects.FieldKeyExecutionMode, "reusable")
	transactional := koi.GetBoolOr(result, objects.FieldKeyTransactional, false)

	maxRuntimeSeconds := koi.GetIntOr(result, objects.FieldKeyMaxRuntimeSeconds, 3600)
	if maxRuntimeSeconds <= 0 {
		maxRuntimeSeconds = 3600
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
		if lastRun, ok := koi.GetTime(result, objects.FieldKeyLastRunAt); ok && !lastRun.IsZero() {
			SchedulerJobLoaderLog(jl.logger).Debug(LogEventSchedulerJobLoaderSkippingOneTimeAlreadyRun).
				JobID(jobID).
				Log()
			return nil, nil // Signal to skip
		}
	}

	// Parse timestamps
	var lastRunAt, nextRunAt *time.Time
	if t, ok := koi.GetTime(result, objects.FieldKeyLastRunAt); ok {
		lastRunAt = &t
	}
	if t, ok := koi.GetTime(result, objects.FieldKeyNextRunAt); ok {
		nextRunAt = &t
	}

	// Parse log_level
	logLevel := koi.GetStringOr(result, objects.FieldKeyLogLevel, "default")
	if logLevel != "default" && logLevel != "verbose" && logLevel != "debug" {
		SchedulerJobLoaderLog(jl.logger).Warn(LogEventSchedulerJobLoaderInvalidLogLevel).
			JobID(jobID).
			String("log_level", logLevel).
			Log()
		logLevel = "default"
	}

	priority := koi.GetStringOr(result, objects.FieldKeyPriority, JobPriorityNormal)
	if priority != JobPriorityNormal && priority != JobPriorityHigh && priority != JobPriorityCritical {
		SchedulerJobLoaderLog(jl.logger).Warn(LogEventSchedulerJobLoaderInvalidPriority).
			JobID(jobID).
			String("priority", priority).
			Log()
		priority = JobPriorityNormal
	}

	allowParallel := koi.GetBoolOr(result, objects.FieldKeyAllowParallelExecution, false)
	status := koi.Status(result)

	createdAt, _ := koi.GetTime(result, objects.FieldKeyCreatedAt)

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
		Status:                 status,
		ExecutionMode:          executionMode,
		Transactional:          transactional,
		MaxRuntimeSeconds:      maxRuntimeSeconds,
		CreatedAt:              createdAt,
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
	job.Command = koi.GetString(result, objects.FieldKeyCommand)
	job.CommandArgs = koi.GetStringSlice(result, objects.FieldKeyCommandArgs)
	job.WorkingDirectory = koi.GetString(result, objects.FieldKeyWorkingDirectory)

	// environment_variables are parsed for all job types in parseEnvironmentVariables (HydrateJob)
	job.RetryCount = koi.GetIntOr(result, objects.FieldKeyRetryCount, 0)
	job.RetryDelaySeconds = koi.GetIntOr(result, objects.FieldKeyRetryDelaySeconds, 5)

	// Parse callback fields
	job.CallbackOnCompletion = koi.GetString(result, objects.FieldKeyCallbackOnCompletion)
	job.CallbackOnError = koi.GetString(result, objects.FieldKeyCallbackOnError)
	job.CallbackOnStatus = koi.GetString(result, objects.FieldKeyCallbackOnStatus)
	job.CallbackType = koi.GetString(result, objects.FieldKeyCallbackType)

	if meta := koi.GetMap(result, objects.FieldKeyMetadata); len(meta) > 0 {
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
	port := koi.GetIntOr(result, objects.FieldKeyListenerPort, 8080)
	if port <= 0 {
		port = 8080
	}
	job.ListenerPort = port
	job.ListenerPath = koi.GetStringOr(result, objects.FieldKeyListenerPath, "/callbacks")

	job.IdleShutdownSeconds = koi.GetIntOr(result, objects.FieldKeyIdleShutdownSeconds, 300)

	if routes := koi.GetMap(result, objects.FieldKeyRouteHandlers); len(routes) > 0 {
		job.RouteHandlers = make(map[string]string)
		for route, handler := range routes {
			if handlerStr, ok := handler.(string); ok {
				job.RouteHandlers[route] = handlerStr
			}
		}
	}

	job.AuthType = koi.GetString(result, objects.FieldKeyAuthType)
	if authConfig := koi.GetMap(result, objects.FieldKeyAuthConfig); len(authConfig) > 0 {
		job.AuthConfig = make(map[string]any)
		maps.Copy(job.AuthConfig, authConfig)
	}
}

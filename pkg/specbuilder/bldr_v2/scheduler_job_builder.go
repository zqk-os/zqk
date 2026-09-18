package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// SchedulerJobBuilder builds the scheduler_job spec at version v2_0_0
// File: bldr_v2/scheduler_job_builder.go - version is encoded in package/directory name
type SchedulerJobBuilder struct {
	*builders.BaseSpecBuilder
}

// NewSchedulerJobBuilder creates a new builder for scheduler_job spec version v2_0_0
func NewSchedulerJobBuilder() *SchedulerJobBuilder {
	builder := &SchedulerJobBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("scheduler_job", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a scheduled automation task (context refresh, manifest generation, etc.) with triggers and actions.\\nStructural fields (CAS rehash): identity and execution semantics—job_type, trigger_type, command/schedule,\\nexecution_mode, callbacks, auth, listeners, routes—anything that changes validation or how the scheduler runs the job.\\nRuntime_delta fields (overlay): bounded operational state—status/title (inherited), timestamps, enabled, category,\\npriority, log_level, retries, last/next run, max_runtime tuning—high churn without rewriting the structural blob.\\nPrivilege Requirements: Read (read:scheduler_job or read:* or admin), Create/Update (write:scheduler_job or write:* or admin), Delete (delete:scheduler_job or delete:* or admin), Execute/Trigger (execute:scheduler_job or execute:* or admin), Manage Scheduler (manage:scheduler or admin).\\nLifecycle: scheduler_job_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addSchedulerJobFields()

	return builder
}

// addSchedulerJobFields adds the scheduler_job fields
func (b *SchedulerJobBuilder) addSchedulerJobFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("action_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used to invoke the action.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("action registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the handler/resolver action.").
			Security("non-sensitive").
			SystemUsage([]any{
				"execution",
			}).
			Validation("Reference format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("SCH-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("allow_parallel_execution", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used for missed-trigger catch-up and conflict manager; when true, multiple instances can run concurrently.").
			Cardinality("one").
			Criticality("association").
			Default(false).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When true, multiple instances of this job may run concurrently (e.g. catch-up triggers for missed runs). When false, scheduler uses job-type default (e.g. cache_prewarm, audit_event_aggregation allow concurrent; run_wrapper does not).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-024"))
	b.AddFieldBuilder(builders.NewFieldBuilder("auth_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("configures auth validation.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("scheduler (for callback_listener job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Authentication configuration (for callback_listener job_type). Type-specific settings (JWT secret, x509 CA cert, API key, OAuth2 client ID/secret).").
			Security("sensitive (contains secrets/credentials).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("object (auth type-specific key-value pairs).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-025"))
	b.AddFieldBuilder(builders.NewFieldBuilder("auth_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines auth mechanism.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default("none").
			Dependencies("scheduler (for callback_listener job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Authentication type for callback listener (for callback_listener job_type).").
			Security("sensitive (determines security posture).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum (none, jwt, x509, api_key, oauth2).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"none",
				"jwt",
				"x509",
				"api_key",
				"oauth2",
			}).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-024"))
	b.AddFieldBuilder(builders.NewFieldBuilder("callback_on_completion", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("invoked after successful command execution.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Callback URL or command to execute on successful job completion (for run_wrapper job_type).").
			Security("sensitive (may contain credentials in URL).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string (URL or command).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("callback_on_error", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("invoked after command failure (including retry exhaustion).").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Callback URL or command to execute on job failure (for run_wrapper job_type).").
			Security("sensitive (may contain credentials in URL).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string (URL or command).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("callback_on_status", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("invoked periodically during command execution (if supported by command).").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Callback URL or command to execute for incremental status updates (for run_wrapper job_type).").
			Security("sensitive (may contain credentials in URL).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string (URL or command).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("callback_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines how callback is invoked.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("webhook").
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of callback mechanism (webhook=HTTP POST, command=execute command, event=emit system event).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum (webhook, command, event).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"webhook",
				"command",
				"event",
			}).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used for job grouping and filtering.").
			Cardinality("one").
			Criticality("association").
			Default("maintenance").
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Job category for grouping and organization (e.g., \\\\\\\\\\\\\\\"maintenance\\\\\\\\\\\\\\\", \\\\\\\\\\\\\\\"monitoring\\\\\\\\\\\\\\\", \\\\\\\\\\\\\\\"cleanup\\\\\\\\\\\\\\\", \\\\\\\\\\\\\\\"cache\\\\\\\\\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
				"reporting",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("command", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used to execute external processes.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Command to execute (for run_wrapper job_type). Full path or command name.").
			Security("sensitive (command execution).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string (command path or name).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("command_args", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("passed to command execution.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Command arguments (for run_wrapper job_type). Array of strings.").
			Security("sensitive (command arguments may contain sensitive data).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("array of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used to enable/disable jobs.").
			Cardinality("one").
			Criticality("association").
			Default(true).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether the job is currently enabled.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("environment_variables", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("sets environment for command execution.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (inherits scheduler's environment)").
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Environment variables to set for command execution (for run_wrapper job_type). Key-value pairs.").
			Security("sensitive (may contain secrets).").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("object (key-value pairs).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("execution_mode", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines if job is disabled after execution.").
			Cardinality("one").
			Criticality("composition").
			Default("reusable").
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether the job can be executed multiple times (reusable) or only once (one_time).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum (reusable, one_time).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"reusable",
				"one_time",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("idle_shutdown_seconds", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines idle shutdown behavior (lambda-style).").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("300 (5 minutes)").
			Dependencies("scheduler (for callback_listener job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Idle timeout in seconds before listener server shuts down (for callback_listener job_type). 0 = never shutdown.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer (0-3600).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("SCH-022"))
	b.AddFieldBuilder(builders.NewFieldBuilder("job_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines handler.").
			Cardinality("one").
			Criticality("composition").
			Default("context_refresh").
			Dependencies("scheduler runtime.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Kind of job (context_refresh, manifest_snapshot, test_runner, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"context_refresh",
				"manifest_snapshot",
				"test_runner",
				"aggregation",
				"lifecycle_check",
				"audit_event_aggregation",
				"change_journal_aggregation",
				"integrity_check",
				"cache_prewarm",
				"cache_invalidation",
				"cascade_update",
				"operation_execution",
				"run_wrapper",
				"callback_listener",
				"metrics_collection",
				"retention_tolerance",
				"scheduler_job_retention",
				"autofix_batch_cleanup",
				"aggregation_metrics_cleanup",
				"generic_metrics_cleanup",
				"maintenance",
				"object_validation",
				"scheduler_events_aggregation",
				"cleanup",
				"convergence_session_tick",
				"data_cell_envelope_tick",
				"auto_transition",
				"cap_orchestrator",
				"emergency_manager",
				"idle_cleanup",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SCH-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_run_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for job health monitoring.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler.").
			Lifecycle("mutable (set by scheduler).").
			Observability("yes").
			Purpose("ISO-8601 timestamp of last execution.").
			Security("non-sensitive").
			SystemUsage([]any{
				"monitoring",
				"reporting",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z|)$`).
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SCH-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("listener_path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("sets HTTP route prefix.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("/callbacks").
			Dependencies("scheduler (for callback_listener job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Base path for callback listener endpoints (for callback_listener job_type). Default is /callbacks.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string (URL path).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-021"))
	b.AddFieldBuilder(builders.NewFieldBuilder("listener_port", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("sets HTTP server port.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(8080).
			Dependencies("scheduler (for callback_listener job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Port number for callback listener server (for callback_listener job_type).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer (1024-65535).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("SCH-020"))
	b.AddFieldBuilder(builders.NewFieldBuilder("log_level", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("controls logging verbosity for troubleshooting.").
			Cardinality("one").
			Criticality("association").
			Default("default").
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Logging verbosity level for this job:\n- default: Standard logging (Info/Warn/Error only)\n- verbose: Include command output (stdout/stderr) in logs\n- debug: Full debug logging including all execution details\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
				"troubleshooting",
			}).
			Validation("enum (default, verbose, debug).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"default",
				"verbose",
				"debug",
			}).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-026"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_runtime_seconds", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used to timeout and kill hung jobs.").
			Cardinality("one").
			Criticality("composition").
			Default(3600).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Maximum execution time in seconds before job is killed (prevents hangs).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer 1-86400 (1 second to 24 hours).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("SCH-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for bundle metadata and execution parameters.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Structured metadata dictionary for test bundles and execution parameters.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
				"test_execution",
			}).
			Validation("Object containing metadata.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:public").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-035"))
	b.AddFieldBuilder(builders.NewFieldBuilder("next_run_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for job scheduling.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler.").
			Lifecycle("mutable (recalculated by scheduler).").
			Observability("yes").
			Purpose("ISO-8601 timestamp of next scheduled execution.").
			Security("non-sensitive").
			SystemUsage([]any{
				"monitoring",
				"reporting",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SCH-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used for scheduling order (critical jobs run before normal).").
			Cardinality("one").
			Criticality("association").
			Default("normal").
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scheduling priority. Higher priority jobs are submitted before lower when the scheduler\nloads and runs immediate/triggered work (e.g. cache_prewarm, cache_invalidation, retention).\nUse critical for jobs that must run first so caches and critical work complete before others.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum (normal, high, critical).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"normal",
				"high",
				"critical",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-027"))
	b.AddFieldBuilder(builders.NewFieldBuilder("retry_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines retry behavior on failure.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(0).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Number of retry attempts on failure (for run_wrapper job_type). 0 = no retries.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer (0-10).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("SCH-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("retry_delay_seconds", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines retry delay.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(5).
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Delay in seconds between retry attempts (for run_wrapper job_type).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer (0-3600).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("SCH-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("route_handlers", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("configures route routing.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default("null (uses default routes)").
			Dependencies("scheduler (for callback_listener job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Route-to-handler mappings for callback listener (for callback_listener job_type). Key-value pairs map route to handler_type.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("object (route -> handler_type mappings).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-023"))
	b.AddFieldBuilder(builders.NewFieldBuilder("schedule_expression", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("scheduling (timer jobs only).").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("scheduler (only for timer trigger_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Cron/ISO schedule or cadence definition (required for timer trigger_type, optional otherwise).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("cron expression or ISO duration.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("transactional", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines if job operations are wrapped in a transaction (all-or-nothing).").
			Cardinality("one").
			Criticality("composition").
			Default(false).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("If true, all storage operations within the job are wrapped in a transaction. On success, all operations commit atomically. On failure, all operations rollback.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-023"))
	b.AddFieldBuilder(builders.NewFieldBuilder("trigger_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines how job is scheduled/triggered.").
			Cardinality("one").
			Criticality("composition").
			Default("timer").
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("How the job is triggered (timer=cron schedule, manual=user-triggered, immediate=run once now, workflow=workflow event, event=system event).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum (timer, manual, immediate, workflow, event, lifecycle).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"timer",
				"manual",
				"immediate",
				"workflow",
				"event",
				"lifecycle",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SCH-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("working_directory", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("sets CWD for command execution.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (uses scheduler's working directory)").
			Dependencies("scheduler (for run_wrapper job_type).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Working directory for command execution (for run_wrapper job_type).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string (directory path).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCH-012"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *SchedulerJobBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *SchedulerJobBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *SchedulerJobBuilder) GetOntology() string {
	return "scheduler_job"
}

func init() {
	builders.RegisterBuilder(NewSchedulerJobBuilder())
}

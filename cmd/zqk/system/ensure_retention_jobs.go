package system

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	schedcore "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	ensureJobsListTimeout   = 30 * time.Second
	ensureJobsCreateTimeout = 60 * time.Second
)

// EnsureRetentionJobsResult is the result of ensure-retention-jobs for --format json/yaml.
// Driven by .zqk/specs/configs/scheduler_maintenance_config.yaml (required_jobs).
// CreatedJobIDs is the dynamic list of job IDs created; legacy bools are kept for JSON compat.
type EnsureRetentionJobsResult struct {
	ObjectValidationJobCreated           bool     `json:"object_validation_job_created" yaml:"object_validation_job_created"`
	SchedulerEventsAggregationJobCreated bool     `json:"scheduler_events_aggregation_job_created" yaml:"scheduler_events_aggregation_job_created"`
	RetentionToleranceJobCreated         bool     `json:"retention_tolerance_job_created" yaml:"retention_tolerance_job_created"`
	RetentionToleranceJobUpdated         []string `json:"retention_tolerance_jobs_updated,omitempty" yaml:"retention_tolerance_jobs_updated,omitempty"`
	AuditAggregationJobCreated           bool     `json:"audit_aggregation_job_created" yaml:"audit_aggregation_job_created"`
	AuditAggregationJobsUpdated          []string `json:"audit_aggregation_jobs_updated,omitempty" yaml:"audit_aggregation_jobs_updated,omitempty"`
	MaintenanceJobCreated                bool     `json:"maintenance_job_created" yaml:"maintenance_job_created"`
	MaintenanceJobUpdated                bool     `json:"maintenance_job_updated" yaml:"maintenance_job_updated"`
	ObjectCountReportJobCreated          bool     `json:"object_count_report_job_created" yaml:"object_count_report_job_created"`
	ImprovementReportJobCreated          bool     `json:"improvement_report_job_created" yaml:"improvement_report_job_created"`
	CachePrewarmJobCreated               bool     `json:"cache_prewarm_job_created" yaml:"cache_prewarm_job_created"`
	AutofixBatchCleanupJobCreated        bool     `json:"autofix_batch_cleanup_job_created" yaml:"autofix_batch_cleanup_job_created"`
	AutofixRunJobCreated                 bool     `json:"autofix_run_job_created" yaml:"autofix_run_job_created"`
	AutofixProcessPendingJobCreated      bool     `json:"autofix_process_pending_job_created" yaml:"autofix_process_pending_job_created"`
	PreCommitLintJobCreated              bool     `json:"pre_commit_lint_job_created" yaml:"pre_commit_lint_job_created"`
	PreCommitPolicyJobCreated            bool     `json:"pre_commit_policy_job_created" yaml:"pre_commit_policy_job_created"`
	PreCommitIntegrityJobCreated         bool     `json:"pre_commit_integrity_job_created" yaml:"pre_commit_integrity_job_created"`
	CreatedJobIDs                        []string `json:"created_job_ids,omitempty" yaml:"created_job_ids,omitempty"`
	JobsSyncedFromTemplate               []string `json:"jobs_synced_from_template,omitempty" yaml:"jobs_synced_from_template,omitempty"`
	AlreadySatisfied                     bool     `json:"already_satisfied" yaml:"already_satisfied"`
	Message                              string   `json:"message" yaml:"message"`
}

// createdJobInfo drives setResultCreatedByID and buildEnsureResultMessage from one table.
// When adding a new required job, add one entry here (and a legacy bool on the result if desired).
var createdJobInfo = map[string]struct {
	Message string
	SetBool func(*EnsureRetentionJobsResult)
}{
	"SCH-val":                     {"created object_validation job (SCH-val)", func(r *EnsureRetentionJobsResult) { r.ObjectValidationJobCreated = true }},
	"SCH-evag":                    {"created scheduler_events_aggregation job (SCH-evag)", func(r *EnsureRetentionJobsResult) { r.SchedulerEventsAggregationJobCreated = true }},
	"SCH-retention-tolerance":     {"created retention_tolerance job", func(r *EnsureRetentionJobsResult) { r.RetentionToleranceJobCreated = true }},
	"SCH-audit-event-aggregation": {"created audit_event_aggregation job", func(r *EnsureRetentionJobsResult) { r.AuditAggregationJobCreated = true }},
	"SCH-maintenance-wal":         {"created maintenance WAL trigger job", func(r *EnsureRetentionJobsResult) { r.MaintenanceJobCreated = true }},
	"SCH-objcount-report":         {"created object-count-report job", func(r *EnsureRetentionJobsResult) { r.ObjectCountReportJobCreated = true }},
	"SCH-improvement-report":      {"created improvement-report hourly job", func(r *EnsureRetentionJobsResult) { r.ImprovementReportJobCreated = true }},
	"SCH-cache-prewarm":           {"created cache_prewarm job (SCH-cache-prewarm)", func(r *EnsureRetentionJobsResult) { r.CachePrewarmJobCreated = true }},
	"SCH-autofix-batch-cleanup":   {"created autofix_batch_cleanup job", func(r *EnsureRetentionJobsResult) { r.AutofixBatchCleanupJobCreated = true }},
	"SCH-autofix-run":             {"created autofix_run job", func(r *EnsureRetentionJobsResult) { r.AutofixRunJobCreated = true }},
	"SCH-autofix-process-pending": {"created autofix_process_pending job (SCH-autofix-process-pending)", func(r *EnsureRetentionJobsResult) { r.AutofixProcessPendingJobCreated = true }},
	"SCH-pre-commit-lint":         {"created pre-commit lint job (SCH-pre-commit-lint)", func(r *EnsureRetentionJobsResult) { r.PreCommitLintJobCreated = true }},
	"SCH-pre-commit-policy":       {"created pre-commit policy job (SCH-pre-commit-policy)", func(r *EnsureRetentionJobsResult) { r.PreCommitPolicyJobCreated = true }},
	"SCH-pre-commit-integrity":    {"created pre-commit integrity job (SCH-pre-commit-integrity)", func(r *EnsureRetentionJobsResult) { r.PreCommitIntegrityJobCreated = true }},
	"SCH-cleanup":                 {"created cleanup job (SCH-cleanup)", nil},
	"SCH-dce-tick":                {"created data_cell_envelope_tick job (SCH-dce-tick)", nil},
}

// NewEnsureRetentionJobsCmd creates the ensure-retention-jobs command.
// Uses spec-driven builder for help/flags; RunE is bound here. All changes go through the zqk CLI (process-data-cli-only).
func NewEnsureRetentionJobsCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemEnsureRetentionJobsCommandBuilder()
	cli.BindAsyncProgress(cmd, runEnsureRetentionJobs)
	return cmd
}

// EnsureRetentionJobsInProject ensures the maintenance bundle scheduler jobs exist: maintenance WAL trigger,
// object-count-report, retention_tolerance, and audit_event_aggregation. Creates from templates, fixes job_type,
// and syncs schedule_expression / environment_variables from templates onto existing jobs when they differ.
// preferredProvider should be the same ObjectStorageProvider the daemon uses (e.g. global cache GetOrCreate) when
// ensure runs immediately before scheduler LoadJobs — otherwise ensure opens a separate factory and stream locations /
// buffers can diverge from the cached provider instance. Pass nil for CLI callers (opens storage via NewStorageFactory).
// Returns the result and nil error, or nil result and error on list/create failure.
func EnsureRetentionJobsInProject(projectRoot string, logger logging.Logger, preferredProvider storage.ObjectStorageProvider) (*EnsureRetentionJobsResult, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root is empty")
	}
	return ensureRetentionJobsCore(projectRoot, logger, nil, preferredProvider)
}

func runEnsureRetentionJobs(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	result, err := ensureRetentionJobsCore(projectRoot, logger, cmd.Context(), nil)
	if err != nil {
		return err
	}
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		return cli.WriteOutput(cmd, []byte(result.Message+"\n"))
	}
}

// ensureRetentionJobsCore runs list/create/update from centralized config (scheduler_maintenance_config.yaml).
// Single source of truth: required_jobs; all maintenance jobs are created or corrected from templates.
func ensureRetentionJobsCore(projectRoot string, logger logging.Logger, listContext context.Context, preferredProvider storage.ObjectStorageProvider) (*EnsureRetentionJobsResult, error) {
	if listContext == nil {
		listContext = context.Background() // Background: request-or-shutdown derived
	}
	// Use absolute project root so stream registry path is unambiguous.
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	cfg, err := config.NewSchedulerMaintenanceLoader(projectRoot).Load()
	if err != nil {
		return nil, errfmt.Newf("load scheduler maintenance config").Wrap(err)
	}
	// Build path alias cache so stream-backed Create (AppendToStream) can resolve segment dir.
	storage.BuildPathAliasCacheForProject(projectRoot)
	ctx := pkgctx.NewSystemContext()
	var provider storage.ObjectStorageProvider
	if preferredProvider != nil {
		provider = preferredProvider
	} else {
		factory, errFactory := storage.NewStorageFactory(ctx, projectRoot)
		if errFactory != nil {
			return nil, errfmt.Newf("storage factory").Wrap(errFactory)
		}
		provider = factory.GetStorage()
		if provider == nil {
			return nil, errfmt.Errorf("storage provider is nil")
		}
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	listCtx, listCancel := context.WithTimeout(listContext, ensureJobsListTimeout)
	defer listCancel()
	qr, err := provider.List(listCtx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindSchedulerJob, Limit: 0})
	if err != nil {
		return nil, errfmt.Newf("list scheduler_job").Wrap(err)
	}
	jobList := qr.Objects
	if jobList == nil {
		jobList = []map[string]any{}
	}
	ids := make(map[string]bool)
	existingByID := make(map[string]map[string]any)
	var idList []string
	for _, obj := range jobList {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
			ids[id] = true
			idList = append(idList, id)
			existingByID[id] = obj
		}
	}

	result := EnsureRetentionJobsResult{}
	const maintenanceWALMinRuntime = 300

	for _, entry := range cfg.RequiredJobs {
		if entry.ID == emptyValue || entry.JobType == emptyValue || entry.TemplateFile == emptyValue {
			continue
		}
		templatePath := filepath.Join(projectRoot, entry.TemplateFile)
		if !ids[entry.ID] {
			created, updatedID := ensureJobFromTemplateInProcess(logger, provider, secCtx, projectRoot, templatePath, entry.JobType, entry.ID)
			if created {
				setResultCreatedByID(&result, entry.ID, true)
			}
			if updatedID != emptyValue {
				setResultUpdatedByID(&result, entry.ID, updatedID)
			}
			continue
		}
		// Job exists: correct job_type or SCH-maintenance-wal max_runtime if wrong.
		existing := existingByID[entry.ID]
		existingJobType, _ := existing[objects.FieldKeyJobType].(string)
		if existingJobType != entry.JobType {
			if updateJobTypeInProcess(logger, provider, secCtx, entry.ID, entry.JobType) {
				setResultUpdatedByID(&result, entry.ID, entry.ID)
			}
		}
		if entry.ID == schedcore.MaintenanceWALTriggerJobID && entry.JobType == "maintenance" {
			maxRun := 0
			switch v := existing[objects.FieldKeyMaxRuntimeSeconds].(type) {
			case int:
				maxRun = v
			case float64:
				maxRun = int(v)
			}
			if maxRun < maintenanceWALMinRuntime {
				updateCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Background: request-or-shutdown derived
				err := provider.Update(updateCtx, secCtx, entry.ID, map[string]any{
					objects.FieldKeyJobType:           "maintenance",
					objects.FieldKeyMaxRuntimeSeconds: maintenanceWALMinRuntime,
				})
				cancel()
				if err != nil {
					logging.Fluent(logger).Warn("Update SCH-maintenance-wal max_runtime_seconds failed").ObjectID(entry.ID).WithError(err).Log()
				} else {
					result.MaintenanceJobUpdated = true
				}
			}
		}
		if syncExistingJobFromTemplate(logger, provider, secCtx, entry.ID, templatePath, existing) {
			result.JobsSyncedFromTemplate = append(result.JobsSyncedFromTemplate, entry.ID)
		}
	}

	result.AlreadySatisfied = len(result.CreatedJobIDs) == 0 &&
		!result.MaintenanceJobUpdated &&
		len(result.RetentionToleranceJobUpdated) == 0 && len(result.AuditAggregationJobsUpdated) == 0 &&
		len(result.JobsSyncedFromTemplate) == 0

	if result.AlreadySatisfied {
		result.Message = fmt.Sprintf("Maintenance bundle already satisfied (scheduler_job list count: %d, IDs: %s). Required jobs from scheduler_maintenance_config.yaml present.", len(jobList), strings.Join(idList, ", "))
	} else {
		result.Message = buildEnsureResultMessage(&result)
	}
	return &result, nil
}

func setResultCreatedByID(r *EnsureRetentionJobsResult, id string, created bool) {
	if !created {
		return
	}
	r.CreatedJobIDs = append(r.CreatedJobIDs, id)
	if info, ok := createdJobInfo[id]; ok && info.SetBool != nil {
		info.SetBool(r)
	}
}

func setResultUpdatedByID(r *EnsureRetentionJobsResult, id, updatedID string) {
	switch id {
	case schedcore.RetentionToleranceCatchAllJobID:
		r.RetentionToleranceJobUpdated = append(r.RetentionToleranceJobUpdated, updatedID)
	case schedcore.DefaultAuditEventAggregationSchedulerJobID:
		r.AuditAggregationJobsUpdated = append(r.AuditAggregationJobsUpdated, updatedID)
	}
}

func buildEnsureResultMessage(r *EnsureRetentionJobsResult) string {
	var parts []string
	for _, id := range r.CreatedJobIDs {
		if info, ok := createdJobInfo[id]; ok && info.Message != emptyValue {
			parts = append(parts, info.Message)
		} else {
			parts = append(parts, fmt.Sprintf("created job (%s)", id))
		}
	}
	if r.MaintenanceJobUpdated {
		parts = append(parts, "corrected SCH-maintenance-wal (maintenance WAL trigger)")
	}
	if len(r.RetentionToleranceJobUpdated) > 0 {
		parts = append(parts, fmt.Sprintf("updated job_type for %s", strings.Join(r.RetentionToleranceJobUpdated, ", ")))
	}
	if len(r.AuditAggregationJobsUpdated) > 0 {
		parts = append(parts, fmt.Sprintf("updated audit aggregation jobs: %s", strings.Join(r.AuditAggregationJobsUpdated, ", ")))
	}
	if len(r.JobsSyncedFromTemplate) > 0 {
		parts = append(parts, fmt.Sprintf("synced schedule/env from templates: %s", strings.Join(r.JobsSyncedFromTemplate, ", ")))
	}
	if len(parts) == 0 {
		return "No jobs created or updated."
	}
	return strings.Join(parts, "; ") + ". Source: scheduler_maintenance_config.yaml."
}

// ensureJobFromTemplateInProcess reads the template YAML and creates the scheduler_job in storage (same process as list).
// If create fails with already-exists and defaultID is set, updates job_type on that ID. Returns (created, updatedID).
func ensureJobFromTemplateInProcess(logger logging.Logger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, projectRoot, templatePath, jobType, defaultID string) (created bool, updatedID string) {
	if _, statErr := fileutil.Stat(templatePath); statErr != nil {
		logging.Fluent(logger).Warn("Template not found, skipping create").Path(templatePath).WithError(statErr).Log()
		return false, ""
	}
	data, err := fileutil.ReadFile(templatePath)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to read template").Path(templatePath).WithError(err).Log()
		return false, ""
	}
	var obj map[string]any
	if err = yaml.Unmarshal(data, &obj); err != nil {
		logging.Fluent(logger).Warn("Failed to parse template YAML").Path(templatePath).WithError(err).Log()
		return false, ""
	}
	obj[objects.FieldKeyKind] = objects.KindSchedulerJob
	if obj[objects.FieldKeySourceType] == nil {
		obj[objects.FieldKeySourceType] = "internal"
	}
	ctx, cancel := context.WithTimeout(context.Background(), ensureJobsCreateTimeout) // Background: request-or-shutdown derived
	defer cancel()
	// Sync create so jobs persist to stream registry immediately (avoid write-behind; list/daemon see them).
	ctx = storage.WithSyncCreateForSchedulerJob(ctx)
	if err = provider.Create(ctx, secCtx, obj); err != nil {
		if (strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "AlreadyExists")) && defaultID != emptyValue {
			if updateJobTypeInProcess(logger, provider, secCtx, defaultID, jobType) {
				return false, defaultID
			}
		}
		logging.Fluent(logger).Warn("Create failed").TemplatePath(templatePath).WithError(err).Log()
		return false, ""
	}
	logging.Fluent(logger).Info("Created scheduler job from template").TemplatePath(templatePath).Log()
	return true, ""
}

func updateJobTypeInProcess(logger logging.Logger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id, jobType string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Background: request-or-shutdown derived
	defer cancel()
	err := provider.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyJobType: jobType})
	if err != nil {
		logging.Fluent(logger).Warn("Update job_type failed").ObjectID(id).WithError(err).Log()
		return false
	}
	logging.Fluent(logger).Info("Updated job_type").ObjectID(id).SchedulerJobType(jobType).Log()
	return true
}

// syncExistingJobFromTemplate applies schedule_expression and (when present in the template)
// environment_variables from the template file so ensure-retention-jobs updates live objects
// when templates change. Returns true if an update was applied.
func syncExistingJobFromTemplate(logger logging.Logger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id, templatePath string, existing map[string]any) bool {
	data, err := fileutil.ReadFile(templatePath)
	if err != nil {
		return false
	}
	schedule, tmplEnv, tmplHasEnv, err := parseTemplateScheduleAndEnv(data)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to parse template for sync").Path(templatePath).WithError(err).Log()
		return false
	}
	updates := map[string]any{}
	if schedule != emptyValue && objects.StringField(existing, objects.FieldKeyScheduleExpression) != schedule {
		updates[objects.FieldKeyScheduleExpression] = schedule
	}
	if tmplHasEnv {
		existingEnv := environmentVariablesMapFromObject(existing)
		if !environmentStringMapsEqual(tmplEnv, existingEnv) {
			updates[objects.FieldKeyEnvironmentVariables] = tmplEnv
		}
	}
	if len(updates) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Background: request-or-shutdown derived
	defer cancel()
	if err = provider.Update(ctx, secCtx, id, updates); err != nil {
		logging.Fluent(logger).Warn("Sync scheduler_job from template failed").ObjectID(id).Path(templatePath).WithError(err).Log()
		return false
	}
	logging.Fluent(logger).Info("Synced scheduler_job from template").ObjectID(id).Path(templatePath).Log()
	return true
}

func parseTemplateScheduleAndEnv(data []byte) (schedule string, env map[string]any, hasEnv bool, err error) {
	var obj map[string]any
	if uerr := yaml.Unmarshal(data, &obj); uerr != nil {
		return "", nil, false, uerr
	}
	if s, ok := obj[objects.FieldKeyScheduleExpression].(string); ok {
		schedule = strings.TrimSpace(s)
	}
	raw, ok := obj[objects.FieldKeyEnvironmentVariables]
	if !ok {
		return schedule, nil, false, nil
	}
	if raw == nil {
		return schedule, map[string]any{}, true, nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return schedule, v, true, nil
	case map[any]any:
		m := make(map[string]any, len(v))
		for k, val := range v {
			if ks, ok := k.(string); ok {
				m[ks] = val
			}
		}
		return schedule, m, true, nil
	default:
		return schedule, nil, false, nil
	}
}

func environmentVariablesMapFromObject(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	raw := obj[objects.FieldKeyEnvironmentVariables]
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return v
	case map[string]string:
		m := make(map[string]any, len(v))
		for k, val := range v {
			m[k] = val
		}
		return m
	case map[any]any:
		m := make(map[string]any, len(v))
		for k, val := range v {
			if ks, ok := k.(string); ok {
				m[ks] = val
			}
		}
		return m
	default:
		return nil
	}
}

func environmentStringMapsEqual(a, b map[string]any) bool {
	return maps.Equal(envToStringMap(a), envToStringMap(b))
}

func envToStringMap(m map[string]any) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		switch t := v.(type) {
		case string:
			out[k] = t
		default:
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

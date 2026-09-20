package system

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	autoFixBatchPrefix        = "AUTOFIX-"
	autoFixBatchStatusPending = "pending"
	autoFixJobPrefix          = "SCH-AUTOFIX-"
	autoFixJobTypeRunWrapper  = "run_wrapper"
	autoFixJobTriggerManual   = "manual"
	autoFixJobCategory        = "auto_fix"
	autoFixJobExecutionOnce   = "one_time"
	autoFixJobStatusActive    = "active"
	autoFixJobLogLevel        = "default"
	autoFixJobReuseMsg        = "Reusing existing one-time job for auto-fix batch"
)

// AutoFixBatch represents a batch of auto-fixable issues to be processed
type AutoFixBatch struct {
	BatchID     string `json:"batch_id"`
	ProjectRoot string `json:"project_root"`
	// Issues is the legacy flat representation: one row per (object, issue).
	// Kept for backward compatibility when reading older AUTOFIX-*.json files.
	Issues []AutoFixBatchIssue `json:"issues,omitempty"`

	// Objects is the preferred representation: one row per object with multiple issues.
	// This avoids duplicating object metadata in the batch file and allows the processor
	// to reuse parsed object state when applying multiple fixes.
	Objects   []AutoFixBatchObject `json:"objects,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	Status    string               `json:"status"` // "pending", "processing", "completed", "failed"
	Progress  AutoFixBatchProgress `json:"progress"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
	Trace     []AutoFixIssueTrace  `json:"trace,omitempty"`
}

// AutoFixBatchIssue represents a single issue in a batch
type AutoFixBatchIssue struct {
	ObjectID   string `json:"object_id"`
	ObjectKind string `json:"object_kind"`
	FilePath   string `json:"file_path"`
	Issue      Issue  `json:"issue"`
}

// AutoFixBatchObject represents one object and all issues to apply to it.
// Used by the newer batch file format.
type AutoFixBatchObject struct {
	ObjectID   string  `json:"object_id"`
	ObjectKind string  `json:"object_kind"`
	FilePath   string  `json:"file_path"`
	Issues     []Issue `json:"issues"`
}

// AutoFixBatchProgress tracks progress of batch processing
type AutoFixBatchProgress struct {
	Total      int       `json:"total"`
	Processed  int       `json:"processed"`
	Fixed      int       `json:"fixed"`
	Failed     int       `json:"failed"`
	Skipped    int       `json:"skipped"`
	LastUpdate time.Time `json:"last_update"`
}

// AutoFixIssueTrace captures per-issue execution and persistence outcome.
// This provides deterministic traceability for every attempted fix in a batch.
type AutoFixIssueTrace struct {
	ObjectID       string    `json:"object_id"`
	ObjectKind     string    `json:"object_kind"`
	Category       string    `json:"category"`
	Message        string    `json:"message"`
	AutoFixable    bool      `json:"auto_fixable"`
	Status         string    `json:"status"`
	FixMessage     string    `json:"fix_message,omitempty"`
	Persistence    string    `json:"persistence,omitempty"`
	Persisted      bool      `json:"persisted"`
	Error          string    `json:"error,omitempty"`
	ProcessedAtUTC time.Time `json:"processed_at_utc"`
}

// AutoFixSchedulerBatcher manages batching and submission of auto-fix work to scheduler
type AutoFixSchedulerBatcher struct {
	projectRoot string
	logger      logging.Logger
	batchSize   int // Maximum issues per batch (default: 100)
}

// NewAutoFixSchedulerBatcher creates a new auto-fix scheduler batcher
func NewAutoFixSchedulerBatcher(projectRoot string, logger logging.Logger) *AutoFixSchedulerBatcher {
	return &AutoFixSchedulerBatcher{
		projectRoot: projectRoot,
		logger:      logger,
		batchSize:   100, // Default batch size
	}
}

// CollectAutoFixableIssues collects all fixable issues from check results (AutoFixable, Tier 4, or with FixCommand).
// Aligns with --auto-fix-scheduler and shouldProcessIssue so Tier 4 and guidance-style fixes are batched too.
func (asb *AutoFixSchedulerBatcher) CollectAutoFixableIssues(results []CheckResult) []AutoFixBatchIssue {
	var batchIssues []AutoFixBatchIssue

	for _, result := range results {
		for _, issue := range result.Issues {
			if IsIssueFixableForBatch(issue) {
				batchIssues = append(batchIssues, AutoFixBatchIssue{
					ObjectID:   result.ObjectID,
					ObjectKind: result.ObjectKind,
					FilePath:   result.FilePath,
					Issue:      issue,
				})
			}
		}
	}

	return batchIssues
}

// CreateBatches splits issues into batches for processing
func (asb *AutoFixSchedulerBatcher) CreateBatches(issues []AutoFixBatchIssue) []*AutoFixBatch {
	var batches []*AutoFixBatch

	for i := 0; i < len(issues); i += asb.batchSize {
		end := i + asb.batchSize
		if end > len(issues) {
			end = len(issues)
		}

		chunk := issues[i:end]
		// Group chunk issues by object so the batch file doesn't duplicate object metadata.
		byObject := make(map[string]*AutoFixBatchObject, len(chunk))
		orderedIDs := make([]string, 0, len(chunk))
		for _, it := range chunk {
			if it.ObjectID == emptyValue {
				continue
			}
			obj := byObject[it.ObjectID]
			if obj == nil {
				obj = &AutoFixBatchObject{
					ObjectID:   it.ObjectID,
					ObjectKind: it.ObjectKind,
					FilePath:   it.FilePath,
					Issues:     make([]Issue, 0, 4),
				}
				byObject[it.ObjectID] = obj
				orderedIDs = append(orderedIDs, it.ObjectID)
			}
			obj.Issues = append(obj.Issues, it.Issue)
		}
		objects := make([]AutoFixBatchObject, 0, len(orderedIDs))
		for _, id := range orderedIDs {
			if obj := byObject[id]; obj != nil {
				objects = append(objects, *obj)
			}
		}

		batch := &AutoFixBatch{
			BatchID:     fmt.Sprintf("%s%d", autoFixBatchPrefix, time.Now().UnixNano()),
			ProjectRoot: asb.projectRoot,
			Objects:     objects,
			CreatedAt:   time.Now(),
			Status:      autoFixBatchStatusPending,
			Progress: AutoFixBatchProgress{
				Total:      len(chunk),
				LastUpdate: time.Now(),
			},
		}

		batches = append(batches, batch)
	}

	return batches
}

// SubmitBatchToScheduler submits an auto-fix batch to the scheduler as a run_wrapper job
func (asb *AutoFixSchedulerBatcher) SubmitBatchToScheduler(ctx *cli.Context, batch *AutoFixBatch) (string, error) {
	// Create a temporary file with batch data
	batchFile := filepath.Join(asb.projectRoot, paths.ProjectDataDir, paths.AutofixDir, fmt.Sprintf("%s.json", batch.BatchID))
	if err := fileutil.MkdirAll(filepath.Dir(batchFile), paths.DirPerm755); err != nil {
		return "", errfmt.Newf("failed to create batch directory").Wrap(err)
	}

	// Write batch data to file
	batchData, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return "", errfmt.Newf("failed to marshal batch").Wrap(err)
	}

	if err := fileutil.WriteFile(batchFile, batchData, paths.FilePerm644); err != nil { //nolint:gosec // Batch files - 0600 is acceptable for user-readable files
		return "", errfmt.Newf("failed to write batch file").Wrap(err)
	}
	submissionPersisted := false
	defer func() {
		if cleanupErr := removeUnsubmittedAutoFixBatchFile(batchFile, submissionPersisted); cleanupErr != nil {
			logging.Fluent(asb.logger).Warn("Failed to remove unsubmitted auto-fix batch file").
				File(batchFile).
				WithError(cleanupErr).
				Log()
		}
	}()

	// Create scheduler job command
	// The command will execute: zqk system auto-fix-batch --batch-file <file>
	command := paths.ResolveProductCLI(asb.projectRoot)
	commandArgs := []string{
		"system",
		"auto-fix-batch",
		"--batch-file", batchFile,
		"--project-root", asb.projectRoot,
	}

	// Create job data for scheduler
	jobID := fmt.Sprintf("%s%d", autoFixJobPrefix, time.Now().UnixNano())
	now := time.Now()

	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            autoFixJobStatusActive,
		objects.FieldKeyJobType:           autoFixJobTypeRunWrapper,
		objects.FieldKeyTriggerType:       autoFixJobTriggerManual, // Use manual so we can trigger via queue
		objects.FieldKeyCategory:          autoFixJobCategory,
		objects.FieldKeyExecutionMode:     autoFixJobExecutionOnce,
		objects.FieldKeyMaxRuntimeSeconds: 3600, // 1 hour timeout
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyTitle:             fmt.Sprintf("Auto-fix batch: %d issues", batch.Progress.Total),
		objects.FieldKeyDescription:       fmt.Sprintf("Auto-fix batch %s processing %d issues", batch.BatchID, batch.Progress.Total),
		objects.FieldKeyCommand:           command,
		objects.FieldKeyCommandArgs:       commandArgs,
		objects.FieldKeyRetryCount:        1, // Retry once on failure
		objects.FieldKeyRetryDelaySeconds: 30,
		objects.FieldKeyCreatedAt:         zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:         zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:         pkgctx.SystemAccountID,
		objects.FieldKeyWorkingDirectory:  asb.projectRoot,
		objects.FieldKeyLogLevel:          autoFixJobLogLevel, // spec enum: default, verbose, debug (not "info")
	}

	// Submit job to scheduler via storage
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, asb.projectRoot)
	if err != nil {
		return "", errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	storageProvider := storageFactory.GetStorage()
	if storageProvider == nil {
		return "", errfmt.Errorf("storage provider is nil")
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Check if a job with this ID already exists (for one_time jobs, reuse existing)
	existingJobs, listErr := storageProvider.List(stdctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind:    objects.KindSchedulerJob,
		Filters: map[string]any{objects.FieldKeyID: jobID},
		Limit:   1,
	})

	if listErr == nil && len(existingJobs.Objects) > 0 {
		existingJob := existingJobs.Objects[0]
		executionMode, _ := existingJob[objects.FieldKeyExecutionMode].(string)

		// For one_time jobs, update the existing job instead of creating a new one
		if executionMode == autoFixJobExecutionOnce {
			// Update the existing job: reset enabled, update timestamps and command/args
			updates := map[string]any{
				objects.FieldKeyEnabled:          true,
				objects.FieldKeyUpdatedAt:        now.Format(time.RFC3339),
				objects.FieldKeyUpdatedBy:        pkgctx.SystemAccountID,
				objects.FieldKeyTitle:            fmt.Sprintf("Auto-fix batch: %d issues", batch.Progress.Total),
				objects.FieldKeyDescription:      fmt.Sprintf("Auto-fix batch %s processing %d issues", batch.BatchID, batch.Progress.Total),
				objects.FieldKeyCommand:          command,
				objects.FieldKeyCommandArgs:      commandArgs,
				objects.FieldKeyWorkingDirectory: asb.projectRoot,
			}

			// Update the existing job object (storage.Update handles ID->hash mapping for CAS)
			if err := storageProvider.Update(stdctx, secCtx, jobID, updates); err != nil {
				return "", errfmt.Newf("failed to update existing scheduler job").Wrap(err)
			}

			logging.Fluent(asb.logger).Info(autoFixJobReuseMsg).
				JobID(jobID).
				BatchID(batch.BatchID).
				Log()
		} else {
			// For reusable jobs, if one exists with same ID, that's an error
			return "", errfmt.Errorf("scheduler job with ID %s already exists (execution_mode: %s)", jobID, executionMode)
		}
	} else {
		// No existing job found - create new one
		// Note: storage.Create expects (ctx, secCtx, obj) where obj contains the ID
		jobData[objects.FieldKeyID] = jobID
		createCtx := pkgctx.WithPromoteOnCreate(stdctx)
		if err := storageProvider.Create(createCtx, secCtx, jobData); err != nil {
			return "", errfmt.Newf("failed to create scheduler job").Wrap(err)
		}
	}
	submissionPersisted = true

	// Enqueue trigger request so scheduler picks up the job
	triggerQueue := scheduler.NewJobTriggerQueue(asb.projectRoot)
	if err := triggerQueue.EnqueueTriggerRequest(jobID); err != nil {
		logging.Fluent(asb.logger).Warn("Failed to enqueue trigger request (job created but may not run immediately)").
			JobID(jobID).
			WithError(err).
			Log()
		// Don't fail - job is created, scheduler may pick it up on reload
	}

	logging.Fluent(asb.logger).Info("Auto-fix batch submitted to scheduler").
		BatchID(batch.BatchID).
		JobID(jobID).
		Int("issue_count", batch.Progress.Total).
		Log()

	return jobID, nil
}

func removeUnsubmittedAutoFixBatchFile(batchFile string, submissionPersisted bool) error {
	if submissionPersisted {
		return nil
	}
	if err := fileutil.Remove(batchFile); err != nil && !fileutil.IsNotExist(err) {
		return err
	}
	return nil
}

// SubmitAllBatches submits all batches to the scheduler
func (asb *AutoFixSchedulerBatcher) SubmitAllBatches(ctx *cli.Context, batches []*AutoFixBatch) ([]string, error) {
	var jobIDs []string

	for _, batch := range batches {
		jobID, err := asb.SubmitBatchToScheduler(ctx, batch)
		if err != nil {
			logging.Fluent(asb.logger).Warn("Failed to submit batch to scheduler").
				BatchID(batch.BatchID).
				WithError(err).
				Log()
			continue
		}

		jobIDs = append(jobIDs, jobID)
	}

	if len(jobIDs) == 0 {
		return nil, errfmt.Errorf("failed to submit any batches to scheduler")
	}

	logging.Fluent(asb.logger).Info("All auto-fix batches submitted").
		Int("batch_count", len(batches)).
		Int("job_count", len(jobIDs)).
		Log()

	return jobIDs, nil
}

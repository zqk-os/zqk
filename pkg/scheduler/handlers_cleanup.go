package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	baseMetricEnum "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/metrics"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// Log events for autofix_batch_cleanup handler (POL-CODE-007 stable keys).
const (
	LogEventAutofixBatchCleanupJobStart                  = JobTypeAutofixBatchCleanup + "_job_start"
	LogEventAutofixBatchCleanupSkipNoAutofixDir          = JobTypeAutofixBatchCleanup + "_skip_no_autofix_dir"
	LogEventAutofixBatchCleanupReadAutofixDirFailed      = JobTypeAutofixBatchCleanup + "_read_autofix_dir_failed"
	LogEventAutofixBatchCleanupRemovedOldUnprocessedFile = JobTypeAutofixBatchCleanup + "_removed_old_unprocessed_file"
	LogEventAutofixBatchCleanupReadBatchFileFailed       = JobTypeAutofixBatchCleanup + "_read_batch_file_failed"
	LogEventAutofixBatchCleanupParseBatchFileFailed      = JobTypeAutofixBatchCleanup + "_parse_batch_file_failed"
	LogEventAutofixBatchCleanupBatchFileMissingBatchID   = JobTypeAutofixBatchCleanup + "_batch_file_missing_batch_id"
	LogEventAutofixBatchCleanupMetricBuildFailed         = JobTypeAutofixBatchCleanup + "_metric_build_failed"
	LogEventAutofixBatchCleanupMetricCreateFailed        = JobTypeAutofixBatchCleanup + "_metric_create_failed"
	LogEventAutofixBatchCleanupPersistErrorRecordFailed  = JobTypeAutofixBatchCleanup + "_persist_error_record_failed"
	LogEventAutofixBatchCleanupMetricErrorPersisted      = JobTypeAutofixBatchCleanup + "_metric_error_persisted"
	LogEventAutofixBatchCleanupDeleteBatchFileFailed     = JobTypeAutofixBatchCleanup + "_delete_batch_file_failed"
	LogEventAutofixBatchCleanupJobCompleted              = JobTypeAutofixBatchCleanup + "_job_completed"
	LogEventAutofixBatchCleanupPartialErrors             = JobTypeAutofixBatchCleanup + "_partial_errors"
)

// AutofixBatchCleanupHandler cleans up processed auto-fix batch files
type AutofixBatchCleanupHandler struct {
	storage     storagepkg.ObjectStorageProvider
	logger      logging.Logger
	slog        *SchedulerLogRoot
	projectRoot string
}

// NewAutofixBatchCleanupHandler creates a new autofix batch cleanup handler
// NewAutofixBatchCleanupHandler creates a new autofix batch cleanup handler
func NewAutofixBatchCleanupHandler(storage storagepkg.ObjectStorageProvider, projectRoot string) AutofixBatchCleanupHandlerInterface {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	return &AutofixBatchCleanupHandler{
		storage:     storage,
		logger:      logger,
		slog:        SLog(logger),
		projectRoot: projectRoot,
	}
}

func (h *AutofixBatchCleanupHandler) schedulerLog() *SchedulerLogRoot {
	if h.slog != nil {
		return h.slog
	}
	return SLog(h.logger)
}

// Default max age for unprocessed AUTOFIX-*.json batch files (hours). Older files are deleted to avoid piling up.
// Use 1 so that an hourly job run removes unprocessed files from the previous hour(s). Set job env AUTOFIX_BATCH_MAX_AGE_HOURS=0 to delete all unprocessed.
const defaultAutofixBatchMaxAgeHours = 1

// maxErrorTitleLength caps the title length for error records so we stay within field limits.
const maxErrorTitleLength = 200

// buildBatchMetricErrorRecord builds a minimal base_metric with status=error for a failed create.
// Callers can persist it so the failure is queryable (e.g. filter status=error); remediation: run `zqk system check --verbose`.
func buildBatchMetricErrorRecord(metricID, batchID, nowStr string, createErr error) map[string]any {
	title := fmt.Sprintf("Auto-fix batch %s: create failed - %s", batchID, createErr.Error())
	if len(title) > maxErrorTitleLength {
		title = title[:maxErrorTitleLength-3] + "..."
	}
	builder := bldr_instance_v1.NewBaseMetricInstanceBuilder(objects.DefaultSchemaVersion)
	builder.ID(metricID)
	builder.Status(baseMetricEnum.StatusError)
	builder.SetField(objects.FieldKeyTitle, title)
	builder.
		SetMetricType("system").
		SetSource("auto_fix_batch").
		SetTags([]string{"system", "auto_fix", "batch_processing", "error"}).
		SetCollectionCount(0).
		SetFirstSeen(nowStr).
		SetLastSeen(nowStr)
	builder.SetField(objects.FieldKeyBatchID, batchID)
	obj, err := builder.Build()
	if err != nil {
		return nil
	}
	return obj
}

// Execute runs autofix batch cleanup via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *AutofixBatchCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunAutofixBatchCleanupViaPipeline(ctx, h, job)
}

// executeAutofixBatchCleanupCore cleans up processed batch files and old unprocessed AUTOFIX-*.json. Called from RunAutofixBatchCleanupViaPipeline NORMALIZE stage.
func (h *AutofixBatchCleanupHandler) executeAutofixBatchCleanupCore(ctx context.Context, job *ScheduledJob) error {
	h.schedulerLog().Info(LogEventAutofixBatchCleanupJobStart).
		JobID(job.ID).
		Log()

	autofixDir := filepath.Join(h.projectRoot, paths.ProjectDataDir, "autofix")

	// Check if directory exists
	if _, err := fileutil.Stat(autofixDir); fileutil.IsNotExist(err) {
		h.schedulerLog().Debug(LogEventAutofixBatchCleanupSkipNoAutofixDir).
			JobID(job.ID).
			Log()
		return nil
	}

	// Max age for unprocessed AUTOFIX-*.json (hours). Issues may have been fixed by recover-cas or system check.
	maxAgeHours := defaultAutofixBatchMaxAgeHours
	if job != nil && job.EnvironmentVariables != nil {
		if s, ok := job.EnvironmentVariables[EnvKeyAutofixBatchMaxAgeHours]; ok && s != emptyValue {
			if n, err := strconv.Atoi(s); err == nil && n >= 0 {
				maxAgeHours = n
			}
		}
	}
	maxAge := time.Duration(maxAgeHours) * time.Hour
	cutoff := time.Now().Add(-maxAge)

	// Read directory
	entries, err := fileutil.ReadDir(autofixDir)
	if err != nil {
		h.schedulerLog().Error(LogEventAutofixBatchCleanupReadAutofixDirFailed, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("failed to read autofix directory").Wrap(err)
	}

	var processedFiles []string
	var metricsCaptured int
	var filesDeleted int
	var errors []string

	secCtx := pkgctx.NewSystemSecurityContext()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		filePath := filepath.Join(autofixDir, fileName)

		// Delete old unprocessed AUTOFIX-*.json (e.g. issues fixed by recover-cas; batch never run or renamed)
		if strings.HasPrefix(fileName, "AUTOFIX-") && strings.HasSuffix(fileName, ".json") {
			info, err := fileutil.Stat(filePath)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				if removeErr := fileutil.Remove(filePath); removeErr != nil {
					errors = append(errors, fmt.Sprintf("failed to delete old batch %s: %v", fileName, removeErr))
				} else {
					filesDeleted++
					h.schedulerLog().Debug(LogEventAutofixBatchCleanupRemovedOldUnprocessedFile).
						JobID(job.ID).
						File(fileName).
						Log()
				}
			}
			continue
		}

		// Only process files with FIXED_ or PROCESSED_ prefix
		if !strings.HasPrefix(fileName, "FIXED_") && !strings.HasPrefix(fileName, "PROCESSED_") {
			continue
		}

		processedFiles = append(processedFiles, filePath)

		// Read batch file to check if metrics already captured
		batchData, err := fileutil.ReadFile(filePath)
		if err != nil {
			h.schedulerLog().Warn(LogEventAutofixBatchCleanupReadBatchFileFailed).
				WithFields(append(jobLogFieldsWithErr(job, err), logging.String("file", fileName))...).
				Log()
			errors = append(errors, fmt.Sprintf("failed to read %s: %v", fileName, err))
			continue
		}

		// Parse batch file to extract batch_id and progress info
		var batchDataMap map[string]any
		if err := json.Unmarshal(batchData, &batchDataMap); err != nil {
			h.schedulerLog().Warn(LogEventAutofixBatchCleanupParseBatchFileFailed).
				WithFields(append(jobLogFieldsWithErr(job, err), logging.String("file", fileName))...).
				Log()
			errors = append(errors, fmt.Sprintf("failed to parse %s: %v", fileName, err))
			// Still delete the file even if we can't parse it
			if err := fileutil.Remove(filePath); err == nil {
				filesDeleted++
			}
			continue
		}

		// Extract batch_id
		batchID, _ := batchDataMap[objects.FieldKeyBatchID].(string)
		if batchID == emptyValue {
			h.schedulerLog().Warn(LogEventAutofixBatchCleanupBatchFileMissingBatchID).
				JobID(job.ID).
				File(fileName).
				Log()
			// Still delete the file
			if err := fileutil.Remove(filePath); err == nil {
				filesDeleted++
			}
			continue
		}

		// Check if metrics already captured by looking for base_metric with this batch_id
		metricsAlreadyCaptured := false
		if h.storage != nil && batchID != emptyValue {
			storageCtx := pkgctx.NewStorageContext()
			filter := storagepkg.ListFilter{
				Kind: objects.KindBaseMetric,
				Filters: map[string]any{
					objects.FieldKeyBatchID: batchID,
				},
				Limit: 1,
			}
			result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
			if err == nil && len(result.Objects) > 0 {
				metricsAlreadyCaptured = true
			}
		}

		// Capture metrics if not already captured
		if !metricsAlreadyCaptured && h.storage != nil && batchID != emptyValue {
			// Extract progress data from batch
			progressMap, _ := batchDataMap["progress"].(map[string]any)
			processed, _ := progressMap[objects.FieldKeyProcessed].(float64)
			fixed, _ := progressMap[objects.FieldKeyFixed].(float64)
			failed, _ := progressMap[objects.FieldKeyFailed].(float64)
			skipped, _ := progressMap[objects.FieldKeySkipped].(float64)

			// Calculate duration from batch data
			var duration time.Duration
			createdAtStr, _ := batchDataMap[objects.FieldKeyCreatedAt].(string)
			lastUpdateStr, _ := progressMap["last_update"].(string)
			if createdAtStr != emptyValue && lastUpdateStr != emptyValue {
				if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
					if lastUpdate, err := time.Parse(time.RFC3339, lastUpdateStr); err == nil {
						duration = lastUpdate.Sub(createdAt)
					}
				}
			}

			// Create base_metric using instance builder (same pattern as audit_aggregation, file_lock_metrics_collector).
			now := time.Now().UTC()
			nowStr := now.Format(time.RFC3339)
			tags := []string{"system", "auto_fix", "batch_processing"}
			if failed > 0 {
				tags = append(tags, "has_errors")
			}
			metricID := fmt.Sprintf("BAS-%d", now.UnixNano())

			builder := bldr_instance_v1.NewBaseMetricInstanceBuilder(objects.DefaultSchemaVersion)
			// Use a lifecycle-valid generic status for base_metric; detailed outcome
			// is captured in fields like processed/fixed/failed.
			builder.ID(metricID)
			builder.Status(baseMetricEnum.StatusImplemented)
			builder.SetField(objects.FieldKeyTitle, fmt.Sprintf("Auto-fix batch %s: %.0f processed, %.0f fixed", batchID, processed, fixed))
			builder.
				SetMetricType("system").
				SetSource("auto_fix_batch").
				SetTags(tags).
				SetCollectionCount(1).
				SetFirstSeen(nowStr).
				SetLastSeen(nowStr)
			builder.SetField(objects.FieldKeyBatchID, batchID)
			builder.SetField(objects.FieldKeyProcessed, int(processed))
			builder.SetField(objects.FieldKeyFixed, int(fixed))
			builder.SetField(objects.FieldKeyFailed, int(failed))
			builder.SetField(objects.FieldKeySkipped, int(skipped))
			builder.SetField(objects.FieldKeyDurationSeconds, duration.Seconds())
			if processed > 0 {
				builder.SetField(objects.FieldKeySuccessRate, float64(fixed)/float64(processed)*100.0)
			}

			metricObj, buildErr := builder.Build()
			if buildErr != nil {
				h.schedulerLog().Debug(LogEventAutofixBatchCleanupMetricBuildFailed).
					BatchID(batchID).
					WithError(buildErr).
					Log()
				errors = append(errors, fmt.Sprintf("failed to build metric for %s: %v", batchID, buildErr))
			} else {
				// Create synchronously so we can persist status=error on failure (queryable; lifecycle supports error).
				metricCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				createErr := h.storage.Create(metricCtx, secCtx, metricObj)
				cancel()
				if createErr != nil {
					h.schedulerLog().Warn(LogEventAutofixBatchCleanupMetricCreateFailed).
						BatchID(batchID).
						WithFields(logErrField(createErr)...).
						Log()
					// Persist with status=error so we have a record and don't retry the same failed create.
					// Remediation: list objects with status=error; run `zqk system check --verbose` for details.
					errObj := buildBatchMetricErrorRecord(metricID, batchID, nowStr, createErr)
					if errObj != nil {
						if errFallback := h.storage.Create(context.Background(), secCtx, errObj); errFallback != nil { // Background: request-or-shutdown derived
							h.schedulerLog().Debug(LogEventAutofixBatchCleanupPersistErrorRecordFailed).
								BatchID(batchID).
								WithFields(logErrField(errFallback)...).
								Log()
						} else {
							metricsCaptured++
							h.schedulerLog().Info(LogEventAutofixBatchCleanupMetricErrorPersisted).
								BatchID(batchID).
								MetricID(metricID).
								Log()
						}
					}
					errors = append(errors, fmt.Sprintf("batch %s: %v", batchID, createErr))
				} else {
					metricsCaptured++
				}
			}
		}

		// Delete the file
		removeErr := fileutil.Remove(filePath)
		when.When(func() bool { return removeErr != nil }).Then(func() {
			h.schedulerLog().Warn(LogEventAutofixBatchCleanupDeleteBatchFileFailed).
				WithFields(append(jobLogFieldsWithErr(job, removeErr), logging.String("file", fileName))...).
				Log()
			errors = append(errors, fmt.Sprintf("failed to delete %s: %v", fileName, removeErr))
		}).OrElse(func() {
			filesDeleted++
		}).Run()
	}

	h.schedulerLog().Info(LogEventAutofixBatchCleanupJobCompleted).
		JobID(job.ID).
		FilesProcessed(len(processedFiles)).
		MetricsCaptured(metricsCaptured).
		FilesDeleted(filesDeleted).
		Int("errors", len(errors)).
		Log()

	if len(errors) > 0 {
		h.schedulerLog().Warn(LogEventAutofixBatchCleanupPartialErrors).
			JobID(job.ID).
			ErrorCount(len(errors)).
			Log()
		// Don't fail the job - partial cleanup is better than none
	}

	return nil
}

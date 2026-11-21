package scheduler

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// Log events for scheduler_job_retention handler (POL-CODE-007 stable keys).
const (
	LogEventSchedulerJobRetentionJobStart                 = JobTypeSchedulerJobRetention + "_job_start"
	LogEventSchedulerJobRetentionConfig                   = JobTypeSchedulerJobRetention + "_config"
	LogEventSchedulerJobRetentionListFailed               = JobTypeSchedulerJobRetention + "_list_failed"
	LogEventSchedulerJobRetentionBulkDeleteFailed         = JobTypeSchedulerJobRetention + "_bulk_delete_failed"
	LogEventSchedulerJobRetentionRemoveLogDirFailed       = JobTypeSchedulerJobRetention + "_remove_log_dir_failed"
	LogEventSchedulerJobRetentionRemoveOrphanLogDirFailed = JobTypeSchedulerJobRetention + "_remove_orphan_log_dir_failed"
	LogEventSchedulerJobRetentionJobCompleted             = JobTypeSchedulerJobRetention + "_job_completed"
	LogEventSchedulerJobRetentionCASFlushFailed           = JobTypeSchedulerJobRetention + "_cas_flush_failed"
)

// Default retention and batch limits for scheduler job cleanup.
const (
	defaultSchedulerJobRetentionDays       = 7
	defaultSchedulerJobRetentionBatchSize  = 100
	defaultSchedulerJobRetentionMaxBatches = 50
	autofixJobIDPrefix                     = "SCH-AUTOFIX-"
)

// schedulerJobRetentionCandidateListFilters is the list surface for one_time jobs
// the sweeper may delete. IsSchedulerJobMarkedForDeletion already treats lifecycle
// terminal statuses (archived) as marked, but List only returns rows that match a
// filter — archived+enabled=true never appeared in the enabled=false / status=disabled
// passes. Including trigger_type=manual also surfaces stale SCH-AUTOFIX-* one_time jobs
// left active+enabled. TRACK: POL
func schedulerJobRetentionCandidateListFilters(batchSize int) []storagepkg.ListFilter {
	mk := func(extra map[string]any) storagepkg.ListFilter {
		filters := map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime}
		maps.Copy(filters, extra)
		return storagepkg.ListFilter{
			Kind:    objects.KindSchedulerJob,
			Filters: filters,
			SortBy:  objects.FieldKeyCreatedAt,
			SortAsc: true,
			Limit:   batchSize,
		}
	}
	return []storagepkg.ListFilter{
		mk(map[string]any{objects.FieldKeyEnabled: false}),
		mk(map[string]any{objects.FieldKeyStatus: StatusDisabled}),
		mk(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived}),
		mk(map[string]any{objects.FieldKeyTriggerType: TriggerTypeManual}),
	}
}

// activeRunningJobIDs returns the set of job IDs currently executing in-process or recorded
// in the on-disk running-jobs snapshot so retention never reaps an active job.
func activeRunningJobIDs(projectRoot string) map[string]bool {
	running := make(map[string]bool)
	if sched, ok := GetGlobalSchedulerIfAvailable(); ok && sched != nil && sched.conflictMgr != nil {
		for _, id := range sched.conflictMgr.GetRunningJobIDs() {
			if id != emptyValue {
				running[id] = true
			}
		}
	}
	if snapIDs, _, ok := LoadRunningJobsSnapshot(projectRoot); ok {
		for _, id := range snapIDs {
			if id != emptyValue {
				running[id] = true
			}
		}
	}
	return running
}

// isSchedulerJobRetentionCandidate returns true when a one_time job is not currently running
// and is either marked for deletion or is a stale SCH-AUTOFIX-* one_time job.
func isSchedulerJobRetentionCandidate(id string, obj map[string]any, runningIDs map[string]bool) bool {
	if runningIDs[id] {
		return false
	}
	if IsSchedulerJobMarkedForDeletion(obj) {
		return true
	}
	execMode, _ := obj[objects.FieldKeyExecutionMode].(string)
	return execMode == ExecutionModeOneTime && strings.HasPrefix(id, autofixJobIDPrefix)
}

// buildSchedulerJobIDSet returns an O(1) lookup set of existing scheduler_job IDs from the CAS
// index (or storage.List fallback), avoiding N storage.Read calls during orphan log cleanup.
func (h *SchedulerJobRetentionHandler) buildSchedulerJobIDSet(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext) (map[string]bool, bool) {
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		if cas, casErr := fileStorage.GetContentAddressableStorage(objects.KindSchedulerJob); casErr == nil && cas != nil {
			if ids, listErr := cas.ListIDs(); listErr == nil {
				idSet := make(map[string]bool, len(ids))
				for _, id := range ids {
					if id != emptyValue {
						idSet[id] = true
					}
				}
				return idSet, true
			}
		}
	}
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindSchedulerJob, Limit: 0})
	if err != nil {
		return nil, false
	}
	idSet := make(map[string]bool, len(res.Objects))
	for _, obj := range res.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
			idSet[id] = true
		}
	}
	return idSet, true
}

// SchedulerJobRetentionHandler archives and deletes old one_time scheduler jobs that have already run
// (enabled=false), so the job store does not grow indefinitely. Reusable jobs (timer, manual, etc.)
// are never touched.
type SchedulerJobRetentionHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
	onProgress  ProgressFunc
}

// NewSchedulerJobRetentionHandler creates a new scheduler job retention handler.
// NewSchedulerJobRetentionHandler creates a new scheduler job retention handler
func NewSchedulerJobRetentionHandler(storage storagepkg.ObjectStorageProvider, projectRoot string) SchedulerJobRetentionHandlerInterface {
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}
	return &SchedulerJobRetentionHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// SetProgressFunc sets an optional callback for progress (e.g. CLI stderr).
func (h *SchedulerJobRetentionHandler) SetProgressFunc(fn ProgressFunc) {
	h.onProgress = fn
}

func (h *SchedulerJobRetentionHandler) emitProgress(msg string) {
	if h.onProgress != nil {
		h.onProgress(msg)
	}
}

// getRetentionConfig returns retention days, batch size, and max batches from job env or defaults.
func getSchedulerJobRetentionConfig(job *ScheduledJob) (retentionDays, batchSize, maxBatches int) {
	retentionDays = defaultSchedulerJobRetentionDays
	batchSize = defaultSchedulerJobRetentionBatchSize
	maxBatches = defaultSchedulerJobRetentionMaxBatches
	if scheduledJobEnvMissing(job) {
		return retentionDays, batchSize, maxBatches
	}
	if s, ok := job.EnvironmentVariables[EnvKeyRetentionDays]; ok && s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			retentionDays = n
		}
	}
	if s, ok := job.EnvironmentVariables[EnvKeyBatchSize]; ok && s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			batchSize = n
		}
	}
	if s, ok := job.EnvironmentVariables[EnvKeyMaxBatches]; ok && s != emptyValue {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxBatches = n
		}
	}
	return retentionDays, batchSize, maxBatches
}

// Execute runs scheduler job retention via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *SchedulerJobRetentionHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunSchedulerJobRetentionViaPipeline(ctx, h, job)
}

// executeSchedulerJobRetentionCore lists one_time, enabled=false scheduler_job objects older than retention, then deletes them. Called from RunSchedulerJobRetentionViaPipeline NORMALIZE stage. Caller must set storagepkg.WithCLIOperation on ctx.
func (h *SchedulerJobRetentionHandler) executeSchedulerJobRetentionCore(ctx context.Context, job *ScheduledJob) error {
	h.emitProgress("Starting scheduler job retention (cleanup old one_time jobs)...")
	SLog(h.logger).Info(LogEventSchedulerJobRetentionJobStart).
		JobID(job.ID).
		Log()

	// Run CAS recovery for scheduler_job so stale index entries are removed before retention runs.
	runCASRecoveryForKind(ctx, h.projectRoot, objects.KindSchedulerJob, h.logger, h.storage)

	retentionDays, batchSize, maxBatches := getSchedulerJobRetentionConfig(job)
	var cutoff time.Time
	when.When(func() bool { return retentionDays <= 0 }).Then(func() {
		cutoff = time.Time{} // zero: delete all matching regardless of age (one-off cleanup)
	}).OrElse(func() {
		cutoff = time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	}).Run()
	cutoffStr := cutoff.Format(time.RFC3339)
	if retentionDays <= 0 {
		cutoffStr = "any (RETENTION_DAYS=0)"
	}

	SLog(h.logger).Info(LogEventSchedulerJobRetentionConfig).
		JobID(job.ID).
		Int("retention_days", retentionDays).
		String("cutoff", cutoffStr).
		BatchSize(batchSize).
		MaxBatches(maxBatches).
		Log()

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := &pkgctx.StorageContext{}
	runningIDs := activeRunningJobIDs(h.projectRoot)

	// List one_time jobs that are done (enabled=false, status=disabled), archived,
	// or stale SCH-AUTOFIX-* manual one_time jobs. Predicate still refuses reusable jobs.
	var totalDeleted int
	for batchNum := 0; batchNum < maxBatches; batchNum++ {
		h.emitProgress("Listing and cleaning scheduler_job batch...")
		objectsByID := make(map[string]map[string]any)
		for _, listFilter := range schedulerJobRetentionCandidateListFilters(batchSize) {
			result, err := h.storage.List(ctx, secCtx, storageCtx, listFilter)
			if err != nil {
				SLog(h.logger).Warn(LogEventSchedulerJobRetentionListFailed).
					WithFields(jobLogFieldsByIDAndErr(job.ID, err)...).
					Log()
				continue
			}
			for _, obj := range result.Objects {
				if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue && id != job.ID {
					objectsByID[id] = obj
				}
			}
		}
		if len(objectsByID) == 0 {
			break
		}
		var toDelete []string
		for id, obj := range objectsByID {
			if !isSchedulerJobRetentionCandidate(id, obj, runningIDs) {
				continue
			}
			var t time.Time
			if lastRun, ok := obj[objects.FieldKeyLastRunAt].(string); ok && lastRun != emptyValue {
				if parsed, err := time.Parse(time.RFC3339, lastRun); err == nil {
					t = parsed
				}
			}
			if t.IsZero() {
				if created, ok := obj[objects.FieldKeyCreatedAt].(string); ok && created != emptyValue {
					if parsed, err := time.Parse(time.RFC3339, created); err == nil {
						t = parsed
					}
				}
			}
			if retentionDays <= 0 {
				toDelete = append(toDelete, id)
			} else if !t.IsZero() && t.Before(cutoff) {
				toDelete = append(toDelete, id)
			}
		}
		if len(toDelete) == 0 {
			break
		}
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, toDelete, false, 20)
			if delErr != nil {
				SLog(h.logger).Warn(LogEventSchedulerJobRetentionBulkDeleteFailed).
					WithFields(jobLogFieldsByIDAndErr(job.ID, delErr)...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, toDelete, false)
			if delErr != nil {
				SLog(h.logger).Warn(LogEventSchedulerJobRetentionBulkDeleteFailed).
					WithFields(jobLogFieldsByIDAndErr(job.ID, delErr)...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(objects.KindSchedulerJob)

		if h.projectRoot != emptyValue {
			if queue := caspkg.GetGlobalListingIndexWriteQueue(); queue != nil {
				_ = queue.FlushKind(objects.KindSchedulerJob, 5*time.Second)
			}
		}

		// Remove log directories for deleted job IDs so logs don't accumulate.
		for _, id := range toDelete {
			logDir := filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", id)
			if err := fileutil.RemoveAll(logDir); err != nil && !fileutil.IsNotExist(err) {
				SLog(h.logger).Warn(LogEventSchedulerJobRetentionRemoveLogDirFailed).
					JobID(job.ID).
					String("deleted_job_id", id).
					WithError(err).
					Log()
			}
		}
		if n < len(toDelete) {
			break
		}
	}

	// Remove orphaned log dirs (job no longer exists) using O(1) CAS/index lookup.
	if h.projectRoot != emptyValue {
		logRoot := filepath.Join(h.projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler")
		entries, err := fileutil.ReadDir(logRoot)
		if err == nil {
			idSet, hasIDSet := h.buildSchedulerJobIDSet(ctx, secCtx, storageCtx)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				jobID := e.Name()
				if jobID == emptyValue || jobID == job.ID {
					continue
				}
				isOrphan := false
				if hasIDSet {
					isOrphan = !idSet[jobID]
				} else {
					_, getErr := h.storage.Read(ctx, secCtx, jobID)
					isOrphan = getErr != nil && (errors.Is(getErr, storagepkg.ErrObjectNotFound) || strings.Contains(getErr.Error(), "not found"))
				}
				if isOrphan {
					orphanDir := filepath.Join(logRoot, jobID)
					if rmErr := fileutil.RemoveAll(orphanDir); rmErr != nil && !fileutil.IsNotExist(rmErr) {
						SLog(h.logger).Warn(LogEventSchedulerJobRetentionRemoveOrphanLogDirFailed).
							JobID(job.ID).
							String("orphan_dir", jobID).
							WithError(rmErr).
							Log()
					}
				}
			}
		}
	}

	if totalDeleted > 0 {
		SLog(h.logger).Info(LogEventSchedulerJobRetentionJobCompleted).
			JobID(job.ID).
			Deleted(totalDeleted).
			Log()
		// Flush CAS index so deletes are persisted (same pattern as retention_tolerance).
		if queue := caspkg.GetGlobalListingIndexWriteQueue(); queue != nil {
			if flushErr := queue.FlushKind(objects.KindSchedulerJob, 20*time.Second); flushErr != nil {
				SLog(h.logger).Warn(LogEventSchedulerJobRetentionCASFlushFailed).
					WithFields(jobLogFieldsByIDAndErr(job.ID, flushErr)...).
					Log()
			}
		}
	}

	h.emitProgress("Scheduler job retention complete.")
	return nil
}

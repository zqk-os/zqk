package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

const (
	// TriggerQueueDir is the directory for job trigger queue files (relative to project data dir)
	TriggerQueueDir = paths.SchedulerDir + "/" + paths.SchedulerTriggersDir
	// TriggerQueueFile is the name of the trigger queue file
	TriggerQueueFile = paths.SchedulerTriggersQueueFile

	pipelineKindJobTriggerQueueDequeueTriggerRequests = "job_trigger_queue_dequeue_trigger_requests"
	triggerQueueLockTimeout                           = 5 * time.Second
	triggerQueueRequestedByFmt                        = "pid:%d"
	triggerQueueKeyCount                              = "count"
	triggerQueueKeyJobID                              = "job_id"
	triggerQueueKeyJobIDs                             = "job_ids"
	triggerQueueKeyMessage                            = "message"
	triggerQueueKeyError                              = "error"
	triggerQueueMsgDequeued                           = "Dequeued trigger requests"
	triggerQueueMsgProcessing                         = "Processing trigger request from queue"
	triggerQueueMsgTriggerFailed                      = "Failed to trigger job from queue"
	triggerQueueMsgReloadRetry                        = "Job not in scheduler cache, reloading jobs from storage then retrying trigger"
	triggerQueueErrCreateLockFmt                      = "failed to create file lock: %w"
	triggerQueueErrAcquireLockFmt                     = "failed to acquire lock for queue: %w"
	triggerQueueErrCreateDirFmt                       = "failed to create queue directory: %w"
	triggerQueueErrWriteQueueFmt                      = "failed to write queue: %w"
	triggerQueueEventTypeTriggerFailed                = "trigger_queue_trigger_failed"
)

// JobTriggerRequest represents a request to trigger a job
type JobTriggerRequest struct {
	JobID         string    `json:"job_id"`
	RequestedAt   time.Time `json:"requested_at"`
	RequestedBy   string    `json:"requested_by"`   // Process ID or user identifier
	TriggerOrigin string    `json:"trigger_origin"` // e.g. "pre_commit"; when set, job callbacks run only for this origin
	// ReloadRetries is set when the daemon re-enqueues after "job not found" so we only retry once (avoids infinite loop).
	ReloadRetries int `json:"reload_retries,omitempty"`

	// For lifecycle triggers when the scheduler is down
	IsLifecycleTrigger bool           `json:"is_lifecycle_trigger,omitempty"`
	LifecycleKind      string         `json:"lifecycle_kind,omitempty"`
	LifecycleFrom      string         `json:"lifecycle_from,omitempty"`
	LifecycleTo        string         `json:"lifecycle_to,omitempty"`
	LifecycleData      map[string]any `json:"lifecycle_data,omitempty"`
}

// JobTriggerQueue manages cross-process job trigger requests
// Uses file locking to ensure atomic operations across processes
type JobTriggerQueue struct {
	projectRoot         string
	queueFile           string
	lockFile            string
	mu                  sync.RWMutex
	logger              logging.Logger
	lastCASReconcileAt  time.Time
	casDebounceInterval time.Duration
}

var (
	triggerLockRegistryMu sync.Mutex
	triggerLockRegistry   = make(map[string]*sync.Mutex)
)

func getTriggerQueueMutex(lockFile string) *sync.Mutex {
	triggerLockRegistryMu.Lock()
	defer triggerLockRegistryMu.Unlock()
	m, exists := triggerLockRegistry[lockFile]
	if !exists {
		m = &sync.Mutex{}
		triggerLockRegistry[lockFile] = m
	}
	return m
}

const (
	// defaultCASReconcileDebounceInterval is the maximum rate at which full CAS filesystem scans
	// are performed for trigger batches when all jobs are already in cache.
	defaultCASReconcileDebounceInterval = 15 * time.Second
)

// SetCASDebounceInterval overrides the default CAS reconciliation debounce interval (primarily for tests).
func (q *JobTriggerQueue) SetCASDebounceInterval(d time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.casDebounceInterval = d
}

// shouldReconcileCASForBatch checks if CAS reconciliation should run for this batch.
// Cached batches are debounced (default 15s). A cache miss always scans: CLI one-shots
// retry three times at TriggerQueuePollInterval (500ms), so a 2s floor after daemon-start
// reconcile made `scheduler submit` fail admission (not_in_cache_after_retries) while the
// YAML was already on disk. Invalid IDs still stop after maxTestBundleTriggerRetries.
func (q *JobTriggerQueue) shouldReconcileCASForBatch(hasMissingJob, needsCAS bool) bool {
	if !needsCAS {
		return false
	}
	if hasMissingJob {
		return true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	debounce := q.casDebounceInterval
	if debounce <= 0 {
		debounce = defaultCASReconcileDebounceInterval
	}
	if q.lastCASReconcileAt.IsZero() {
		return true
	}
	return time.Since(q.lastCASReconcileAt) >= debounce
}

func (q *JobTriggerQueue) markCASReconciled() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.lastCASReconcileAt = time.Now()
}

// NewJobTriggerQueue creates a new job trigger queue
// NewJobTriggerQueue creates a new job trigger queue
func NewJobTriggerQueue(projectRoot string) JobTriggerQueueInterface {
	queueDir := filepath.Join(projectRoot, paths.ProjectDataDir, TriggerQueueDir)
	queueFile := filepath.Join(queueDir, TriggerQueueFile)
	lockFile := filepath.Join(queueDir, TriggerQueueFile+".lock")

	return &JobTriggerQueue{
		projectRoot: projectRoot,
		queueFile:   queueFile,
		lockFile:    lockFile,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// EnqueueLifecycleTrigger adds a lifecycle trigger request to the queue.
func (q *JobTriggerQueue) EnqueueLifecycleTrigger(kind, fromState, toState string, objectData map[string]any) error {
	inMemMu := getTriggerQueueMutex(q.lockFile)
	inMemMu.Lock()
	defer inMemMu.Unlock()

	fileLock, err := storagepkg.NewFileLock(q.lockFile)
	if err != nil {
		return errfmt.Errorf(triggerQueueErrCreateLockFmt, err)
	}
	defer fileLock.Close()

	if err := fileLock.LockWithTimeout(triggerQueueLockTimeout); err != nil {
		return errfmt.Errorf(triggerQueueErrAcquireLockFmt, err)
	}
	defer func() { _ = fileLock.Unlock() }()

	queueDir := filepath.Dir(q.queueFile)
	if err := fileutil.MkdirAll(queueDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf(triggerQueueErrCreateDirFmt, err)
	}

	requests, err := q.readQueue()
	if err != nil {
		requests = []JobTriggerRequest{}
	}

	request := JobTriggerRequest{
		RequestedAt:        time.Now(),
		RequestedBy:        fmt.Sprintf(triggerQueueRequestedByFmt, os.Getpid()),
		IsLifecycleTrigger: true,
		LifecycleKind:      kind,
		LifecycleFrom:      fromState,
		LifecycleTo:        toState,
		LifecycleData:      objectData,
	}
	requests = append(requests, request)

	if err := q.writeQueue(requests); err != nil {
		return errfmt.Errorf(triggerQueueErrWriteQueueFmt, err)
	}

	SchedulerTriggerQueueLog(q.logger).Info("lifecycle_trigger_enqueued").
		String("kind", kind).
		String("from", fromState).
		String("to", toState).
		Log()

	return nil
}

// EnqueueTriggerRequest adds a job trigger request to the queue (no trigger origin).
// Uses file locking to ensure atomic operations across processes.
func (q *JobTriggerQueue) EnqueueTriggerRequest(jobID string) error {
	return q.EnqueueTriggerRequestWithOrigin(jobID, "")
}

// EnqueueTriggerRequestWithOrigin adds a job trigger request to the queue with an optional trigger origin.
// When origin is TriggerOriginPreCommit ("pre_commit"), the scheduler will run the job's callback_on_completion
// (and callback_on_error) only for this run. Use for pre-commit triggered runs so timer runs do not write pre-commit results.
func (q *JobTriggerQueue) EnqueueTriggerRequestWithOrigin(jobID, triggerOrigin string) error {
	inMemMu := getTriggerQueueMutex(q.lockFile)
	inMemMu.Lock()
	defer inMemMu.Unlock()

	// Create file lock for cross-process coordination
	fileLock, err := storagepkg.NewFileLock(q.lockFile)
	if err != nil {
		return errfmt.Errorf(triggerQueueErrCreateLockFmt, err)
	}
	defer fileLock.Close()

	// Acquire lock with timeout (5 seconds)
	// This prevents indefinite blocking if another process is stuck
	if err := fileLock.LockWithTimeout(triggerQueueLockTimeout); err != nil {
		return errfmt.Errorf(triggerQueueErrAcquireLockFmt, err)
	}
	defer func() { _ = fileLock.Unlock() }() //nolint:errcheck // Lock cleanup errors are non-critical

	// Create queue directory if it doesn't exist
	queueDir := filepath.Dir(q.queueFile)
	if err := fileutil.MkdirAll(queueDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf(triggerQueueErrCreateDirFmt, err)
	}

	// Read existing queue (while holding lock)
	requests, err := q.readQueue()
	if err != nil {
		// If file doesn't exist, start with empty queue
		requests = []JobTriggerRequest{}
	}

	// Deduplicate: skip if the same job ID (with the same origin) is already pending.
	// Prevents spurious "job not found" events when multiple sources enqueue the same
	// persistent job (e.g. daemon startup + system check both enqueue SCH-cache-prewarm).
	for _, existing := range requests {
		if existing.JobID == jobID && existing.TriggerOrigin == triggerOrigin {
			SchedulerTriggerQueueLog(q.logger).Debug(LogEventSchedulerTriggerQueueSkippedDuplicateEnqueue).
				JobID(jobID).
				String("trigger_origin", triggerOrigin).
				Log()
			return nil
		}
	}

	// Add new request
	request := JobTriggerRequest{
		JobID:         jobID,
		RequestedAt:   time.Now(),
		RequestedBy:   fmt.Sprintf(triggerQueueRequestedByFmt, os.Getpid()),
		TriggerOrigin: triggerOrigin,
	}
	requests = append(requests, request)

	// Write queue back (while holding lock)
	if err := q.writeQueue(requests); err != nil {
		return errfmt.Errorf(triggerQueueErrWriteQueueFmt, err)
	}

	SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueRequestEnqueued).
		JobID(jobID).
		String("queue_file", q.queueFile).
		Int("queue_length", len(requests)).
		Log()

	return nil
}

// EnqueueTriggerRequests appends multiple job trigger requests in a single lock/write.
// Use this from CLI (e.g. zqk test run) to avoid lock contention when enqueueing many jobs.
// triggerOrigin is passed to each request (e.g. "" or TriggerOriginPreCommit).
func (q *JobTriggerQueue) EnqueueTriggerRequests(jobIDs []string, triggerOrigin string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	inMemMu := getTriggerQueueMutex(q.lockFile)
	inMemMu.Lock()
	defer inMemMu.Unlock()

	fileLock, err := storagepkg.NewFileLock(q.lockFile)
	if err != nil {
		return errfmt.Errorf(triggerQueueErrCreateLockFmt, err)
	}
	defer fileLock.Close()

	if err := fileLock.LockWithTimeout(triggerQueueLockTimeout); err != nil {
		return errfmt.Errorf(triggerQueueErrAcquireLockFmt, err)
	}
	defer func() { _ = fileLock.Unlock() }() //nolint:errcheck

	queueDir := filepath.Dir(q.queueFile)
	if err := fileutil.MkdirAll(queueDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf(triggerQueueErrCreateDirFmt, err)
	}

	requests, err := q.readQueue()
	if err != nil {
		requests = []JobTriggerRequest{}
	}

	now := time.Now()
	requestedBy := fmt.Sprintf(triggerQueueRequestedByFmt, os.Getpid())
	for _, jobID := range jobIDs {
		requests = append(requests, JobTriggerRequest{
			JobID:         jobID,
			RequestedAt:   now,
			RequestedBy:   requestedBy,
			TriggerOrigin: triggerOrigin,
		})
	}

	if err := q.writeQueue(requests); err != nil {
		return errfmt.Errorf(triggerQueueErrWriteQueueFmt, err)
	}

	SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueRequestsEnqueuedBatch).
		Int(triggerQueueKeyCount, len(jobIDs)).
		String("queue_file", q.queueFile).
		Int("queue_length", len(requests)).
		Log()

	return nil
}

// EnqueueTriggerRequestStructs appends full trigger request structs (e.g. with ReloadRetries set).
// Used when the daemon re-enqueues after "job not found" so we can pass a one-time retry without looping.
func (q *JobTriggerQueue) EnqueueTriggerRequestStructs(toAppend []JobTriggerRequest) error {
	if len(toAppend) == 0 {
		return nil
	}
	inMemMu := getTriggerQueueMutex(q.lockFile)
	inMemMu.Lock()
	defer inMemMu.Unlock()

	fileLock, err := storagepkg.NewFileLock(q.lockFile)
	if err != nil {
		return errfmt.Errorf(triggerQueueErrCreateLockFmt, err)
	}
	defer fileLock.Close()

	if err := fileLock.LockWithTimeout(triggerQueueLockTimeout); err != nil {
		return errfmt.Errorf(triggerQueueErrAcquireLockFmt, err)
	}
	defer func() { _ = fileLock.Unlock() }() //nolint:errcheck

	queueDir := filepath.Dir(q.queueFile)
	if err := fileutil.MkdirAll(queueDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf(triggerQueueErrCreateDirFmt, err)
	}

	requests, err := q.readQueue()
	if err != nil {
		requests = []JobTriggerRequest{}
	}

	requests = append(requests, toAppend...)
	if err := q.writeQueue(requests); err != nil {
		return errfmt.Errorf(triggerQueueErrWriteQueueFmt, err)
	}
	return nil
}

// PeekTriggerRequests reads pending trigger requests without removing them.
// Best-effort: no file lock is held, so the result may be stale if the daemon is modifying the queue.
// Used by CLI (e.g. scheduler activity) to show "pending in queue" count.
// Returns empty slice with nil error when the queue file does not exist (no triggers enqueued yet).
func (q *JobTriggerQueue) PeekTriggerRequests() ([]JobTriggerRequest, error) {
	if fi, err := fileutil.Stat(q.queueFile); err != nil || fi.Size() <= 4 {
		return []JobTriggerRequest{}, nil
	}
	requests, err := q.readQueue()
	if err != nil {
		if fileutil.IsNotExist(err) {
			return []JobTriggerRequest{}, nil
		}
		return nil, err
	}
	return requests, nil
}

// DequeueTriggerRequests reads and clears all pending trigger requests
// Uses file locking to ensure atomic read-and-clear operation
func (q *JobTriggerQueue) DequeueTriggerRequests(limit int) ([]JobTriggerRequest, error) {
	// Fast path: if the queue file does not exist or contains no requests (size <= 4, e.g. "[]" or empty),
	// skip acquiring the file lock and pipeline overhead completely.
	if fi, err := fileutil.Stat(q.queueFile); err != nil || fi.Size() <= 4 {
		return []JobTriggerRequest{}, nil
	}

	type dequeueState struct {
		fileLock  *storagepkg.FileLock
		inMemMu   *sync.Mutex
		acquired  bool
		requests  []JobTriggerRequest
		remainder []JobTriggerRequest
	}

	pl := pipeline.NewBuilder(pipelineKindJobTriggerQueueDequeueTriggerRequests, q.logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(q.logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			inMemMu := getTriggerQueueMutex(q.lockFile)
			if !inMemMu.TryLock() {
				return &dequeueState{acquired: false, requests: []JobTriggerRequest{}}, nil
			}

			// Create file lock for cross-process coordination
			fileLock, err := storagepkg.NewFileLock(q.lockFile)
			if err != nil {
				inMemMu.Unlock()
				return nil, errfmt.Errorf(triggerQueueErrCreateLockFmt, err)
			}

			// Try to acquire lock (non-blocking) - if another process is enqueueing, skip this cycle
			acquired, err := fileLock.TryLock()
			if err != nil {
				_ = fileLock.Close()
				inMemMu.Unlock()
				return nil, errfmt.Newf("failed to try lock").Wrap(err)
			}
			if !acquired {
				_ = fileLock.Close()
				inMemMu.Unlock()
				// Lock is held by another process (likely enqueueing)
				// Return empty slice - we'll try again next cycle
				return &dequeueState{acquired: false, requests: []JobTriggerRequest{}}, nil
			}

			// Lock acquired: keep it held across subsequent stages.
			return &dequeueState{fileLock: fileLock, inMemMu: inMemMu, acquired: true}, nil
		}).
		AddStage(StageReadQueue, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*dequeueState](payload)
			if !ok {
				return &dequeueState{acquired: false, requests: []JobTriggerRequest{}}, nil
			}
			if !in.acquired || in.fileLock == nil {
				return in, nil
			}

			// Read queue (while holding lock)
			requests, err := q.readQueue()
			if err != nil {
				if fileutil.IsNotExist(err) {
					in.requests = []JobTriggerRequest{}
					return in, nil
				}

				// Fatal error: match original behavior (unlock/close via defer).
				_ = in.fileLock.Unlock()
				_ = in.fileLock.Close()
				in.fileLock = nil
				return nil, err
			}

			if limit > 0 && len(requests) > limit {
				in.requests = requests[:limit]
				in.remainder = requests[limit:]
			} else {
				in.requests = requests
				in.remainder = []JobTriggerRequest{}
			}
			return in, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*dequeueState](payload)
			if !ok {
				return nil, nil
			}
			if !in.acquired || in.fileLock == nil {
				return in, nil
			}

			// If no requests were dequeued, the queue on disk is unchanged; do not touch disk.
			if len(in.requests) == 0 {
				return in, nil
			}

			// Write the remainder back to the queue (while holding lock)
			if err := q.writeQueue(in.remainder); err != nil {
				SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedClearAfterRead).
					WithFields(logErrField(err)...).
					Log()
				// Don't fail - we still return the requests
			}
			return in, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*dequeueState](payload)
			if !ok {
				return &dequeueState{acquired: false, requests: []JobTriggerRequest{}}, nil
			}

			if in.fileLock != nil && in.acquired {
				_ = in.fileLock.Unlock()
				_ = in.fileLock.Close()
			}
			if in.inMemMu != nil {
				in.inMemMu.Unlock()
			}

			if len(in.requests) > 0 {
				jobIDs := extractJobIDs(in.requests)
				SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueDequeuedSummary).
					Int(triggerQueueKeyCount, len(in.requests)).
					String(triggerQueueKeyJobIDs, fmt.Sprintf("%v", jobIDs)).
					Log()
			}

			if in.requests == nil {
				in.requests = []JobTriggerRequest{}
			}
			return in, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	out, runErr := pl.Run(pctx, &dequeueState{})
	if runErr != nil {
		return nil, runErr
	}
	ds, ok := nildecode.DecodeNonNilPayload[*dequeueState](out)
	if !ok {
		return []JobTriggerRequest{}, nil
	}
	return ds.requests, nil
}

// extractJobIDs extracts job IDs from requests for logging
func extractJobIDs(requests []JobTriggerRequest) []string {
	ids := make([]string, len(requests))
	for i, req := range requests {
		ids[i] = req.JobID
	}
	return ids
}

// readQueue reads the trigger queue from disk
func (q *JobTriggerQueue) readQueue() ([]JobTriggerRequest, error) {
	data, err := fileutil.ReadFile(q.queueFile)
	if err != nil {
		return nil, err
	}

	var requests []JobTriggerRequest
	if err := json.Unmarshal(data, &requests); err != nil {
		return nil, errfmt.Newf("failed to parse queue file").Wrap(err)
	}

	return requests, nil
}

// HasPendingTriggerWithOrigin reports whether the on-disk trigger queue contains a pending request for
// jobID with the given trigger origin (cross-process complement to in-memory conflict tracking).
func (q *JobTriggerQueue) HasPendingTriggerWithOrigin(jobID, triggerOrigin string) (bool, error) {
	if q == nil || jobID == "" {
		return false, nil
	}
	inMemMu := getTriggerQueueMutex(q.lockFile)
	inMemMu.Lock()
	defer inMemMu.Unlock()

	fileLock, err := storagepkg.NewFileLock(q.lockFile)
	if err != nil {
		return false, errfmt.Errorf(triggerQueueErrCreateLockFmt, err)
	}
	defer fileLock.Close()

	if err := fileLock.LockWithTimeout(triggerQueueLockTimeout); err != nil {
		return false, errfmt.Errorf(triggerQueueErrAcquireLockFmt, err)
	}
	defer func() { _ = fileLock.Unlock() }()

	requests, err := q.readQueue()
	if err != nil {
		if errors.Is(err, fileutil.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, r := range requests {
		if r.JobID == jobID && r.TriggerOrigin == triggerOrigin {
			return true, nil
		}
	}
	return false, nil
}

// writeQueue writes the trigger queue to disk
func (q *JobTriggerQueue) writeQueue(requests []JobTriggerRequest) error {
	data, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal queue").Wrap(err)
	}

	// Write to temporary file first, then rename (atomic write)
	tmpFile := q.queueFile + ".tmp"
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write queue file").Wrap(err)
	}

	if err := fileutil.Rename(tmpFile, q.queueFile); err != nil {
		return errfmt.Newf("failed to rename queue file").Wrap(err)
	}

	return nil
}

// TriggerQueuePollInterval is how often the daemon checks for new trigger requests (e.g. SCH-AUTOFIX-*, test bundles).
// Shorter interval (500ms) processes enqueued triggers sooner so jobs do not trickle.
const TriggerQueuePollInterval = 500 * time.Millisecond

// watchTriggerQueuePollInterval is the ticker period after WatchTriggerQueue's first immediate drain.
// It defaults to TriggerQueuePollInterval; tests may set it very large to assert the first batch runs
// without waiting for the ticker.
var watchTriggerQueuePollInterval = TriggerQueuePollInterval

// ExcessiveMissingTestBundleThreshold defines when missing SCH-run-* jobs in one dequeue batch
// should emit an explicit "excessive" alert event.
const ExcessiveMissingTestBundleThreshold = 10

// maxTestBundleTriggerRetries is how many times we re-enqueue a missing SCH-run-* trigger
// (after CAS reconcile + reload) before treating the ID as stale. Covers residual races without infinite loops.
const maxTestBundleTriggerRetries = 3

func batchContainsTestBundleJobID(requests []JobTriggerRequest) bool {
	for _, r := range requests {
		if strings.HasPrefix(r.JobID, TestBundleJobIDPrefix) {
			return true
		}
	}
	return false
}

// jobIDLooksLikeCrossProcessTimestampTestRunner matches scripts/test-runner.sh ids (SCH-<unix seconds>).
// Exactly 10 digits: unix seconds through year 2286.
// remove when trigger classification uses category / trigger_origin only (no SCH-<digits> shape).
func jobIDLooksLikeCrossProcessTimestampTestRunner(jobID string) bool {
	return jobIDAllDigitSuffixLen(jobID, 10, 10)
}

// jobIDLooksLikeCLISubmitNanos matches `zqk scheduler submit` ids (SCH-<unix nano>, 16–19 digits).
// Must not overlap the 10-digit test-runner shape.
func jobIDLooksLikeCLISubmitNanos(jobID string) bool {
	return jobIDAllDigitSuffixLen(jobID, 16, 19)
}

func jobIDAllDigitSuffixLen(jobID string, minLen, maxLen int) bool {
	if !strings.HasPrefix(jobID, "SCH-") {
		return false
	}
	suffix := strings.TrimPrefix(jobID, "SCH-")
	if len(suffix) < minLen || len(suffix) > maxLen {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		c := suffix[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func triggerWarrantsCacheMissRetry(r JobTriggerRequest) bool {
	if strings.HasPrefix(r.JobID, TestBundleJobIDPrefix) {
		return true
	}
	if jobIDLooksLikeCASInstanceSchedulerJobID(r.JobID) {
		return true
	}
	if jobIDLooksLikeCrossProcessTimestampTestRunner(r.JobID) {
		return true
	}
	return jobIDLooksLikeCLISubmitNanos(r.JobID) || r.TriggerOrigin == TriggerOriginCLISubmit || r.TriggerOrigin == TriggerOriginPreCommit
}

func triggerIsCLIOneShot(r JobTriggerRequest) bool {
	return r.TriggerOrigin == TriggerOriginCLISubmit || r.TriggerOrigin == TriggerOriginPreCommit || jobIDLooksLikeCLISubmitNanos(r.JobID)
}

// jobIDLooksLikeCASInstanceSchedulerJobID matches ids from zqk object create (e.g. SCH-<timestamp_ns>-<8+ hex>).
// Unlike all-digit SCH-<unix>, these include a hyphen before the content hash; the CAS list index can lag
// cross-process creates until EnsureCASIndexPopulatedFromScan(scheduler_job). See CAS_LIST_GET_CONSISTENCY.md.
func jobIDLooksLikeCASInstanceSchedulerJobID(jobID string) bool {
	prefixes := validation.GetGlobalIDPrefixesConfig().GetPrefixesForKind(objects.KindSchedulerJob)
	rest := ""
	for _, p := range prefixes {
		if strings.HasPrefix(jobID, p) {
			rest = strings.TrimPrefix(jobID, p)
			break
		}
	}
	if rest == emptyValue {
		return false
	}
	i := strings.Index(rest, "-")
	if i <= 0 || i >= len(rest)-1 {
		return false
	}
	tsPart, hashPart := rest[:i], rest[i+1:]
	if len(tsPart) < 10 || len(hashPart) < 8 {
		return false
	}
	for _, c := range tsPart {
		if c < '0' || c > '9' {
			return false
		}
	}
	for _, c := range hashPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// batchNeedsCASReconcileBeforeTriggerReload is true when the batch may include scheduler_job rows
// created by another process before the CAS list index caught up (SCH-run-* leftover jobs, test-runner
// SCH-<timestamp>, SCH-<ns>-<hash> from object create, or SCH-run-test-*). See CAS_LIST_GET_CONSISTENCY.md and ReconcileSchedulerJobCASIndex.
// prioritizeCachePrewarmTriggers moves DefaultCachePrewarmJobID (SCH-cache-prewarm) to the front of a dequeue
// batch so cache_prewarm runs before bulk SCH-run-* triggers in the same poll cycle.
func prioritizeCachePrewarmTriggers(requests []JobTriggerRequest) []JobTriggerRequest {
	if len(requests) <= 1 {
		return requests
	}
	var pref, rest []JobTriggerRequest
	for _, r := range requests {
		if r.JobID == DefaultCachePrewarmJobID {
			pref = append(pref, r)
		} else {
			rest = append(rest, r)
		}
	}
	if len(pref) == 0 {
		return requests
	}
	return append(pref, rest...)
}

func batchNeedsCASReconcileBeforeTriggerReload(requests []JobTriggerRequest) bool {
	if batchContainsTestBundleJobID(requests) {
		return true
	}
	for _, r := range requests {
		if jobIDLooksLikeCrossProcessTimestampTestRunner(r.JobID) {
			return true
		}
		if jobIDLooksLikeCASInstanceSchedulerJobID(r.JobID) {
			return true
		}
		if jobIDLooksLikeCLISubmitNanos(r.JobID) || r.TriggerOrigin == TriggerOriginCLISubmit || r.TriggerOrigin == TriggerOriginPreCommit {
			return true
		}
	}
	return false
}

// drainAndProcessTriggerBatch dequeues one batch from the trigger queue and invokes TriggerJob for each request.
// Shared by WatchTriggerQueue so the first drain runs without waiting for the poll ticker (daemon must enqueue SCH-cache-prewarm before starting this watcher).
func (q *JobTriggerQueue) drainAndProcessTriggerBatch(ctx context.Context, scheduler *Scheduler) {
	// Dequeue and process trigger requests (batch of 5)
	requests, err := q.DequeueTriggerRequests(5)
	if err != nil {
		SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedDequeue).
			WithFields(logErrField(err)...).
			Log()
		return
	}

	var didReloadForBatch bool
	triggeredThisBatch := make(map[string]bool)
	missingTestBundleJobs := make(map[string]bool)

	if len(requests) > 0 {
		requests = prioritizeCachePrewarmTriggers(requests)
		jobIDs := make([]string, len(requests))
		for i, r := range requests {
			jobIDs[i] = r.JobID
		}
		scheduler.EmitTriggerQueueEvent(map[string]any{
			objects.FieldKeyEventType: "trigger_queue_dequeued",
			triggerQueueKeyMessage:    triggerQueueMsgDequeued,
			triggerQueueKeyCount:      len(requests),
			triggerQueueKeyJobIDs:     jobIDs,
		})
		scheduler.RecordTriggerQueueDequeued(len(requests))

		hasMissingJob := false
		for _, r := range requests {
			if !r.IsLifecycleTrigger && !scheduler.JobInCache(r.JobID) {
				hasMissingJob = true
				break
			}
		}

		// Cross-process creates (SCH-run-* leftover jobs, test-runner SCH-<unix> / SCH-run-test-*) can
		// lag the CAS list index; list without reconcile may omit new IDs. Sync scan before reload.
		// Gate full CAS reconciliation behind a debounce interval unless a job is missing from cache.
		needsCAS := batchNeedsCASReconcileBeforeTriggerReload(requests)
		reconciledCAS := false
		if q.shouldReconcileCASForBatch(hasMissingJob, needsCAS) {
			if recErr := scheduler.ReconcileSchedulerJobCASIndex(ctx); recErr != nil {
				SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueCASReconcileBeforeBatchFailed).
					Int("request_count", len(requests)).
					WithError(recErr).
					Log()
				scheduler.EmitTriggerQueueEvent(map[string]any{
					objects.FieldKeyEventType: "trigger_queue_cas_reconcile_failed",
					triggerQueueKeyMessage:    recErr.Error(),
					triggerQueueKeyCount:      len(requests),
				})
			} else {
				q.markCASReconciled()
				reconciledCAS = true
			}
		}
		// Refresh job list if CAS was reconciled or if a job in this batch is not yet cached.
		// When all jobs are already in memory, skip full YAML reload to eliminate 500ms CPU churn.
		if reconciledCAS || hasMissingJob {
			if reloadErr := scheduler.ReloadJobs(ctx); reloadErr != nil {
				SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueReloadBeforeBatchFailed).
					Int("request_count", len(requests)).
					WithError(reloadErr).
					Log()
			} else {
				didReloadForBatch = true
			}
		}
	}

	// Process each request. Trigger each job_id at most once per batch to avoid duplicate heavy work.
	// Use non-blocking submit so the loop does not block when the pool is full; on pool full we re-enqueue
	// the remainder (deduplicated) and continue next poll.
	for i, request := range requests {
		if request.IsLifecycleTrigger {
			_ = scheduler.TriggerJobByLifecycle(ctx, request.LifecycleKind, request.LifecycleFrom, request.LifecycleTo, request.LifecycleData)
			continue
		}

		if triggeredThisBatch[request.JobID] {
			scheduler.EmitTriggerQueueEvent(map[string]any{
				objects.FieldKeyEventType: "trigger_queue_duplicate_skipped",
				"message":                 "Skipped duplicate job_id in batch to avoid running same job twice",
				triggerQueueKeyJobID:      request.JobID,
			})
			SchedulerTriggerQueueLog(q.logger).Debug(LogEventSchedulerTriggerQueueSkippedDuplicateInBatch).
				JobID(request.JobID).
				Log()
			continue
		}

		scheduler.TouchActivity() // idle watchdog: we did meaningful work
		scheduler.EmitTriggerQueueEvent(map[string]any{
			objects.FieldKeyEventType: "trigger_queue_processing",
			triggerQueueKeyMessage:    triggerQueueMsgProcessing,
			triggerQueueKeyJobID:      request.JobID,
			"requested_by":            request.RequestedBy,
			"trigger_origin":          request.TriggerOrigin,
		})
		SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueProcessing).
			JobID(request.JobID).
			String("requested_by", request.RequestedBy).
			String("trigger_origin", request.TriggerOrigin).
			Log()

		triggerCtx := ContextWithSubmitNonBlocking(ctx) // non-blocking so loop does not stall when pool full
		if request.TriggerOrigin != emptyValue {
			triggerCtx = ContextWithTriggerOrigin(triggerCtx, request.TriggerOrigin)
		}

		inCache := scheduler.JobInCache(request.JobID)
		if !inCache && !didReloadForBatch {
			// Reload jobs once per batch so newly created jobs (SCH-AUTOFIX-*, SCH-run-*, etc.) are visible.
			// ReloadJobs repopulates cron from storage so timer jobs are preserved.
			// SCH-run-* must use the same path: previously we set didReloadForBatch without reload, which
			// left the cache stale and dropped bulk test-bundle triggers (CAS index lag after bulk job create).
			scheduler.EmitTriggerQueueEvent(map[string]any{
				objects.FieldKeyEventType: "trigger_queue_reload_retry",
				"message":                 "Job not in scheduler cache, reloading jobs from storage then retrying trigger",
				triggerQueueKeyJobID:      request.JobID,
			})
			scheduler.RecordTriggerQueueReloadRetry(request.JobID)
			SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueReloadRetry).
				JobID(request.JobID).
				Log()
			if reloadErr := scheduler.ReloadJobs(ctx); reloadErr != nil {
				if errors.Is(reloadErr, context.Canceled) || errors.Is(reloadErr, context.DeadlineExceeded) {
					// Daemon is shutting down. Requeue the remainder of the batch to prevent dropped triggers.
					remainder := append([]JobTriggerRequest{request}, requests[i+1:]...)
					if err := q.EnqueueTriggerRequestStructs(remainder); err != nil {
						SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedReenqueueTestBundle).
							WithError(err).
							Log()
					}
					return
				}
				scheduler.EmitTriggerQueueEvent(map[string]any{
					objects.FieldKeyEventType: "trigger_queue_reload_failed",
					triggerQueueKeyMessage:    "Failed to reload jobs before retry; job will not run",
					triggerQueueKeyJobID:      request.JobID,
					triggerQueueKeyError:      reloadErr.Error(),
				})
				scheduler.RecordTriggerQueueReloadFailed(request.JobID, reloadErr)
				scheduler.EmitTriggerQueueEvent(map[string]any{
					objects.FieldKeyEventType: triggerQueueEventTypeTriggerFailed,
					triggerQueueKeyMessage:    triggerQueueMsgTriggerFailed,
					triggerQueueKeyJobID:      request.JobID,
					triggerQueueKeyError:      reloadErr.Error(),
				})
				scheduler.RecordTriggerQueueTriggerFailed(request.JobID, reloadErr)
				SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedReloadBeforeRetry).
					WithFields(jobLogFieldsByIDAndErr(request.JobID, reloadErr)...).
					Log()
			} else {
				didReloadForBatch = true
				inCache = scheduler.JobInCache(request.JobID)
			}
		} else if !inCache && didReloadForBatch {
			scheduler.RecordTriggerQueueReloadRetry(request.JobID)
		}

		if !inCache && scheduler.AdmitJobFromStorage(ctx, request.JobID) {
			inCache = true
		}

		if !inCache {
			errMsg := "job not found: " + request.JobID
			if triggerWarrantsCacheMissRetry(request) {
				// Re-enqueue so transient CAS/list lag after cross-process bulk create does not drop
				// triggers (queue is dequeue-and-clear; without re-queue work is lost).
				if request.ReloadRetries < maxTestBundleTriggerRetries {
					reEnqErr := q.EnqueueTriggerRequestStructs([]JobTriggerRequest{{
						JobID:         request.JobID,
						RequestedAt:   time.Now(),
						RequestedBy:   request.RequestedBy,
						TriggerOrigin: request.TriggerOrigin,
						ReloadRetries: request.ReloadRetries + 1,
					}})
					if reEnqErr == nil {
						scheduler.EmitTriggerQueueEvent(map[string]any{
							objects.FieldKeyEventType: "trigger_queue_test_bundle_requeued",
							"message":                 "Re-queued trigger; job not in cache yet (retry after CAS/reload)",
							triggerQueueKeyJobID:      request.JobID,
							"reload_retries":          request.ReloadRetries + 1,
							"max_retries":             maxTestBundleTriggerRetries,
						})
						SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueRequeuedTestBundleRetry).
							JobID(request.JobID).
							Int("reload_retries", request.ReloadRetries+1).
							Log()
						continue
					}
					SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedReenqueueTestBundle).
						JobID(request.JobID).
						WithError(reEnqErr).
						Log()
				}
				if triggerIsCLIOneShot(request) {
					scheduler.FailJobAdmission(ctx, request.JobID, admissionReasonNotInCache)
				} else {
					missingTestBundleJobs[request.JobID] = true
					SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueueSkippedStaleTestBundle).
						JobID(request.JobID).
						Int("reload_retries", request.ReloadRetries).
						Log()
					continue
				}
			}
			// Known persistent maintenance jobs (cache_prewarm, maintenance, etc.) run on their own timer.
			// When a trigger queue entry for one is found stale (job not in cache after reload), the job
			// was likely already run by its cron schedule or was in-flight when the reload window briefly
			// removed it from s.jobs. Silently drop the entry rather than emitting a trigger_failed event
			// that would appear as a confusing error in logs.
			if isSilentStaleTriggerDrop(request.JobID) {
				SchedulerTriggerQueueLog(q.logger).Debug(LogEventSchedulerTriggerQueueSkippedStaleMaintenance).
					JobID(request.JobID).
					Int("reload_retries", request.ReloadRetries).
					Log()
				continue
			}
			scheduler.EmitTriggerQueueEvent(map[string]any{
				objects.FieldKeyEventType: triggerQueueEventTypeTriggerFailed,
				triggerQueueKeyMessage:    triggerQueueMsgTriggerFailed,
				triggerQueueKeyJobID:      request.JobID,
				triggerQueueKeyError:      errMsg,
			})
			scheduler.RecordTriggerQueueTriggerFailed(request.JobID, errors.New(errMsg))
			SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueSkippedNotInCache).
				JobID(request.JobID).
				Log()
			// Re-enqueue once (ReloadRetries=1) so next poll gets a fresh ReloadJobs and one more chance;
			// handles stream-backed visibility delay. Cap at one retry to avoid infinite loop for missing jobs.
			// SCH-run-* uses separate re-enqueue logic above (maxTestBundleTriggerRetries).
			if didReloadForBatch && request.ReloadRetries < 1 {
				if reEnqErr := q.EnqueueTriggerRequestStructs([]JobTriggerRequest{{
					JobID:         request.JobID,
					RequestedAt:   time.Now(),
					RequestedBy:   request.RequestedBy,
					TriggerOrigin: request.TriggerOrigin,
					ReloadRetries: 1,
				}}); reEnqErr != nil {
					SchedulerTriggerQueueLog(q.logger).Debug(LogEventSchedulerTriggerQueueFailedReenqueueNotFoundRetry).
						JobID(request.JobID).
						WithError(reEnqErr).
						Log()
				}
			}
			continue
		}

		err := scheduler.TriggerJob(triggerCtx, request.JobID)
		if err != nil {
			if errors.Is(err, goroutinelabels.ErrPoolFull) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				// Re-enqueue this request and the rest (deduplicated) so they are not lost; next poll will process them.
				// Deduplicate to avoid re-enqueueing the same job_id multiple times and running heavy work twice.
				seen := make(map[string]bool)
				remaining := make([]JobTriggerRequest, 0, len(requests)-i)
				remainingIDs := make([]string, 0, len(requests)-i)
				for j := i; j < len(requests); j++ {
					req := requests[j]
					if !seen[req.JobID] {
						seen[req.JobID] = true
						remaining = append(remaining, req)
						remainingIDs = append(remainingIDs, req.JobID)
					}
				}
				if reEnqErr := q.EnqueueTriggerRequestStructs(remaining); reEnqErr != nil {
					SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedReenqueuePoolFull).
						Int(triggerQueueKeyCount, len(remaining)).
						WithError(reEnqErr).
						Log()
				}
				eventType := "trigger_queue_pool_full"
				reason := "pool_full"
				if errors.Is(err, context.DeadlineExceeded) {
					eventType = "trigger_queue_deadline_exceeded"
					reason = "deadline_exceeded"
				} else if errors.Is(err, context.Canceled) {
					eventType = "trigger_queue_canceled"
					reason = "canceled"
				}
				scheduler.EmitTriggerQueueEvent(map[string]any{
					objects.FieldKeyEventType: eventType,
					triggerQueueKeyMessage:    "Pool full, deadline exceeded, or canceled; re-enqueued remainder for next poll",
					"re_enqueued":             len(remaining),
					triggerQueueKeyJobIDs:     remainingIDs,
					objects.FieldKeyReason:    reason,
				})
				SchedulerTriggerQueueLog(q.logger).Info(LogEventSchedulerTriggerQueuePoolFullRequeuedForPoll).
					Int(triggerQueueKeyCount, len(remaining)).
					String("reason", reason).
					Log()
				break
			}
			scheduler.EmitTriggerQueueEvent(map[string]any{
				objects.FieldKeyEventType: triggerQueueEventTypeTriggerFailed,
				triggerQueueKeyMessage:    triggerQueueMsgTriggerFailed,
				triggerQueueKeyJobID:      request.JobID,
				triggerQueueKeyError:      err.Error(),
			})
			scheduler.RecordTriggerQueueTriggerFailed(request.JobID, err)
			SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueFailedTriggerFromQueue).
				WithFields(jobLogFieldsByIDAndErr(request.JobID, err)...).
				Log()
		} else {
			triggeredThisBatch[request.JobID] = true
		}
	}
	if len(missingTestBundleJobs) > 0 {
		missingIDs := make([]string, 0, len(missingTestBundleJobs))
		for jobID := range missingTestBundleJobs {
			missingIDs = append(missingIDs, jobID)
		}
		scheduler.EmitTriggerQueueEvent(map[string]any{
			objects.FieldKeyEventType: "trigger_queue_test_bundle_jobs_missing",
			triggerQueueKeyMessage:    "Missing expected test-bundle jobs from trigger queue batch",
			triggerQueueKeyCount:      len(missingIDs),
			triggerQueueKeyJobIDs:     missingIDs,
			"hint":                    paths.RewriteCanonicalCLIInvocations("Run `zqk test discover` then `zqk test run` to refresh kernel test_case objects."),
		})
		if len(missingIDs) >= ExcessiveMissingTestBundleThreshold {
			scheduler.EmitTriggerQueueEvent(map[string]any{
				objects.FieldKeyEventType: "trigger_queue_excessive_missing_test_bundles",
				triggerQueueKeyMessage:    "Excessive missing test-bundle jobs detected in one trigger batch",
				triggerQueueKeyCount:      len(missingIDs),
				"threshold":               ExcessiveMissingTestBundleThreshold,
				triggerQueueKeyJobIDs:     missingIDs,
			})
			SchedulerTriggerQueueLog(q.logger).Warn(LogEventSchedulerTriggerQueueExcessiveMissingTestBundles).
				Int("missing_count", len(missingIDs)).
				Int("threshold", ExcessiveMissingTestBundleThreshold).
				Log()
		}
	}
}

// WatchTriggerQueue watches for new trigger requests and processes them.
func (q *JobTriggerQueue) WatchTriggerQueue(ctx context.Context, scheduler *Scheduler) {
	q.drainAndProcessTriggerBatch(ctx, scheduler)
	ticker := time.NewTicker(watchTriggerQueuePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.drainAndProcessTriggerBatch(ctx, scheduler)
		}
	}
}

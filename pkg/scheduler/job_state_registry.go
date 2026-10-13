package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/observability"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// JobExecutionLease represents a persisted occupancy lease for an active job execution.
type JobExecutionLease struct {
	JobID             string    `json:"job_id" yaml:"job_id"`
	ExecutionID       string    `json:"execution_id" yaml:"execution_id"`
	ProcessID         int       `json:"process_id" yaml:"process_id"`
	AcquiredAt        time.Time `json:"acquired_at" yaml:"acquired_at"`
	ExpiresAt         time.Time `json:"expires_at" yaml:"expires_at"`
	MaxRuntimeSeconds int       `json:"max_runtime_seconds,omitempty" yaml:"max_runtime_seconds,omitempty"`
}

// IsValid checks if the lease is active (not expired at given time).
func (l *JobExecutionLease) IsValid(now time.Time) bool {
	if l == nil {
		return false
	}
	if l.ExpiresAt.IsZero() {
		return true
	}
	return now.Before(l.ExpiresAt)
}

// JobExecutionState is the persisted execution state for a scheduler job.
// The registry is file-based so multiple scheduler processes share a single view.
type JobExecutionState struct {
	JobID       string     `yaml:"job_id"`
	ExecutionID string     `yaml:"execution_id"`
	State       string     `yaml:"state"` // in_progress, completed, failed, deferred, skipped
	ProcessID   int        `yaml:"process_id"`
	StartedAt   time.Time  `yaml:"started_at"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty"`

	PolicyDecision string     `yaml:"policy_decision,omitempty"`
	PolicyReason   string     `yaml:"policy_reason,omitempty"`
	DeferUntil     *time.Time `yaml:"defer_until,omitempty"`

	RetryCount int `yaml:"retry_count,omitempty"`
	MaxRetries int `yaml:"max_retries,omitempty"`

	Lease *JobExecutionLease `yaml:"lease,omitempty"`
}

const (
	jobExecutionStateInProgress = "in_progress"
	jobExecutionStateCompleted  = "completed"
	jobExecutionStateFailed     = "failed"
	jobExecutionStateDeferred   = "deferred"
	jobExecutionStateSkipped    = "skipped"

	// Full-tree retention scan is O(state files). Cap how often CompleteExecution may trigger it.
	defaultStateRetentionFullCleanupMinInterval = 5 * time.Minute
)

// stateRetentionFullCleanupMinInterval is overridable in tests.
var stateRetentionFullCleanupMinInterval = defaultStateRetentionFullCleanupMinInterval

func jobStateDirsForLookup(stateDir, jobID string) []string {
	seg := sanitizeJobIDForPathSegment(jobID)
	b := schedulerStateBucket(jobID)
	return []string{
		filepath.Join(stateDir, b, seg),
		filepath.Join(stateDir, seg),
	}
}

// JobStateRegistry provides distributed job state via file-based persistence.
//
// Storage layout (current):
//
//	{projectRoot}/.zqk/scheduler/state/<bucket>/<job_id_segment>/<execution_id>.yaml
//
// bucket groups SCH-run-bundle-*, SCH-run-cmd-*, SCH-run-pkg-*, SCH-run-data-cell-stream-*, etc.;
// IDs that do not match those families use `_other/` (see schedulerStateBucket).
//
// Legacy (still read for cleanup / migration): no bucket segment
//
//	{projectRoot}/.zqk/scheduler/state/<job_id_segment>/<execution_id>.yaml
//
// Older: flat files
//
//	{projectRoot}/.zqk/scheduler/state/<job_id>-<execution_id>.yaml
//
// See docs/architecture/FILESYSTEM_DATA_LAYOUT.md.
type JobStateRegistry struct {
	stateDir string
	locksDir string

	mu sync.RWMutex

	// jobExecutionStateCache provides an in-memory write-through cache for active and recent job states.
	jobExecutionStateCache map[string]*JobExecutionState
	// knownIdleJobs tracks jobs with no in-progress/deferred executions to eliminate redundant disk scans.
	knownIdleJobs map[string]struct{}

	fileCacheMu    sync.RWMutex
	fileStateCache map[string]cachedFileState

	// retention: how long completed/failed files are retained (best-effort cleanup).
	retention time.Duration

	// recorder emits infrastructure metrics (state retention, etc.); nil uses no-op at emit time.
	recorder observability.Recorder

	// Full-tree retention cleanup is rate-limited + single-flight. Historically CompleteExecution
	// called cleanupCompletedBestEffort on every finish, which ReadFile'd every state YAML under
	// .zqk/scheduler/state (thousands of files) while concurrent job completions ran in parallel.
	// That drove OS thread growth (blocked opens) and multi-GB RSS — showstopper under bulk test-job create.
	//
	cleanupMu           sync.Mutex
	lastFullCleanupAt   time.Time
	fullCleanupInFlight bool
	fullCleanupTestHook func() // optional; tests count scheduled full cleanups
}

type cachedFileState struct {
	modTime time.Time
	size    int64
	state   *JobExecutionState
}

// stateRetentionCleanupStats counts filesystem effects of retention cleanup (measurable operations signal).
type stateRetentionCleanupStats struct {
	RemovedStateFiles int
	RemovedEmptyDirs  int
	RemoveErrors      int
}

// SetObservabilityRecorder wires the same observability pipeline used by scheduler job metrics.
// Typically the DefaultSchedulerMetricsCollector from NewSchedulerWithProjectRoot.
func (r *JobStateRegistry) SetObservabilityRecorder(rec observability.Recorder) {
	if r == nil {
		return
	}
	r.recorder = rec
}

type JobStateSummary struct {
	TotalFiles        int            `json:"total_files" yaml:"total_files"`
	ByState           map[string]int `json:"by_state" yaml:"by_state"`
	StaleInProgress   int            `json:"stale_in_progress" yaml:"stale_in_progress"`
	StaleThresholdSec int            `json:"stale_threshold_sec" yaml:"stale_threshold_sec"`
}

// NewJobStateRegistry creates a registry rooted at:
//
//	{projectRoot}/.zqk/scheduler/state/
//
// NewJobStateRegistry creates a new job state registry
func NewJobStateRegistry(projectRoot string) JobStateRegistryInterface {
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.StateDir)
	locksDir := filepath.Join(stateDir, "locks")
	return &JobStateRegistry{
		stateDir:               stateDir,
		locksDir:               locksDir,
		retention:              7 * 24 * time.Hour,
		jobExecutionStateCache: make(map[string]*JobExecutionState),
		knownIdleJobs:          make(map[string]struct{}),
		fileStateCache:         make(map[string]cachedFileState),
	}
}

func (r *JobStateRegistry) ensureCachesLocked() {
	if r.jobExecutionStateCache == nil {
		r.jobExecutionStateCache = make(map[string]*JobExecutionState)
	}
	if r.knownIdleJobs == nil {
		r.knownIdleJobs = make(map[string]struct{})
	}
}

func (r *JobStateRegistry) updateCacheActiveLocked(jobID string, state *JobExecutionState) {
	r.ensureCachesLocked()
	r.jobExecutionStateCache[jobID] = state
	delete(r.knownIdleJobs, jobID)
}

func (r *JobStateRegistry) updateCacheTerminalLocked(jobID string, state *JobExecutionState) {
	r.ensureCachesLocked()
	r.jobExecutionStateCache[jobID] = state
	r.knownIdleJobs[jobID] = struct{}{}
}

func (r *JobStateRegistry) markIdleLocked(jobID string) {
	r.ensureCachesLocked()
	r.jobExecutionStateCache[jobID] = nil
	r.knownIdleJobs[jobID] = struct{}{}
}

// GetState returns the execution state for a job
func (r *JobStateRegistry) GetState(jobID string) (*JobExecutionState, error) {
	return r.GetExecutionState(jobID)
}

// UpdateState updates the execution state for a job
func (r *JobStateRegistry) UpdateState(jobID string, state *JobExecutionState) error {
	if err := r.writeStateAtomically(state); err != nil {
		return err
	}
	if state != nil {
		id := jobID
		if id == emptyValue {
			id = state.JobID
		}
		if id != emptyValue {
			r.mu.Lock()
			if state.State == jobExecutionStateInProgress || state.State == jobExecutionStateDeferred {
				r.updateCacheActiveLocked(id, state)
			} else {
				r.updateCacheTerminalLocked(id, state)
			}
			r.mu.Unlock()
		}
	}
	return nil
}

// ListStates returns all in-progress states
func (r *JobStateRegistry) ListStates() ([]*JobExecutionState, error) {
	return r.ListInProgress()
}

// RegisterExecution registers the start of a job execution and marks it in_progress.
func (r *JobStateRegistry) RegisterExecution(jobID, executionID string, processID int) error {
	_, err := r.RegisterExecutionWithLease(jobID, executionID, processID, 0)
	return err
}

// RegisterExecutionWithLease registers the start of a job execution, marks it in_progress, and establishes a JobExecutionLease.
func (r *JobStateRegistry) RegisterExecutionWithLease(jobID, executionID string, processID int, maxRuntimeSeconds int) (*JobExecutionLease, error) {
	if strings.TrimSpace(jobID) == emptyValue {
		return nil, errfmt.Errorf("jobID required")
	}
	if strings.TrimSpace(executionID) == emptyValue {
		return nil, errfmt.Errorf("executionID required")
	}

	now := time.Now().UTC()
	runtimeDur := time.Duration(maxRuntimeSeconds) * time.Second
	if runtimeDur <= 0 {
		runtimeDur = 1 * time.Hour
	}
	expiresAt := now.Add(runtimeDur)

	lease := &JobExecutionLease{
		JobID:             jobID,
		ExecutionID:       executionID,
		ProcessID:         processID,
		AcquiredAt:        now,
		ExpiresAt:         expiresAt,
		MaxRuntimeSeconds: maxRuntimeSeconds,
	}

	state := &JobExecutionState{
		JobID:          jobID,
		ExecutionID:    executionID,
		State:          jobExecutionStateInProgress,
		ProcessID:      processID,
		StartedAt:      now,
		PolicyDecision: "execute",
		Lease:          lease,
	}

	lockPath := r.lockPathForJobID(jobID)
	if err := withFileLock(lockPath, 10*time.Second, func() error {
		if err := fileutil.MkdirAll(r.stateDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("create state dir").Wrap(err)
		}
		_ = r.writeStateHintsFile() // best-effort
		if err := r.writeStateAtomically(state); err != nil {
			return err
		}
		r.mu.Lock()
		r.updateCacheActiveLocked(jobID, state)
		r.mu.Unlock()
		return nil
	}); err != nil {
		return nil, err
	}

	return lease, nil
}

// Summarize returns per-state counts and stale in_progress/deferred count.
func (r *JobStateRegistry) Summarize(staleAfter time.Duration) (*JobStateSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := &JobStateSummary{
		ByState:           map[string]int{},
		StaleThresholdSec: int(staleAfter.Seconds()),
	}
	now := time.Now().UTC()
	_ = r.forEachStateYAML(func(path string) error {
		st, ok := r.readCachedJobExecutionStateFile(path)
		if !ok {
			return nil
		}
		out.TotalFiles++
		out.ByState[st.State]++
		if staleAfter > 0 && (st.State == jobExecutionStateInProgress || st.State == jobExecutionStateDeferred) && now.Sub(st.StartedAt) > staleAfter {
			out.StaleInProgress++
		}
		return nil
	})

	return out, nil
}

func readJobExecutionStateFile(path string) (*JobExecutionState, bool) {
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var st JobExecutionState
	if err := yaml.Unmarshal(b, &st); err != nil {
		return nil, false
	}
	return &st, true
}

func cloneJobExecutionState(s *JobExecutionState) *JobExecutionState {
	if s == nil {
		return nil
	}
	cp := *s
	if s.CompletedAt != nil {
		t := *s.CompletedAt
		cp.CompletedAt = &t
	}
	if s.DeferUntil != nil {
		t := *s.DeferUntil
		cp.DeferUntil = &t
	}
	if s.Lease != nil {
		l := *s.Lease
		cp.Lease = &l
	}
	return &cp
}

func (r *JobStateRegistry) getCachedFileState(path string, modTime time.Time, size int64) *JobExecutionState {
	if r == nil {
		return nil
	}
	r.fileCacheMu.RLock()
	defer r.fileCacheMu.RUnlock()
	if r.fileStateCache == nil {
		return nil
	}
	c, ok := r.fileStateCache[path]
	if !ok {
		return nil
	}
	if c.size != size || !c.modTime.Equal(modTime) {
		return nil
	}
	return c.state
}

func (r *JobStateRegistry) setCachedFileState(path string, modTime time.Time, size int64, state *JobExecutionState) {
	if r == nil || state == nil {
		return
	}
	r.fileCacheMu.Lock()
	defer r.fileCacheMu.Unlock()
	if r.fileStateCache == nil {
		r.fileStateCache = make(map[string]cachedFileState)
	}
	r.fileStateCache[path] = cachedFileState{
		modTime: modTime,
		size:    size,
		state:   cloneJobExecutionState(state),
	}
}

func (r *JobStateRegistry) evictCachedFileState(path string) {
	if r == nil {
		return
	}
	r.fileCacheMu.Lock()
	defer r.fileCacheMu.Unlock()
	if r.fileStateCache != nil {
		delete(r.fileStateCache, path)
	}
}

func (r *JobStateRegistry) readCachedJobExecutionStateFile(path string) (*JobExecutionState, bool) {
	fi, err := fileutil.Stat(path)
	if err != nil {
		r.evictCachedFileState(path)
		return nil, false
	}
	modTime := fi.ModTime()
	size := fi.Size()

	if st := r.getCachedFileState(path, modTime, size); st != nil {
		return cloneJobExecutionState(st), true
	}

	st, ok := readJobExecutionStateFile(path)
	if !ok {
		return nil, false
	}

	r.setCachedFileState(path, modTime, size, st)
	return cloneJobExecutionState(st), true
}

func (r *JobStateRegistry) readStateDirEntries() ([]fileutil.DirEntry, error) {
	if r == nil || r.stateDir == emptyValue {
		return nil, nil
	}
	entries, err := fileutil.ReadDir(r.stateDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}

// MigrateLegacyFlatStateFilesBestEffort moves top-level *.yaml files (legacy flat layout) into
// <job_id_segment>/<execution_id>.yaml. Safe to call repeatedly; skips reserved names, nested dirs,
// and files that are not valid JobExecutionState YAML. If the nested target already exists with the
// same job/execution, the flat duplicate is removed.
func (r *JobStateRegistry) MigrateLegacyFlatStateFilesBestEffort() (moved int, err error) {
	entries, err := r.readStateDirEntries()
	if err != nil || len(entries) == 0 {
		return 0, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") || isReservedStateEntry(name) {
			continue
		}
		flatPath := filepath.Join(r.stateDir, name)
		st, ok := r.readCachedJobExecutionStateFile(flatPath)
		if !ok || strings.TrimSpace(st.JobID) == emptyValue || strings.TrimSpace(st.ExecutionID) == emptyValue {
			continue
		}
		dest := r.stateFilePathForJobExecution(st.JobID, st.ExecutionID)
		if filepath.Clean(dest) == filepath.Clean(flatPath) {
			continue
		}
		if err := fileutil.MkdirAll(filepath.Dir(dest), paths.DirPerm755); err != nil {
			continue
		}
		if st2, ok2 := r.readCachedJobExecutionStateFile(dest); ok2 && st2.JobID == st.JobID && st2.ExecutionID == st.ExecutionID {
			_ = fileutil.Remove(flatPath)
			r.evictCachedFileState(flatPath)
			moved++
			continue
		}
		if err := fileutil.Rename(flatPath, dest); err != nil {
			// Cross-volume or other rename failure: copy then remove source.
			if b, readErr := fileutil.ReadFile(flatPath); readErr == nil {
				if err2 := fileutil.WriteFile(dest, b, paths.FilePerm600); err2 == nil {
					_ = fileutil.Remove(flatPath)
					r.evictCachedFileState(flatPath)
					moved++
				}
			}
			continue
		}
		r.evictCachedFileState(flatPath)
		moved++
	}
	return moved, nil
}

// MigrateUnbucketedJobStateDirsBestEffort moves legacy per-job directories from
// state/<job_id_segment>/ to state/<bucket>/<job_id_segment>/ so the top level is bucket folders
// plus locks/. Safe to call repeatedly; skips bucket directories, locks, and paths already in place.
func (r *JobStateRegistry) MigrateUnbucketedJobStateDirsBestEffort() (moved int, err error) {
	entries, err := r.readStateDirEntries()
	if err != nil || len(entries) == 0 {
		return 0, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "locks" || isSchedulerStateBucketDir(name) {
			continue
		}
		src := filepath.Join(r.stateDir, name)
		jobID, jidErr := r.readJobIDFromJobStateDir(src)
		if jidErr != nil || strings.TrimSpace(jobID) == emptyValue {
			continue
		}
		b := schedulerStateBucket(jobID)
		seg := sanitizeJobIDForPathSegment(jobID)
		dst := filepath.Join(r.stateDir, b, seg)
		if filepath.Clean(src) == filepath.Clean(dst) {
			continue
		}
		if err := fileutil.MkdirAll(filepath.Join(r.stateDir, b), paths.DirPerm755); err != nil {
			continue
		}
		if _, statErr := fileutil.Stat(dst); statErr == nil {
			// Merge yaml files into existing destination directory.
			sub, rerr := fileutil.ReadDir(src)
			if rerr != nil {
				continue
			}
			for _, se := range sub {
				if se.IsDir() || !strings.HasSuffix(se.Name(), ".yaml") {
					continue
				}
				from := filepath.Join(src, se.Name())
				to := filepath.Join(dst, se.Name())
				if _, eerr := fileutil.Stat(to); eerr == nil {
					_ = fileutil.Remove(from)
					moved++
					continue
				}
				if err := fileutil.Rename(from, to); err != nil {
					bb, rerr := fileutil.ReadFile(from)
					if rerr != nil {
						continue
					}
					if werr := fileutil.WriteFile(to, bb, paths.FilePerm600); werr == nil {
						_ = fileutil.Remove(from)
						moved++
					}
					continue
				}
				moved++
			}
			_ = fileutil.Remove(src)
			continue
		}
		if err := fileutil.Rename(src, dst); err != nil {
			// Cross-volume or extra files: copy tree
			sub, rerr := fileutil.ReadDir(src)
			if rerr != nil {
				continue
			}
			if err := fileutil.MkdirAll(dst, paths.DirPerm755); err != nil {
				continue
			}
			for _, se := range sub {
				if se.IsDir() || !strings.HasSuffix(se.Name(), ".yaml") {
					continue
				}
				from := filepath.Join(src, se.Name())
				to := filepath.Join(dst, se.Name())
				bb, rerr := fileutil.ReadFile(from)
				if rerr != nil {
					continue
				}
				if werr := fileutil.WriteFile(to, bb, paths.FilePerm600); werr != nil {
					continue
				}
				_ = fileutil.Remove(from)
				moved++
			}
			_ = fileutil.Remove(src)
			continue
		}
		moved++
	}
	return moved, nil
}

func (r *JobStateRegistry) readJobIDFromJobStateDir(dir string) (string, error) {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		st, ok := r.readCachedJobExecutionStateFile(filepath.Join(dir, e.Name()))
		if !ok {
			continue
		}
		if strings.TrimSpace(st.JobID) != emptyValue {
			return st.JobID, nil
		}
	}

	return "", nil
}

func readJobIDFromJobStateDir(dir string) (string, error) {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		st, ok := readJobExecutionStateFile(filepath.Join(dir, e.Name()))
		if !ok {
			continue
		}
		if strings.TrimSpace(st.JobID) != emptyValue {
			return st.JobID, nil
		}
	}

	return "", nil
}

func (r *JobStateRegistry) writeStateHintsFile() error {
	hintsPath := filepath.Join(r.stateDir, "_README.yaml")
	if _, err := fileutil.Stat(hintsPath); err == nil {
		return nil
	}
	content := []byte(
		"purpose: Per-execution scheduler job state files for cross-process visibility.\n" +
			"layout_current: <bucket>/<job_id_segment>/<execution_id>.yaml (bucket = " + schedulerStateBucketREADMEBucketLegend() + ")\n" + paths.RewriteCanonicalCLIInvocations("layout_legacy: <job_id_segment>/<execution_id>.yaml (no bucket; still read — run: zqk scheduler state --migrate)\n") +
			"layout_flat: <job_id>-<execution_id>.yaml (top-level flat; migrate moves into nested layout)\n" +
			"states: [in_progress, completed, failed, deferred, skipped]\n" +
			"notes:\n" +
			"  - in_progress/deferred files may remain if a run is interrupted.\n" +
			"  - completed/failed/skipped files are retained then cleaned best-effort (~7d).\n" +
			"cli_hints:\n" + paths.RewriteCanonicalCLIInvocations("  - zqk scheduler state\n") + paths.RewriteCanonicalCLIInvocations("  - zqk scheduler history --job-id <JOB_ID>\n"),
	)
	return fileutil.WriteFile(hintsPath, content, paths.FilePerm600)
}

// GetExecutionState returns the current persisted state for a job.
// If multiple execution files exist, it returns the first in_progress/deferred match.
func (r *JobStateRegistry) GetExecutionState(jobID string) (*JobExecutionState, error) {
	if strings.TrimSpace(jobID) == emptyValue {
		return nil, errfmt.Errorf("jobID required")
	}

	r.mu.RLock()
	if r.jobExecutionStateCache != nil {
		if st, ok := r.jobExecutionStateCache[jobID]; ok {
			r.mu.RUnlock()
			if st == nil || (st.State != jobExecutionStateInProgress && st.State != jobExecutionStateDeferred) {
				return nil, nil
			}
			return st, nil
		}
	}
	if r.knownIdleJobs != nil {
		if _, idle := r.knownIdleJobs[jobID]; idle {
			r.mu.RUnlock()
			return nil, nil
		}
	}
	r.mu.RUnlock()

	for _, jobDir := range jobStateDirsForLookup(r.stateDir, jobID) {
		if entries, err := fileutil.ReadDir(jobDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
					continue
				}
				st, ok := r.readCachedJobExecutionStateFile(filepath.Join(jobDir, e.Name()))
				if !ok || st.JobID != jobID {
					continue
				}
				switch st.State {
				case jobExecutionStateInProgress, jobExecutionStateDeferred:
					r.mu.Lock()
					r.updateCacheActiveLocked(jobID, st)
					r.mu.Unlock()
					return st, nil
				}
			}
		}
	}

	// Legacy flat layout
	entries, err := r.readStateDirEntries()
	if err != nil || len(entries) == 0 {
		if err == nil {
			r.mu.Lock()
			r.markIdleLocked(jobID)
			r.mu.Unlock()
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") || isReservedStateEntry(e.Name()) {
			continue
		}
		st, ok := r.readCachedJobExecutionStateFile(filepath.Join(r.stateDir, e.Name()))
		if !ok || st.JobID != jobID {
			continue
		}
		switch st.State {
		case jobExecutionStateInProgress, jobExecutionStateDeferred:
			r.mu.Lock()
			r.updateCacheActiveLocked(jobID, st)
			r.mu.Unlock()
			return st, nil
		}
	}

	r.mu.Lock()
	r.markIdleLocked(jobID)
	r.mu.Unlock()

	return nil, nil
}

// GetActiveLease returns the active JobExecutionLease for a job if an in_progress state exists
// and has not expired. Returns nil if no active lease exists.
func (r *JobStateRegistry) GetActiveLease(jobID string) (*JobExecutionLease, error) {
	st, err := r.GetExecutionState(jobID)
	if err != nil {
		return nil, err
	}
	if st == nil || st.State != jobExecutionStateInProgress {
		return nil, nil
	}
	now := time.Now().UTC()
	if st.Lease != nil {
		if st.Lease.IsValid(now) {
			return st.Lease, nil
		}
		return nil, nil
	}
	// Fallback when state was written without explicit lease: check StartedAt against 1 hour default
	if now.Sub(st.StartedAt) < 1*time.Hour {
		return &JobExecutionLease{
			JobID:       st.JobID,
			ExecutionID: st.ExecutionID,
			ProcessID:   st.ProcessID,
			AcquiredAt:  st.StartedAt,
			ExpiresAt:   st.StartedAt.Add(1 * time.Hour),
		}, nil
	}
	return nil, nil
}

// ListInProgress returns all states currently in in_progress or deferred.
func (r *JobStateRegistry) ListInProgress() ([]*JobExecutionState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*JobExecutionState
	_ = r.forEachStateYAML(func(path string) error {
		st, ok := r.readCachedJobExecutionStateFile(path)
		if !ok {
			return nil
		}
		if st.State == jobExecutionStateInProgress || st.State == jobExecutionStateDeferred {
			out = append(out, st)
		}
		return nil
	})
	return out, nil
}

// CompleteExecution marks a matching executionID as completed or failed.
// jobID should be set when known so the state file can be updated in O(1); if empty, a legacy scan is used.
// result is best-effort; it can contain "failed"/"error" to map to failed.
func (r *JobStateRegistry) CompleteExecution(jobID, executionID, result string) error {
	if strings.TrimSpace(executionID) == emptyValue {
		return errfmt.Errorf("executionID required")
	}

	lockPath := r.lockPathForExecutionID(executionID)
	return withFileLock(lockPath, 10*time.Second, func() error {
		var targetPath string
		var target *JobExecutionState

		if strings.TrimSpace(jobID) != emptyValue {
			p := r.stateFilePathForJobExecution(jobID, executionID)
			if st, ok := r.readCachedJobExecutionStateFile(p); ok && st.ExecutionID == executionID {
				targetPath = p
				target = st
			}
		}

		if target == nil {
			var err error
			targetPath, target, err = r.findExecutionStateByExecutionID(executionID)
			if err != nil {
				return err
			}
		}

		if target == nil {
			return nil // best-effort
		}

		now := time.Now().UTC()
		target.CompletedAt = &now
		if target.Lease != nil {
			target.Lease.ExpiresAt = now
		}

		resultLower := strings.ToLower(strings.TrimSpace(result))
		switch {
		case strings.Contains(resultLower, "fail"), strings.Contains(resultLower, "error"):
			target.State = jobExecutionStateFailed
		case strings.Contains(resultLower, "skip"):
			target.State = jobExecutionStateSkipped
		default:
			target.State = jobExecutionStateCompleted
		}

		if err := r.writeStateToPathAtomically(targetPath, target); err != nil {
			return err
		}

		resolvedJobID := target.JobID
		if resolvedJobID == emptyValue {
			resolvedJobID = jobID
		}
		if resolvedJobID != emptyValue {
			r.mu.Lock()
			r.updateCacheTerminalLocked(resolvedJobID, target)
			r.mu.Unlock()
		}

		// Hot path: only scan this job's directory (O(executions for this job)), never the whole tree.
		_ = r.cleanupCompletedInDirBestEffort(filepath.Dir(targetPath))
		// Cold path: occasional full-tree retention, async + single-flight + rate-limited.
		r.scheduleFullRetentionCleanup()
		return nil
	})
}

func (r *JobStateRegistry) findExecutionStateByExecutionID(executionID string) (string, *JobExecutionState, error) {
	var targetPath string
	var target *JobExecutionState
	err := r.forEachStateYAML(func(path string) error {
		if target != nil {
			return nil
		}
		st, ok := r.readCachedJobExecutionStateFile(path)
		if !ok {
			return nil
		}
		if st.ExecutionID == executionID {
			targetPath = path
			target = st
		}
		return nil
	})

	if err != nil {
		return "", nil, err
	}
	return targetPath, target, nil
}

// DeferExecution marks the current job execution as deferred (keeping the same executionID).
func (r *JobStateRegistry) DeferExecution(jobID, reason string, deferUntil *time.Time) error {
	if strings.TrimSpace(jobID) == emptyValue {
		return errfmt.Errorf("jobID required")
	}

	lockPath := r.lockPathForJobID(jobID)
	return withFileLock(lockPath, 10*time.Second, func() error {
		var targetPath string
		var target *JobExecutionState

	outerDeferSearch:
		for _, jobDir := range jobStateDirsForLookup(r.stateDir, jobID) {
			if entries, err := fileutil.ReadDir(jobDir); err == nil {
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
						continue
					}
					path := filepath.Join(jobDir, e.Name())
					st, ok := r.readCachedJobExecutionStateFile(path)
					if !ok || st.JobID != jobID || st.State != jobExecutionStateInProgress {
						continue
					}
					targetPath = path
					target = st
					break outerDeferSearch
				}
			}
		}

		if target == nil {
			entries, err := r.readStateDirEntries()
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") || isReservedStateEntry(e.Name()) {
					continue
				}
				path := filepath.Join(r.stateDir, e.Name())
				st, ok := r.readCachedJobExecutionStateFile(path)
				if !ok || st.JobID != jobID || st.State != jobExecutionStateInProgress {
					continue
				}
				targetPath = path
				target = st
				break
			}
		}

		if target == nil {
			return nil
		}

		target.State = jobExecutionStateDeferred
		target.PolicyDecision = "defer"
		target.PolicyReason = reason
		target.DeferUntil = deferUntil

		if err := r.writeStateToPathAtomically(targetPath, target); err != nil {
			return err
		}

		r.mu.Lock()
		r.updateCacheActiveLocked(jobID, target)
		r.mu.Unlock()

		return nil
	})
}

func (r *JobStateRegistry) writeStateToPathAtomically(targetPath string, state *JobExecutionState) error {
	b, err := yaml.Marshal(state)
	if err != nil {
		return errfmt.Newf("encode job state").Wrap(err)
	}
	tmp := filepath.Join(filepath.Dir(targetPath), fmt.Sprintf(".tmp-%d-%s", time.Now().UnixNano(), filepath.Base(targetPath)))
	if err := fileutil.WriteFile(tmp, b, paths.FilePerm600); err != nil {
		return errfmt.Newf("write temp job state").Wrap(err)
	}
	if err := fileutil.Rename(tmp, targetPath); err != nil {
		_ = fileutil.Remove(tmp)
		return errfmt.Newf("rename job state").Wrap(err)
	}
	if fi, err := fileutil.Stat(targetPath); err == nil {
		r.setCachedFileState(targetPath, fi.ModTime(), fi.Size(), state)
	}
	return nil
}

func (r *JobStateRegistry) writeStateAtomically(state *JobExecutionState) error {
	statePath := r.stateFilePathForJobExecution(state.JobID, state.ExecutionID)
	if err := fileutil.MkdirAll(filepath.Dir(statePath), paths.DirPerm755); err != nil {
		return errfmt.Newf("create job state dir").Wrap(err)
	}
	return r.writeStateToPathAtomically(statePath, state)
}

// scheduleFullRetentionCleanup starts at most one full-tree retention pass, and not more often
// than stateRetentionFullCleanupMinInterval. Safe to call from CompleteExecution (does not block).
func (r *JobStateRegistry) scheduleFullRetentionCleanup() {
	if r == nil || r.retention <= 0 {
		return
	}
	r.cleanupMu.Lock()
	if r.fullCleanupInFlight {
		r.cleanupMu.Unlock()
		return
	}
	if !r.lastFullCleanupAt.IsZero() && time.Since(r.lastFullCleanupAt) < stateRetentionFullCleanupMinInterval {
		r.cleanupMu.Unlock()
		return
	}
	r.fullCleanupInFlight = true
	hook := r.fullCleanupTestHook
	r.cleanupMu.Unlock()

	if hook != nil {
		hook()
	}
	goroutinelabels.StartNamedGoroutine("job-state-registry", "manage job states asynchronously", func() {
		func() {
			defer func() {
				r.cleanupMu.Lock()
				r.fullCleanupInFlight = false
				r.lastFullCleanupAt = time.Now()
				r.cleanupMu.Unlock()
			}()
			_ = r.cleanupCompletedBestEffort()
		}()
	})
}

// cleanupCompletedInDirBestEffort removes expired terminal state files in a single job directory.
// Used on the CompleteExecution hot path so finishing one job never walks thousands of siblings.
func (r *JobStateRegistry) cleanupCompletedInDirBestEffort(jobDir string) error {
	if r.retention <= 0 || strings.TrimSpace(jobDir) == emptyValue {
		return nil
	}
	entries, err := fileutil.ReadDir(jobDir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().UTC().Add(-r.retention)
	var stats stateRetentionCleanupStats
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(jobDir, e.Name())
		r.maybeRemoveExpiredStateFile(path, cutoff, &stats)
	}
	r.recordStateRetentionCleanup(stats)
	return nil
}

func (r *JobStateRegistry) maybeRemoveExpiredStateFile(path string, cutoff time.Time, stats *stateRetentionCleanupStats) {
	st, ok := r.readCachedJobExecutionStateFile(path)
	if !ok {
		return
	}
	if st.State != jobExecutionStateCompleted && st.State != jobExecutionStateFailed && st.State != jobExecutionStateSkipped {
		return
	}

	if st.CompletedAt == nil || !st.CompletedAt.Before(cutoff) {
		return
	}
	if rmErr := fileutil.Remove(path); rmErr != nil {
		stats.RemoveErrors++
		return
	}
	r.evictCachedFileState(path)
	stats.RemovedStateFiles++
	if rmErr := fileutil.Remove(filepath.Dir(path)); rmErr == nil {
		stats.RemovedEmptyDirs++
	}
}

func (r *JobStateRegistry) cleanupCompletedBestEffort() error {
	if r.retention <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-r.retention)
	var stats stateRetentionCleanupStats
	err := r.forEachStateYAML(func(path string) error {
		r.maybeRemoveExpiredStateFile(path, cutoff, &stats)
		return nil
	})
	if err != nil {
		return err
	}
	r.recordStateRetentionCleanup(stats)
	return nil
}

func (r *JobStateRegistry) recordStateRetentionCleanup(stats stateRetentionCleanupStats) {
	rec := r.recorder
	if rec == nil {
		rec = observability.GetNoOpRecorder()
	}
	if !rec.IsEnabled() {
		return
	}
	if stats.RemovedStateFiles == 0 && stats.RemoveErrors == 0 {
		return
	}
	builder := observability.NewBuilder("scheduler_job_state_retention_cleanup").
		WithField("removed_state_files", stats.RemovedStateFiles).
		WithField("removed_empty_dirs", stats.RemovedEmptyDirs).
		WithField("remove_errors", stats.RemoveErrors).
		WithTags("scheduler", "infrastructure", "state_retention")
	_ = rec.Record("scheduler_job_state_retention_cleanup", builder)
}

func (r *JobStateRegistry) stateFilePathForJobExecution(jobID, executionID string) string {
	base := executionID + ".yaml"
	if len(base) > 220 {
		sum := sha256.Sum256([]byte(jobID + "\x00" + executionID))
		base = hex.EncodeToString(sum[:]) + ".yaml"
	}
	return filepath.Join(r.stateDir, schedulerStateBucket(jobID), sanitizeJobIDForPathSegment(jobID), base)
}

func sanitizeJobIDForPathSegment(jobID string) string {
	s := strings.TrimSpace(jobID)
	if s == emptyValue {
		return "_unknown"
	}
	s = strings.ReplaceAll(s, string(fileutil.PathSeparator), "_")
	if runtime.GOOS == "windows" {
		s = strings.ReplaceAll(s, ":", "_")
	}
	if len(s) > 200 {
		sum := sha256.Sum256([]byte(jobID))
		return hex.EncodeToString(sum[:16])
	}
	return s
}

func isReservedStateEntry(name string) bool {
	return name == "_README.yaml" || strings.HasPrefix(name, ".")
}

// forEachStateYAML invokes fn for each job execution state file (bucket layout, legacy nested layout, legacy flat files).
func (r *JobStateRegistry) forEachStateYAML(fn func(path string) error) error {
	entries, err := fileutil.ReadDir(r.stateDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if isReservedStateEntry(name) {
			continue
		}
		if !e.IsDir() {
			if strings.HasSuffix(name, ".yaml") {
				if err := fn(filepath.Join(r.stateDir, name)); err != nil {
					return err
				}
			}
			continue
		}
		if name == "locks" {
			continue
		}
		if isSchedulerStateBucketDir(name) {
			bucketPath := filepath.Join(r.stateDir, name)
			jobDirs, err := fileutil.ReadDir(bucketPath)
			if err != nil {
				continue
			}
			for _, je := range jobDirs {
				if !je.IsDir() {
					continue
				}
				jobPath := filepath.Join(bucketPath, je.Name())
				yfiles, err := fileutil.ReadDir(jobPath)
				if err != nil {
					continue
				}
				for _, y := range yfiles {
					if y.IsDir() || !strings.HasSuffix(y.Name(), ".yaml") {
						continue
					}
					if err := fn(filepath.Join(jobPath, y.Name())); err != nil {
						return err
					}
				}
			}
			continue
		}
		// Legacy layout: job segment directory directly under state/
		jobPath := filepath.Join(r.stateDir, name)
		sub, err := fileutil.ReadDir(jobPath)
		if err != nil {
			continue
		}
		for _, se := range sub {
			if se.IsDir() || !strings.HasSuffix(se.Name(), ".yaml") {
				continue
			}
			if err := fn(filepath.Join(jobPath, se.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *JobStateRegistry) lockPathForJobID(jobID string) string {
	sum := sha256.Sum256([]byte(jobID))
	name := hex.EncodeToString(sum[:]) + ".lock"
	return filepath.Join(r.locksDir, name)
}

func (r *JobStateRegistry) lockPathForExecutionID(executionID string) string {
	sum := sha256.Sum256([]byte(executionID))
	name := hex.EncodeToString(sum[:]) + ".lock"
	return filepath.Join(r.locksDir, name)
}

// withFileLock is a small internal helper for atomic state updates.
func withFileLock(lockPath string, timeout time.Duration, fn func() error) error {
	if err := fileutil.MkdirAll(filepath.Dir(lockPath), paths.DirPerm755); err != nil {
		return errfmt.Newf("create lock dir").Wrap(err)
	}

	fl, err := storagepkg.NewFileLock(lockPath)
	if err != nil {
		return errfmt.Newf("new file lock").Wrap(err)
	}
	defer func() { _ = fl.Close() }()

	if err := fl.LockWithTimeout(timeout); err != nil {
		_ = fl.Close()
		return errfmt.Newf("lock with timeout").Wrap(err)
	}
	defer func() { _ = fl.Unlock() }()

	return fn()
}

// CleanStaleLocks removes orphaned lock files in state/locks/ that are older than the threshold
// and not currently locked by any process.
func (r *JobStateRegistry) CleanStaleLocks(staleAge time.Duration) (int, error) {
	if r == nil || r.locksDir == "" {
		return 0, nil
	}

	entries, err := fileutil.ReadDir(r.locksDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cleaned := 0
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}

		path := filepath.Join(r.locksDir, e.Name())
		info, err := fileutil.Stat(path)
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > staleAge {
			// Try to acquire lock non-blocking to verify it is orphaned/not held
			fl, err := storagepkg.NewFileLock(path)
			if err != nil {
				continue
			}

			acquired, err := fl.TryLock()
			if err == nil && acquired {
				// We hold the lock exclusively. Safe to delete.
				_ = fileutil.Remove(path)
				_ = fl.Unlock()
				_ = fl.Close()
				cleaned++
			} else if fl != nil {
				_ = fl.Close()
			}
		}
	}
	return cleaned, nil
}

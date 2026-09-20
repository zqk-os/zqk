package scheduler

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// testBundlesEventsMu serializes appends to the single test-bundles events.jsonl so multiple jobs don't interleave.
var testBundlesEventsMu sync.Mutex

// AppendTestBundleEvent appends one JSONL event to the shared test-bundles/events.jsonl.
// Caller must ensure entry contains "job_id" so consumers can attribute events. Best-effort; errors are ignored.
func AppendTestBundleEvent(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue || jobID == emptyValue || entry == nil {
		return
	}
	if entry["job_id"] == nil {
		entry["job_id"] = jobID
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	path := TestBundlesEventsFilePath(projectRoot)
	testBundlesEventsMu.Lock()
	defer testBundlesEventsMu.Unlock()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	f, err := fileutil.OpenFile(path, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		return
	}
	_, _ = f.Write(data)
	_, _ = f.WriteString("\n")
	_ = f.Close()
}

// AppendTestBundleHealthEvent appends one line to test-bundles/health.jsonl (outcome, fingerprint, suggested reruns).
// Best-effort; errors ignored. Serialized with the same mutex as events.jsonl to avoid interleaving.
func AppendTestBundleHealthEvent(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue || jobID == emptyValue || entry == nil {
		return
	}
	if entry[KeyJobID] == nil {
		entry[KeyJobID] = jobID
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	path := TestBundlesHealthFilePath(projectRoot)
	testBundlesEventsMu.Lock()
	defer testBundlesEventsMu.Unlock()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	f, err := fileutil.OpenFile(path, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		return
	}
	_, _ = f.Write(data)
	_, _ = f.WriteString("\n")
	_ = f.Close()
}

// AppendTestBundleProgressEvent appends one line to test-bundles/progress.jsonl.
// Best-effort; errors ignored. Serialized with the same mutex as events.jsonl.
func AppendTestBundleProgressEvent(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue || jobID == emptyValue || entry == nil {
		return
	}
	if entry[KeyJobID] == nil {
		entry[KeyJobID] = jobID
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	path := TestBundlesProgressFilePath(projectRoot)
	testBundlesEventsMu.Lock()
	defer testBundlesEventsMu.Unlock()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	f, err := fileutil.OpenFile(path, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		return
	}
	_, _ = f.Write(data)
	_, _ = f.WriteString("\n")
	_ = f.Close()
}

// TrackAndAppendTestBundleProgress tracks the progress of the test bundles and writes tallies to progress.jsonl.
func TrackAndAppendTestBundleProgress(projectRoot, jobID string, eventType string) {
	if projectRoot == emptyValue || jobID == emptyValue {
		return
	}

	if !IsTestBundleJob(jobID) {
		return
	}

	// Load all saved test bundles to identify the known IDs dynamically
	knownIDs := make(map[string]bool)
	bundleStorageDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.TestBundlesDir)
	if entries, err := fileutil.ReadDir(bundleStorageDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				name := entry.Name()[:len(entry.Name())-5] // Remove .json extension
				knownIDs["SCH-run-bundle-"+name] = true
			}
		}
	}

	// Fallback to treating the current job ID as the only known ID if the directory is empty/missing
	if len(knownIDs) == 0 {
		knownIDs[jobID] = true
	}

	// Load existing progress from progress.jsonl
	path := TestBundlesProgressFilePath(projectRoot)

	// Read all previous progress events to reconstruct current state of all jobs
	states := make(map[string]string)
	for k := range knownIDs {
		states[k] = "pending"
	}

	f, err := fileutil.Open(path)
	if err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err == nil {
				jID, _ := m["job_id"].(string)
				eType, _ := m[objects.FieldKeyEventType].(string)
				if jID != "" && eType != "" {
					states[jID] = eType
				}
			}
		}
		_ = f.Close()
	}

	// Update current job state
	newState := "pending"
	if eventType == "started" {
		newState = "running"
	} else if eventType == "passed" || eventType == "pass" || eventType == "completed" {
		newState = "pass"
	} else if eventType == "failed" || eventType == "fail" || eventType == "test_fail" {
		newState = "fail"
	} else if eventType == "skipped" {
		newState = "skipped"
	}
	states[jobID] = newState

	// Compute tallies
	totalCount := len(knownIDs)
	completedCount := 0
	passCount := 0
	failCount := 0
	runningCount := 0
	pendingCount := 0

	for _, s := range states {
		switch s {
		case "pass":
			completedCount++
			passCount++
		case "fail":
			completedCount++
			failCount++
		case "skipped":
			completedCount++
		case "running":
			runningCount++
		case "pending":
			pendingCount++
		}
	}

	// Emit bundle_progress event
	entry := map[string]any{
		KeyTimestamp:      zqktime.NowRFC3339UTC(),
		KeyEventType:      newState,
		"completed_count": completedCount,
		"total_count":     totalCount,
		"pass_count":      passCount,
		"fail_count":      failCount,
		"running_count":   runningCount,
		"pending_count":   pendingCount,
	}

	AppendTestBundleProgressEvent(projectRoot, jobID, entry)
}

// jobLogBufferSize is the size of the in-memory buffer per job (balance I/O efficiency vs memory).
// Flush when buffered data reaches this so we don't bloat memory with many concurrent jobs.
const jobLogBufferSize = 32 * 1024

type jobLogWriter struct {
	file *bufio.Writer
	fd   *fileutil.File
	path string
	mu   sync.Mutex
}

var jobLogRegistry = struct {
	mu sync.Mutex
	m  map[string]*jobLogWriter
}{m: make(map[string]*jobLogWriter)}

func jobLogKey(projectRoot, jobID string) string {
	return projectRoot + "\x00" + jobID
}

// GetOrCreateJobLogWriter returns a buffered writer for the job's events file, creating it if needed.
// Do not use for test-bundle jobs (SCH-run-*); they write to the shared test-bundles/events.jsonl via AppendTestBundleEvent.
// Callers must use WriteJobLogLine and then CloseJobLogWriter when the job ends.
// Safe for concurrent use; each job has one writer.
func GetOrCreateJobLogWriter(projectRoot, jobID string) (*jobLogWriter, error) {
	if IsTestBundleJob(jobID) {
		return nil, nil // test-bundle events go to shared file; no per-job writer
	}
	key := jobLogKey(projectRoot, jobID)
	jobLogRegistry.mu.Lock()
	if w := jobLogRegistry.m[key]; w != nil {
		jobLogRegistry.mu.Unlock()
		return w, nil
	}
	logDir := JobLogDir(projectRoot, jobID)
	eventsPath := JobEventsFilePath(projectRoot, jobID)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		jobLogRegistry.mu.Unlock()
		return nil, err
	}
	f, err := fileutil.OpenFile(eventsPath, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		jobLogRegistry.mu.Unlock()
		return nil, err
	}
	w := &jobLogWriter{
		file: bufio.NewWriterSize(f, jobLogBufferSize),
		fd:   f,
		path: eventsPath,
	}
	jobLogRegistry.m[key] = w
	jobLogRegistry.mu.Unlock()
	return w, nil
}

// WriteLine appends a single JSONL line (caller must pass one line, no newline). Flushes if buffer >= jobLogBufferSize.
func (w *jobLogWriter) WriteLine(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Write(data); err != nil {
		return err
	}
	if err := w.file.WriteByte('\n'); err != nil {
		return err
	}
	// Flush on every write so short-lived jobs and low-volume logs are visible immediately.
	return w.file.Flush()
}

// Close flushes, closes the file, trims the log file if configured, and removes the writer from the registry.
func (w *jobLogWriter) Close(projectRoot string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.file.Flush()
	_ = w.fd.Close()
	maxLines := config.GetMaxJobLogLines(projectRoot)
	trimJobLogFileIfNeeded(w.path, maxLines)
}

// CloseJobLogWriter flushes and closes the buffered writer for the job (if any) and runs trim.
// No-op for test-bundle jobs (they use shared events.jsonl; no per-job writer). Call when a job run ends.
func CloseJobLogWriter(projectRoot, jobID string) {
	if IsTestBundleJob(jobID) {
		return
	}
	key := jobLogKey(projectRoot, jobID)
	jobLogRegistry.mu.Lock()
	w := jobLogRegistry.m[key]
	if w != nil {
		delete(jobLogRegistry.m, key)
		jobLogRegistry.mu.Unlock()
		w.Close(projectRoot)
		return
	}
	jobLogRegistry.mu.Unlock()
}

// CloseAllJobLogWriters flushes and closes all registered job log writers (e.g. on scheduler shutdown).
func CloseAllJobLogWriters(projectRoot string) {
	jobLogRegistry.mu.Lock()
	snapshot := make(map[string]*jobLogWriter, len(jobLogRegistry.m))
	for k, v := range jobLogRegistry.m {
		snapshot[k] = v
	}
	jobLogRegistry.m = make(map[string]*jobLogWriter)
	jobLogRegistry.mu.Unlock()
	for _, w := range snapshot {
		w.Close(projectRoot)
	}
}

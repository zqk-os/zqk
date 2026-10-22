package scheduler

import (
	"bufio"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// testBundlesEventsMu serializes appends to the single test-bundles events.jsonl so multiple jobs don't interleave.
var testBundlesEventsMu sync.Mutex

func appendTestBundleJSONL(path, jobID string, entry map[string]any) {
	if jobID == emptyValue || entry == nil {
		return
	}
	if entry[KeyJobID] == nil && entry["job_id"] == nil {
		entry[KeyJobID] = jobID
	}
	testBundlesEventsMu.Lock()
	defer testBundlesEventsMu.Unlock()
	_ = fileutil.AppendJSONLine(path, entry)
}

// AppendTestBundleEvent appends one JSONL event to the shared test-bundles/events.jsonl.
// Caller must ensure entry contains "job_id" so consumers can attribute events. Best-effort; errors are ignored.
func AppendTestBundleEvent(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue {
		return
	}
	appendTestBundleJSONL(TestBundlesEventsFilePath(projectRoot), jobID, entry)
}

// AppendTestBundleHealthEvent appends one line to test-bundles/health.jsonl (outcome, fingerprint, suggested reruns).
// Best-effort; errors ignored. Serialized with the same mutex as events.jsonl to avoid interleaving.
func AppendTestBundleHealthEvent(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue {
		return
	}
	appendTestBundleJSONL(TestBundlesHealthFilePath(projectRoot), jobID, entry)
}

// AppendTestBundleProgressEvent appends one line to test-bundles/progress.jsonl.
// Best-effort; errors ignored. Serialized with the same mutex as events.jsonl.
func AppendTestBundleProgressEvent(projectRoot, jobID string, entry map[string]any) {
	if projectRoot == emptyValue {
		return
	}
	appendTestBundleJSONL(TestBundlesProgressFilePath(projectRoot), jobID, entry)
}

// TrackAndAppendTestBundleProgress tracks the progress of the test bundles and writes tallies to progress.jsonl.
// Deprecated: test bundles are deprecated. This function is a safe no-op to eliminate full-file scans and GC thrashing.
func TrackAndAppendTestBundleProgress(projectRoot, jobID string, eventType string) {
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
// Do not use for test-bundle jobs (SCH-run-bundle-*); they write to the shared test-bundles/events.jsonl via AppendTestBundleEvent.
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

// CloseAllJobLogWriters flushes and closes registered job log writers.
// When projectRoot is provided, only writers matching that project root are closed and evicted,
// preventing concurrent test runs from closing or evicting each other's active writers.
func CloseAllJobLogWriters(projectRoot string) {
	jobLogRegistry.mu.Lock()
	snapshot := make(map[string]*jobLogWriter)
	if projectRoot == emptyValue {
		for k, v := range jobLogRegistry.m {
			snapshot[k] = v
		}
		jobLogRegistry.m = make(map[string]*jobLogWriter)
	} else {
		prefix := projectRoot + "\x00"
		for k, v := range jobLogRegistry.m {
			if strings.HasPrefix(k, prefix) {
				snapshot[k] = v
				delete(jobLogRegistry.m, k)
			}
		}
	}
	jobLogRegistry.mu.Unlock()
	for _, w := range snapshot {
		w.Close(projectRoot)
	}
}

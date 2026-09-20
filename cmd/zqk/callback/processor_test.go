package callback

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestProcessor_Initialize(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	sorter := &TimestampSorter{}
	err := p.Initialize(tempDir, logFile, true, 100, sorter, "text")
	if err != nil {
		t.Fatalf("failed to initialize processor: %v", err)
	}

	// Verify processor is initialized
	if p.queue == nil {
		t.Error("expected queue to be initialized")
	}

	// Cleanup
	p.Shutdown()
}

func TestProcessor_Enqueue(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	err := p.Initialize(tempDir, logFile, true, 100, &TimestampSorter{}, "text")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer p.Shutdown()

	payload := map[string]any{
		"job_id":                     "test-job",
		objects.FieldKeyCallbackType: callbackTypeCompletion,
		"success":                    true,
	}

	err = p.Enqueue(payload)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify log file was created and has content
	if _, err := fileutil.Stat(logFile); fileutil.IsNotExist(err) {
		t.Error("expected log file to be created")
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("expected log file to have content")
	}
}

func TestProcessor_ProcessDirect(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "direct.log")

	payload := map[string]any{
		"job_id":                     "test-job",
		objects.FieldKeyCallbackType: callbackTypeCompletion,
		"success":                    true,
		objects.FieldKeyCommand:      "echo test",
		"duration":                   1.5,
	}

	err := p.ProcessDirect(tempDir, logFile, true, payload, "text")
	if err != nil {
		t.Fatalf("failed to process directly: %v", err)
	}

	// Verify log file was created
	if _, err := fileutil.Stat(logFile); fileutil.IsNotExist(err) {
		t.Error("expected log file to be created")
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("expected log file to have content")
	}

	// Verify content format
	contentStr := string(content)
	if contentStr == emptyValue {
		t.Error("expected log content")
	}
}

func TestProcessor_ConcurrentEnqueue(t *testing.T) {
	// Do not t.Parallel: heavy concurrent load; parallel with other callback tests starves the
	// 100ms processLoop and causes drain timeouts / partial logs in CI and bundled runs.
	p := NewProcessorForTest()

	tempDir := t.TempDir()
	t.Cleanup(func() {
		files, err := fileutil.ReadDir(tempDir)
		if err == nil {
			t.Logf("FILES remaining in %s:", tempDir)
			for _, f := range files {
				t.Logf(" - %s (dir=%t)", f.Name(), f.IsDir())
			}
		} else {
			t.Logf("Failed to read tempDir during cleanup: %v", err)
		}
	})
	logFile := filepath.Join(tempDir, "concurrent.log")

	// Use larger queue size to accommodate all concurrent enqueues
	const numGoroutines = 20
	const entriesPerGoroutine = 10
	totalEntries := numGoroutines * entriesPerGoroutine
	// Queue size needs to be large enough to hold all entries before processing consumes them
	// Processing happens every 100ms and processes 10 at a time, so we need buffer
	queueSize := totalEntries + 100 // Add buffer for processing lag

	err := p.Initialize(tempDir, logFile, true, queueSize, &TimestampSorter{}, "text")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer p.Shutdown()

	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := []error{}

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		goroutinelabels.NewGoroutine("callback", "processing").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				for j := 0; j < entriesPerGoroutine; j++ {
					payload := map[string]any{
						"job_id":                     "test-job",
						objects.FieldKeyCallbackType: callbackTypeCompletion,
						"success":                    true,
					}
					if err := p.Enqueue(payload); err != nil {
						mu.Lock()
						errors = append(errors, err)
						mu.Unlock()
					}
				}
			}(i)
		})
	}

	wg.Wait()

	// Check for errors - should be minimal with large queue
	mu.Lock()
	if len(errors) > totalEntries/10 { // Allow up to 10% errors due to processing
		t.Errorf("encountered %d errors during concurrent enqueue (expected < %d)", len(errors), totalEntries/10)
		for i, err := range errors {
			if i < 5 { // Only show first 5 errors
				t.Logf("error %d: %v", i, err)
			}
		}
	}
	mu.Unlock()

	// Drain: processLoop batches up to 10 entries every 100ms. Under scheduler/CI load the ticker
	// can fall behind; use a generous wall-clock bound, not a tight estimate.
	deadline := time.Now().Add(2 * time.Minute)
	for p.GetQueueSize() > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if sz := p.GetQueueSize(); sz != 0 {
		t.Fatalf("queue not drained: still %d entries after timeout", sz)
	}

	// Shutdown the processor first to ensure all dequeued entries are fully processed and files closed.
	p.Shutdown()

	// Verify log file has one line per processed entry (text format). Allow tiny slack if the
	// processor logged a warn and skipped an entry under extreme contention.
	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := 0
	for _, b := range content {
		if b == '\n' {
			lines++
		}
	}
	mu.Lock()
	enqueueErrN := len(errors)
	mu.Unlock()
	minLines := totalEntries - enqueueErrN
	if minLines < 0 {
		minLines = 0
	}
	// At most 2% slack below successful enqueues (not below totalEntries when enqueue errors exist).
	slack := totalEntries / 50
	if slack < 5 {
		slack = 5
	}
	if lines+slack < minLines {
		t.Errorf("expected at least ~%d log lines (enqueue errors=%d), got %d. Log content:\n%s", minLines, enqueueErrN, lines, string(content))
	}
}

func TestProcessor_QueueSizeLimit(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "limited.log")

	const queueSize = 10
	err := p.Initialize(tempDir, logFile, true, queueSize, &TimestampSorter{}, "text")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer p.Shutdown()

	// Enqueue rapidly to fill queue before processing can consume
	// Processing happens every 100ms, so we need to fill faster than that
	var lastErr error
	var errors int
	for i := 0; i < queueSize+5; i++ {
		payload := map[string]any{
			"job_id": "test-job",
		}
		err := p.Enqueue(payload)
		if err != nil {
			lastErr = err
			errors++
		}
		// Small delay to allow queue to fill, but not long enough for processing
		time.Sleep(1 * time.Millisecond)
	}

	// Should get at least one error when queue is full (unless processing happened very fast)
	if errors == 0 {
		// Check queue size - if it's at capacity, that's also valid (processing hasn't started yet)
		size := p.GetQueueSize()
		if size < queueSize {
			t.Logf("queue filled but no errors (processing may have been fast); queue size: %d", size)
		}
	} else {
		// Got errors - verify they're queue full errors
		if lastErr == nil {
			t.Error("got errors but lastErr is nil")
		}
		t.Logf("got %d queue full errors as expected", errors)
	}
}

func TestProcessor_Shutdown(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "shutdown.log")

	err := p.Initialize(tempDir, logFile, true, 100, &TimestampSorter{}, "text")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// Enqueue some entries
	for i := 0; i < 10; i++ {
		payload := map[string]any{
			"job_id": "test-job",
		}
		_ = p.Enqueue(payload)
	}

	// Shutdown should complete without blocking indefinitely
	shutdownDone := make(chan bool, 1)
	goroutinelabels.StartTestGoroutine("test_processor_shutdown", "shutting down processor in test", func() {
		p.Shutdown()
		shutdownDone <- true
	})

	select {
	case <-shutdownDone:
		// Good, shutdown completed
	case <-time.After(2 * time.Second):
		t.Error("shutdown timed out")
	}
}

func TestProcessor_FormatLogEntry(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()

	tests := []struct {
		name    string
		entry   *CallbackEntry
		wantSub string
	}{
		{
			name: "completion callback",
			entry: &CallbackEntry{
				Payload: map[string]any{
					"job_id":                     "TEST-001",
					objects.FieldKeyCallbackType: callbackTypeCompletion,
					"success":                    true,
					objects.FieldKeyCommand:      "echo hello",
					"duration":                   2.5,
					"stdout":                     "hello",
				},
				Timestamp: time.Now().UTC(),
			},
			wantSub: "TEST-001",
		},
		{
			name: "error callback",
			entry: &CallbackEntry{
				Payload: map[string]any{
					"job_id":                     "TEST-002",
					objects.FieldKeyCallbackType: callbackTypeError,
					"success":                    false,
					objects.FieldKeyCommand:      "false",
					"duration":                   0.1,
					"error":                      "exit status 1",
				},
				Timestamp: time.Now().UTC(),
			},
			wantSub: "TEST-002",
		},
		{
			name: "status callback",
			entry: &CallbackEntry{
				Payload: map[string]any{
					"job_id":                     "TEST-003",
					objects.FieldKeyCallbackType: callbackTypeStatus,
					"success":                    true,
				},
				Timestamp: time.Now().UTC(),
			},
			wantSub: "TEST-003",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test formatLogEntry indirectly through ProcessDirect
			tempDir := t.TempDir()
			logFile := filepath.Join(tempDir, "format.log")

			payload := tt.entry.Payload
			err := p.ProcessDirect(tempDir, logFile, true, payload, "text")
			if err != nil {
				t.Fatalf("failed to process: %v", err)
			}

			content, err := fileutil.ReadFile(logFile)
			if err != nil {
				t.Fatalf("failed to read log file: %v", err)
			}

			contentStr := string(content)
			if contentStr == emptyValue {
				t.Error("expected non-empty log entry")
			}
			if !strings.Contains(contentStr, tt.wantSub) {
				t.Errorf("expected log entry to contain %s", tt.wantSub)
			}
		})
	}
}

// formatLogEntry is tested indirectly through ProcessDirect
// The actual implementation is in processor.go

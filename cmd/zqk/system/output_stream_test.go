package system

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestOutputStream_AsyncValidation tests that output streams work correctly
// in async validation scenarios (goroutines, buffered channels, etc.)
// Focus on file-based output since that's what we use in production
func TestOutputStream_AsyncValidation(t *testing.T) {
	t.Parallel()
	// Test 1: File-based output (bypasses buffering)
	t.Run("FileBasedOutput", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_output.log")

		f, err := fileutil.OpenFile(tmpFile, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		fmt.Fprintf(f, "TEST: File-based output\n")
		_ = f.Close()

		// Read back
		data, err := fileutil.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		if string(data) != "TEST: File-based output\n" {
			t.Errorf("Expected 'TEST: File-based output\\n', got '%s'", string(data))
		}
	})

	// Test 4: File-based output from goroutine
	t.Run("FileBasedOutputFromGoroutine", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_output_async.log")

		var wg sync.WaitGroup
		goroutinelabels.NewGoroutine("test_file_writer", "writing to file in output stream test").
			WithWaitGroup(&wg).
			StartSimple(func() {
				f, err := fileutil.OpenFile(tmpFile, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
				if err != nil {
					t.Errorf("Failed to create test file: %v", err)
					return
				}
				fmt.Fprintf(f, "TEST: File-based output from goroutine\n")
				_ = f.Close()
			})
		wg.Wait()

		// Give it a moment for file to be written
		time.Sleep(10 * time.Millisecond)

		// Read back
		data, err := fileutil.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		if string(data) != "TEST: File-based output from goroutine\n" {
			t.Errorf("Expected 'TEST: File-based output from goroutine\\n', got '%s'", string(data))
		}
	})

	// Test 5: Multiple goroutines writing to same file
	t.Run("ConcurrentFileWrites", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_output_concurrent.log")

		var wg sync.WaitGroup
		numGoroutines := 10
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			goroutinelabels.NewGoroutine("app", "test helper").StartSimple(func() {
				func(id int) {
					defer wg.Done()
					f, err := fileutil.OpenFile(tmpFile, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
					if err != nil {
						t.Errorf("Failed to create test file: %v", err)
						return
					}
					fmt.Fprintf(f, "TEST: Goroutine %d\n", id)
					_ = f.Close()
				}(i)
			})
		}
		wg.Wait()

		// Give it a moment for all writes to complete
		time.Sleep(50 * time.Millisecond)

		// Read back and verify we got all messages
		data, err := fileutil.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		expectedLines := numGoroutines
		actualLines := bytes.Count(data, []byte("\n"))
		if actualLines != expectedLines {
			t.Errorf("Expected %d lines, got %d. Content: %s", expectedLines, actualLines, string(data))
		}
	})

	// Test 6: Simulate async validation scenario
	t.Run("AsyncValidationScenario", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_async_validation.log")

		// Simulate what happens in async validation:
		// 1. Main goroutine enqueues work
		// 2. Worker goroutines process work
		// 3. Workers write debug output

		workChan := make(chan string, 10)
		var wg sync.WaitGroup

		// Start workers
		numWorkers := 4
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("app", "test worker").StartSimple(func() {
				func(workerID int) {
					defer wg.Done()
					for work := range workChan {
						// Simulate processing work and writing debug output
						f, err := fileutil.OpenFile(tmpFile, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
						if err != nil {
							t.Errorf("Worker %d: Failed to open file: %v", workerID, err)
							continue
						}
						fmt.Fprintf(f, "Worker %d: Processing %s\n", workerID, work)
						_ = f.Close()
					}
				}(i)
			})
		}

		// Enqueue work
		workItems := []string{"AUD-271", "AUD-831", "AUD-832"}
		for _, item := range workItems {
			workChan <- item
		}
		close(workChan)

		// Wait for workers to finish
		wg.Wait()

		// Give it a moment for all writes to complete
		time.Sleep(50 * time.Millisecond)

		// Read back and verify
		data, err := fileutil.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		// Should have 3 lines (one per work item)
		expectedLines := len(workItems)
		actualLines := bytes.Count(data, []byte("\n"))
		if actualLines != expectedLines {
			t.Errorf("Expected %d lines, got %d. Content:\n%s", expectedLines, actualLines, string(data))
		}

		// Verify all work items were processed
		for _, item := range workItems {
			if !bytes.Contains(data, []byte(item)) {
				t.Errorf("Expected to find '%s' in output, but didn't. Content:\n%s", item, string(data))
			}
		}
	})
}

// TestOutputStream_RealScenario tests the actual scenario we're debugging
func TestOutputStream_RealScenario(t *testing.T) {
	t.Parallel()
	// This test simulates the exact scenario where:
	// 1. We check AUD-271
	// 2. Async validator processes it
	// 3. autoFixIssues is called
	// 4. updateHashInRegistryWithInstance is called
	// 5. We need to see debug output from both

	tmpFile := filepath.Join(t.TempDir(), "test_real_scenario.log")

	// Simulate the async validation flow
	workChan := make(chan validationWork, 10)
	var wg sync.WaitGroup

	// Start worker (simulating async validator worker)
	goroutinelabels.NewGoroutine("test_work_processor", "processing work in output stream test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			for work := range workChan {
				// Simulate autoFixIssues being called
				//nolint:gosec // Test code - format string is safe
				debugMsg := fmt.Sprintf("DEBUG autoFixIssues: obj.ID='%s', filePath='%s'\n",
					work.objID, work.filePath)

				// Write to file (simulating our debug file approach)
				f, err := fileutil.OpenFile(tmpFile, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
				if err == nil {
					//nolint:gosec // Test code - format string is safe
					fmt.Fprintf(f, "%s", debugMsg)
					_ = f.Close()
				}

				// Simulate updateHashInRegistryWithInstance being called
				//nolint:gosec // Test code - format string is safe
				debugMsg2 := fmt.Sprintf("DEBUG updateHashInRegistryWithInstance: obj.ID='%s', filePath='%s', filename='%s'\n",
					work.objID, work.filePath, filepath.Base(work.filePath))

				f2, err := fileutil.OpenFile(tmpFile, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
				if err == nil {
					//nolint:gosec // Test code - format string is safe
					fmt.Fprintf(f2, "%s", debugMsg2)
					_ = f2.Close()
				}
			}
		})

	// Enqueue work for AUD-271
	workChan <- validationWork{
		objID:    "AUD-271",
		filePath: "/path/to/AUD-271.yaml",
	}
	close(workChan)

	// Wait for worker to finish
	wg.Wait()

	// Give it a moment for writes to complete
	time.Sleep(50 * time.Millisecond)

	// Read back and verify
	data, err := fileutil.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Should have 2 debug messages
	expectedLines := 2
	actualLines := bytes.Count(data, []byte("\n"))
	if actualLines != expectedLines {
		t.Errorf("Expected %d lines, got %d. Content:\n%s", expectedLines, actualLines, string(data))
	}

	// Verify AUD-271 is in the output
	if !bytes.Contains(data, []byte("AUD-271")) {
		t.Errorf("Expected to find 'AUD-271' in output, but didn't. Content:\n%s", string(data))
	}

	// Verify both debug messages are present
	if !bytes.Contains(data, []byte("autoFixIssues")) {
		t.Errorf("Expected to find 'autoFixIssues' in output, but didn't. Content:\n%s", string(data))
	}
	if !bytes.Contains(data, []byte("updateHashInRegistryWithInstance")) {
		t.Errorf("Expected to find 'updateHashInRegistryWithInstance' in output, but didn't. Content:\n%s", string(data))
	}
}

type validationWork struct {
	objID    string
	filePath string
}

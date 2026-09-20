package validation

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// TestAsyncValidator_OutputQueue_NoDeadlock tests the real-world scenario:
// - Many objects being validated concurrently
// - ResultCallback writing to OutputQueue
// - Progress updates being sent
// - Stop() being called while validation is in progress
// This reproduces the check command scenario with output queue integration
func TestAsyncValidator_OutputQueue_NoDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping heavy output-queue/no-deadlock test in short mode")
	}
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Create output queue and writer (simulating check command setup)
	outputQueue := NewOutputQueue(10000) // Large queue to handle many objects
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	outputWriter := NewOutputWriter(ctx, outputQueue, logger)

	// Register handlers
	stdoutBuf := &bytes.Buffer{}
	stdoutHandler := NewWriterOutputHandler(stdoutBuf, false)
	outputWriter.RegisterHandler("stdout", stdoutHandler)

	// Start output writer
	outputWriter.Start()
	defer func() { _ = outputWriter.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Create validator with realistic worker count
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second)

	// Set up validation function (simulating real validation)
	validationFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		// Simple validation - just create a state
		return &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Checksum:      "test-checksum",
			Issues:        []ValidationIssue{},
		}, nil
	})
	validator.SetValidationFunc(validationFunc)

	// Set up ResultCallback (simulating setupResultCallback from async_check.go)
	var resultCount atomic.Int64
	callback := func(state *ValidationState, err error) {
		if err != nil {
			return
		}
		// Write result to output queue (non-blocking)
		packet := OutputPacket{
			ChannelID: "stdout",
			Data:      []byte("result\n"),
		}
		_ = outputQueue.Enqueue(packet) //nolint:errcheck // Test helper - error handling not critical
		resultCount.Add(1)
	}
	validator.SetResultCallback(callback)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Create test files and enqueue many objects (simulating 15k objects, but use smaller number for test speed)
	numObjects := 1000
	testFiles := make([]string, numObjects)
	for i := 0; i < numObjects; i++ {
		testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), fmt.Sprintf("TEST-%d.yaml", i))
		if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
			t.Fatalf(ConstMagic32b1c202, err)
		}
		if err := fileutil.WriteFile(testFile, []byte(fmt.Sprintf(ConstMagicdb620444, i)), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
		testFiles[i] = testFile
		_ = validator.Enqueue(
			fmt.Sprintf("TEST-%d", i),
			"test_object",
			testFile,
			1,
		)
	}

	// Wait a bit for processing to start
	time.Sleep(200 * time.Millisecond)

	// Monitor progress channel (simulating drain goroutine)
	progressChan := validator.GetProgress()
	validatorCtx := validator.GetContext() // Get context to detect Stop()
	var progressCount atomic.Int64
	var wg sync.WaitGroup

	// Start progress monitor
	// Use validator context to detect when Stop() is called
	goroutinelabels.NewGoroutine(ConstMagic37ac5037, ConstMagicd8aef27a).
		WithWaitGroup(&wg).
		StartSimple(func() {
			for {
				select {
				case progress, ok := <-progressChan:
					if !ok {
						// Channel closed - exit immediately
						return
					}
					progressCount.Add(1)
					_ = progress
				case <-validatorCtx.Done():
					// Validator context cancelled (Stop() was called) - exit
					// Channel should be closed soon, but don't wait for it
					return
				}
			}
		})

	// Wait a bit more for validation to progress
	time.Sleep(500 * time.Millisecond)

	// Stop validator while validation is in progress (this is the critical test)
	stopStart := time.Now()
	stopErr := validator.Stop()
	stopDuration := time.Since(stopStart)

	if stopErr != nil {
		t.Errorf(ConstMagic1a06a983, stopErr)
	}

	// Stop should complete within timeout
	maxExpectedDuration := 4 * time.Second
	if stopDuration > maxExpectedDuration {
		t.Errorf(ConstMagic929ff366, stopDuration)
	} else {
		t.Logf(ConstMagic2287369e, stopDuration, maxExpectedDuration)
	}

	// Wait for progress monitor to finish
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("validation_test", ConstMagic7250d181).StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		t.Logf(ConstMagicdbcf88b8, progressCount.Load())
	case <-time.After(2 * time.Second):
		t.Fatal(ConstMagicde739308)
	}

	// Check results
	finalResultCount := resultCount.Load()
	t.Logf("Results written: %d/%d", finalResultCount, numObjects)
}

// TestAsyncValidator_OutputQueue_FullQueue_NoDeadlock tests that a full output queue
// doesn't cause deadlocks. When the queue is full, writes should be non-blocking.
func TestAsyncValidator_OutputQueue_FullQueue_NoDeadlock(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Create small output queue to force it to fill up
	outputQueue := NewOutputQueue(10) // Very small queue
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	outputWriter := NewOutputWriter(ctx, outputQueue, logger)

	// Register handlers
	stdoutBuf := &bytes.Buffer{}
	stdoutHandler := NewWriterOutputHandler(stdoutBuf, false)
	outputWriter.RegisterHandler("stdout", stdoutHandler)

	// Start output writer
	outputWriter.Start()
	defer func() { _ = outputWriter.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Create validator
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second)

	// Set up validation function
	validationFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		return &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Checksum:      "test-checksum",
			Issues:        []ValidationIssue{},
		}, nil
	})
	validator.SetValidationFunc(validationFunc)

	// Set up ResultCallback that writes to queue
	var droppedCount atomic.Int64
	callback := func(state *ValidationState, err error) {
		if err != nil {
			return
		}
		// Try to write to queue (should be non-blocking)
		packet := OutputPacket{
			ChannelID: "stdout",
			Data:      []byte("result\n"),
		}
		if err := outputQueue.Enqueue(packet); err != nil {
			// Queue full - this is expected and OK
			droppedCount.Add(1)
		}
	}
	validator.SetResultCallback(callback)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Create test files and enqueue many objects to fill the queue
	numObjects := 100
	for i := 0; i < numObjects; i++ {
		testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), fmt.Sprintf("TEST-%d.yaml", i))
		if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
			t.Fatalf(ConstMagic32b1c202, err)
		}
		if err := fileutil.WriteFile(testFile, []byte(fmt.Sprintf(ConstMagicdb620444, i)), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
		_ = validator.Enqueue(
			fmt.Sprintf("TEST-%d", i),
			"test_object",
			testFile,
			1,
		)
	}

	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)

	// Stop should not deadlock even with full queue
	stopStart := time.Now()
	stopErr := validator.Stop()
	stopDuration := time.Since(stopStart)

	if stopErr != nil {
		t.Errorf(ConstMagic1a06a983, stopErr)
	}

	// Stop should complete quickly
	maxExpectedDuration := 4 * time.Second
	if stopDuration > maxExpectedDuration {
		t.Errorf(ConstMagic3adab2c0, stopDuration)
	} else {
		t.Logf(ConstMagic2287369e, stopDuration, maxExpectedDuration)
	}

	// Some results may have been dropped (expected with small queue)
	t.Logf(ConstMagicd57cf154, droppedCount.Load())
}

// TestAsyncValidator_OutputQueue_ConcurrentStop_NoDeadlock tests that calling Stop()
// concurrently with active validation and output writing doesn't deadlock
func TestAsyncValidator_OutputQueue_ConcurrentStop_NoDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip(ConstMagica31f9f1a)
	}
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Create output queue
	outputQueue := NewOutputQueue(1000)
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	outputWriter := NewOutputWriter(ctx, outputQueue, logger)

	// Register handlers
	stdoutBuf := &bytes.Buffer{}
	stdoutHandler := NewWriterOutputHandler(stdoutBuf, false)
	outputWriter.RegisterHandler("stdout", stdoutHandler)

	// Start output writer
	outputWriter.Start()
	defer func() { _ = outputWriter.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Create validator
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second)

	// Set up validation function
	validationFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		return &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Checksum:      "test-checksum",
			Issues:        []ValidationIssue{},
		}, nil
	})
	validator.SetValidationFunc(validationFunc)

	// Set up ResultCallback
	callback := func(state *ValidationState, err error) {
		if err != nil {
			return
		}
		packet := OutputPacket{
			ChannelID: "stdout",
			Data:      []byte("result\n"),
		}
		_ = outputQueue.Enqueue(packet) //nolint:errcheck // Test helper - error handling not critical
	}
	validator.SetResultCallback(callback)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Create test files and enqueue many objects
	numObjects := 500
	for i := 0; i < numObjects; i++ {
		testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), fmt.Sprintf("TEST-%d.yaml", i))
		if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
			t.Fatalf(ConstMagic32b1c202, err)
		}
		if err := fileutil.WriteFile(testFile, []byte(fmt.Sprintf(ConstMagicdb620444, i)), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
		_ = validator.Enqueue(
			fmt.Sprintf("TEST-%d", i),
			"test_object",
			testFile,
			1,
		)
	}

	// Wait for processing to start
	time.Sleep(200 * time.Millisecond)

	// Call Stop() from multiple goroutines concurrently
	var wg sync.WaitGroup
	stopDone := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("validation_test", ConstMagic981fe807).StartSimple(func() {
			defer wg.Done()
			err := validator.Stop()
			stopDone <- err
		})
	}

	// Wait for all Stop() calls with timeout
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("validation_test", ConstMagic05c10955).StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		t.Log(ConstMagic0d04aa65)
	case <-time.After(5 * time.Second):
		t.Fatal(ConstMagic952f39f9)
	}

	// Check results
	close(stopDone)
	stopCount := 0
	for err := range stopDone {
		if err != nil {
			t.Errorf(ConstMagic1a06a983, err)
		}
		stopCount++
	}
	if stopCount != 3 {
		t.Errorf(ConstMagicaac9a31e, stopCount)
	}
}

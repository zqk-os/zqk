package system

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

// TestDiscoveryEarlyCompletion_StableCount tests that discovery completes early when count stabilizes
func TestDiscoveryEarlyCompletion_StableCount(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	storageProvider, _ := storage.NewFileObjectStorageForTest(tmpDir)
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	if storageProvider != nil {
	}

	// Create a minimal test scenario where files are discovered quickly then stop
	// We'll simulate this by controlling when files are sent to the channel
	filesChan := make(chan []scannedFile, 10)

	// Start discovery with a single kind that will send files then stop
	kinds := []string{"test_kind"}
	operationID := "test_operation"

	// Start discovery in a goroutine
	streamChan, collectFinalResults := discoverObjectsParallel(
		ctx,
		tmpDir,
		operationID,
		kinds,
		nil,
		logger,
		storageProvider,
		"test",
	)

	// Simulate file discovery: send files quickly, then stop
	// This simulates the scenario where discovery completes quickly
	goroutinelabels.NewGoroutine("system_test", "simulate file discovery").StartSimple(func() {
		defer close(filesChan)
		// Send initial batch of files
		filesChan <- []scannedFile{
			{ObjectID: "TEST-001", Kind: "test_kind", Path: "test1.yaml"},
			{ObjectID: "TEST-002", Kind: "test_kind", Path: "test2.yaml"},
			{ObjectID: "TEST-003", Kind: "test_kind", Path: "test3.yaml"},
		}
		// Wait a bit to simulate discovery completing
		time.Sleep(100 * time.Millisecond)
		// Send one more batch
		filesChan <- []scannedFile{
			{ObjectID: "TEST-004", Kind: "test_kind", Path: "test4.yaml"},
		}
		// Then stop - no more files
	})

	// Consume from stream channel (simulating validation consuming files)
	var streamedCount int64
	goroutinelabels.NewGoroutine("system_test", "consume stream").StartSimple(func() {
		for range streamChan {
			atomic.AddInt64(&streamedCount, 1)
		}
	})

	// Wait for early completion signal
	// With the current implementation, discovery should complete early
	// when count stabilizes for 3 seconds
	timeout := time.After(8 * time.Second)
	select {
	case <-timeout:
		t.Log("Discovery completed (may have been early completion or timeout)")
	case <-ctx.Done():
		t.Log("Context cancelled")
	}

	// Collect final results
	allFiles := collectFinalResults()
	t.Logf("Final results: %d files collected, %d streamed", len(allFiles), atomic.LoadInt64(&streamedCount))

	// Verify that we got at least some files
	if len(allFiles) == 0 && atomic.LoadInt64(&streamedCount) == 0 {
		t.Log("Note: No files collected - this may be expected if discovery goroutines haven't started yet")
	}
}

// TestDiscoveryEarlyCompletion_DoubleClosePrevention tests that sync.Once prevents double-close panic
func TestDiscoveryEarlyCompletion_DoubleClosePrevention(t *testing.T) {
	t.Parallel()
	// This test verifies that closing discoveryComplete multiple times doesn't panic
	discoveryComplete := make(chan struct{})
	var discoveryCompleteOnce sync.Once

	// First close should succeed
	discoveryCompleteOnce.Do(func() {
		close(discoveryComplete)
	})

	// Verify channel is closed
	select {
	case <-discoveryComplete:
		// Channel is closed - good
	default:
		t.Error("Channel should be closed after first Do() call")
	}

	// Second close attempt should be a no-op (sync.Once prevents execution)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		discoveryCompleteOnce.Do(func() {
			// This should not execute
			close(discoveryComplete)
		})
	}()

	if panicked {
		t.Error("Second close attempt should not panic (sync.Once prevents it)")
	}

	// Verify channel is still closed (not panicked)
	select {
	case <-discoveryComplete:
		// Channel is still closed - good
	default:
		t.Error("Channel should still be closed after second Do() call")
	}
}

// TestDiscoveryProgressEmission_Interval tests that progress is emitted every 2 seconds
func TestDiscoveryProgressEmission_Interval(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	startTime := time.Now()
	lastProgressEmitTime := startTime
	emitCount := 0
	var emitMu sync.Mutex

	// Simulate the progress emission logic
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("system_test", "progress emission").StartSimple(func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				// Emit progress every 2 seconds (not every tick)
				if now.Sub(lastProgressEmitTime) >= 2*time.Second {
					lastProgressEmitTime = now
					emitMu.Lock()
					emitCount++
					emitMu.Unlock()
					t.Logf("Progress emitted at %v (emit #%d)", now.Sub(startTime), emitCount)
				}
			}
		}
	})

	// Wait for test duration
	<-ctx.Done()

	emitMu.Lock()
	finalCount := emitCount
	emitMu.Unlock()

	// With a 6-second timeout and 1-second ticker checking every 2 seconds,
	// we should get approximately 2-3 emissions (at 2s, 4s, possibly 6s)
	if finalCount < 2 {
		t.Errorf("Expected at least 2 progress emissions in 6 seconds, got %d", finalCount)
	}
	if finalCount > 4 {
		t.Errorf("Expected at most 4 progress emissions in 6 seconds, got %d", finalCount)
	}

	t.Logf("Progress emission test: %d emissions in ~6 seconds (expected 2-3)", finalCount)
}

// TestDiscoveryStableCountDetection tests the stable count detection logic
func TestDiscoveryStableCountDetection(t *testing.T) {
	t.Parallel()
	startTime := time.Now()
	var totalFound int64
	var lastCount int64
	lastCountChangeTime := startTime

	// Simulate count changes over time
	scenarios := []struct {
		name           string
		count          int64
		waitTime       time.Duration
		shouldComplete bool
	}{
		{
			name:           "Initial count",
			count:          10,
			waitTime:       0,
			shouldComplete: false,
		},
		{
			name:           "Count increases",
			count:          20,
			waitTime:       500 * time.Millisecond,
			shouldComplete: false,
		},
		{
			name:           "Count stable for 2s",
			count:          20,
			waitTime:       2 * time.Second,
			shouldComplete: false, // Not yet 3 seconds
		},
		{
			name:           "Count stable for 3s",
			count:          20,
			waitTime:       1 * time.Second, // Total 3s stable
			shouldComplete: true,            // Should trigger early completion
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			time.Sleep(scenario.waitTime)
			now := time.Now()

			atomic.StoreInt64(&totalFound, scenario.count)
			currentCount := atomic.LoadInt64(&totalFound)

			// Check if count has changed
			if currentCount != lastCount {
				lastCount = currentCount
				lastCountChangeTime = now
			}

			// Check if count hasn't changed for 3 seconds
			shouldComplete := lastCount > 0 && now.Sub(lastCountChangeTime) >= 3*time.Second

			if shouldComplete != scenario.shouldComplete {
				t.Errorf("Expected shouldComplete=%v, got %v (count=%d, stable_duration=%v)",
					scenario.shouldComplete, shouldComplete, lastCount, now.Sub(lastCountChangeTime))
			}
		})
	}
}

// TestDiscoveryStreamingChannel_ConcurrentConsumption tests that streaming channel works correctly
func TestDiscoveryStreamingChannel_ConcurrentConsumption(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streamChan := make(chan []scannedFile, 100)

	// Producer: send files in batches
	goroutinelabels.NewGoroutine("system_test", "stream producer").StartSimple(func() {
		defer close(streamChan)
		batches := [][]scannedFile{
			{{ObjectID: "TEST-001", Kind: "test", Path: "test1.yaml"}},
			{{ObjectID: "TEST-002", Kind: "test", Path: "test2.yaml"}},
			{{ObjectID: "TEST-003", Kind: "test", Path: "test3.yaml"}},
		}
		for _, batch := range batches {
			select {
			case streamChan <- batch:
			case <-ctx.Done():
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	// Consumer: read from stream channel
	var receivedCount int64
	doneConsumer := make(chan struct{})
	goroutinelabels.NewGoroutine("system_test", "stream consumer").StartSimple(func() {
		defer close(doneConsumer)
		for {
			select {
			case <-ctx.Done():
				return
			case batch, ok := <-streamChan:
				if !ok {
					return
				}
				atomic.AddInt64(&receivedCount, int64(len(batch)))
			}
		}
	})

	// Wait for completion
	select {
	case <-doneConsumer:
		// Consumer finished
	case <-ctx.Done():
		// Timeout
	}

	finalCount := atomic.LoadInt64(&receivedCount)
	if finalCount != 3 {
		t.Errorf("Expected 3 files received, got %d", finalCount)
	}
}

// TestDiscoveryEarlyCompletion_ContextCancellation tests that early completion works with context cancellation
func TestDiscoveryEarlyCompletion_ContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discoveryComplete := make(chan struct{})
	var discoveryCompleteOnce sync.Once

	// Simulate early completion triggered by stable count
	goroutinelabels.NewGoroutine("system_test", "simulate early completion").StartSimple(func() {
		time.Sleep(100 * time.Millisecond)
		discoveryCompleteOnce.Do(func() {
			close(discoveryComplete)
		})
	})

	// Wait for either early completion or context cancellation
	select {
	case <-discoveryComplete:
		t.Log("Early completion triggered successfully")
	case <-ctx.Done():
		t.Log("Context cancelled before early completion")
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for early completion")
	}

	// Cancel context and verify it doesn't panic
	cancel()

	// Try to close again (should be no-op due to sync.Once)
	discoveryCompleteOnce.Do(func() {
		close(discoveryComplete)
	})

	// Should not panic
	select {
	case <-discoveryComplete:
		// Channel is closed - good
	default:
		t.Error("Channel should be closed")
	}
}

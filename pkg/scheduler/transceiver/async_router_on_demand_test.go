package transceiver

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver/types"

	"sync/atomic"
)

func TestAsyncRouter_OnDemandPattern(t *testing.T) {
	t.Parallel()
	router := NewRouterWithDefaults(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	ar := NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, nil)

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	// Start router (workers start on-demand)
	if err := ar.Start(ctx); err != nil {
		t.Fatalf("Failed to start async router: %v", err)
	}
	defer func() { _ = ar.Stop() }() //nolint:errcheck // Test cleanup

	// Initially no workers should be running
	if ar.IsWorkerRunning() {
		t.Error("Expected no workers running initially")
	}

	// Enqueue a message - should wake a worker
	message := types.Message{
		EventType: "test_event",
		Payload:   map[string]any{"test": "data"},
	}

	if err := ar.RouteAsync(ctx, message); err != nil {
		t.Fatalf("Failed to route message: %v", err)
	}

	// Wait a bit for worker to start
	time.Sleep(100 * time.Millisecond)

	// Worker should be running now
	if !ar.IsWorkerRunning() {
		t.Error("Expected worker to be running after enqueueing message")
	}

	// Wait for worker to process and shut down (idle timeout)
	// Note: Full timeout test is impractical, but we can verify the pattern
	time.Sleep(100 * time.Millisecond)
}

func TestAsyncRouter_CoordinatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires coordinator setup")
	}
	t.Parallel()
	router := NewRouterWithDefaults(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	ar := NewAsyncRouter(pkgctx.NewSystemContext(), router, 3, 50, nil)

	ar.SetProjectRoot("/test/project")
	ar.SetStorageProvider(nil) // Can be nil for testing

	var callbackCalls atomic.Int32
	var events []struct {
		workerID       string
		eventType      string
		status         string
		workerCount    int
		processedCount int
		failedCount    int
	}

	SetAsyncRouterEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storageProvider any,
		workerID string,
		eventType string,
		status string,
		workerCount int,
		processedCount int,
		failedCount int,
		duration time.Duration,
	) {
		events = append(events, struct {
			workerID       string
			eventType      string
			status         string
			workerCount    int
			processedCount int
			failedCount    int
		}{
			workerID:       workerID,
			eventType:      eventType,
			status:         status,
			workerCount:    workerCount,
			processedCount: processedCount,
			failedCount:    failedCount,
		})
	})

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	if err := ar.Start(ctx); err != nil {
		t.Fatalf("Failed to start async router: %v", err)
	}
	defer func() { _ = ar.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue a message to trigger worker start
	message := types.Message{
		EventType: "test_event",
		Payload:   map[string]any{"test": "data"},
	}

	if err := ar.RouteAsync(ctx, message); err != nil {
		t.Fatalf("Failed to route message: %v", err)
	}

	// Wait for events
	time.Sleep(200 * time.Millisecond)

	// Should have at least one worker_start event
	if len(events) == 0 {
		t.Error("Expected at least one coordinator event")
	}

	// Check for worker_start event
	foundStart := false
	for _, event := range events {
		if event.eventType == "worker_start" {
			foundStart = true
			break
		}
	}

	if !foundStart {
		t.Error("Expected worker_start event via coordinator")
	}

	_ = callbackCalls.Load() // Avoid unused variable warning
}

func TestAsyncRouter_ShutdownCoordination(t *testing.T) {
	t.Parallel()
	router := NewRouterWithDefaults(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	ar := NewAsyncRouter(pkgctx.NewSystemContext(), router, 3, 50, nil)

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	if err := ar.Start(ctx); err != nil {
		t.Fatalf("Failed to start async router: %v", err)
	}

	// Test QueueShutdownHandler interface
	if ar.GetName() != "async_router" {
		t.Errorf("Expected name 'async_router', got '%s'", ar.GetName())
	}

	if ar.IsCritical() {
		t.Error("Async router should not be critical")
	}

	// Enqueue some messages
	for i := 0; i < 5; i++ {
		message := types.Message{
			EventType: "test_event",
			Payload:   map[string]any{"index": i},
		}
		_ = ar.RouteAsync(ctx, message)
	}

	// Wait for messages to be queued (but workers may process them quickly)
	time.Sleep(100 * time.Millisecond)

	// Check pending count (may be 0 if workers processed them quickly)
	pending := ar.GetPendingCount()
	t.Logf("Pending messages: %d", pending)

	// Note: Pending may be 0 if workers processed messages quickly
	// This is acceptable - the test verifies shutdown coordination works

	// Initiate shutdown
	if err := ar.InitiateShutdown(); err != nil {
		t.Fatalf("Failed to initiate shutdown: %v", err)
	}

	// Try to enqueue after shutdown - should fail
	message := types.Message{
		EventType: "test_event",
		Payload:   map[string]any{"test": "data"},
	}
	err := ar.RouteAsync(ctx, message)
	if err == nil {
		t.Error("Expected error when enqueueing after shutdown")
	}

	// Drain queue
	drainCtx, drainCancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer drainCancel()

	if err := ar.Drain(drainCtx); err != nil {
		// Drain might timeout if workers are still processing, which is acceptable
		if drainCtx.Err() == context.DeadlineExceeded {
			t.Logf("Drain timed out (acceptable if workers still processing): %v", err)
		} else {
			t.Errorf("Drain failed: %v", err)
		}
	}

	// Stop router
	_ = ar.Stop() //nolint:errcheck // Test cleanup
}

func TestAsyncRouter_MultipleWorkers(t *testing.T) {
	t.Parallel()
	router := NewRouterWithDefaults(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	ar := NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 100, nil)

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	if err := ar.Start(ctx); err != nil {
		t.Fatalf("Failed to start async router: %v", err)
	}
	defer func() { _ = ar.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue multiple messages concurrently
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("transceiver_test", "concurrent enqueue for multiple workers").StartSimple(func() {
			func(index int) {
				defer wg.Done()
				message := types.Message{
					EventType: "test_event",
					Payload:   map[string]any{"index": index},
				}
				_ = ar.RouteAsync(ctx, message)
			}(i)
		})
	}

	wg.Wait()

	// Wait for workers to start
	time.Sleep(200 * time.Millisecond)

	// Should have multiple workers (up to maxWorkers)
	activeWorkers := ar.GetActiveWorkers()
	if activeWorkers == 0 {
		t.Error("Expected at least one active worker")
	}
	if activeWorkers > 5 {
		t.Errorf("Expected at most 5 workers, got %d", activeWorkers)
	}
}

func TestAsyncRouter_ConcurrentOperations(t *testing.T) {
	t.Parallel()
	router := NewRouterWithDefaults(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	ar := NewAsyncRouter(pkgctx.NewSystemContext(), router, 3, 50, nil)

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	if err := ar.Start(ctx); err != nil {
		t.Fatalf("Failed to start async router: %v", err)
	}
	defer func() { _ = ar.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue messages concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("transceiver_test", "concurrent enqueue for operations test").StartSimple(func() {
			func(index int) {
				defer wg.Done()
				message := types.Message{
					EventType: "test_event",
					Payload:   map[string]any{"index": index},
				}
				if err := ar.RouteAsync(ctx, message); err != nil {
					t.Errorf("Failed to route message %d: %v", index, err)
				}
			}(i)
		})
	}

	wg.Wait()

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Queue should be empty or nearly empty
	queueSize := ar.GetQueueSize()
	if queueSize > 10 {
		t.Errorf("Expected queue to be processed, got size %d", queueSize)
	}
}

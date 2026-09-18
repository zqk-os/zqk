package storage_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// waitForCondition polls until condition is true or ctx is done (same behavior as pkg/testing.WaitForCondition).
func waitForCondition(ctx context.Context, condition func() bool, pollInterval time.Duration) bool {
	if condition() {
		return true
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}

func TestFileLockMetricsAsyncCollector_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test with async collector in short mode - requires proper cleanup")
	}
	mockOSP := storage.NewMockObjectStorageForMetricsTests()

	collector := storage.NewFileLockMetricsAsyncCollector(mockOSP)

	storage.ResetFileLockMetrics()
	metrics := storage.GetFileLockMetrics()
	metrics.RecordAcquisition(100 * time.Microsecond)
	metrics.RecordContention()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	var metricID string
	var collectErr error
	done := make(chan struct{})

	err := collector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd,
		func(id string, err error) {
			metricID = id
			collectErr = err
			close(done)
		})
	if err != nil {
		t.Fatalf("Failed to queue metrics collection: %v", err)
	}

	select {
	case <-done:
		if collectErr != nil {
			t.Fatalf("Metrics collection failed: %v", collectErr)
		}
		if metricID == "" {
			t.Fatal("Metric ID not set")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Metrics collection timed out")
	}

	enqueued, dropped := collector.GetAsyncCollectorStats()
	if enqueued < 1 {
		t.Fatalf("expected enqueued >= 1, got %d", enqueued)
	}
	if dropped < 0 {
		t.Fatalf("expected dropped >= 0, got %d", dropped)
	}

	collector.Stop()
}

func TestFileLockMetricsAsyncCollector_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test with async collector in short mode - requires proper cleanup")
	}
	mockOSP := storage.NewMockObjectStorageForMetricsTests()

	collector := storage.NewFileLockMetricsAsyncCollector(mockOSP)
	defer collector.Stop()

	storage.ResetFileLockMetrics()
	metrics := storage.GetFileLockMetrics()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_metrics_collector_%d", i), fmt.Sprintf("collecting metrics %d in async test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				metrics.RecordAcquisition(100 * time.Microsecond)

				err := collector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd,
					func(id string, err error) {
						if err != nil {
							errors <- err
						}
					})
				if err != nil {
					errors <- err
				}
			})
	}

	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case err := <-errors:
		t.Fatalf("Concurrent collection error: %v", err)
	case <-ctx.Done():
	}

	collector.Stop()
}

func TestFileLockMetricsAsyncCollector_Stop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test with async collector in short mode - requires proper cleanup")
	}
	mockOSP := storage.NewMockObjectStorageForMetricsTests()

	collector := storage.NewFileLockMetricsAsyncCollector(mockOSP)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	doneCount := 0
	doneMu := sync.Mutex{}
	doneChan := make(chan struct{}, 5)

	for i := 0; i < 5; i++ {
		_ = collector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd,
			func(id string, err error) {
				doneMu.Lock()
				doneCount++
				doneMu.Unlock()
				doneChan <- struct{}{}
			}) //nolint:errcheck // Test setup - errors are acceptable
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := 0
	_ = waitForCondition(ctx, func() bool {
		select {
		case <-doneChan:
			received++
			return received >= 3
		default:
			return false
		}
	}, 50*time.Millisecond)

	collector.Stop()

	collector.Stop()
	collector.Stop()
}

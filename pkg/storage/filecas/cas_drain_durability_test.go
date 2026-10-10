package filecas

import (
	"context"
	"errors"
	"fmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testFixtureFilenameDurable  = "cas-durable-fixture.yaml"
	testFixtureFilenameBoundary = "cas-boundary-fixture.yaml"
	testFixtureFilenameConcur   = "cas-concurrent-fixture-%d.yaml"
	testPayloadData             = "id: OBJ-CAS-DURABLE-PAYLOAD\nkind: object\n"
	errTestWriteFixtureFailed   = "failed to write test fixture: %v"
	errTestOpenFixtureFailed    = "failed to open test fixture: %v"
	errTestDrainWithCtxFailed   = "DrainDarwinSyncQueueContext failed unexpectedly: %v"
	errTestQueueNotDrained      = "expected Darwin sync queue to be fully drained, but pending count is %d"
	errTestNilExpectedNoPanic   = "expected nil file to be handled safely, got %v"
	errTestPrecancelledExpected = "expected cancelled context error, got %v"
	errTestConcurrentSyncFailed = "concurrent publish sync failed: %v"
	errTestDrainTimeoutFailed   = "expected DrainDarwinSyncQueue to succeed within timeout, got %v"
	errTestShutdownInitiateFail = "InitiateDarwinSyncShutdown failed: %v"
	errTestSynchronousUnderShut = "expected synchronous fallback under shutdown, got error: %v"
)

// TestDrainDarwinSyncQueue_DurabilityAndCompleteness verifies primary functional acceptance:
// background CAS publish syncs are queued and subsequently drained to disk without data loss.
func TestDrainDarwinSyncQueue_DurabilityAndCompleteness(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, testFixtureFilenameDurable)

	if err := fileutil.WriteFile(filePath, []byte(testPayloadData), fileutil.StandardFilePerm); err != nil {
		t.Fatalf(errTestWriteFixtureFailed, err)
	}

	f, err := fileutil.Open(filePath)
	if err != nil {
		t.Fatalf(errTestOpenFixtureFailed, err)
	}

	if err := CasPublishSyncFileOS(f); err != nil {
		t.Fatalf(errTestConcurrentSyncFailed, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := DrainDarwinSyncQueueContext(ctx); err != nil {
		t.Fatalf(errTestDrainWithCtxFailed, err)
	}

	if !IsDarwinSyncQueueDrained() {
		t.Fatalf(errTestQueueNotDrained, DarwinSyncQueuePendingCount())
	}

	// Secondary check: DrainDarwinSyncQueue with duration timeout
	if err := DrainDarwinSyncQueue(2 * time.Second); err != nil {
		t.Fatalf(errTestDrainTimeoutFailed, err)
	}
}

// TestDrainDarwinSyncQueue_BoundaryAndCancelledContext verifies boundary conditions,
// pre-cancelled contexts, nil inputs, and timeout parameters.
func TestDrainDarwinSyncQueue_BoundaryAndCancelledContext(t *testing.T) {
	// Nil file must return nil without panicking
	if err := CasPublishSyncFileOS(nil); err != nil {
		t.Fatalf(errTestNilExpectedNoPanic, err)
	}

	// Zero or negative timeout must default safely
	if err := DrainDarwinSyncQueue(0); err != nil {
		t.Fatalf(errTestDrainTimeoutFailed, err)
	}

	if err := DrainDarwinSyncQueue(-1 * time.Second); err != nil {
		t.Fatalf(errTestDrainTimeoutFailed, err)
	}

	// Pre-cancelled context should return context error on Darwin
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DrainDarwinSyncQueueContext(cancelledCtx)
	if runtime.GOOS == "darwin" {
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf(errTestPrecancelledExpected, err)
		}
	} else if err != nil {
		t.Fatalf(errTestDrainWithCtxFailed, err)
	}
}

// TestDrainDarwinSyncQueue_ConcurrencyAndShutdownDrain verifies integration conformance
// under concurrent publication load and graceful shutdown drain semantics.
func TestDrainDarwinSyncQueue_ConcurrencyAndShutdownDrain(t *testing.T) {
	tmpDir := t.TempDir()
	const workerCount = 8

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		wID := i
		goroutinelabels.NewGoroutine("filecas_test", "darwin sync queue worker").StartSimple(func() {
			defer wg.Done()
			path := filepath.Join(tmpDir, fmt.Sprintf(testFixtureFilenameConcur, wID))
			if err := fileutil.WriteFile(path, []byte(testPayloadData), fileutil.StandardFilePerm); err != nil {
				return
			}
			f, err := fileutil.Open(path)
			if err != nil {
				return
			}
			_ = CasPublishSyncFileOS(f)
		})
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := DrainDarwinSyncQueueContext(ctx); err != nil {
		t.Fatalf(errTestDrainWithCtxFailed, err)
	}

	if !IsDarwinSyncQueueDrained() {
		t.Fatalf(errTestQueueNotDrained, DarwinSyncQueuePendingCount())
	}

	// Verify shutdown initiation enforces synchronous fallback
	if err := InitiateDarwinSyncShutdown(); err != nil {
		t.Fatalf(errTestShutdownInitiateFail, err)
	}

	boundaryPath := filepath.Join(tmpDir, testFixtureFilenameBoundary)
	if err := fileutil.WriteFile(boundaryPath, []byte(testPayloadData), fileutil.StandardFilePerm); err != nil {
		t.Fatalf(errTestWriteFixtureFailed, err)
	}
	boundaryFile, err := fileutil.Open(boundaryPath)
	if err != nil {
		t.Fatalf(errTestOpenFixtureFailed, err)
	}

	if err := CasPublishSyncFileOS(boundaryFile); err != nil {
		t.Fatalf(errTestSynchronousUnderShut, err)
	}

	// Reset shutdown flag
	ResetDarwinSyncShutdownForTesting()
}

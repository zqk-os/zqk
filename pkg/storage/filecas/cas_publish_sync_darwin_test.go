//go:build darwin

package filecas

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestQueueOrSync_FallsBackWhenQueueCannotAccept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas-object.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-1\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	if err := queueOrSync(make(chan *fileutil.File), f); err != nil {
		t.Fatalf("queueOrSync fallback: %v", err)
	}
	if _, err := f.Stat(); err == nil {
		t.Fatal("fallback left duplicated file descriptor open")
	}
}

func TestQueueOrSync_QueuesWhenCapacityIsAvailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas-object.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-1\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	queue := make(chan *fileutil.File, 1)

	if err := queueOrSync(queue, f); err != nil {
		t.Fatalf("queueOrSync enqueue: %v", err)
	}
	select {
	case queued := <-queue:
		if queued != f {
			t.Fatal("queueOrSync enqueued a different file descriptor")
		}
		if err := syncAndClose(queued); err != nil {
			t.Fatalf("sync queued descriptor: %v", err)
		}
	default:
		t.Fatal("queueOrSync did not enqueue with available capacity")
	}
}

func TestQueueOrSync_PropagatesFallbackSyncError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas-object.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-1\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}

	if err := queueOrSync(make(chan *fileutil.File), f); err == nil {
		t.Fatal("queueOrSync hid fallback sync failure")
	}
}

func TestDrainDarwinSyncQueue(t *testing.T) {
	// Draining an empty or cleanly initialized queue must succeed within timeout.
	if err := DrainDarwinSyncQueue(time.Second); err != nil {
		t.Fatalf("DrainDarwinSyncQueue failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "drain-test.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-DRAIN\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	// Trigger darwin sync queue init and queue the file.
	darwinSyncQueueOnce.Do(initDarwinSyncQueue)
	if err := queueOrSync(darwinSyncQueue, f); err != nil {
		t.Fatalf("queueOrSync failed: %v", err)
	}

	// Drain queue should wait for background sync & close to complete.
	if err := DrainDarwinSyncQueue(2 * time.Second); err != nil {
		t.Fatalf("DrainDarwinSyncQueue after queueing failed: %v", err)
	}
}

func TestDrainDarwinSyncQueueContext_SuccessAndPendingCount(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	path := filepath.Join(t.TempDir(), "drain-ctx-test.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-DRAIN-CTX\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	darwinSyncQueueOnce.Do(initDarwinSyncQueue)
	if err := queueOrSync(darwinSyncQueue, f); err != nil {
		t.Fatalf("queueOrSync failed: %v", err)
	}

	if err := DrainDarwinSyncQueueContext(ctx); err != nil {
		t.Fatalf("DrainDarwinSyncQueueContext failed: %v", err)
	}

	if !IsDarwinSyncQueueDrained() {
		t.Errorf("expected queue to be drained, got pending: %d", DarwinSyncQueuePendingCount())
	}
}

func TestDrainDarwinSyncQueueContext_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := DrainDarwinSyncQueueContext(ctx)
	if err == nil {
		t.Fatal("expected error with pre-cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestInitiateDarwinSyncShutdown_SynchronousFallback(t *testing.T) {
	darwinSyncQueueOnce.Do(initDarwinSyncQueue)

	if err := InitiateDarwinSyncShutdown(); err != nil {
		t.Fatalf("InitiateDarwinSyncShutdown failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "shutdown-test.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-SHUTDOWN\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	// Under shutdown, queueOrSync must immediately execute syncAndClose synchronously
	if err := queueOrSync(darwinSyncQueue, f); err != nil {
		t.Fatalf("queueOrSync under shutdown failed: %v", err)
	}

	// File descriptor should already be closed by synchronous fallback
	if _, err := f.Stat(); err == nil {
		t.Fatal("expected file descriptor to be closed after synchronous fallback")
	}

	// Reset shutdown flag for other tests
	darwinSyncShutdown.Store(false)
}



//go:build darwin

package filecas

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	darwinSyncQueueOnce sync.Once
	darwinSyncQueue     chan *fileutil.File
	darwinSyncMu        sync.Mutex
	darwinSyncDrainedCh chan struct{}
	darwinSyncPending   atomic.Int64
	darwinSyncShutdown  atomic.Bool
	darwinStrictSync    atomic.Bool
)

func addDarwinSyncPending() {
	darwinSyncMu.Lock()
	if darwinSyncPending.Add(1) == 1 {
		darwinSyncDrainedCh = make(chan struct{})
	}
	darwinSyncMu.Unlock()
}

func doneDarwinSyncPending() {
	darwinSyncMu.Lock()
	if darwinSyncPending.Add(-1) == 0 && darwinSyncDrainedCh != nil {
		close(darwinSyncDrainedCh)
		darwinSyncDrainedCh = nil
	}
	darwinSyncMu.Unlock()
}

func darwinSyncDrainChan() <-chan struct{} {
	darwinSyncMu.Lock()
	ch := darwinSyncDrainedCh
	darwinSyncMu.Unlock()
	return ch
}

// SetDarwinStrictSync configures whether Darwin CAS publishes execute synchronous fsync (TDE-F-REL-001).
func SetDarwinStrictSync(strict bool) {
	darwinStrictSync.Store(strict)
}

// IsDarwinStrictSync reports whether synchronous fsync is active for Darwin CAS publishes.
func IsDarwinStrictSync() bool {
	if darwinStrictSync.Load() {
		return true
	}
	return os.Getenv("ZQK_DARWIN_CAS_SYNC_STRICT") == "1" || os.Getenv("ZQK_CAS_STRICT_DURABILITY") == "1"
}

func initDarwinSyncQueue() {
	darwinSyncQueue = make(chan *fileutil.File, 1024)
	goroutinelabels.NewGoroutine("storage.darwin_cas_sync_queue", "background fsync/close of CAS publish dup FDs on darwin").
		StartSimple(func() {
			for f := range darwinSyncQueue {
				if f != nil {
					if err := syncAndClose(f); err != nil {
						logging.LogSwallowedError(err)
					}
				}
				doneDarwinSyncPending()
			}
		})
}

func syncAndClose(f *fileutil.File) error {
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(syncErr, closeErr)
}

func queueOrSync(queue chan<- *fileutil.File, f *fileutil.File) error {
	isGlobalQueue := (queue == darwinSyncQueue)
	if isGlobalQueue {
		// If shutdown has been initiated, do not queue asynchronously;
		// immediately sync synchronously to guarantee durability before process exit.
		if darwinSyncShutdown.Load() {
			return syncAndClose(f)
		}
		addDarwinSyncPending()
	}
	select {
	case queue <- f:
		return nil
	default:
		// Queue pressure must not silently discard durability. Fall back to a
		// synchronous fsync so a successful publish still has crash semantics.
		err := syncAndClose(f)
		if isGlobalQueue {
			doneDarwinSyncPending()
		}
		return err
	}
}

// InitiateDarwinSyncShutdown signals that the darwin CAS sync queue is preparing for shutdown,
// forcing all subsequent publish syncs to execute synchronously.
func InitiateDarwinSyncShutdown() error {
	darwinSyncShutdown.Store(true)
	return nil
}

// ResetDarwinSyncShutdownForTesting resets the shutdown flag for testing.
func ResetDarwinSyncShutdownForTesting() {
	darwinSyncShutdown.Store(false)
}

// IsDarwinSyncQueueDrained reports whether there are zero pending sync operations.
func IsDarwinSyncQueueDrained() bool {
	return darwinSyncPending.Load() == 0
}

// DarwinSyncQueuePendingCount returns the current count of pending background sync operations.
func DarwinSyncQueuePendingCount() int64 {
	return darwinSyncPending.Load()
}

// DrainDarwinSyncQueueContext waits until all pending background CAS syncs complete or ctx is cancelled.
func DrainDarwinSyncQueueContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ch := darwinSyncDrainChan()
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DrainDarwinSyncQueue waits up to timeout for all pending background CAS syncs to complete.
func DrainDarwinSyncQueue(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := DrainDarwinSyncQueueContext(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("timed out waiting for darwin CAS sync queue to drain")
		}
		return err
	}
	return nil
}

// CasPublishSyncFileOS queues asynchronous background fsync on macOS to prevent
// stalling synchronous CAS publishes on F_FULLFSYNC while ensuring background durability (K:F-L-RELIABILITY-001).
func CasPublishSyncFileOS(f *fileutil.File) error {
	if f == nil {
		return nil
	}
	if IsDarwinStrictSync() {
		return f.Sync()
	}
	darwinSyncQueueOnce.Do(initDarwinSyncQueue)
	dupFD, err := fileutil.Open(f.Name())
	if err != nil {
		return err
	}
	return queueOrSync(darwinSyncQueue, dupFD)
}

// CasPublishSyncDirOS persists the rename's directory entry on macOS.
func CasPublishSyncDirOS(dirPath string) error {
	return fileutil.SyncDir(dirPath)
}

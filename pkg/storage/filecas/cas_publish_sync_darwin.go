//go:build darwin

package filecas

import (
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	darwinSyncQueueOnce sync.Once
	darwinSyncQueue     chan *fileutil.File
	darwinSyncWG        sync.WaitGroup
)

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
				darwinSyncWG.Done()
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
		darwinSyncWG.Add(1)
	}
	select {
	case queue <- f:
		return nil
	default:
		// Queue pressure must not silently discard durability. Fall back to a
		// synchronous fsync so a successful publish still has crash semantics.
		err := syncAndClose(f)
		if isGlobalQueue {
			darwinSyncWG.Done()
		}
		return err
	}
}

// DrainDarwinSyncQueue waits up to timeout for all pending background CAS syncs to complete.
func DrainDarwinSyncQueue(timeout time.Duration) error {
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("storage.darwin_cas_sync_drain", "await darwin CAS sync queue drain").
		StartSimple(func() {
			darwinSyncWG.Wait()
			close(done)
		})

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return errors.New("timed out waiting for darwin CAS sync queue to drain")
	}
}

// casPublishSyncFileOS queues asynchronous background fsync on macOS to prevent
// stalling synchronous CAS publishes on F_FULLFSYNC while ensuring background durability (K:F-L-RELIABILITY-001).
func CasPublishSyncFileOS(f *fileutil.File) error {
	if f == nil {
		return nil
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

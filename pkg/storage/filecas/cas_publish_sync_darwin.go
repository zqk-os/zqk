//go:build darwin

package filecas

import (
	"errors"
	"sync"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	darwinSyncQueueOnce sync.Once
	darwinSyncQueue     chan *fileutil.File
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
			}
		})
}

func syncAndClose(f *fileutil.File) error {
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(syncErr, closeErr)
}

func queueOrSync(queue chan<- *fileutil.File, f *fileutil.File) error {
	select {
	case queue <- f:
		return nil
	default:
		// Queue pressure must not silently discard durability. Fall back to a
		// synchronous fsync so a successful publish still has crash semantics.
		return syncAndClose(f)
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

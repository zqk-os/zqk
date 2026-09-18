//go:build darwin

package filecas

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
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
					_ = f.Sync()
					_ = f.Close()
				}
			}
		})
}

// casPublishSyncFileOS queues asynchronous background fsync on macOS to prevent
// stalling synchronous CAS publishes on F_FULLFSYNC while ensuring background durability (K:F-L-RELIABILITY-001).
func CasPublishSyncFileOS(f *fileutil.File) error {
	if f == nil {
		return nil
	}
	darwinSyncQueueOnce.Do(initDarwinSyncQueue)
	if dupFD, err := fileutil.Open(f.Name()); err == nil {
		select {
		case darwinSyncQueue <- dupFD:
		default:
			_ = dupFD.Close()
		}
	}
	return nil
}

// casPublishSyncDirOS skips directory fsync on macOS for the same F_FULLFSYNC reason.
func CasPublishSyncDirOS(string) error {
	return nil
}

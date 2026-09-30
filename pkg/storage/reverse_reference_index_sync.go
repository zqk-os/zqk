package storage

import (
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// Reverse-ref disk sync: CUD used to update memory only, so the next CLI process
// loaded a stale .zqk/cache/reverse-reference-index.json (membership / dependents miss).
// incremental invalidate + near-real-time persist.

const reverseReferencePersistDebounce = 250 * time.Millisecond

var (
	revRefBoundRoot    atomic.Value // string
	revRefPersistTimer atomic.Pointer[time.Timer]
)

// BindReverseReferenceIndexProjectRoot ties the global reverse-ref index to a project
// and loads the on-disk cache if not already ready. Call from storage construction.
func BindReverseReferenceIndexProjectRoot(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	revRefBoundRoot.Store(projectRoot)
	ensureReverseReferenceIndexLoaded(projectRoot)
}

// WarmReverseReferenceIndex asynchronously warms the reverse reference index in the background if not ready.
func WarmReverseReferenceIndex(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	if index.IsReady() {
		return
	}
	goroutinelabels.NewGoroutine("storage.reverse_ref_index_warm", "warming reverse reference index in background").
		StartSimple(func() {
			ensureReverseReferenceIndexLoaded(projectRoot)
		})
}

func reverseReferenceBoundProjectRoot() string {
	v := revRefBoundRoot.Load()
	if v == nil {
		return emptyValue
	}
	s, _ := v.(string)
	return s
}

func ensureReverseReferenceIndexLoaded(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	if index.IsReady() {
		return
	}
	ok, err := index.LoadCache(projectRoot)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageReverseRefIndexLoadedDebug).
			WithError(err).
			String("op", "load_cache").
			Log()
		// I/O error must not mark ready-empty
		// (delete would treat "no dependents" as authoritative).
		return
	}
	if !ok {
		// Missing or incomplete cache: do not mark ready-empty (fail-closed).
		// Reverse-reference lookups must not treat absent index as "0 dependents".
		// Full rebuild is BuildFromScan (system check --refresh-cache) or CUD indexing.
		return
	}
}

// scheduleReverseReferenceIndexPersist debounces SaveCache so burst CUD does not rewrite
// the JSON file on every single object write.
func scheduleReverseReferenceIndexPersist(projectRoot string) {
	if projectRoot == emptyValue {
		projectRoot = reverseReferenceBoundProjectRoot()
	}
	if projectRoot == emptyValue {
		return
	}
	root := projectRoot
	timerReady := make(chan *time.Timer, 1)
	t := time.AfterFunc(reverseReferencePersistDebounce, func() {
		readyT := <-timerReady
		timerReady <- readyT // put it back for any other readers, though there should be none
		// Only clear if we are still the scheduled timer; a newer AfterFunc must stay.
		revRefPersistTimer.CompareAndSwap(readyT, nil)
		index := GetGlobalReverseReferenceIndex()
		if err := index.SaveCache(root); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Debug(LogEventStorageReverseRefIndexSavedDebug).
				WithError(err).
				String("op", "save_cache").
				Log()
		}
	})
	timerReady <- t
	if old := revRefPersistTimer.Swap(t); old != nil {
		_ = old.Stop()
	}
}

// FlushReverseReferenceIndexPersist runs any pending SaveCache immediately (tests).
func FlushReverseReferenceIndexPersist() {
	root := reverseReferenceBoundProjectRoot()
	if root == emptyValue {
		return
	}
	index := GetGlobalReverseReferenceIndex()
	old := revRefPersistTimer.Swap(nil)
	if old != nil {
		if old.Stop() {
			_ = index.SaveCache(root)
		}
		return
	}
	_ = index.SaveCache(root)
}

func afterReverseReferenceIndexMutation() {
	root := reverseReferenceBoundProjectRoot()
	if root == emptyValue {
		return
	}
	ensureReverseReferenceIndexLoaded(root)
	scheduleReverseReferenceIndexPersist(root)
}

// StopReverseReferenceIndexPersistForTest cancels pending debounce timer and unbinds root for tests.
func StopReverseReferenceIndexPersistForTest(projectRoot string) {
	if old := revRefPersistTimer.Swap(nil); old != nil {
		_ = old.Stop()
	}
	if projectRoot != emptyValue && reverseReferenceBoundProjectRoot() == projectRoot {
		revRefBoundRoot.Store(emptyValue)
	}
}

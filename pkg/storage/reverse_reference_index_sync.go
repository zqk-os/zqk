package storage

import (
	"path/filepath"
	"sync"
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

	unboundTestRootsMu sync.RWMutex
	unboundTestRoots   = make(map[string]struct{})
)

func forEachTestProjectRootKey(projectRoot string, fn func(key string)) {
	if projectRoot == emptyValue {
		return
	}
	clean := filepath.Clean(projectRoot)
	fn(clean)
	if abs, err := filepath.Abs(clean); err == nil && abs != clean {
		fn(abs)
	}
}

func markTestProjectRootUnbound(projectRoot string) {
	unboundTestRootsMu.Lock()
	defer unboundTestRootsMu.Unlock()
	forEachTestProjectRootKey(projectRoot, func(key string) {
		unboundTestRoots[key] = struct{}{}
	})
}

func isTestProjectRootUnbound(projectRoot string) bool {
	if projectRoot == emptyValue {
		return false
	}
	clean := filepath.Clean(projectRoot)
	unboundTestRootsMu.RLock()
	defer unboundTestRootsMu.RUnlock()
	if _, ok := unboundTestRoots[clean]; ok {
		return true
	}
	if abs, err := filepath.Abs(clean); err == nil && abs != clean {
		_, ok := unboundTestRoots[abs]
		return ok
	}
	return false
}

func unmarkTestProjectRootUnbound(projectRoot string) {
	unboundTestRootsMu.Lock()
	defer unboundTestRootsMu.Unlock()
	forEachTestProjectRootKey(projectRoot, func(key string) {
		delete(unboundTestRoots, key)
	})
}

// BindReverseReferenceIndexProjectRoot ties the global reverse-ref index to a project
// and loads the on-disk cache if not already ready. Call from storage construction.
func BindReverseReferenceIndexProjectRoot(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	unmarkTestProjectRootUnbound(projectRoot)
	revRefBoundRoot.Store(projectRoot)
	ensureReverseReferenceIndexLoaded(projectRoot)
}

func getUnreadyReverseReferenceIndex(projectRoot string) *ReverseReferenceIndex {
	if projectRoot == emptyValue {
		return nil
	}
	index := GetGlobalReverseReferenceIndex()
	if index.IsReady() {
		return nil
	}
	return index
}

// WarmReverseReferenceIndex asynchronously warms the reverse reference index in the background if not ready.
func WarmReverseReferenceIndex(projectRoot string) {
	if getUnreadyReverseReferenceIndex(projectRoot) == nil {
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
	index := getUnreadyReverseReferenceIndex(projectRoot)
	if index == nil {
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

func resolveReverseReferenceProjectRoot(projectRoot string) string {
	if projectRoot == emptyValue {
		return reverseReferenceBoundProjectRoot()
	}
	return projectRoot
}

// scheduleReverseReferenceIndexPersist debounces SaveCache so burst CUD does not rewrite
// the JSON file on every single object write.
func scheduleReverseReferenceIndexPersist(projectRoot string) {
	root := resolveReverseReferenceProjectRoot(projectRoot)
	if root == emptyValue || isTestProjectRootUnbound(root) {
		return
	}
	timerReady := make(chan *time.Timer, 1)
	t := time.AfterFunc(reverseReferencePersistDebounce, func() {
		readyT := <-timerReady
		timerReady <- readyT // put it back for any other readers, though there should be none
		// Only clear if we are still the scheduled timer; a newer AfterFunc must stay.
		revRefPersistTimer.CompareAndSwap(readyT, nil)
		if reverseReferenceBoundProjectRoot() == emptyValue || !isProjectRootEquivalent(reverseReferenceBoundProjectRoot(), root) {
			return
		}
		if isTestProjectRootUnbound(root) {
			return
		}
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
	if root == emptyValue || isTestProjectRootUnbound(root) {
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
	if root == emptyValue || isTestProjectRootUnbound(root) {
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
	if projectRoot != emptyValue {
		markTestProjectRootUnbound(projectRoot)
		if reverseReferenceBoundProjectRoot() == projectRoot || isProjectRootEquivalent(reverseReferenceBoundProjectRoot(), projectRoot) {
			revRefBoundRoot.Store(emptyValue)
		}
	} else if bound := reverseReferenceBoundProjectRoot(); bound != emptyValue {
		markTestProjectRootUnbound(bound)
		revRefBoundRoot.Store(emptyValue)
	}
}

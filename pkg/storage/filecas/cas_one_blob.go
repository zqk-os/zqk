package filecas

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage/file"
)

var (
	kindSweepLocksMu sync.Mutex
	kindSweepLocks   = make(map[string]*sync.Mutex)
)

func getKindSweepMutex(dir string) *sync.Mutex {
	kindSweepLocksMu.Lock()
	defer kindSweepLocksMu.Unlock()
	m, ok := kindSweepLocks[dir]
	if !ok {
		m = &sync.Mutex{}
		kindSweepLocks[dir] = m
	}
	return m
}

// WithSweepLock acquires an in-process mutex and a directory-level lock (flock with timeout)
// to prevent concurrent CAS updates from creating duplicate blobs or stranding .tmp files.
func (cas *ContentAddressableStorage) WithSweepLock(fn func() error) error {
	mu := getKindSweepMutex(cas.kindDir)
	mu.Lock()
	defer mu.Unlock()

	lockPath := filepath.Join(cas.kindDir, ".cas_sweep.lock")
	fl, err := file.NewFileLock(lockPath)
	if err == nil {
		defer func() { _ = fl.Close() }()
		if err := fl.LockWithTimeout(10 * time.Second); err == nil {
			defer func() { _ = fl.Unlock() }()
		}
	}
	return fn()
}

// ensureExactlyOneLiveCASBlob deletes every live hash YAML for objectID except
// keeperHash.yaml, then fails if a second blob is still on disk.
// Call after a successful CAS write, before Update returns.
// TRACK: BLI-CEF-R19-CAS-INTERMEDIATE-LEAK-001 — write-path prevention; detection alone is not enough.
func ensureExactlyOneLiveCASBlob(objectID, keeperHash, kindDir string) error {
	if objectID == emptyValue || keeperHash == emptyValue || kindDir == emptyValue {
		return nil
	}
	keeperBase := keeperHash + ".yaml"
	blobs := listLiveCASBlobsForObjectID(objectID, kindDir)
	var extras []string
	for _, p := range blobs {
		if filepath.Base(p) != keeperBase {
			extras = append(extras, p)
		}
	}
	if len(extras) == 0 {
		return nil
	}

	for _, p := range extras {
		if err := RemoveOrphanCASHashFileSync(p); err != nil {
			return err
		}
		InvalidateCasHashFilePeekCache(filepath.Base(p))
	}

	// Double-check only if extras were actually found and removed
	var remainingExtras []string
	for _, p := range listLiveCASBlobsForObjectID(objectID, kindDir) {
		if filepath.Base(p) != keeperBase {
			remainingExtras = append(remainingExtras, p)
		}
	}
	if len(remainingExtras) > 0 {
		return errfmt.Errorf("CAS update left %d extra blob(s) for %s (keeper %s): %s", len(remainingExtras), objectID, keeperHash, strings.Join(remainingExtras, ", "))
	}
	return nil
}

func listLiveCASBlobsForObjectID(objectID, kindDir string) []string {
	if objectID == emptyValue || kindDir == emptyValue {
		return nil
	}
	var out []string
	scanDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !CasHashFilenameRe.MatchString(name) {
				continue
			}
			p := filepath.Join(dir, name)
			if CasHashFilePeekObjectID(p) == objectID {
				out = append(out, p)
			}
		}
	}

	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			scanDir(filepath.Join(kindDir, e.Name()))
		} else if CasHashFilenameRe.MatchString(e.Name()) {
			p := filepath.Join(kindDir, e.Name())
			if CasHashFilePeekObjectID(p) == objectID {
				out = append(out, p)
			}
		}
	}
	return out
}

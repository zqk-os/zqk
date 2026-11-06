package filecas

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/file"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

const casSweepLockTTL = 15 * time.Second

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

func isProcessAlive(pid int) bool {
	return syscallutil.IsProcessAlive(pid)
}

func readLockPID(path string) int {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return 0
	}
	trimmed := strings.TrimSpace(string(data))
	if pid, parseErr := strconv.Atoi(trimmed); parseErr == nil && pid > 0 {
		return pid
	}
	return 0
}

func sweepStaleSweepLock(lockPath string) {
	fi, err := fileutil.Stat(lockPath)
	if err != nil {
		return
	}
	age := time.Since(fi.ModTime())
	pid := readLockPID(lockPath)
	isDead := pid > 0 && !isProcessAlive(pid)
	if !isDead && age < casSweepLockTTL {
		return
	}
	fl, err := file.NewFileLock(lockPath)
	if err != nil {
		return
	}
	acquired, lockErr := fl.TryLock()
	if acquired && lockErr == nil {
		if rmErr := fileutil.Remove(lockPath); rmErr != nil && !fileutil.IsNotExist(rmErr) {
			logging.LogSwallowedError(rmErr)
		}
		if unlockErr := fl.Unlock(); unlockErr != nil {
			logging.LogSwallowedError(unlockErr)
		}
	}
	if closeErr := fl.Close(); closeErr != nil {
		logging.LogSwallowedError(closeErr)
	}
}

// WithSweepLock acquires an in-process mutex and a directory-level lock (flock with timeout)
// to prevent concurrent CAS updates from creating duplicate blobs or stranding .tmp files.
// Reconciles and sweeps stale lock files (older than 10-30s or orphaned PID), and cleanly releases
// and unlinks the lock on completion or failure to avoid lock contention.
func (cas *ContentAddressableStorage) WithSweepLock(fn func() error) error {
	mu := getKindSweepMutex(cas.kindDir)
	mu.Lock()
	defer mu.Unlock()

	lockPath := filepath.Join(cas.kindDir, ".cas_sweep.lock")
	sweepStaleSweepLock(lockPath)

	fl, err := file.NewFileLock(lockPath)
	if err != nil {
		return fn()
	}

	locked := false
	lockErr := fl.LockWithTimeout(10 * time.Second)
	if lockErr == nil {
		locked = true
		if wErr := fileutil.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), paths.FilePerm644); wErr != nil {
			logging.LogSwallowedError(wErr)
		}
	} else {
		sweepStaleSweepLock(lockPath)
		if retryErr := fl.LockWithTimeout(2 * time.Second); retryErr == nil {
			locked = true
			if wErr := fileutil.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), paths.FilePerm644); wErr != nil {
				logging.LogSwallowedError(wErr)
			}
		} else {
			logging.LogSwallowedError(retryErr)
		}
	}

	defer func() {
		if locked {
			if rmErr := fileutil.Remove(lockPath); rmErr != nil && !fileutil.IsNotExist(rmErr) {
				logging.LogSwallowedError(rmErr)
			}
			if unlockErr := fl.Unlock(); unlockErr != nil {
				logging.LogSwallowedError(unlockErr)
			}
		}
		if closeErr := fl.Close(); closeErr != nil {
			logging.LogSwallowedError(closeErr)
		}
	}()

	return fn()
}

// ensureExactlyOneLiveCASBlob deletes every live hash YAML for objectID except
// keeperHash.yaml, then fails if a second blob is still on disk.
// Call after a successful CAS write, before Update returns.
// write-path prevention; detection alone is not enough.
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
		entries, err := fileutil.ReadDir(dir)
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

	entries, err := fileutil.ReadDir(kindDir)
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

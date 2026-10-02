package storage

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	locknames "github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func readOptionalCacheData(cachePath string) ([]byte, bool, error) {
	data, err := fileutil.ReadFile(cachePath)
	if fileutil.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errfmt.Newf(ConstMiscFailedToReadCacheFile).Wrap(err)
	}
	return data, true, nil
}

func (r *ReverseReferenceIndex) LoadCache(projectRoot string) (bool, error) {
	cachePath := r.getCacheFilePath(projectRoot)

	data, ok, err := readOptionalCacheData(cachePath)
	if err != nil || !ok {
		return false, err
	}

	// Parse cache file
	var cacheData struct {
		Metadata *ReverseReferenceIndexMetadata `json:"metadata"`
		Index    map[string][]string            `json:"index"`
	}
	if err := json.Unmarshal(data, &cacheData); err != nil {
		// Cache file is corrupted - will be rebuilt
		return false, nil
	}

	// Validate cache metadata
	if cacheData.Metadata == nil {
		// Invalid cache - will be rebuilt
		return false, nil
	}

	// Check if project root matches
	if !isProjectRootEquivalent(cacheData.Metadata.ProjectRoot, projectRoot) {
		// Different project - cache is invalid
		return false, nil
	}

	// Check version compatibility
	if cacheData.Metadata.Version != reverseReferenceIndexVersion {
		// Version mismatch - cache is invalid
		return false, nil
	}

	// Cache is valid - load it (after I/O operations)
	var entryCount int
	var errLock = r.withWriteLock(
		locknames.LockNameReverseReferenceIndexLoad,
		func() error {
			if r.index == nil {
				r.index = make(map[string][]string)
			}
			r.index = cacheData.Index
			r.forwardIndex = make(map[string][]string)
			if r.index != nil {
				for referencedID, deps := range r.index {
					for _, dep := range deps {
						r.forwardIndex[dep] = append(r.forwardIndex[dep], referencedID)
					}
				}
			}
			r.metadata = cacheData.Metadata
			r.cacheDir = filepath.Dir(cachePath)
			r.isReady.Store(true)
			entryCount = len(r.index)
			return nil
		},
	)
	if errLock != nil {
		return false, errfmt.Errorf("reverse reference index lock timeout during load: %w", errLock)
	}

	StorageLog(newReverseRefIndexLockLogger()).Debug(LogEventStorageReverseRefIndexLoadedDebug).
		EntryCount(entryCount).
		BuildTime(zqktime.FormatRFC3339UTC(cacheData.Metadata.BuildTime)).
		Log()

	return true, nil
}

// SaveCache saves the cache to disk
func (r *ReverseReferenceIndex) SaveCache(projectRoot string) error {
	saveStart := time.Now()
	var entryCount int
	var errCount = r.withReadLock(
		locknames.LockNameReverseReferenceIndexSaveGetCount,
		func() error {
			entryCount = len(r.index)
			return nil
		},
	)
	if errCount != nil {
		return errfmt.Errorf("reverse reference index lock timeout getting count for save: %w", errCount)
	}

	cachePath := r.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)

	if err := fileutil.EnsureDir(cacheDir); err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateCacheDirectory).Wrap(err)
	}

	// Update metadata and prepare cache data
	var cacheData struct {
		Metadata *ReverseReferenceIndexMetadata `json:"metadata"`
		Index    map[string][]string            `json:"index"`
	}
	var errPrepare = r.withWriteLock(
		locknames.LockNameReverseReferenceIndexSavePrepare,
		func() error {
			r.metadata = &ReverseReferenceIndexMetadata{
				Version:     reverseReferenceIndexVersion,
				BuildTime:   time.Now(),
				ProjectRoot: projectRoot,
				EntryCount:  entryCount,
			}

			cacheData.Index = make(map[string][]string, len(r.index))
			for k, v := range r.index {
				dependents := make([]string, len(v))
				copy(dependents, v)
				cacheData.Index[k] = dependents
			}
			cacheData.Metadata = r.metadata
			return nil
		},
	)
	if errPrepare != nil {
		return errfmt.Errorf("reverse reference index lock timeout preparing save cache: %w", errPrepare)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalCache).Wrap(err)
	}

	// Add trailing newline
	data = append(data, '\n')

	if err := fileutil.WriteDurableStandardFile(cachePath, data); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteCacheFile).Wrap(err)
	}

	StorageLog(newReverseRefIndexLockLogger()).Debug(LogEventStorageReverseRefIndexSavedDebug).
		EntryCount(entryCount).
		ElapsedString(time.Since(saveStart).Round(time.Millisecond).String()).
		Log()

	return nil
}

// ReferencedIDCount returns the number of referenced IDs in the index (for reporting).

func isProjectRootEquivalent(a, b string) bool {
	if a == b {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && absA == absB {
		return true
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

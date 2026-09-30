// Extracted from reverse_reference_index.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	stdcontext "context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	locknames "github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func (r *ReverseReferenceIndex) LoadCache(projectRoot string) (bool, error) {
	cachePath := r.getCacheFilePath(projectRoot)

	data, err := fileutil.ReadFile(cachePath)
	if fileutil.IsNotExist(err) {
		// Cache doesn't exist - will be built fresh
		return false, nil
	}
	if err != nil {
		return false, errfmt.Newf(ConstMiscFailedToReadCacheFile).Wrap(err)
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
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var entryCount int
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_116 = concurrency.WithLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
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
	if err_swallow_116 != nil {
		logging.LogSwallowedError(err_swallow_116)
	}

	StorageLog(logger).Debug(LogEventStorageReverseRefIndexLoadedDebug).
		EntryCount(entryCount).
		BuildTime(zqktime.FormatRFC3339UTC(cacheData.Metadata.BuildTime)).
		Log()

	return true, nil
}

// SaveCache saves the cache to disk
func (r *ReverseReferenceIndex) SaveCache(projectRoot string) error {
	saveStart := time.Now()
	var entryCount int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_117 = concurrency.WithRLockTimeout(
		&r.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameReverseReferenceIndexSaveGetCount,
		func() error {
			entryCount = len(r.index)
			return nil
		},
	)
	if err_swallow_117 != nil {
		logging.LogSwallowedError(err_swallow_117)
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
	ctx2, cancel2 := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel2()
	var err_swallow_118 = concurrency.WithLockTimeout(
		&r.mu,
		ctx2,
		nil,
		logging.NewLockLoggerAdapter(logger),
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
	if err_swallow_118 != nil {
		return errfmt.Errorf("reverse reference index lock timeout preparing save cache: %w", err_swallow_118)
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

	StorageLog(logger).Debug(LogEventStorageReverseRefIndexSavedDebug).
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

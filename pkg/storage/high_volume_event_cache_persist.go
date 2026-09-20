// Extracted from pkg/storage/high_volume_event_cache.go (BLI-CEF-STORAGE-DECOMPOSE-001).
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
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

// LoadCache loads the cache from disk if it exists
// Returns true if cache was successfully loaded, false if cache needs to be rebuilt
func (c *HighVolumeEventCache) LoadCache(projectRoot string) (bool, error) {
	cachePath := c.getCacheFilePath(projectRoot)
	c.cacheDir = filepath.Dir(cachePath)

	data, err := fileutil.ReadFile(cachePath)
	if fileutil.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errfmt.Newf(ConstMiscFailedToReadCacheFile).Wrap(err)
	}

	// Parse cache file (v1: entries; v2: buckets)
	var cacheData struct {
		Metadata *HighVolumeEventCacheMetadata         `json:"metadata"`
		Entries  map[string]*HighVolumeEventCacheEntry `json:"entries"`
		Buckets  highVolumeEventCacheV2Buckets         `json:"buckets"`
	}
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return false, nil
	}

	if cacheData.Metadata == nil {
		return false, nil
	}

	if cacheData.Metadata.ProjectRoot != projectRoot {
		return false, nil
	}

	version := cacheData.Metadata.Version
	if version != highVolumeEventCacheVersion && version != "1.0" {
		return false, nil
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var err_swallow_16 = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheLoad,
		func() error {
			when.When(func() bool { return version == "2.0" && len(cacheData.Buckets) > 0 }).Then(func() {
				c.cache = c.flatMapFromBuckets(cacheData.Buckets)
			}).OrElseWhen(func() bool { return len(cacheData.Entries) > 0 }).Then(func() {
				c.cache = cacheData.Entries
			}).OrElse(func() {
				c.cache = make(map[string]*HighVolumeEventCacheEntry)
			}).Run()
			c.metadata = cacheData.Metadata
			if c.metadata != nil {
				c.metadata.HasStatus = false
			}
			c.rebuildTimeIndex()
			c.evictOldestToCap(maxHighVolumeEventCacheEntries)
			return nil
		},
	)
	if err_swallow_16 != nil {
		logging.LogSwallowedError(err_swallow_16)
	}

	StorageLog(logger).Debug(LogEventStorageHighVolumeCacheLoadedDebug).
		EntryCount(len(c.cache)).
		ProjectRoot(projectRoot).
		Log()
	return true, nil
}

// flatMapFromBuckets converts v2 buckets (kind → event_type → id → minimal) into id → full entry map.
// Must be called with write lock held (or during load before any other use).
func (c *HighVolumeEventCache) flatMapFromBuckets(buckets highVolumeEventCacheV2Buckets) map[string]*HighVolumeEventCacheEntry {
	out := make(map[string]*HighVolumeEventCacheEntry)
	for kind, byEventType := range buckets {
		if byEventType == nil {
			continue
		}
		for eventType, byID := range byEventType {
			if byID == nil {
				continue
			}
			for id, m := range byID {
				if m == nil {
					continue
				}
				out[id] = &HighVolumeEventCacheEntry{
					ID:        id,
					Kind:      kind,
					CreatedAt: m.CreatedAt,
					EventType: eventType,
					Exists:    true,
				}
			}
		}
	}
	return out
}

// bucketsFromCache builds v2 buckets from the flat cache (kind → event_type → id → minimal).
// Must be called with read or write lock held.
func (c *HighVolumeEventCache) bucketsFromCache() highVolumeEventCacheV2Buckets {
	buckets := make(highVolumeEventCacheV2Buckets)
	for id, e := range c.cache {
		if e == nil {
			continue
		}
		kind := e.Kind
		if kind == emptyValue {
			kind = objects.KindAuditEvent
		}
		eventType := e.EventType
		minimal := &highVolumeEventCacheEntryMinimal{CreatedAt: e.CreatedAt}
		if byKind, ok := buckets[kind]; !ok {
			buckets[kind] = map[string]map[string]*highVolumeEventCacheEntryMinimal{
				eventType: {id: minimal},
			}
		} else {
			if byEventType, ok := byKind[eventType]; !ok {
				byKind[eventType] = map[string]*highVolumeEventCacheEntryMinimal{id: minimal}
			} else {
				byEventType[id] = minimal
			}
		}
	}
	return buckets
}

// SaveCache saves the cache to disk
// Follows established pattern: minimize lock hold time, especially during I/O
// Pattern: RLock to read count → I/O operations → Lock to prepare data → I/O operations
func (c *HighVolumeEventCache) SaveCache(projectRoot string) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var entryCount int
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_17 = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheSaveGetCount,
		func() error {
			entryCount = len(c.cache)
			return nil
		},
	)
	if err_swallow_17 !=

		nil {
		logging.LogSwallowedError(err_swallow_17)
	}

	cachePath := c.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)
	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateCacheDirectory).Wrap(err)
	}

	// Step 3: Prepare cache data with Lock (update metadata; build v2 buckets for smaller on-disk size)
	var cacheData struct {
		Metadata *HighVolumeEventCacheMetadata `json:"metadata"`
		Buckets  highVolumeEventCacheV2Buckets `json:"buckets"`
	}
	ctx2, cancel2 := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel2()
	var err_swallow_18 = concurrency.WithLockTimeout(
		&c.mu,
		ctx2,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheSavePrepare,
		func() error {
			when.When(func() bool { return c.metadata == nil }).Then(func() {
				c.metadata = &HighVolumeEventCacheMetadata{
					Version:     highVolumeEventCacheVersion,
					BuildTime:   time.Now().UTC(),
					ProjectRoot: projectRoot,
					EntryCount:  entryCount,
				}
			}).OrElse(func() {
				c.metadata.Version = highVolumeEventCacheVersion
				c.metadata.BuildTime = time.Now().UTC()
				c.metadata.EntryCount = entryCount

			}).Run()
			cacheData.Metadata = c.metadata
			cacheData.Buckets = c.bucketsFromCache()
			return nil
		},
	)
	if err_swallow_18 !=

		nil {
		logging.LogSwallowedError(err_swallow_18)
	}

	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalCache).Wrap(err)
	}

	if err := fileutil.WriteDurableSecureFile(cachePath, data); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteCacheFile).Wrap(err)
	}

	return nil
}

package objectidcache

import (
	stdcontext "context"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

// SnapshotStats returns in-memory size and whether maps are uninitialized.
func (c *ObjectIDCache) SnapshotStats() (entryCount int, mapsNil bool) {
	if c == nil {
		return 0, true
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheTryLoadCount,
		func() error {
			mapsNil = c.byKind == nil
			entryCount = len(c.idToKind)
			return nil
		},
	)
	return entryCount, mapsNil
}

// CollectIDs returns a snapshot of cached object IDs.
func (c *ObjectIDCache) CollectIDs() []string {
	if c == nil {
		return nil
	}
	var ids []string
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheTryLoadCount,
		func() error {
			ids = make([]string, 0, len(c.idToKind))
			for id := range c.idToKind {
				ids = append(ids, id)
			}
			return nil
		},
	)
	return ids
}

// KindBucketCount returns how many entries are stored for kind.
func (c *ObjectIDCache) KindBucketCount(kind string) int {
	if c == nil || kind == emptyValue {
		return 0
	}
	var n int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheCountKind,
		func() error {
			n = len(c.byKind[kind])
			return nil
		},
	)
	return n
}

// ForEachEntry visits every cached entry (full path expanded). fn must not call back into the cache.
func (c *ObjectIDCache) ForEachEntry(fn func(*ObjectIDCacheEntry)) {
	if c == nil || fn == nil {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameHashRegistryCollectModified,
		func() error {
			for kind, list := range c.byKind {
				for i := range list {
					fn(c.entryFromBucket(kind, &list[i]))
				}
			}
			return nil
		},
	)
}

const LockNameHashRegistryCollectModified = "hash_registry_collect_modified"

// NotifyCacheProgress emits load/build progress if a notifier is installed.
func (c *ObjectIDCache) NotifyCacheProgress(status, message string) {
	c.notifyCacheProgress(status, message)
}

// WarmCASIndexesFromCache populates CAS indexes from the object ID cache.
func WarmCASIndexesFromCache(ctx stdcontext.Context, projectRoot string, cache *ObjectIDCache, storageForWarm storage.ObjectStorageProvider, flushTimeout time.Duration) {
	warmCASIndexesFromCache(ctx, projectRoot, cache, storageForWarm, flushTimeout)
}

// SetProgressNotifier installs (or clears, if n is nil) load/build progress callbacks.
func (c *ObjectIDCache) SetProgressNotifier(n ObjectIDCacheProgressNotifier) {
	if c == nil {
		return
	}
	if n == nil {
		n = noOpCacheProgressNotifier
	}
	c.progressNotifier.Store(&progressNotifierHolder{n: n})
}

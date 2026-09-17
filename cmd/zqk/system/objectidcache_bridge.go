package system

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/objectidcache"
	"github.com/lanceman/zqk/pkg/storage"
)

// Type aliases keep existing CLI call sites compiling after the ObjectIDCache extract.
type (
	ObjectIDCache                 = objectidcache.ObjectIDCache
	ObjectIDCacheEntry            = objectidcache.ObjectIDCacheEntry
	ObjectIDCacheMetadata         = objectidcache.ObjectIDCacheMetadata
	ObjectIDCacheProgressNotifier = objectidcache.ObjectIDCacheProgressNotifier
	KindBucketEntry               = objectidcache.KindBucketEntry
	ObjectIDCacheFileV2           = objectidcache.ObjectIDCacheFileV2
)

func GetGlobalObjectIDCache() *ObjectIDCache {
	return objectidcache.GetGlobalObjectIDCache()
}

func NewObjectIDCache() *ObjectIDCache {
	return objectidcache.NewObjectIDCache()
}

func EnsureObjectIDCacheReady(ctx context.Context, projectRoot string, forceRebuild bool, notifier ObjectIDCacheProgressNotifier, storageForWarm storage.ObjectStorageProvider) error {
	return objectidcache.EnsureObjectIDCacheReady(ctx, projectRoot, forceRebuild, notifier, storageForWarm)
}

func InvalidateObjectIDCache(id string) {
	objectidcache.InvalidateObjectIDCache(id)
}

func UpdateObjectIDCache(id, kind, filePath string) error {
	return objectidcache.UpdateObjectIDCache(id, kind, filePath)
}

func InvalidateObjectIDCacheKind(kind string) {
	objectidcache.InvalidateObjectIDCacheKind(kind)
}

func CleanStaleCacheEntries(projectRoot string) int {
	return objectidcache.CleanStaleCacheEntries(projectRoot)
}

func BulkInvalidateObjectIDCache(ids []string, projectRoot string) int {
	return objectidcache.BulkInvalidateObjectIDCache(ids, projectRoot)
}

func ParseObjectIDCacheFile(data []byte) (byKind map[string][]KindBucketEntry, idToKind map[string]string, metadata *ObjectIDCacheMetadata, countByKind map[string]int, err error) {
	return objectidcache.ParseObjectIDCacheFile(data)
}

func TryLoadObjectIDCacheOnly(projectRoot string) bool {
	return objectidcache.TryLoadObjectIDCacheOnly(projectRoot)
}

func TriggerBackgroundObjectIDCacheBuild(projectRoot string) {
	objectidcache.TriggerBackgroundObjectIDCacheBuild(projectRoot)
}

func TriggerBackgroundObjectIDCacheForceRebuild(projectRoot string) {
	objectidcache.TriggerBackgroundObjectIDCacheForceRebuild(projectRoot)
}

func WaitProjectCacheBackgroundWork(ctx context.Context, projectRoot string) error {
	return objectidcache.WaitProjectCacheBackgroundWork(ctx, projectRoot)
}

func RegisterCacheSidecarsIdleCallback(fn func(projectRoot string)) (remove func()) {
	return objectidcache.RegisterCacheSidecarsIdleCallback(fn)
}

func WarmCASIndexesFromCache(ctx context.Context, projectRoot string, cache *ObjectIDCache, storageForWarm storage.ObjectStorageProvider, flushTimeout time.Duration) {
	objectidcache.WarmCASIndexesFromCache(ctx, projectRoot, cache, storageForWarm, flushTimeout)
}

func getStorageProviderForCache(projectRoot string) storage.ObjectStorageProvider {
	return objectidcache.GetStorageProviderForCache(projectRoot)
}

func tryBuildAndSaveReverseReferenceIndexSync(projectRoot string, discoveryTimeout time.Duration) bool {
	return objectidcache.TryBuildAndSaveReverseReferenceIndexSync(projectRoot, discoveryTimeout)
}

func projectCacheBgHas(projectRoot string) bool {
	return objectidcache.HasProjectCacheBackgroundState(projectRoot)
}

const MaxObjectIDCacheEntries = objectidcache.MaxObjectIDCacheEntries

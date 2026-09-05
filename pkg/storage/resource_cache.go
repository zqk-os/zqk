package storage

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// ResourceCache provides thread-safe caching of resources with lazy initialization
// Uses sync.Map + sync.Once pattern for efficient concurrent access
// Generic implementation that can cache any type of resource by key
type ResourceCache[T any] struct {
	cache          sync.Map // map[string]*cachedResource[T]
	hitsTotal      atomic.Int64
	missesTotal    atomic.Int64
	creationsTotal atomic.Int64
}

// GetResourceCacheStats returns lifetime counters for hits, misses, and resource creations.
func (rc *ResourceCache[T]) GetResourceCacheStats() (hits, misses, creations int64) {
	if rc == nil {
		return 0, 0, 0
	}
	return rc.hitsTotal.Load(), rc.missesTotal.Load(), rc.creationsTotal.Load()
}

// cachedResource holds a cached resource with initialization state
type cachedResource[T any] struct {
	key      string
	resource T
	once     sync.Once
	initErr  error
}

// GetOrCreate retrieves a cached resource or creates it if it doesn't exist
// initFunc is called exactly once per key (thread-safe)
// Returns the resource and any initialization error
func (rc *ResourceCache[T]) GetOrCreate(ctx context.Context, key string, initFunc func(ctx context.Context, key string) (T, error)) (T, error) {
	var zero T

	// Get or create cached entry for this key (thread-safe using sync.Map)
	cachedVal, isLoaded := rc.cache.LoadOrStore(key, &cachedResource[T]{
		key: key,
	})
	if isLoaded {
		rc.hitsTotal.Add(1)
	} else {
		rc.missesTotal.Add(1)
	}
	cached := cachedVal.(*cachedResource[T])

	// Initialize once per key (thread-safe, no mutex needed)
	cached.once.Do(func() {
		rc.creationsTotal.Add(1)
		resource, err := initFunc(ctx, key)
		if err != nil {
			cached.initErr = err
			return
		}
		cached.resource = resource
	})

	// Return cached resource if initialization succeeded
	if cached.initErr == nil {
		return cached.resource, nil
	}

	// Initialization failed - return error
	return zero, cached.initErr
}

// Get retrieves a cached resource without creating it
// Returns the resource and true if found, or zero value and false if not found
func (rc *ResourceCache[T]) Get(key string) (T, bool) {
	var zero T

	val, ok := rc.cache.Load(key)
	if !ok {
		rc.missesTotal.Add(1)
		return zero, false
	}

	cached := val.(*cachedResource[T])
	if cached.initErr != nil {
		rc.missesTotal.Add(1)
		return zero, false
	}

	rc.hitsTotal.Add(1)
	return cached.resource, true
}

// Delete removes a cached resource
func (rc *ResourceCache[T]) Delete(key string) {
	rc.cache.Delete(key)
}

// Clear removes all cached resources
func (rc *ResourceCache[T]) Clear() {
	rc.cache.Range(func(key, value any) bool {
		rc.cache.Delete(key)
		return true
	})
}

// StorageProviderCache is a specialized cache for ObjectStorageProvider instances
// Caches providers per projectRoot to avoid expensive factory creation
type StorageProviderCache struct {
	cache *ResourceCache[ObjectStorageProvider]
}

// GetStorageProviderCacheStats returns lifetime counters for hits, misses, and provider creations.
func (spc *StorageProviderCache) GetStorageProviderCacheStats() (hits, misses, creations int64) {
	if spc == nil || spc.cache == nil {
		return 0, 0, 0
	}
	return spc.cache.GetResourceCacheStats()
}

// NewStorageProviderCache creates a new storage provider cache
func NewStorageProviderCache() *StorageProviderCache {
	return &StorageProviderCache{
		cache: &ResourceCache[ObjectStorageProvider]{},
	}
}

// GetOrCreate retrieves a cached storage provider or creates it if it doesn't exist
// Uses NewStorageFactory to create the provider and returns a RoutingObjectStorage
func (spc *StorageProviderCache) GetOrCreate(ctx context.Context, projectRoot string) (ObjectStorageProvider, error) {
	return spc.cache.GetOrCreate(ctx, projectRoot, func(ctx context.Context, key string) (ObjectStorageProvider, error) {
		storageFactory, err := NewStorageFactory(ctx, key)
		if err != nil {
			return nil, errfmt.Newf(ConstMiscFailedToCreateStorageFactory).Wrap(err)
		}

		// Return a BatchingObjectStorage wrapped around RoutingObjectStorage
		routingStorage := NewRoutingObjectStorage(storageFactory)
		return NewBatchingObjectStorage(routingStorage), nil
	})
}

// Get retrieves a cached storage provider without creating it
func (spc *StorageProviderCache) Get(projectRoot string) (ObjectStorageProvider, bool) {
	return spc.cache.Get(projectRoot)
}

// Delete removes a cached storage provider
func (spc *StorageProviderCache) Delete(projectRoot string) {
	spc.cache.Delete(projectRoot)
}

// Clear removes all cached storage providers
func (spc *StorageProviderCache) Clear() {
	spc.cache.Clear()
}

// Global storage provider cache (shared across all callers)
var (
	globalStorageProviderCache     = NewStorageProviderCache()
	globalStorageProviderCacheOnce sync.Once
)

// GetGlobalStorageProviderCache returns the global storage provider cache
// Thread-safe singleton pattern
func GetGlobalStorageProviderCache() *StorageProviderCache {
	globalStorageProviderCacheOnce.Do(func() {
		// Cache already initialized above
	})
	return globalStorageProviderCache
}

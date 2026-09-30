package storage

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
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
	initErr  error
	ready    chan struct{}
}

// GetOrCreate retrieves a cached resource or creates it if it doesn't exist.
// initFunc is called lazily. If initialization succeeds, the resource is cached and shared.
// If initialization fails due to a transient error, the entry is evicted atomically via CompareAndDelete
// allowing concurrent or subsequent callers to retry rather than permanently poisoning the cache.
func (rc *ResourceCache[T]) GetOrCreate(ctx context.Context, key string, initFunc func(ctx context.Context, key string) (T, error)) (T, error) {
	var zero T

	for {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		newEntry := &cachedResource[T]{
			key:   key,
			ready: make(chan struct{}),
		}

		cachedVal, isLoaded := rc.cache.LoadOrStore(key, newEntry)
		cached := cachedVal.(*cachedResource[T])

		if !isLoaded {
			// This goroutine won the race to initialize the resource.
			rc.missesTotal.Add(1)
			rc.creationsTotal.Add(1)

			resource, err := initFunc(ctx, key)
			if err != nil {
				cached.initErr = err
				close(cached.ready)
				// Atomically evict this failed entry so future attempts can retry cleanly.
				rc.cache.CompareAndDelete(key, cached)
				return zero, err
			}

			cached.resource = resource
			close(cached.ready)
			return cached.resource, nil
		}

		// Another goroutine is currently initializing or has already initialized the resource.
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-cached.ready:
		}

		// Check if initialization succeeded.
		if cached.initErr == nil {
			rc.hitsTotal.Add(1)
			return cached.resource, nil
		}

		// The in-flight initialization attempt failed.
		// That attempt will have evicted itself via CompareAndDelete.
		// Loop and retry initialization.
	}
}

// Get retrieves a cached resource without creating it.
// Returns the resource and true if found and fully initialized.
// If the resource is not present, initialization failed, or initialization is still in-flight,
// returns zero value and false.
func (rc *ResourceCache[T]) Get(key string) (T, bool) {
	var zero T

	val, ok := rc.cache.Load(key)
	if !ok {
		rc.missesTotal.Add(1)
		return zero, false
	}

	cached := val.(*cachedResource[T])
	select {
	case <-cached.ready:
		if cached.initErr != nil {
			rc.missesTotal.Add(1)
			return zero, false
		}
		rc.hitsTotal.Add(1)
		return cached.resource, true
	default:
		// Initialization is still in flight; resource is not ready.
		rc.missesTotal.Add(1)
		return zero, false
	}
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

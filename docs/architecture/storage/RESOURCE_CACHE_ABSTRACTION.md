# Resource Cache Abstraction

**Date:** 2026-01-22  
**Status:** Implemented  
**Purpose:** Abstract the `sync.Map` + `sync.Once` caching pattern for reuse across the codebase

## Problem

The pattern for caching expensive-to-create resources (like `ObjectStorageProvider`) was duplicated across the codebase:
- Manual `sync.Map` management
- Manual `sync.Once` per key
- Error handling duplication
- Type casting boilerplate

## Solution

Created a generic `ResourceCache[T]` abstraction that:
- Provides type-safe caching for any resource type
- Handles `sync.Map` + `sync.Once` pattern internally
- Supports lazy initialization with error handling
- Thread-safe by design

## Implementation

### Generic ResourceCache

**Location:** `pkg/storage/resource_cache.go`

```go
type ResourceCache[T any] struct {
    cache sync.Map // map[string]*cachedResource[T]
}

func (rc *ResourceCache[T]) GetOrCreate(
    ctx context.Context, 
    key string, 
    initFunc func(ctx context.Context, key string) (T, error),
) (T, error)
```

**Features:**
- Generic type parameter `T` for any resource type
- Thread-safe initialization using `sync.Once` per key
- Error handling for initialization failures
- Type-safe retrieval without casting

### Specialized StorageProviderCache

**Location:** `pkg/storage/resource_cache.go`

```go
type StorageProviderCache struct {
    cache *ResourceCache[ObjectStorageProvider]
}

func (spc *StorageProviderCache) GetOrCreate(
    ctx context.Context, 
    projectRoot string,
) (ObjectStorageProvider, error)
```

**Features:**
- Specialized for `ObjectStorageProvider` caching
- Uses `NewStorageFactory` internally
- Global singleton via `GetGlobalStorageProviderCache()`

## Migration

### Before (audit_events_helper.go)

```go
var (
    cachedStorageProviders sync.Map // map[string]*cachedStorageProvider
)

type cachedStorageProvider struct {
    provider    ObjectStorageProvider
    projectRoot string
    once        sync.Once
    initErr     error
}

// Usage:
cachedVal, _ := cachedStorageProviders.LoadOrStore(projectRoot, &cachedStorageProvider{
    projectRoot: projectRoot,
})
cached := cachedVal.(*cachedStorageProvider)

cached.once.Do(func() {
    storageFactory, err := NewStorageFactory(ctx, projectRoot)
    if err != nil {
        cached.initErr = err
        return
    }
    cached.provider = storageFactory.GetStorage()
    if cached.provider == nil {
        cached.initErr = fmt.Errorf("storage provider is nil")
        return
    }
})

if cached.initErr == nil && cached.provider != nil {
    storageProvider = cached.provider
} else {
    return nil
}
```

### After (audit_events_helper.go)

```go
var (
    cachedStorageProviders = GetGlobalStorageProviderCache()
)

// Usage:
provider, err := cachedStorageProviders.GetOrCreate(ctx, projectRoot)
if err != nil {
    logger.Warn("Failed to get or create storage provider", logging.Error(err))
    return nil
}
storageProvider = provider
```

**Benefits:**
- ✅ Reduced from ~30 lines to ~5 lines
- ✅ No manual type casting
- ✅ No manual `sync.Once` management
- ✅ Consistent error handling
- ✅ Reusable across codebase

## Usage Examples

### Caching Storage Providers

```go
cache := GetGlobalStorageProviderCache()
provider, err := cache.GetOrCreate(ctx, projectRoot)
if err != nil {
    return err
}
// Use provider...
```

### Caching Other Resources

```go
type MyResource struct {
    // ...
}

cache := &ResourceCache[MyResource]{}
resource, err := cache.GetOrCreate(ctx, "key", func(ctx context.Context, key string) (MyResource, error) {
    // Initialize resource
    return MyResource{}, nil
})
```

## Future Opportunities

### 1. CAS Instance Caching (FileObjectStorage)

**Current:** Uses manual `sync.Map` + `sync.Once` pattern  
**Location:** `pkg/storage/object_storage_file.go`  
**Pattern:**
```go
casInstances sync.Map // map[string]*ContentAddressableStorage
casOnce      sync.Map // map[string]*sync.Once
```

**Potential Migration:**
```go
casCache := &ResourceCache[*ContentAddressableStorage]{}
cas, err := casCache.GetOrCreate(ctx, kind, func(ctx context.Context, kind string) (*ContentAddressableStorage, error) {
    return NewContentAddressableStorage(kindDir, kind), nil
})
```

**Note:** This is per-instance state, not global, so migration would require refactoring `FileObjectStorage` struct.

### 2. Other Caching Opportunities

- Batch ID generators (if converted to cache pattern)
- Spec loaders (if caching is needed)
- Validator instances (if caching is needed)

## API Reference

### ResourceCache[T]

```go
// GetOrCreate retrieves or creates a cached resource
func (rc *ResourceCache[T]) GetOrCreate(
    ctx context.Context,
    key string,
    initFunc func(ctx context.Context, key string) (T, error),
) (T, error)

// Get retrieves a cached resource without creating it
func (rc *ResourceCache[T]) Get(key string) (T, bool)

// Delete removes a cached resource
func (rc *ResourceCache[T]) Delete(key string)

// Clear removes all cached resources
func (rc *ResourceCache[T]) Clear()
```

### StorageProviderCache

```go
// GetOrCreate retrieves or creates a cached storage provider
func (spc *StorageProviderCache) GetOrCreate(
    ctx context.Context,
    projectRoot string,
) (ObjectStorageProvider, error)

// Get retrieves a cached storage provider
func (spc *StorageProviderCache) Get(projectRoot string) (ObjectStorageProvider, bool)

// Delete removes a cached storage provider
func (spc *StorageProviderCache) Delete(projectRoot string)

// Clear removes all cached storage providers
func (spc *StorageProviderCache) Clear()

// GetGlobalStorageProviderCache returns the global singleton
func GetGlobalStorageProviderCache() *StorageProviderCache
```

## Testing

The abstraction is tested implicitly through:
- `audit_events_helper.go` usage (existing tests)
- Integration tests for audit event creation
- No breaking changes to existing functionality

## Benefits

1. **Code Reuse:** Single implementation for caching pattern
2. **Type Safety:** Generic types prevent casting errors
3. **Consistency:** Same pattern across all cached resources
4. **Maintainability:** Changes to caching logic in one place
5. **Testability:** Easier to test caching behavior in isolation

## Status

✅ **Completed:**
- Generic `ResourceCache[T]` implementation
- Specialized `StorageProviderCache` implementation
- Migration of `audit_events_helper.go` to use abstraction
- Global singleton for storage provider cache
- Code compiles and tests pass

**Next Steps:**
- Consider migrating `FileObjectStorage` CAS caching (requires struct refactoring)
- Document pattern for future use cases
- Add unit tests for `ResourceCache` if needed

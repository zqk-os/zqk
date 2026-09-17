package system

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/storage"
)

// HashRegistryCacheType is a thread-safe cache for hash registries
// directoryRegistryPool manages hash registry instances per directory
// Uses sync.Map for thread-safe access without global lock contention
// Each directory (e.g., audit_event/2026-01) gets its own registry instance
type directoryRegistryPool struct {
	registries sync.Map // map[string]storage.HashRegistryProvider (keyed by "kind:dir")
}

// GetOrCreate retrieves or creates a hash registry for the given kind and directory
// Thread-safe: sync.Map handles fine-grained locking internally
// OPTIMIZATION: Caches registry instances and only reloads when file changes
// This avoids expensive disk I/O on every validation
func (p *directoryRegistryPool) GetOrCreate(ctx stdcontext.Context, kind, dir string) storage.HashRegistryProvider {
	key := formatHashRegistryCacheKey(kind, dir)

	// Try to get existing registry from cache
	if reg, ok := p.registries.Load(key); ok {
		// Registry exists in cache - Load() will check mtime and skip if unchanged
		return reg.(storage.HashRegistryProvider)
	}

	// Create new registry instance
	reg := storage.NewHashRegistry(ctx, kind, dir)
	if err := reg.Load(); err != nil {
		// Log but continue - hash registry may not exist yet
		// (This is expected for new directories)
	}

	// Cache the registry instance (Load() now checks mtime to avoid unnecessary reloads)
	p.registries.Store(key, reg)

	return reg
}

// Get retrieves a registry from the pool (returns nil if not found)
func (p *directoryRegistryPool) Get(kind, dir string) (storage.HashRegistryProvider, bool) {
	key := formatHashRegistryCacheKey(kind, dir)
	if reg, ok := p.registries.Load(key); ok {
		return reg.(storage.HashRegistryProvider), true
	}
	return nil, false
}

// Delete removes a registry from the pool (for cache invalidation)
func (p *directoryRegistryPool) Delete(kind, dir string) {
	key := formatHashRegistryCacheKey(kind, dir)
	p.registries.Delete(key)
}

// GetByKind retrieves a registry for non-bucketed objects (keyed by kind only)
func (p *directoryRegistryPool) GetByKind(kind string) (storage.HashRegistryProvider, bool) {
	return p.Get(kind, "")
}

// SetByKind stores a registry for non-bucketed objects (keyed by kind only)
func (p *directoryRegistryPool) SetByKind(kind string, reg storage.HashRegistryProvider) {
	key := fmt.Sprintf("%s:", kind) // Empty dir for non-bucketed
	p.registries.Store(key, reg)
}

// Legacy HashRegistryCacheType for backward compatibility (non-bucketed objects)
// This maintains the old interface for code that hasn't been updated yet
const (
	// cacheKeySeparator is used to separate kind and directory in cache keys for bucketed objects
	cacheKeySeparator = ":"
)

// formatHashRegistryCacheKey formats a cache key for hash registry lookup
// For bucketed objects: returns "kind:fileDir"
// For non-bucketed objects: returns "kind"
func formatHashRegistryCacheKey(kind, fileDir string) string {
	if fileDir == emptyValue {
		return kind
	}
	return fmt.Sprintf("%s%s%s", kind, cacheKeySeparator, fileDir)
}

type HashRegistryCacheType struct {
	mu    sync.RWMutex
	cache map[string]storage.HashRegistryProvider
}

// deferredHashCheck represents a hash integrity check that should be performed
// after all fixes are complete to prevent tail-chasing issues
type deferredHashCheck struct {
	obj      *parser.ParsedObject
	filePath string
	kind     string
	content  []byte
	registry storage.HashRegistryProvider
}

// Get retrieves a hash registry from cache
func (c *HashRegistryCacheType) Get(kind string) (storage.HashRegistryProvider, bool) {
	var reg storage.HashRegistryProvider
	var ok bool
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logger,
		LockNameHashRegistryCacheGet,
		func() error {
			reg, ok = c.cache[kind]
			return nil
		},
	)
	return reg, ok
}

// Set stores a hash registry in cache
func (c *HashRegistryCacheType) Set(kind string, reg storage.HashRegistryProvider) {
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithLockTimeout(
		&c.mu,
		ctx,
		nil,
		logger,
		LockNameHashRegistryCacheSet,
		func() error {
			c.cache[kind] = reg
			return nil
		},
	)
}

// Delete removes a hash registry from cache
func (c *HashRegistryCacheType) Delete(kind string) {
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithLockTimeout(
		&c.mu,
		ctx,
		nil,
		logger,
		LockNameHashRegistryCacheDelete,
		func() error {
			delete(c.cache, kind)
			return nil
		},
	)
}

// Reload reloads a hash registry from disk into the cache
// OPTIMIZATION: Release lock before file I/O to prevent blocking other workers
func (c *HashRegistryCacheType) Reload(kind string) {
	var reg storage.HashRegistryProvider
	var ok bool
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logger,
		LockNameHashRegistryCacheReloadCheck,
		func() error {
			reg, ok = c.cache[kind]
			return nil
		},
	)

	if ok && reg != nil {
		// Reload the registry from disk to get latest hashes (NO LOCK HELD)
		//nolint:errcheck // Best-effort operation
		_ = reg.Load() // Ignore errors - best effort
	}
}

// ReloadAll reloads all hash registries in the cache from disk
// OPTIMIZATION: Release lock before file I/O to prevent blocking other workers
func (c *HashRegistryCacheType) ReloadAll() {
	var registries []storage.HashRegistryProvider
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameHashRegistryCacheReloadAll,
		func() error {
			// Copy registry references to avoid holding lock during I/O
			registries = make([]storage.HashRegistryProvider, 0, len(c.cache))
			for _, reg := range c.cache {
				if reg != nil {
					registries = append(registries, reg)
				}
			}
			return nil
		},
	)

	// Reload all registries (NO LOCK HELD during file I/O)
	for _, reg := range registries {
		//nolint:errcheck // Best-effort operation
		_ = reg.Load() // Ignore errors - best effort
	}
}

// getHashRegistryForFile returns the appropriate hash registry for a file
// This unified function handles both bucketed and non-bucketed objects correctly
// For bucketed objects (fileDir != kindDir), uses cache key "kind:fileDir" and loads from fileDir
// For non-bucketed objects, uses cache key "kind" and loads from kindDir
// This ensures consistency between validation and auto-fix paths
//
// Returns nil if kind is unknown or kindDir cannot be determined.
// Callers should handle nil return value gracefully (e.g., by creating a new registry or skipping the check).
func getHashRegistryForFile(stdCtx stdcontext.Context, filePath, kind, projectRoot string, hashRegistryCache *HashRegistryCacheType) storage.HashRegistryProvider {
	// Ensure we have a valid context
	if stdCtx == nil {
		stdCtx = pkgctx.NewSystemContext()
	}

	if projectRoot == emptyValue {
		projectRoot = ProjectRootOrResolve(projectRoot)
	}

	fileDir := filepath.Dir(filePath)
	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		// Unknown kind - return nil
		return nil
	}

	isBucketed := fileDir != kindDir

	var cacheKey string
	var registryDir string

	if isBucketed {
		// Bucketed object: registry is in file's subdirectory
		cacheKey = formatHashRegistryCacheKey(kind, fileDir)
		registryDir = fileDir
	} else {
		// Non-bucketed object: registry is in kind directory
		cacheKey = kind
		registryDir = kindDir
	}

	// Try to get from cache first
	if hashRegistryCache != nil {
		if cachedReg, ok := hashRegistryCache.Get(cacheKey); ok {
			return cachedReg
		}
	}

	// Not in cache - create new instance (without loading yet)
	registry := storage.NewHashRegistry(stdCtx, kind, registryDir)

	// Load from disk AFTER releasing cache lock (file I/O can be slow)
	// This prevents blocking other workers on cache lock during file I/O
	if err := registry.Load(); err != nil {
		// Hash registry doesn't exist yet - will be created on first save
		// This is not an error, just means it's a new registry
	}

	// Cache it for future use (lock only held briefly for cache update)
	if hashRegistryCache != nil {
		hashRegistryCache.Set(cacheKey, registry)
	}

	return registry
}

package context

import (
	"context"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
)

// CacheOperation represents the type of cache operation needed
type CacheOperation string

const (
	// CacheOperationNone indicates no cache operation is needed
	CacheOperationNone CacheOperation = ""
	// CacheOperationUpdate indicates the cache entry should be updated (for create/update)
	CacheOperationUpdate CacheOperation = "update"
	// CacheOperationInvalidate indicates the cache entry should be invalidated (for delete)
	CacheOperationInvalidate CacheOperation = "invalidate"
	// CacheOperationInvalidateAndUpdate indicates both invalidate old and update new (for ID changes)
	CacheOperationInvalidateAndUpdate CacheOperation = "invalidate_and_update"
)

// CacheContext carries cache operation metadata for storage operations
// This allows the storage layer to understand what cache operations are needed
// without directly depending on the cache implementation
type CacheContext struct {
	// Operation indicates what cache operation should be performed
	Operation CacheOperation

	// OldID is the old ID (for invalidate operations or ID changes)
	OldID string

	// NewID is the new ID (for update operations or ID changes)
	NewID string

	// Kind is the object kind (for update operations)
	Kind string

	// FilePath is the file path (for update operations)
	FilePath string
}

// WithCacheOperation adds cache operation metadata to the context
// This should be called by the CLI layer before storage operations
func WithCacheOperation(ctx context.Context, cacheCtx *CacheContext) context.Context {
	if cacheCtx == nil {
		return ctx
	}
	return context.WithValue(ctx, cacheContextKey{}, cacheCtx)
}

// GetCacheContext retrieves cache operation metadata from the context
// Returns nil if no cache context is present
func GetCacheContext(ctx context.Context) *CacheContext {
	if cacheCtx, ok := ctx.Value(cacheContextKey{}).(*CacheContext); ok {
		return cacheCtx
	}
	return nil
}

// WithCacheUpdate creates a context with cache update operation
// Use this for create and update operations
func WithCacheUpdate(ctx context.Context, id, kind, filePath string) context.Context {
	return WithCacheOperation(ctx, &CacheContext{
		Operation: CacheOperationUpdate,
		NewID:     id,
		Kind:      kind,
		FilePath:  filePath,
	})
}

// WithCacheInvalidate creates a context with cache invalidation operation
// Use this for delete operations
func WithCacheInvalidate(ctx context.Context, id string) context.Context {
	return WithCacheOperation(ctx, &CacheContext{
		Operation: CacheOperationInvalidate,
		OldID:     id,
	})
}

// WithCacheIDChange creates a context with cache invalidation and update operation
// Use this when an object's ID is being changed
func WithCacheIDChange(ctx context.Context, oldID, newID, kind, newFilePath string) context.Context {
	return WithCacheOperation(ctx, &CacheContext{
		Operation: CacheOperationInvalidateAndUpdate,
		OldID:     oldID,
		NewID:     newID,
		Kind:      kind,
		FilePath:  newFilePath,
	})
}

// CacheInvalidationContext carries bulk cache invalidation metadata
// This is used for bulk operations where multiple cache entries need to be invalidated
// It implements ProcessableContext to support async processing through the listener pipeline
type CacheInvalidationContext struct {
	// IDs is the list of object IDs to invalidate from the cache
	IDs []string

	// ProjectRoot is the project root path (required for saving the cache)
	ProjectRoot string

	// Reason is an optional description of why the invalidation is happening
	Reason string

	// State tracks the processing state (implements ProcessableContext)
	state ContextState

	// ctx is the Go context for cancellation/timeout
	ctx context.Context

	// mu protects state access
	mu sync.RWMutex
}

// NewCacheInvalidationContext creates a new cache invalidation context
func NewCacheInvalidationContext(ids []string, projectRoot, reason string) *CacheInvalidationContext {
	return &CacheInvalidationContext{
		IDs:         ids,
		ProjectRoot: projectRoot,
		Reason:      reason,
		state:       StatePending,
		ctx:         NewSystemContext(),
	}
}

// WithContext sets the Go context for cancellation/timeout
func (c *CacheInvalidationContext) WithContext(ctx context.Context) *CacheInvalidationContext {
	c.ctx = ctx
	return c
}

// GetState returns the current state (implements ProcessableContext)
func (c *CacheInvalidationContext) GetState() ContextState {
	var state ContextState
	_ = concurrency.WithRLockCtx(
		&c.mu,
		NewSystemContext(),
		"cache_invalidation_context_get_state",
		func() error {
			state = c.state
			return nil
		},
	)
	return state
}

// SetState sets the state (implements ProcessableContext)
func (c *CacheInvalidationContext) SetState(state ContextState) {
	_ = concurrency.WithLockCtx(
		&c.mu,
		NewSystemContext(),
		"cache_invalidation_context_set_state",
		func() error {
			c.state = state
			return nil
		},
	)
}

// GetContext returns the Go context (implements ProcessableContext)
func (c *CacheInvalidationContext) GetContext() context.Context {
	return c.ctx
}

// CacheFreshnessContext carries cache freshness validation metadata
// This is used to trigger asynchronous cache validation and cleanup
// It implements ProcessableContext to support async processing through the listener pipeline
type CacheFreshnessContext struct {
	// ProjectRoot is the project root path (required for cache operations)
	ProjectRoot string

	// Reason is an optional description of why the freshness check is happening
	Reason string

	// TriggerOperation indicates what operation triggered this freshness check
	// (e.g., "create", "update", "delete", "bulk_create", "bulk_update", "bulk_delete")
	TriggerOperation string

	// AffectedKinds is an optional list of object kinds that were affected
	// This can be used to optimize the freshness check to only validate specific kinds
	AffectedKinds []string

	// State tracks the processing state (implements ProcessableContext)
	state ContextState

	// ctx is the Go context for cancellation/timeout
	ctx context.Context

	// mu protects state access
	mu sync.RWMutex
}

// NewCacheFreshnessContext creates a new cache freshness context
func NewCacheFreshnessContext(projectRoot, reason, triggerOperation string) *CacheFreshnessContext {
	return &CacheFreshnessContext{
		ProjectRoot:      projectRoot,
		Reason:           reason,
		TriggerOperation: triggerOperation,
		AffectedKinds:    []string{},
		state:            StatePending,
		ctx:              NewSystemContext(),
	}
}

// WithAffectedKinds sets the affected kinds for this freshness check
func (c *CacheFreshnessContext) WithAffectedKinds(kinds []string) *CacheFreshnessContext {
	c.AffectedKinds = kinds
	return c
}

// WithContext sets the Go context for cancellation/timeout
func (c *CacheFreshnessContext) WithContext(ctx context.Context) *CacheFreshnessContext {
	c.ctx = ctx
	return c
}

// GetState returns the current state (implements ProcessableContext)
func (c *CacheFreshnessContext) GetState() ContextState {
	var state ContextState
	_ = concurrency.WithRLockCtx(
		&c.mu,
		NewSystemContext(),
		"cache_freshness_context_get_state",
		func() error {
			state = c.state
			return nil
		},
	)
	return state
}

// SetState sets the state (implements ProcessableContext)
func (c *CacheFreshnessContext) SetState(state ContextState) {
	_ = concurrency.WithLockCtx(
		&c.mu,
		NewSystemContext(),
		"cache_freshness_context_set_state",
		func() error {
			c.state = state
			return nil
		},
	)
}

// GetContext returns the Go context (implements ProcessableContext)
func (c *CacheFreshnessContext) GetContext() context.Context {
	return c.ctx
}

// cacheContextKey is the key type for cache context in context.Context
type cacheContextKey struct{}

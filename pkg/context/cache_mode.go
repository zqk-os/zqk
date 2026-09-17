package context

import stdctx "context"

// CacheMode controls behavior of caches (including validation cache) based on
// execution context: default CLI, unit tests, integration tests, smoke tests, etc.
type CacheMode string

const (
	// CacheModeDefault is the normal behavior (async background persistence).
	CacheModeDefault CacheMode = ""
	// CacheModeTestSync forces synchronous persistence for tests that need
	// deterministic behavior (no goroutine timing / sleeps).
	CacheModeTestSync CacheMode = "test_sync"
)

type cacheModeKey struct{}

// WithCacheMode attaches a cache mode to the context.
func WithCacheMode(ctx stdctx.Context, mode CacheMode) stdctx.Context {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if mode == CacheModeDefault {
		return ctx
	}
	return stdctx.WithValue(ctx, cacheModeKey{}, mode)
}

// GetCacheMode retrieves the cache mode from the context.
// Returns CacheModeDefault when not set.
func GetCacheMode(ctx stdctx.Context) CacheMode {
	if ctx == nil {
		return CacheModeDefault
	}
	if v, ok := ctx.Value(cacheModeKey{}).(CacheMode); ok {
		return v
	}
	return CacheModeDefault
}

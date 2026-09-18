package storage

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// EnsureTestIdentityCacheHandler installs a succeeding no-op so hermetic tests ACK
// without the CLI wiring. Tests that need fail-closed nil-handler set the handler
// to nil after construction.
func EnsureTestIdentityCacheHandler() {
	ensureTestIdentityCacheHandler()
}

func ensureTestIdentityCacheHandler() {
	if cacheOperationHandler == nil {
		SetCacheOperationHandler(func(*pkgctx.CacheContext) error { return nil })
	}
}
